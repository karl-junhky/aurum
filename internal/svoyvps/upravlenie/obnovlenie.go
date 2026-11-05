package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var (
	zamenitProgrammu     = server.ZamenitProgrammu
	programmaNaMeste     = server.ProgrammaNaMesteSeychas
	podtverditObnovlenie = server.PodtverditObnovlenie
	otkatitObnovlenie    = server.OtkatitObnovlenie

	// srokPodyoma: за столько после замены новая программа обязана подняться,
	// а сервер снова пустить SSH. VPN через этот же сервер рвётся вместе с
	// программой и встаёт заново, вход через туннель его ждёт.
	srokPodyoma = 2 * time.Minute
	shagPodyoma = 5 * time.Second
	// srokVozvrata: прежняя программа возвращается и тогда, когда дело уже
	// отменено. Её перезапуск рвёт вход через туннель этого же сервера, и
	// срок покрывает новый вход после него.
	srokVozvrata = 2 * time.Minute
	// zapasPodtverzhdeniya остаётся от таймера отката на сервере после
	// проверки ключей: подтверждение обязано успеть раньше него.
	zapasPodtverzhdeniya = time.Minute
)

// ShagPerezapusk: новая программа на месте, служба на сервере
// перезапускается, клиент ждёт её новым входом.
const ShagPerezapusk Shag = "perezapusk"

// Obnovlenie это всё, что нужно обновлению программы сервера (З2).
type Obnovlenie struct {
	Puti      Puti
	Vhod      ssh.Zapros
	Programma io.ReadSeeker
	// Versiya это номер программы в Programma (0.7.0): по нему видно, что
	// поднялась именно она.
	Versiya string
	// Klyuchi это ключи видов, что провозили из сети человека до обновления:
	// после него хоть один обязан провезти (Д4). Пусто - судит одна
	// программа сервера.
	Klyuchi  []protokol.Server
	Proverka proverka.Opcii
	Zapusk   proverka.Zapusk
	Hod      func(Hod)
}

// ObnovlenieItog это итог обновления. Bylo и Versiya - номера программы до
// и после.
type ObnovlenieItog struct {
	Vhod          Svedeniya
	Bylo, Versiya string
	// Uzhe: эта программа уже стояла, менять было нечего.
	Uzhe     bool
	Proverki []proverka.Itog
}

// NeObnovilsya: новая программа встала, но не поднялась или не провезла
// ключи, и прежняя возвращена. Vozvrat - почему вернуть её сразу не вышло:
// тогда её вернёт таймер на сервере, через 10 минут от замены.
type NeObnovilsya struct {
	Prichina error
	Vozvrat  error
}

func (n *NeObnovilsya) Error() string {
	s := "новая программа сервера не прошла проверку: " + n.Prichina.Error()
	if n.Vozvrat != nil {
		s += "; прежняя сразу не вернулась: " + n.Vozvrat.Error()
	}
	return s
}

func (n *NeObnovilsya) Unwrap() error { return n.Prichina }

// NePodnyalas: новая программа не поднялась за srokPodyoma. Posledneye -
// что видели последним.
type NePodnyalas struct{ Posledneye error }

func (n *NePodnyalas) Error() string {
	return fmt.Sprintf("новая программа не поднялась за %v: %v", srokPodyoma, n.Posledneye)
}

func (n *NePodnyalas) Unwrap() error { return n.Posledneye }

var (
	// ErrZamenaNeVstala: на диске после замены не новая программа - замена
	// оборвалась до подмены.
	ErrZamenaNeVstala = errors.New("новая программа так и не встала на место")
	// ErrKlyuchiNeVezut: после обновления ни один ключ не провёз из сети
	// человека, хотя до него провозили.
	ErrKlyuchiNeVezut = errors.New("после обновления ни один ключ не провёз")
	// ErrOtkatilsyaSam: подтверждение опоздало, таймер сервера уже вернул
	// прежнюю программу.
	ErrOtkatilsyaSam = errors.New("сервер вернул прежнюю программу раньше, чем пришло подтверждение")
	// ErrProverkaDolgo: проверка ключей не уложилась в свой срок или в то,
	// что осталось от таймера отката.
	ErrProverkaDolgo = errors.New("проверка ключей не уложилась в срок")
)

// NePodtverzhdeno: новая программа прошла проверку, а подтверждение до
// сервера не дошло. Через 10 минут от замены таймер вернёт прежнюю.
type NePodtverzhdeno struct{ Err error }

