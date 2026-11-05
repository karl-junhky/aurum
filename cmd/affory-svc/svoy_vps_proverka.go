package main

import (
	"context"
	"errors"
	"log"
	"os"
	"slices"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// «Проверить сервер» (З1) и срок сертификата в общем статусе (Е8, §3.4).
// Проверка идёт тем же заданием, что установка и снос: по кнопке окна и
// раз в сутки сама, пока работает служба. Компьютер человека может не
// включаться неделю, поэтому сертификат продлевает программа сервера, а
// служба лишь узнаёт, как идут дела, и говорит о беде в статусе.

const (
	vidProverka = "proverka"

	// pervayaProverkaSvoih: служба только встала, сеть и туннель ещё
	// поднимаются.
	pervayaProverkaSvoih = 2 * time.Minute
	periodProverkiSvoih  = time.Hour
	// svezhestProverki: сервер, проверенный раньше, проверяется снова. Удачи
	// и неудачи считаются одинаково: молчащий сервер не долбится каждый час.
	svezhestProverki = 24 * time.Hour
)

// srokProverki держит вход и осмотр сервера: у каждой команды SSH свой срок
// до 10 минут, а задание на службу одно. Здоровая проверка идёт секунды,
// срыв прямой дороги на VPN добавляет полминуты. Ключи, что не проходили из
// сети, гоняются после со своим сроком proverka.Predel.
var srokProverki = 3 * time.Minute

var errProverkaDolgo = errors.New("проверка не уложилась в 3 минуты: сервер отвечает слишком медленно")

// PortVps это вход своего сервера глазами осмотра: vid reality или hy2.
type PortVps struct {
	Vid      string `json:"vid"`
	Port     int    `json:"port"`
	Slushaet bool   `json:"slushaet"`
}

// ProverkaVps это итог проверки для карточки. Какой сервер проверялся,
// говорит HodVps.Server.
type ProverkaVps struct {
	// Versiya пуста, если программа сервера не ответила: тогда причина в
	// ProgrammaOtkaz.
	Versiya        string    `json:"versiya,omitempty"`
	ProgrammaOtkaz string    `json:"programma_otkaz,omitempty"`
	YadroZhivo     bool      `json:"yadro_zhivo"`
	OshibkaYadra   string    `json:"oshibka_yadra,omitempty"`
	Porty          []PortVps `json:"porty"`
	// Срок сертификата в Unix-секундах; ноль - сертификата на диске нет или
	// программа не ответила.
	SertDo      int64  `json:"sert_do,omitempty"`
	SertTrevoga bool   `json:"sert_trevoga,omitempty"`
	SertOtkaz   string `json:"sert_otkaz,omitempty"`
	// Maska: godna, negodna или ne-provereno (нет openssl или проверка
	// упала, тогда MaskaOtkaz).
	Maska      string `json:"maska"`
	MaskaOtkaz string `json:"maska_otkaz,omitempty"`
	// Свободная память и место на /var/lib, -1 - не узнали.
	PamyatMB int `json:"pamyat_mb"`
	DiskMB   int `json:"disk_mb"`
}

// svoyVpsProverit проверяет свой сервер по ключу управления.
func (s *Sluzhba) svoyVpsProverit(k protokol.Kadr) protokol.Kadr {
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidProverka, s.deloProverki(z, zapros))
}

// vhodUpravleniya собирает вход root по ключу управления. Ключа на этом
// компьютере нет - отказ KodVpsVzyat: окно предлагает взять сервер.
func (s *Sluzhba) vhodUpravleniya(z SvoyServer) (ssh.Zapros, string, error) {
	podpis, ok, err := s.klyuchUpravleniya(z.Id)
	if err != nil {
		return ssh.Zapros{}, protokol.KodSecretsUnreadable, err
	}
	if !ok {
		return ssh.Zapros{}, protokol.KodVpsVzyat, errNetKlyuchaUpravleniya
	}
	zapros, err := vhodKlyuchom(z, podpis)
	if err != nil {
		return ssh.Zapros{}, protokol.KodSecretsUnreadable, err
	}
	return zapros, "", nil
}

