// Package upravlenie ведёт свой сервер целиком: вход с выбором дороги, осмотр
// и установку по шагам §3.8 плана. Сами шаги живут в пакетах ssh, server,
// konfig, porty, proverka и blok, здесь их порядок и решения между ними.
//
// Пакет живёт в службе: каждый вход под перехватом паники, тексты для
// человека не пишутся. Ошибки несут причину, а слова и совет выбирает
// служба.
package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// Svyaz это вход на сервер: *ssh.Soedinenie за обёрткой, тесты ставят свою.
type Svyaz interface {
	server.Mashina
	Svedeniya() Svedeniya
	// KlyuchRoot кладёт ключ управления в authorized_keys root и входит им
	// новым соединением той же дорогой. pustoy это галочка «Сервер пустой».
	KlyuchRoot(ctx context.Context, podpis gossh.Signer, kommentariy string, pustoy bool) (Svyaz, error)
	// Perevoyti входит root ключом podpis новым соединением той же дорогой.
	Perevoyti(ctx context.Context, podpis gossh.Signer) (Svyaz, error)
	Zakryt()
}

// Svedeniya это что известно о входе.
type Svedeniya struct {
	KlyuchHosta ssh.KlyuchHosta
	// Smenilsya: ключ хоста не тот, что записан, и человек его одобрил.
	Smenilsya bool
	// Root: вошли root, иначе команды идут через sudo.
	Root bool
	// Put называет дорогу для журнала: «напрямую через Ethernet», «через
	// туннель».
	Put    string
	Tunnel bool
}

type soedinenie struct {
	*ssh.Soedinenie
	tunnel bool
}

func (s soedinenie) Svedeniya() Svedeniya {
	return Svedeniya{KlyuchHosta: s.KlyuchHosta, Smenilsya: s.KlyuchHostaSmenilsya,
		Root: s.Root, Put: s.Put, Tunnel: s.tunnel}
}

func (s soedinenie) KlyuchRoot(ctx context.Context, podpis gossh.Signer, kommentariy string, pustoy bool) (Svyaz, error) {
	r, err := s.PolozhitKlyuchRoot(ctx, podpis, kommentariy, pustoy)
	if err != nil {
		return nil, err
	}
	return soedinenie{r, s.tunnel}, nil
}

func (s soedinenie) Perevoyti(ctx context.Context, podpis gossh.Signer) (Svyaz, error) {
	r, err := s.Soedinenie.Perevoyti(ctx, podpis)
	if err != nil {
		return nil, err
	}
	return soedinenie{r, s.tunnel}, nil
}

// Шаги, которые тесты подменяют.
var (
	podklyuchit = func(ctx context.Context, z ssh.Zapros) (Svyaz, error) {
		s, err := ssh.Podklyuchit(ctx, z)
		if err != nil {
			return nil, err
		}
		return soedinenie{s, z.Put.Tunnel}, nil
	}
	seychas = time.Now
)

// Puti это дороги до сервера (§3.5 плана).
type Puti struct {
	// Pryamo это путь через физический адаптер мимо туннеля. Ошибка значит,
	// что такого адаптера нет.
	Pryamo func() (ssh.Put, error)
	// Tunnel это путь через вход ядра. ok false - пути нет: туннель не поднят
	// или вход на 10809 подняла не служба, и через него SSH человека ушёл бы
	// в чужой выход.
	Tunnel func() (put ssh.Put, ok bool)
}

// Molchanie: сервер не ответил ни одной дорогой. Tunnel пуст, если дороги
// через туннель не было.
type Molchanie struct {
	Pryamo, Tunnel error
}

func (m *Molchanie) Error() string {
	if m.Tunnel == nil {
		return "напрямую: " + m.Pryamo.Error() + "; через туннель не проверить"
	}
	return "напрямую: " + m.Pryamo.Error() + "; через туннель: " + m.Tunnel.Error()
}

func (m *Molchanie) Unwrap() []error {
	if m.Tunnel == nil {
		return []error{m.Pryamo}
	}
	return []error{m.Pryamo, m.Tunnel}
}

// molchit: дорога не довела до сервера. Отказ порта тоже: сброс подделывают
// и сами ТСПУ, а через туннель видно, слушает ли там sshd на самом деле.
func molchit(err error) bool {
	switch ssh.PrichinaIz(err) {
	case ssh.PrichinaTishina, ssh.PrichinaObryv, ssh.PrichinaPortZakryt:
		return true
	}
	return false
}

// Voyti входит на сервер: напрямую, а если прямой путь молчит, через туннель
// (§3.5). Отказ входа, ключа хоста или прав второй дорогой не проверяется:
// сервер уже ответил, и ответит так же.
func Voyti(ctx context.Context, p Puti, z ssh.Zapros) (_ Svyaz, err error) {
	defer zashchita("Voyti", &err)
	if p.Pryamo == nil || p.Tunnel == nil {
		return nil, errors.New("вход: дороги до сервера не заданы")
	}
	put, err := p.Pryamo()
	if err != nil {
		return nil, err
	}
	z.Put = put
	s, pryamo := podklyuchit(ctx, z)
	if pryamo == nil {
		return s, nil
	}
	if !molchit(pryamo) || ctx.Err() != nil {
		return nil, pryamo
	}
	put, ok := p.Tunnel()
	if !ok {
		return nil, &Molchanie{Pryamo: pryamo}
	}
	z.Put = put
	s, err = podklyuchit(ctx, z)
	switch {
	case err == nil:
		return s, nil
	case molchit(err):
		return nil, &Molchanie{Pryamo: pryamo, Tunnel: err}
	}
	return nil, err
}

// zashchita ставится первой строкой каждого входа: пакет живёт в службе.
func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/upravlenie."+gde, r)
		*err = fmt.Errorf("паника в upravlenie.%s: %v", gde, r)
	}
}