func (n *NePodtverzhdeno) Error() string {
	return "новая программа работает, а подтверждение не дошло до сервера: " + n.Err.Error()
}

func (n *NePodtverzhdeno) Unwrap() error { return n.Err }

// Obnovit ставит на свой сервер программу из выпуска (З2): вход и осмотр,
// замена с таймером отката, новый вход и ожидание новой программы,
// проверка ключей из сети человека и подтверждение. Всё, что не прошло
// после замены, возвращает прежнюю программу.
func Obnovit(ctx context.Context, o Obnovlenie) (it ObnovlenieItog, err error) {
	defer zashchita("Obnovit", &err)
	if o.Programma == nil || o.Vhod.Vhod.Podpis == nil || o.Versiya == "" || (len(o.Klyuchi) > 0 && o.Zapusk == nil) {
		return it, errors.New("обновление: не задана программа, её версия, ключ управления или запуск проверки")
	}
	hod := func(h Hod) {
		if o.Hod != nil {
			o.Hod(h)
		}
	}
	// Перезапуск программы идёт очередью systemd без этого входа: VPN через
	// этот же сервер делу не помеха (obnovlenie.sh).
	z := o.Vhod
	z.BezPerezapuska = true

	hod(Hod{Shag: ShagVhod})
	s, err := Voyti(ctx, o.Puti, z)
	if err != nil {
		return it, err
	}
	it.Vhod = s.Svedeniya()
	hod(Hod{Shag: ShagOsmotr})
	osm, err := osmotret(ctx, s)
	if err == nil {
		switch ot := osm.Otmetka; {
		case ot == nil:
			err = server.ErrNetUstanovki
		case ot.Sostoyanie != "gotovo":
			err = ErrNeDoshla
		case !nashaOtmetka(ot, z.Vhod.Podpis.PublicKey()):
			err = ErrChuzhayaUstanovka
		}
	}
	if err != nil {
		s.Zakryt()
		return it, err
	}

	hod(Hod{Shag: ShagProgramma})
	// Таймер отката на сервере заводится внутри замены: отсчёт от её начала
	// с запасом.
	zamena := seychas()
	zam, err := zamenitProgrammu(ctx, s, o.Programma)
	s.Zakryt()
	it.Bylo = nomerVersii(zam.Bylo.Versiya)
	var obryv *server.ObryvZameny
	switch {
	case errors.As(err, &obryv):
		// Вход оборвался посреди замены: что на месте, скажет новый вход.
	case err != nil:
		return it, err
	case zam.Uzhe:
		it.Uzhe, it.Versiya = true, nomerVersii(zam.Versiya)
		return it, nil
	}

	// Дальше на сервере может стоять непроверенная новая программа: любой
	// отказ возвращает прежнюю.
	hod(Hod{Shag: ShagPerezapusk})
	s2, err := dozhdatsyaNovoy(ctx, o.Puti, z, o.Versiya)
	if s2 != nil {
		defer func() { s2.Zakryt() }()
	}
	if err != nil {
		return it, vernutPrezhnyuyu(ctx, o.Puti, z, s2, o.Versiya, err)
	}
	if len(o.Klyuchi) > 0 {
		hod(Hod{Shag: ShagProverka})
		op := o.Proverka
		op.Vyhody = vyhody(osm)
		op.PervyyNaVid = false
		op.Hod = func(k proverka.Itog) { hod(Hod{Shag: ShagProverka, Klyuch: &k}) }
		srok := proverka.Predel(len(o.Klyuchi), vidovKlyuchey(o.Klyuchi))
		srok = min(srok, server.SrokOtkata-zapasPodtverzhdeniya-seychas().Sub(zamena))
		pctx, otmena := context.WithTimeout(ctx, srok)
		itogi, err := proverit(pctx, o.Klyuchi, op, o.Zapusk)
		dolgo := pctx.Err() != nil
		otmena()
		it.Proverki = itogi
		if err == nil && !slices.ContainsFunc(itogi, func(x proverka.Itog) bool { return x.Vid == proverka.Vezyot }) {
			err = ErrKlyuchiNeVezut
		}
		if err != nil {
			switch {
			case ctx.Err() != nil:
				err = ctx.Err()
			case dolgo:
				err = ErrProverkaDolgo
			}
			return it, vernutPrezhnyuyu(ctx, o.Puti, z, s2, o.Versiya, err)
		}
	}
	v, err := podtverditObnovlenie(ctx, s2)
	if err != nil {
		return it, &NePodtverzhdeno{err}
	}
	if nomerVersii(v) != o.Versiya {
		return it, ErrOtkatilsyaSam
	}
	it.Versiya = o.Versiya
	return it, nil
}