// deloProverki входит, смотрит, гоняет из сети ключи видов, что не
// проходили из неё (snyatIzSeti), и записывает итог в запись о сервере.
// Отменённая проверка не пишет ничего.
func (s *Sluzhba) deloProverki(z SvoyServer, zapros ssh.Zapros) func(context.Context, *zadanieVps) error {
	return func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id })
		sctx, otmena := context.WithTimeout(ctx, srokProverki)
		defer otmena()
		zd, err := s.vpsProverit(sctx, s.putiVps(), zapros, z.Maska)
		seychas := s.seychas().UTC()
		// Срок вышел посреди шага: шаг называет свою причину, а человеку
		// нужна одна - сервер не успел.
		if err != nil && ctx.Err() == nil && sctx.Err() != nil {
			log.Printf("свой сервер %s: проверка вышла за срок: %v", z.Id, err)
			err = errProverkaDolgo
		}
		if err != nil {
			if ctx.Err() == nil {
				tekst := otkazVps(err).Tekst
				if err := s.otmetitProverku(z.Id, func(x *SvoyServer) {
					// Ключ хоста в записи успели сменить (приём нового), пока
					// проверка шла со старым: её отказ уже не про эту запись.
					if x.KlyuchHostaTip == z.KlyuchHostaTip && x.KlyuchHosta == z.KlyuchHosta {
						x.Proveren, x.ProverkaOtkaz = seychas, tekst
					}
				}); err != nil {
					log.Printf("свой сервер %s: неудача проверки не записана: %v", z.Id, err)
				}
			}
			return err
		}
		p := proverkaDlyaEkrana(z, zd)
		snyat := s.snyatIzSeti(ctx, zad, z, p, zd.Osmotr)
		if err := ctx.Err(); err != nil {
			return err
		}
		zapisana := s.otmetitProverku(z.Id, func(x *SvoyServer) {
			x.Proveren, x.ProverkaOtkaz = seychas, ""
			if len(snyat) > 0 {
				bylo := len(x.NeIzSeti)
				x.NeIzSeti = slices.DeleteFunc(x.NeIzSeti, func(t string) bool { return slices.Contains(snyat, t) })
				// Общая причина писалась на все виды разом и могла назвать
				// снятый: оставшимся остаются их собственные.
				if len(x.NeIzSeti) < bylo {
					x.SetTekst = tekstVidov(x.NeIzSeti, x.SetPoVidam)
				}
				// Свои причины нужны, пока есть кого снимать по одному.
				if len(x.NeIzSeti) < 2 {
					x.SetPoVidam = nil
				}
			}
			// Пароль могли выключить и включить с другого компьютера или руками.
			x.ParolVyklyuchen = slices.Contains(zd.Osmotr.SshdFayly, server.FaylParolya)
			if p.Versiya != "" {
				x.Versiya = p.Versiya
			}
			// Программа не ответила - о сертификате нового не узнали, и
			// прежние сведения остаются со своей отметкой.
			if zd.UzelOtkaz == nil {
				x.SertDo, x.SertTrevoga, x.SertOtkaz, x.SertProveren = p.SertDo, p.SertTrevoga, p.SertOtkaz, seychas
			} else {
				x.ProverkaOtkaz = "программа сервера не ответила: " + p.ProgrammaOtkaz
			}
		})
		if zapisana != nil {
			log.Printf("свой сервер %s: итог проверки не записан: %v", z.Id, zapisana)
		}
		s.pravitHod(zad, func(h *HodVps) {
			h.Put, h.Proverka = zd.Vhod.Put, &p
			if zapisana != nil {
				h.Preduprezhdeniya = append(h.Preduprezhdeniya, OtkazVps{Kod: kodSohraneniya(zapisana), Tekst: sboi.DlyaCheloveka(zapisana)})
			}
		})
		return nil
	}
}

