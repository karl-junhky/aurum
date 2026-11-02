package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// otdatDispetcheru is the seam the connection loop dispatches through.
//
// A package variable and not a field of Sluzhba: the struct lives in another
// file owned by another task, and a test-only flag inside it would sit dead in
// the shipped binary forever. Production reads the real dispatcher through this
// variable, so the seam is not dead code; the test replaces it to make a
// handler panic on demand, which no real command is allowed to do.
var otdatDispetcheru = (*Sluzhba).Obrabotat

// slushatKanal это шов: настоящий слушатель требует прав SYSTEM на владельца
// канала, и тест на хосте его не создаст.
var slushatKanal = kanal.Slushat

// Сколько отказов приёма подряд терпит один слушатель, прежде чем его
// откроют заново.
const maksOtkazovPriyoma = 10

// Obsluzhivat принимает клиентов канала до отмены ctx.
//
// Отказ открыть канал при СТАРТЕ возвращается: занятое имя значит, что кто-то
// выдаёт себя за нас, и отдать ему наших клиентов хуже, чем не стартовать.
// Отказ ПОСЛЕ старта службу не останавливает (Н6 аудита 1.6.1). До 1.7.0 любой
// отказ Accept гасил службу вместе с туннелем, хотя у go-winio слушатель после
// такого отказа жив и следующий Accept проходит: отказ приёма одного клиента
// стоил человеку сети.
func (s *Sluzhba) Obsluzhivat(ctx context.Context) error {
	l, err := slushatKanal()
	if err != nil {
		return err
	}
	for {
		err := s.prinimat(ctx, l)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("канал перестал принимать: %v; открываю заново", err)
		if l = otkrytKanalZanovo(ctx); l == nil {
			return nil
		}
	}
}

// prinimat крутит Accept одного слушателя и возвращается, когда слушатель
// мёртв, отказы идут подряд без конца или отменён ctx. Слушатель закрыт на
// выходе в любом случае.
func (s *Sluzhba) prinimat(ctx context.Context, l net.Listener) error {
	defer l.Close()
	// AfterFunc, а не горутина с <-ctx.Done(): та жила бы до отмены службы и
	// после смены слушателя.
	defer context.AfterFunc(ctx, func() { _ = l.Close() })()
	podryad := 0
	for {
		c, err := l.Accept()
		if err == nil {
			podryad = 0
			fon.Zapustit("соединении", func() { s.obsluzhitOdnogo(ctx, c) })
			continue
		}
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return err
		}
		podryad++
		if podryad >= maksOtkazovPriyoma {
			return fmt.Errorf("%d отказов приёма подряд, последний: %w", podryad, err)
		}
		log.Printf("канал не принял клиента (%d подряд): %v", podryad, err)
		if !podozhdat(ctx, pauzaKanala(podryad)) {
			return ctx.Err()
		}
	}
}

// otkrytKanalZanovo открывает слушатель, пока не выйдет или не отменят ctx.
//
// Сдаваться некуда: без канала служба держит туннель, но ни окно, ни трей
// ничего ей не скажут, и лучше пробовать с потолком паузы, чем стоять глухой.
// Имя может быть занято нашими же живыми соединениями: экземпляры канала
// держат имя, пока их не закроют, и слушатель откроется, когда клиент уйдёт.
func otkrytKanalZanovo(ctx context.Context) net.Listener {
	for popytka := 1; ; popytka++ {
		if !podozhdat(ctx, pauzaKanala(popytka)) {
			return nil
		}
		l, err := slushatKanal()
		if err == nil {
			log.Printf("канал открыт заново с попытки %d", popytka)
			return l
		}
		log.Printf("канал не открылся заново (попытка %d): %v", popytka, err)
	}
}

// nachaloPauzyKanala это первая пауза; тест её укорачивает.
var nachaloPauzyKanala = 100 * time.Millisecond