// dozhdatsyaNovoy входит заново, пока новая программа не встанет на диске и
// не поднимет ядро, не дольше srokPodyoma. Отдаёт и вход, если он есть:
// через него возвращают прежнюю.
func dozhdatsyaNovoy(ctx context.Context, p Puti, z ssh.Zapros, versiya string) (Svyaz, error) {
	konec := seychas().Add(srokPodyoma)
	var (
		s          Svyaz
		posledneye error
	)
	for {
		// Перезапуск стоит в очереди systemd: первая проба через паузу.
		if err := pauza(ctx, shagPodyoma); err != nil {
			return s, err
		}
		if s == nil {
			var err error
			if s, err = Voyti(ctx, p, z); err != nil {
				s, posledneye = nil, err
			}
		}
		if s != nil {
			na, err := programmaNaMeste(ctx, s)
			if err != nil {
				// Вход оборвался вместе с туннелем через этот же сервер: войти
				// заново.
				s.Zakryt()
				s, posledneye = nil, err
			} else if v := nomerVersii(na.Versiya); v != versiya {
				// Замена после обрыва входа могла ещё идти на сервере: ждать
				// до срока. Новая на месте, но молчит - она не поднялась.
				posledneye = ErrZamenaNeVstala
				if v == "" && na.OtkatIdyot {
					posledneye = errors.New("программа на месте не называет свою версию")
				}
			} else {
				u, err := sostoyanie(ctx, s)
				switch {
				case err != nil:
					// Пульт ещё не открыт: программа поднимается.
					posledneye = err
				case nomerVersii(u.Versiya) != versiya:
					posledneye = fmt.Errorf("работает ещё прежняя программа (%s)", u.Versiya)
				case !u.YadroZhivo:
					posledneye = fmt.Errorf("ядро не поднялось: %s", u.Oshibka)
				default:
					return s, nil
				}
			}
		}
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		if !seychas().Before(konec) {
			if errors.Is(posledneye, ErrZamenaNeVstala) {
				return s, ErrZamenaNeVstala
			}
			return s, &NePodnyalas{posledneye}
		}
	}
}

// vernutPrezhnyuyu откатывает обновление до программы, что не versiya.
// Отменённое дело тоже возвращает прежнюю: срок свой, srokVozvrata.
func vernutPrezhnyuyu(ctx context.Context, p Puti, z ssh.Zapros, s Svyaz, versiya string, prichina error) error {
	vctx, otmena := context.WithTimeout(context.WithoutCancel(ctx), srokVozvrata)
	defer otmena()
	return &NeObnovilsya{Prichina: prichina, Vozvrat: otkatit(vctx, p, z, s, versiya)}
}

// otkatit возвращает прежнюю программу входом s, а без него входит заново.
// Перезапуск прежней рвёт и вход через туннель этого же сервера, и ответ
// отката до клиента не доходит: тогда что на месте, говорит новый вход, а
// не дошедший откат повторяется им же. Вход, открытый здесь, здесь и
// закрывается.
func otkatit(ctx context.Context, p Puti, z ssh.Zapros, s Svyaz, versiya string) error {
	if s == nil {
		var err error
		if s, err = Voyti(ctx, p, z); err != nil {
			return err
		}
		defer s.Zakryt()
	}
	_, err := otkatitObnovlenie(ctx, s)
	var osh *server.Oshibka
	if err == nil || errors.As(err, &osh) {
		return err
	}
	for pauza(ctx, shagPodyoma) == nil {
		s2, verr := Voyti(ctx, p, z)
		if verr != nil {
			continue
		}
		na, nerr := programmaNaMeste(ctx, s2)
		switch {
		case nerr != nil:
			// Туннель ещё встаёт: войти снова.
		case nomerVersii(na.Versiya) != versiya && !na.OtkatIdyot:
			s2.Zakryt()
			return nil
		default:
			if _, err = otkatitObnovlenie(ctx, s2); err == nil || errors.As(err, &osh) {
				s2.Zakryt()
				return err
			}
		}
		s2.Zakryt()
	}
	return err
}

func pauza(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// vidovKlyuchey это число разных видов (транспортов) среди ключей.
func vidovKlyuchey(k []protokol.Server) int {
	vidy := map[string]bool{}
	for _, x := range k {
		vidy[x.Transport] = true
	}
	return len(vidy)
}