func proverkaDlyaEkrana(z SvoyServer, zd upravlenie.ZdorovyeServera) ProverkaVps {
	o := zd.Osmotr
	p := ProverkaVps{PamyatMB: o.PamyatMB, DiskMB: o.DiskMB, Porty: []PortVps{}}
	if z.Reality > 0 {
		p.Porty = append(p.Porty, PortVps{Vid: "reality", Port: z.Reality, Slushaet: o.SlushaetSvoy("tcp", z.Reality)})
	}
	if z.Hy2 > 0 {
		p.Porty = append(p.Porty, PortVps{Vid: "hy2", Port: z.Hy2, Slushaet: o.SlushaetSvoy("udp", z.Hy2)})
	}
	if zd.UzelOtkaz != nil {
		p.ProgrammaOtkaz = sboi.DlyaCheloveka(zd.UzelOtkaz)
	} else {
		u := zd.Uzel
		p.Versiya, p.YadroZhivo, p.OshibkaYadra = u.Versiya, u.YadroZhivo, u.Oshibka
		if u.Sert != nil {
			if u.Sert.Est {
				p.SertDo = u.Sert.Do
			}
			p.SertTrevoga, p.SertOtkaz = u.Sert.Trevoga, u.Sert.Otkaz
		}
	}
	switch {
	case zd.MaskaOtkaz != nil:
		p.Maska, p.MaskaOtkaz = "ne-provereno", sboi.DlyaCheloveka(zd.MaskaOtkaz)
	case !zd.MaskaProverena:
		p.Maska = "ne-provereno"
	case zd.Maska:
		p.Maska = "godna"
	default:
		p.Maska = "negodna"
	}
	return p
}

// snyatIzSeti отдаёт виды, которые пора снять с отметки «не прошли из
// сети»: ключ вида провёз проверку из сети человека или его в списке больше
// нет. Иначе «нужно внимание» висело бы до подбора портов заново, хотя адрес
// давно открылся. Гоняются только ключи видов, чей порт на сервере слушает:
// мёртвый на сервере ключ не провезёт, а проверка держала бы задание минутами.
// Плановая проверка ключей не гоняет вовсе: ключ закрытого адреса ждёт до
// минуты, и команды человека отказывали бы минутами раз в сутки. Проверить
// не вышло - отметка остаётся.
func (s *Sluzhba) snyatIzSeti(ctx context.Context, zad *zadanieVps, z SvoyServer, p ProverkaVps, o *server.Osmotr) []string {
	if len(z.NeIzSeti) == 0 {
		return nil
	}
	n, err := s.nabor()
	if err != nil {
		log.Printf("свой сервер %s: ключи из сети не проверены: %v", z.Id, err)
		return nil
	}
	var snyat []string
	var klyuchi []protokol.Server
	vidov := 0
	for _, t := range z.NeIzSeti {
		var vida []protokol.Server
		for _, k := range n.Servery {
			if k.Transport == t && slices.Contains(z.Servery, k.Id) {
				vida = append(vida, k)
			}
		}
		switch {
		case len(vida) == 0:
			snyat = append(snyat, t)
		case slushaetVid(p, t):
			klyuchi, vidov = append(klyuchi, vida...), vidov+1
		}
	}
	if len(klyuchi) == 0 || zad.fonovoe {
		return snyat
	}
	a, err := s.fizicheskiyAdapter()
	if err != nil {
		log.Printf("свой сервер %s: ключи из сети не проверены: %v", z.Id, err)
		return snyat
	}
	s.pravitHod(zad, func(h *HodVps) { h.Shag = string(upravlenie.ShagProverka) })
	kctx, otmena := context.WithTimeout(ctx, proverka.Predel(len(klyuchi), vidov))
	defer otmena()
	itogi, err := s.vpsProveritKlyuchi(kctx, klyuchi, o, proverka.Opcii{Adapter: a.Imya}, s.zapuskProverki)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("свой сервер %s: ключи из сети не проверены: %v", z.Id, err)
		}
		return snyat
	}
	for _, it := range itogi {
		if it.Vid == proverka.Vezyot && !slices.Contains(snyat, it.Server.Transport) {
			snyat = append(snyat, it.Server.Transport)
		}
	}
	return snyat
}