// pauzaKanala растёт вдвое от nachaloPauzyKanala до потолка в 30 с.
func pauzaKanala(popytka int) time.Duration {
	p := nachaloPauzyKanala
	for i := 1; i < popytka && p < 30*time.Second; i++ {
		p *= 2
	}
	return min(p, 30*time.Second)
}

// podozhdat ждёт d и отвечает false, если ctx отменили раньше.
func podozhdat(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (s *Sluzhba) obsluzhitOdnogo(ctx context.Context, c net.Conn) {
	// Registered FIRST so it runs LAST: the connection still gets closed on the
	// way out of a panic, and only then is the panic stopped. Losing one client
	// is bad, losing the process is worse, because the process holds the tunnel.
	defer perehvatitPaniku("соединении")
	defer c.Close()

	otpravit := pisatel(c)

	pervyy, err := kanal.ChitatKadr(c)
	if err != nil {
		return
	}
	// AFTER the first frame, never before: ImpersonateNamedPipeClient returns
	// ERROR_CANNOT_IMPERSONATE until the server has consumed client data. This is
	// documented, easy to miss, and the difference between a real check and a
	// decorative one.
	dopusk, err := kanal.SveritKlienta(c)
	if err != nil {
		// The reason goes to the log, never to the wire: a rejected client is not
		// entitled to learn how the check works.
		log.Printf("клиент отвергнут на кадре %q: %v", pervyy.Imya, err)
		_ = otpravit(otkaz(pervyy.Id, pervyy.Imya, protokol.KodPipeSquatted, "клиент не допущен"))
		return
	}
	// Дальше по этому соединению команды видят, КТО пришёл. Контекст соединения,
	// а не поле службы: см. kanal/dopusk.go.
	ctx = kanal.SDopuskom(ctx, dopusk)

	if pervyy.Imya != "hello" {
		_ = otpravit(otkaz(pervyy.Id, pervyy.Imya, protokol.KodProtocolMismatch,
			"первым кадром должен быть hello"))
		return
	}
	if err := otpravit(s.Obrabotat(ctx, pervyy)); err != nil {
		return
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	// Команды этого соединения знают свой id: подписка на статистику живёт с ним.
	ctx = sPodpischikom(ctx, id)
	fon.Zapustit("рассылке событий", func() { rassylat(sob, otpravit) })

	mesta := make(chan struct{}, komandNaSoedinenie)
	for {
		k, err := kanal.ChitatKadr(c)
		if err != nil {
			return
		}
		// One goroutine per command on purpose: connect blocks for seconds while
		// it waits for the first probe, and a client that cannot ask for status
		// meanwhile is a client that draws a frozen window.
		s.zapustitKomandu(ctx, k, mesta, otpravit)
	}
}

// komandNaSoedinenie это предел команд в работе на одно соединение (L10
// аудита 1.8.0). Окно держит в полёте единицы, а без предела клиент,
// засыпающий канал кадрами, заводил по горутине на каждый.
const komandNaSoedinenie = 32

// srokZapisiOtveta держит писателя (L10 аудита 1.8.0): клиент, переставший
// читать, без него держал запись и с ней все ответы и события соединения
// вечно. Переменная ради теста.
var srokZapisiOtveta = kanal.TaymautOtveta

// pisatel отдаёт запись кадров в соединение: по одному, со сроком. Отказ
// записи закрывает соединение: оборванный кадр рассинхронизировал бы поток,
// а клиент, который не читает, уже не клиент. Ответ больше предела кадра
// уходит отказом с понятным текстом, а не молчанием до срока клиента.
func pisatel(c net.Conn) func(protokol.Kadr) error {
	var pishet sync.Mutex
	return func(k protokol.Kadr) error {
		pishet.Lock()
		defer pishet.Unlock()
		if err := c.SetWriteDeadline(time.Now().Add(srokZapisiOtveta)); err != nil {
			_ = c.Close()
			return err
		}
		err := kanal.PisatKadr(c, k)
		if errors.Is(err, kanal.ErrKadrVelik) {
			log.Printf("ответ на %s не влез в кадр: %v", k.Imya, err)
			err = kanal.PisatKadr(c, otkaz(k.Id, k.Imya, protokol.KodVnutrennyayaOshibka,
				"ответ службы больше 1 МиБ и не передан"))
		}
		if err != nil {
			_ = c.Close()
		}
		return err
	}
}

// zapustitKomandu запускает команду своей горутиной, если на соединении
// есть место, иначе отвечает отказом сразу.
func (s *Sluzhba) zapustitKomandu(ctx context.Context, k protokol.Kadr, mesta chan struct{}, otpravit func(protokol.Kadr) error) {
	select {
	case mesta <- struct{}{}:
	default:
		log.Printf("команда %s отклонена: в работе уже %d команд этого соединения", k.Imya, cap(mesta))
		_ = otpravit(otkaz(k.Id, k.Imya, protokol.KodVnutrennyayaOshibka,
			"слишком много команд разом, повтори"))
		return
	}
	fon.Zapustit("команде "+k.Imya, func() {
		defer func() { <-mesta }()
		s.obsluzhitKomandu(ctx, k, otpravit)
	})
}

// obsluzhitKomandu runs one command and hands the answer back to the client.
//
// Named, not an inline closure, because the panic guard around it has to be
// reachable from a test: the loop above needs a real named pipe and a real
// client token, neither of which a host test can build.
func (s *Sluzhba) obsluzhitKomandu(ctx context.Context, k protokol.Kadr, otpravit func(protokol.Kadr) error) {
	_ = otpravit(s.obrabotatBezPaniki(ctx, k))
}

// obrabotatBezPaniki runs the dispatcher for one frame and turns a panic into a
// refusal frame.
//
// Answering matters as much as surviving: without a frame the client sits out
// its whole deadline and then reports "the service did not answer", which sends
// the human to fix the pipe instead of the command that broke.
//
// Код СВОЙ, а не protocol-mismatch. Полоса Г ставила второй, потому что строки
// «служба сломалась внутри» в §9.1 не было; экран на него говорит «служба и
// программа разных версий, обнови программу», и человек с упавшей командой шёл
// переустанавливать исправную программу. Версии тут сошлись, сломались мы, и
// делать человеку надо ровно одно: повторить.
func (s *Sluzhba) obrabotatBezPaniki(ctx context.Context, k protokol.Kadr) (otv protokol.Kadr) {
	defer func() {
		if r := recover(); r != nil {
			zapisatPaniku("команде "+k.Imya, r)
			otv = otkaz(k.Id, k.Imya, protokol.KodVnutrennyayaOshibka,
				"служба не справилась с действием, повтори его")
		}
	}()
	return otdatDispetcheru(s, ctx, k)
}

// rassylat pushes events to one client until the subscription closes or the
// connection stops taking frames.
//
// Guard внутри, а не у вызывающего: горутина здесь одна, и оставить перехват
// снаружи значило бы завести место, где его можно забыть.
func rassylat(sob <-chan protokol.Kadr, otpravit func(protokol.Kadr) error) {
	// Отвечать некому: событие никто не ждёт по Id. Остаётся журнал и то, ради
	// чего всё затевалось, живой процесс с живым туннелем.
	defer perehvatitPaniku("рассылке событий")
	for k := range sob {
		if err := otpravit(k); err != nil {
			return
		}
	}
}

// perehvatitPaniku is deferred, never called directly: recover only works from
// the deferred function itself.
func perehvatitPaniku(gde string) {
	if r := recover(); r != nil {
		zapisatPaniku(gde, r)
	}
}

// zapisatPaniku turns a panic into a journal entry with the stack.
//
// A silently swallowed panic is worse than a crashed service: the crash at
// least says that something happened, while a swallowed one leaves a command
// that answers "no" for a reason nobody can ever find.
func zapisatPaniku(gde string, r any) {
	fon.ZapisatPaniku(gde, r)
}
