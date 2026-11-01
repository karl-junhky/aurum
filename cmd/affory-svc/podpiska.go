package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Отступ после неудачи. Отметка успеха на диске при неудаче не двигается,
// поэтому без собственного отступа следующий заход начинался бы немедленно:
// холостой цикл на весь процессор и лимит панели заодно.
const otstupPodpiski = 30 * time.Minute

var errPodpiskaNeZadana = errors.New("подписка не задана")
var errObnovlenieZameneno = errors.New("обновление заменено более новым запросом")

// oshibkaNabora и oshibkaSohraneniya отделяют «не смогли прочитать секреты» и
// «не смогли записать» от «панель не ответила». Без разделения все три отказа
// уезжали бы в subscription-unreachable, и человек чинил бы сеть вместо диска.
type oshibkaNabora struct{ err error }

func (o oshibkaNabora) Error() string { return o.err.Error() }
func (o oshibkaNabora) Unwrap() error { return o.err }

type oshibkaSohraneniya struct{ err error }

func (o oshibkaSohraneniya) Error() string { return o.err.Error() }
func (o oshibkaSohraneniya) Unwrap() error { return o.err }

// obnovitPodpisku это ЕДИНСТВЕННЫЙ путь обновления списка из подписки.
//
// Сюда ходят все трое: команда refreshSubscription, задание адреса и
// расписание. Три копии одного и того же неизбежно разъехались бы, а разъезд
// был бы тихим: список наполнялся бы одним путём и не наполнялся другим, ровно
// как в находке 35.
func (s *Sluzhba) obnovitPodpisku(ctx context.Context) (ssylki.Razbor, int, error) {
	n, err := s.nabor()
	if err != nil {
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	return s.obnovitPodpiskuPoId(ctx, n.Aktivnaya)
}

// Ответ сети принадлежит исходной подписке, даже если за время загрузки
// человек переключился на другую. Удалённую запись ответ не воскрешает.
func (s *Sluzhba) obnovitPodpiskuPoId(ctx context.Context, id string) (ssylki.Razbor, int, error) {
	ctx, otmena := context.WithCancel(ctx)
	defer otmena()
	if s.fonCtx != nil {
		stop := context.AfterFunc(s.fonCtx, otmena)
		defer stop()
	}
	if err := ctx.Err(); err != nil {
		return ssylki.Razbor{}, 0, err
	}
	muNabor.Lock()
	n, err := s.nabor()
	if err != nil {
		muNabor.Unlock()
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	z := n.zapisPodpiski(id)
	if z == nil {
		muNabor.Unlock()
		return ssylki.Razbor{}, 0, errPodpiskaNeZadana
	}
	adres := z.Adres
	s.nomerObnovleniya++
	pokolenie := s.nomerObnovleniya
	if s.obnovleniyaPodpisok == nil {
		s.obnovleniyaPodpisok = make(map[string]uint64)
	}
	s.obnovleniyaPodpisok[id] = pokolenie
	muNabor.Unlock()
	defer func() {
		muNabor.Lock()
		if s.obnovleniyaPodpisok[id] == pokolenie {
			delete(s.obnovleniyaPodpisok, id)
		}
		muNabor.Unlock()
	}()
	// Контекст операции для подробного журнала (A7). Адрес подписки это
	// пропуск, поэтому в строку идёт только его обезличенный отпечаток: две
	// записи одной подписки сходятся между собой и не сходятся ни с чем
	// снаружи.
	nachalo := s.seychas()
	zapisatOperatsiyu := func(itog string, err error) {
		_ = s.zhurnalDiag.SobytieOperatsii(diagnostika.Operatsiya{
			Vid:        "podpiska",
			Pokolenie:  pokolenie,
			Dlitelnost: s.seychas().Sub(nachalo),
			Itog:       itog,
			Shag:       string(shagZagruzki(err)),
			Istochnik:  diagnostika.Obezlichit(adres),
		})
	}
	r, err := s.zagruzitPodpisku(ctx, adres)
	if err != nil {
		zapisatOperatsiyu("otkaz", err)
		// Технический текст только сюда: человеку уходит prichinaPodpiski, а
		// разбирать жалобу без исходной ошибки нечем. Адреса в тексте нет,
		// загрузчик вычищает его сам (ssylki.bezAdresa).
		log.Printf("подписка %s: %v", diagnostika.Obezlichit(adres), err)
		muNabor.Lock()
		defer muNabor.Unlock()
		if s.obnovleniyaPodpisok[id] != pokolenie {
			return ssylki.Razbor{}, 0, errObnovlenieZameneno
		}
		ostatok := -1
		if ctx.Err() == nil {
			ostatok = s.otmetitOtkazPodpiski(id, adres, err)
		}
		return r, 0, otkazSOstatkom{err: err, serverov: ostatok}
	}
	serverov := 0
	for i := range r.Servery {
		r.Servery[i].IzPodpiski = true
	}
	aktivnaya := false
	teper := s.seychas()
	if err := s.pravitNabor(func(n *Nabor) error {
		if s.obnovleniyaPodpisok[id] != pokolenie {
			return errObnovlenieZameneno
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		ostanovlena := s.ostanovlena
		s.mu.Unlock()
		if ostanovlena {
			return context.Canceled
		}
		z := n.zapisPodpiski(id)
		if z == nil || z.Adres != adres {
			return errors.New("подписка удалена во время обновления")
		}
		aktivnaya = n.Aktivnaya == id
		if aktivnaya {
			n.Servery = ssylki.Slit(n.Servery, r.Servery)
			serverov = len(n.Servery)
		} else {
			z.Servery = ssylki.Slit(z.Servery, r.Servery)
			serverov = len(z.Servery)
		}
		n.OtmetitObnovlenie(id, teper)
		return nil
	}); err != nil {
		zapisatOperatsiyu("otkaz-zapisi", err)
		return r, 0, oshibkaSohraneniya{err}
	}
	if aktivnaya {
		muNabor.Lock()
		if s.obnovleniyaPodpisok[id] == pokolenie {
			s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodpiskaObnovlena = &teper })
		}
		muNabor.Unlock()
	}
	zapisatOperatsiyu("ok", nil)
	return r, serverov, nil
}

// ZapustitRaspisanie заводит расписание с владельцем, который умеет и отменить
// его, и дождаться. Горутина без такого владельца это класс F ворот Ш8.
// Add и Done в ОДНОМ месте, у владельца. Пока Done жил внутри цикла, прямой
// вызов цикла из теста уводил счётчик в минус, и Wait в уборке висел навсегда:
// красный превращался в зависание, то есть в худшую форму красного.
func (s *Sluzhba) ZapustitRaspisanie() {
	s.fon.Add(1)
	fon.Zapustit("расписании подписки", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "расписание подписки", pauzaPoslePaniki, func() {
			s.raspisaniePodpiski(s.fonCtx)
		})
	})
	s.fon.Add(1)
	fon.Zapustit("расписании обновлений", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "расписание обновлений", pauzaPoslePaniki, func() {
			s.raspisanieObnovleniy(s.fonCtx)
		})
	})
	// Список рекламы крутится всегда и спит, пока блокировка выключена: будит
	// его толчок из setRules.
	s.fon.Add(1)
	fon.Zapustit("расписании списка рекламы", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "расписание списка рекламы", pauzaPoslePaniki, func() {
			s.raspisanieReklamy(s.fonCtx)
		})
	})
	// Подробный журнал крутится ВСЕГДА и молчит, пока настройка выключена.
	// Заводить и гасить горутину по щелчку настройки значит завести гонку там,
	// где такт стоит одного сравнения.
	s.fon.Add(1)
	fon.Zapustit("сборе диагностики", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "сбор диагностики", pauzaPoslePaniki, func() {
			s.sobiratDiagnostiku(s.fonCtx)
		})
	})
	// Помехи тоже всегда: строка о zapret на главном экране нужна и до
	// подключения, и уйти она должна сама, когда zapret закрыли.
	s.fon.Add(1)
	fon.Zapustit("поиске помех", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "поиск помех", pauzaPoslePaniki, func() {
			s.raspisaniePomeh(s.fonCtx)
		})
	})
	s.fon.Add(1)
	fon.Zapustit("сверке часов", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "сверка часов", pauzaPoslePaniki, func() {
			s.raspisanieChasov(s.fonCtx)
		})
	})
	s.fon.Add(1)
	fon.Zapustit("проверке своих серверов", func() {
		defer s.fon.Done()
		fon.SPovtorom(s.fonCtx, "проверка своих серверов", pauzaPoslePaniki, func() {
			s.raspisanieSvoih(s.fonCtx)
		})
	})
}