// slushaetVid: проверка видела порт вида transport слушающим.
func slushaetVid(p ProverkaVps, transport string) bool {
	vid := "reality"
	if transport == "hy2" {
		vid = "hy2"
	}
	return slices.ContainsFunc(p.Porty, func(x PortVps) bool { return x.Vid == vid && x.Slushaet })
}

// otmetitProverku правит запись id, если она ещё есть: сервер могли
// забыть, пока шла проверка.
func (s *Sluzhba) otmetitProverku(id string, fn func(*SvoyServer)) error {
	return s.pravitNabor(func(n *Nabor) error {
		if i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id }); i >= 0 {
			fn(&n.SvoiServery[i])
		}
		return nil
	})
}

// raspisanieSvoih раз в час ищет свой сервер, который давно не
// проверялся, и проверяет один, самый давний. Занятость заданием или
// запор пропускаются до следующего часа.
func (s *Sluzhba) raspisanieSvoih(ctx context.Context) {
	pauza := pervayaProverkaSvoih
	for {
		if !s.zhdat(ctx, pauza) {
			return
		}
		pauza = periodProverkiSvoih
		s.proveritDavnego()
	}
}

func (s *Sluzhba) proveritDavnego() {
	n, err := s.nabor()
	if err != nil {
		log.Printf("плановая проверка своих серверов: %v", err)
		return
	}
	seychas := s.seychas()
	porog := seychas.Add(-svezhestProverki)
	var (
		vybran SvoyServer
		zapros ssh.Zapros
		est    bool
	)
	for _, z := range n.SvoiServery {
		// Отметка из будущего свежей не считается: часы компьютера уходили
		// вперёд, и иначе сервер выпал бы из проверок до той даты.
		svezhaya := z.Proveren.After(porog) && !z.Proveren.After(seychas)
		if svezhaya || (est && !z.Proveren.Before(vybran.Proveren)) {
			continue
		}
		// Без ключа управления проверять нечем: такой сервер ждёт захвата.
		zp, _, err := s.vhodUpravleniya(z)
		if err != nil {
			// Ключ есть, а войти им нельзя: это неудача проверки. Отметка
			// с причиной видна в карточке и не даёт повторять её каждый час.
			if !errors.Is(err, errNetKlyuchaUpravleniya) {
				log.Printf("свой сервер %s: плановая проверка не может войти: %v", z.Id, err)
				tekst := otkazVps(err).Tekst
				if err := s.otmetitProverku(z.Id, func(x *SvoyServer) {
					// Пишется в обход задания: запись могли переписать
					// установкой или захватом, и отказ был про прежнюю.
					if x.Proveren.Equal(z.Proveren) {
						x.Proveren, x.ProverkaOtkaz = seychas.UTC(), tekst
					}
				}); err != nil {
					log.Printf("свой сервер %s: неудача проверки не записана: %v", z.Id, err)
				}
			}
			continue
		}
		vybran, zapros, est = z, zp, true
	}
	if !est {
		return
	}
	if _, tekst := s.nachatVps(vidProverka, true, s.deloProverki(vybran, zapros)); tekst != "" {
		log.Printf("свой сервер %s: плановая проверка отложена: %s", vybran.Id, tekst)
	}
}

// Сертификаты в общем статусе.

// sertSvoego это то, по чему Status() судит о сертификате своего сервера
// без похода в хранилище.
type sertSvoego struct {
	id, imya, otkaz string
	do              int64
	trevoga         bool
	// svezh: когда сведения пришли от программы сервера; otkazProverki -
	// почему последняя проверка их не обновила; klyuch - этот компьютер
	// может проверять сервер.
	svezh         time.Time
	otkazProverki string
	klyuch        bool
	// bezSerta: у сервера нет hy2, и сертификата нет. versiya это программа
	// сервера по последней проверке: старая небезопасная тоже тревога (З2).
	bezSerta bool
	versiya  string
}

// svezhestSvedeniy: сведения старше этого о сроке не судят. Плановая
// проверка обновляет их раз в сутки, двое суток - это проверки не проходят.
const svezhestSvedeniy = 2 * svezhestProverki

// sertySvoih выбирает из набора то, по чему статус судит о своих серверах:
// сертификат (без hy2 его нет) и версию программы. estKlyuch говорит, есть
// ли ключ управления на этом компьютере: запись с отказом проверки могла
// прийти импортом с другого.
func sertySvoih(n Nabor, estKlyuch func(id string) bool) []sertSvoego {
	var serty []sertSvoego
	for _, z := range n.SvoiServery {
		serty = append(serty, sertSvoego{id: z.Id, imya: z.Imya, otkaz: z.SertOtkaz, do: z.SertDo, trevoga: z.SertTrevoga,
			svezh: z.SertProveren, otkazProverki: z.ProverkaOtkaz, klyuch: estKlyuch(z.Id),
			bezSerta: z.Prodlenie == "net", versiya: z.Versiya})
	}
	return serty
}

// estKlyuchUpravleniya смотрит только, лежит ли файл: читать и
// расшифровывать ключ ради статуса незачем.
func (s *Sluzhba) estKlyuchUpravleniya(id string) bool {
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(put)
	return err == nil
}

// svoiTrevogiDlyaStatusa зовётся под mu. Без своих серверов часы не
// спрашиваются.
func (s *Sluzhba) svoiTrevogiDlyaStatusa() []protokol.SvoyaTrevoga {
	if len(s.svoiSerty) == 0 {
		return nil
	}
	return trevogiSvoih(s.svoiSerty, s.seychas())
}

// trevogiSvoih: срок считается от часов этого компьютера, тревога и отказ
// с последней проверки. Тревогу программа сервера ставит сама: сертификата
// нет после отказа, до конца меньше суток или продление не выходит дольше
// шести часов. По старым сведениям о сроке судить нельзя: программа могла
// продлить сертификат без нас. Тогда тревога, только если срок мог выйти, а
// проверка с этого компьютера не проходит. Сервер без ключа управления
// отсюда не проверяется вовсе, и по старым сведениям о нём молчим.
func trevogiSvoih(serty []sertSvoego, seychas time.Time) []protokol.SvoyaTrevoga {
	var trevogi []protokol.SvoyaTrevoga
	for _, x := range serty {
		if x.bezSerta {
			continue
		}
		ostalos := time.Unix(x.do, 0).Sub(seychas)
		staroe := seychas.Sub(x.svezh) > svezhestSvedeniy
		var tekst string
		switch {
		case staroe:
			if !x.klyuch || x.otkazProverki == "" || x.do == 0 || ostalos >= 24*time.Hour {
				continue
			}
			tekst = "о сертификате нет свежих сведений"
			if !x.svezh.IsZero() {
				tekst += " с " + x.svezh.Local().Format("02.01")
			}
			tekst += ", а срок его мог выйти. Проверка не проходит: " + x.otkazProverki
		case x.do > 0 && ostalos <= 0:
			tekst = "срок сертификата вышел, hysteria2 может не подключаться"
		case x.trevoga:
			tekst = "сертификат не продлевается"
		case x.do > 0 && ostalos < 24*time.Hour:
			tekst = "сертификат кончается меньше чем через сутки"
		default:
			continue
		}
		// Отказ продления в старых сведениях мог давно пройти.
		if x.otkaz != "" && !staroe {
			tekst += ": " + x.otkaz
		}
		trevogi = append(trevogi, protokol.SvoyaTrevoga{Id: x.id, Imya: x.imya,
			Tekst: tekst + ". Проверь сервер в разделе «Свой VPN»"})
	}
	// Тревоги о старой программе идут после тревог о сертификате: карточка
	// показывает первую тревогу сервера, а вышедший срок ломает hy2 уже сейчас.
	for _, x := range serty {
		// Обновить программу может только компьютер с ключом управления.
		if x.klyuch && x.versiya != "" && novee(bezopasnayaVersiyaUzla, x.versiya) {
			trevogi = append(trevogi, protokol.SvoyaTrevoga{Id: x.id, Imya: x.imya,
				Tekst: "на сервере программа " + x.versiya + " с уязвимостью, исправленной в новой. Обнови её в разделе «Свой VPN»"})
		}
	}
	return trevogi
}