// raspisaniePodpiski тянет подписку при старте и дальше раз в период.
//
// Отметка берётся С ДИСКА: без неё период отсчитывается от старта службы, то
// есть после каждой перезагрузки токен подписки светится в сети заново.
func (s *Sluzhba) raspisaniePodpiski(ctx context.Context) {
	var poslednyaya time.Time
	if f, err := s.prochitat(); err == nil && f.PodpiskaObnovlena != nil {
		poslednyaya = *f.PodpiskaObnovlena
	}
	var povtor time.Time

	for {
		if pauza := s.srokObnovleniya(poslednyaya, povtor).Sub(s.seychas()); pauza > 0 {
			if !s.zhdat(ctx, pauza) {
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		// Обход ВСЕХ подписок, а не только активной: запасная, за которой не
		// ходят, это адрес, а не подписка, и переключение на неё означало бы
		// поход в сеть ровно в ту минуту, когда человеку нужен туннель.
		// Отступ считается по отказу АКТИВНОЙ: молчащая запасная панель не
		// повод долбить остальные чаще положенного.
		if _, _, err := s.obnovitVsePodpiski(ctx); err != nil {
			povtor = s.seychas().Add(otstupPodpiski)
			continue
		}
		poslednyaya, povtor = s.seychas(), time.Time{}
	}
}

// srokObnovleniya отвечает на один вопрос: когда следующий заход.
func (s *Sluzhba) srokObnovleniya(poslednyaya, povtor time.Time) time.Time {
	if !povtor.IsZero() {
		return povtor
	}
	if poslednyaya.IsZero() {
		// Ни одного успешного обновления: тянем сразу. Это и есть загрузка при
		// старте, которой не существовало.
		return s.seychas()
	}
	return poslednyaya.Add(ssylki.PeriodObnovleniya)
}

func zhdatPoChasam(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
