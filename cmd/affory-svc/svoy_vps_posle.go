package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"slices"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// Управление своим сервером после установки (блок З плана): снос, захват,
// «забыть», «забыть и снять ключ».

const (
	vidSnos   = "snos"
	vidVzyat  = "vzyat"
	vidOtpusk = "otpusk"
	vidParol  = "parol"
)

// ParolVps это итог «Выключить вход по паролю» и «Включить» для окна.
type ParolVps struct {
	// Vyklyuchen: наш файл sshd на сервере лежит, паролем не пускают.
	Vyklyuchen bool `json:"vyklyuchen"`
	// Snaruzhi это вход по паролю после включения: vklyuchen,
	// vyklyuchen_chuzhim (его держит выключенным другая настройка, файлы в
	// Fayly) или neizvestno (sshd не ответил на проверку настроек).
	Snaruzhi string   `json:"snaruzhi,omitempty"`
	Fayly    []string `json:"fayly,omitempty"`
}

// SnosVps это итог сноса для окна.
type SnosVps struct {
	// ParolVernulsya: вход по паролю снова такой, каким был до выключения.
	ParolVernulsya bool `json:"parol_vernulsya,omitempty"`
}

// OtpuskVps это итог «забыть и снять ключ» для окна.
type OtpuskVps struct {
	// KlyuchaNeBylo: строки ключа этого компьютера на сервере уже не было.
	KlyuchaNeBylo bool `json:"klyucha_ne_bylo,omitempty"`
}

// zapisSvoego находит запись о своём сервере по id.
func (s *Sluzhba) zapisSvoego(k protokol.Kadr) (SvoyServer, *protokol.Kadr) {
	var telo struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		o := otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
		return SvoyServer{}, &o
	}
	n, err := s.nabor()
	if err != nil {
		o := otkazIz(k, protokol.KodSecretsUnreadable, err)
		return SvoyServer{}, &o
	}
	i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == telo.Id })
	if i < 0 {
		o := otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "такого сервера в разделе «Свой VPN» нет")
		return SvoyServer{}, &o
	}
	return n.SvoiServery[i], nil
}

var errNetKlyuchaUpravleniya = errors.New("ключа управления этим сервером на компьютере нет: сначала возьми сервер под управление")

// izvestnyy это ключ хоста из записи, с которым сверяется любой вход на этот
// сервер; nil - ключа в записи нет. Неразборный ключ это порча хранилища, а
// не отказ сервера: выход из неё - «Забыть».
func (x SvoyServer) izvestnyy() (*ssh.KlyuchHosta, error) {
	if x.KlyuchHosta == "" {
		return nil, nil
	}
	kh, err := ssh.KlyuchHostaIzZapisi(x.KlyuchHostaTip, x.KlyuchHosta)
	if err != nil {
		return nil, err
	}
	return &kh, nil
}

// vhodKlyuchom это вход на свой сервер root по ключу управления, с ключом
// хоста из записи.
func vhodKlyuchom(z SvoyServer, podpis gossh.Signer) (ssh.Zapros, error) {
	kh, err := z.izvestnyy()
	if err != nil {
		return ssh.Zapros{}, err
	}
	if kh == nil {
		return ssh.Zapros{}, errors.New("в записи о сервере нет ключа хоста")
	}
	return ssh.Zapros{Adres: z.Adres, Port: z.PortSsh, Izvesten: kh,
		Vhod: ssh.Vhod{Polzovatel: "root", Podpis: podpis}}, nil
}

// svoyVpsSnesti снимает с сервера всё своё (З3, §3.9). Ключи группы, запись
// и ключ управления уходят только после удачного сноса.
func (s *Sluzhba) svoyVpsSnesti(k protokol.Kadr) protokol.Kadr {
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidSnos, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id })
		it, err := s.vpsSnesti(ctx, s.putiVps(), zapros)
		if err != nil {
			return err
		}
		otstali, err := s.zabytOtpushchennyy(z.Id)
		if err != nil {
			return err
		}
		s.pravitHod(zad, func(h *HodVps) {
			h.Put, h.Snyat = it.Vhod.Put, &SnosVps{ParolVernulsya: it.Snos.ParolVernulsya}
			if otstali != nil {
				h.Preduprezhdeniya = append(h.Preduprezhdeniya, *otstali)
			}
		})
		return nil
	})
}

// zaprosVzyatiya это данные «Взять под управление»: вход человека и ключ
// хоста с осмотра. Есть запись с ключом хоста - берётся он, а нет ни её, ни
// ключа с осмотра - первая встреча.
type zaprosVzyatiya struct {
	dostupVps
	KlyuchHostaTip string `json:"klyuch_hosta_tip,omitempty"`
	KlyuchHosta    string `json:"klyuch_hosta,omitempty"`
	Imya           string `json:"imya"`
}

// svoyVpsVzyat берёт сервер под управление этого компьютера (З4, §3.9).
func (s *Sluzhba) svoyVpsVzyat(k protokol.Kadr) protokol.Kadr {
	var z zaprosVzyatiya
	if err := json.Unmarshal(k.Telo, &z); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "данные входа не разобраны")
	}
	adres, zapros, err := z.razobrat()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	}
	id := idSvoego(adres)
	n, err := s.nabor()
	if err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	var bylo *SvoyServer
	if i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id }); i >= 0 {
		bylo = &n.SvoiServery[i]
	}
	// Записанный ключ главнее присланного с осмотра: сменившийся ключ
	// принимает только «Систему переустановили».
	if bylo != nil {
		if zapros.Izvesten, err = bylo.izvestnyy(); err != nil {
			return otkazIz(k, protokol.KodSecretsUnreadable, err)
		}
	}
	if zapros.Izvesten == nil && z.KlyuchHosta != "" {
		kh, err := ssh.KlyuchHostaIzZapisi(z.KlyuchHostaTip, z.KlyuchHosta)
		if err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "ключ сервера с проверки не разобран: проверь сервер ещё раз")
		}
		zapros.Izvesten = &kh
	}
	z.Imya = obrezatImya(z.Imya)
	if z.Imya == "" && bylo != nil {
		z.Imya = bylo.Imya
	}
	if z.Imya == "" {
		z.Imya = imyaPoUmolchaniyu(n.SvoiServery, id)
	}
	return s.zapustitVps(k, vidVzyat, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), id })
		podpis, err := s.zavestiKlyuchUpravleniya(id)
		if err != nil {
			return err
		}
		imyaPK, _ := os.Hostname()
		itog, err := s.vpsVzyat(ctx, upravlenie.Vzyatie{
			Puti: s.putiVps(), Vhod: zapros, Imya: z.Imya,
			Upravlenie: podpis, Kommentariy: kommentariyKlyucha(imyaPK, time.Now()),
		})
		if err != nil {
			return err
		}
		osn := osnovaZapisi{Imya: z.Imya, Port: z.Port, Polzovatel: z.Polzovatel}
		if bylo != nil {
			osn.PortyRuchnye, osn.Postavlen = bylo.PortyRuchnye, bylo.Postavlen
		}
		return s.zapisatIPokazat(zad, id, osn, itog)
	})
}

// zabytOtpushchennyy убирает запись о сервере с её ключами и ключ
// управления, когда строки ключа на сервере уже нет (снос, снятие ключа).
// Правила, отставшие от набора, это предупреждение: запись уже убрана.
// Оставшийся файл ключа ничего не открывает, и проваливать из-за него дело
// незачем.
func (s *Sluzhba) zabytOtpushchennyy(id string) (otstali *OtkazVps, err error) {
	switch err := s.ubratZapisSvoego(id); {
	case errors.Is(err, errPravilaOtstali):
		log.Printf("свой сервер %s: %v", id, err)
		otstali = &OtkazVps{Kod: kodSohraneniya(err), Tekst: sboi.DlyaCheloveka(err)}
	case err != nil:
		return nil, &oshibkaZapisi{err}
	}
	if err := s.ubratKlyuchUpravleniya(id); err != nil {
		log.Printf("свой сервер %s: %v", id, err)
	}
	return otstali, nil
}

// svoyVpsOtpustit снимает с сервера строку ключа управления этого компьютера
// и забывает сервер (З5). Сервер не пустил или ключ - последний вход: не
// изменилось ничего, и окно предлагает забыть без снятия (svoyVpsZabyt). VPN
// на сервере работает дальше, ключи на других устройствах тоже.
func (s *Sluzhba) svoyVpsOtpustit(k protokol.Kadr) protokol.Kadr {
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidOtpusk, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id })
		it, err := s.vpsOtpustit(ctx, s.putiVps(), zapros)
		if err != nil {
			return err
		}
		otstali, err := s.zabytOtpushchennyy(z.Id)
		if err != nil {
			return err
		}
		s.pravitHod(zad, func(h *HodVps) {
			h.Put, h.Otpushchen = it.Vhod.Put, &OtpuskVps{KlyuchaNeBylo: !it.Snyat}
			if otstali != nil {
				h.Preduprezhdeniya = append(h.Preduprezhdeniya, *otstali)
			}
		})
		return nil
	})
}

// svoyVpsParol выключает или включает вход по паролю на своём сервере (З6)
// ключом управления. Запись помнит, лежит ли наш файл sshd. После неудачного
// выключения он снят или его снимет таймер на сервере, поэтому запись
// говорит «не выключен» при любом отказе: ошибается она тогда только в
// безопасную сторону, а следующая проверка поправит. Неудачное включение
// оставляет файл как был, и запись не трогается.
func (s *Sluzhba) svoyVpsParol(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Vyklyuchit *bool `json:"vyklyuchit"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil || telo.Vyklyuchit == nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	vyklyuchit := *telo.Vyklyuchit
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidParol, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id })
		p := ParolVps{Vyklyuchen: vyklyuchit}
		var (
			it  upravlenie.ParolItog
			err error
		)
		if vyklyuchit {
			it, err = s.vpsVyklyuchitParol(ctx, s.putiVps(), zapros)
		} else {
			it, err = s.vpsVklyuchitParol(ctx, s.putiVps(), zapros)
			p.Snaruzhi, p.Fayly = it.Posle.Parol, it.Posle.Fayly
		}
		if err != nil {
			if vyklyuchit {
				if err := s.otmetitProverku(z.Id, func(x *SvoyServer) { x.ParolVyklyuchen = false }); err != nil {
					log.Printf("свой сервер %s: отметка пароля не записана: %v", z.Id, err)
				}
			}
			return err
		}
		zapisana := s.otmetitProverku(z.Id, func(x *SvoyServer) { x.ParolVyklyuchen = vyklyuchit })
		if zapisana != nil {
			log.Printf("свой сервер %s: отметка пароля не записана: %v", z.Id, zapisana)
		}
		s.pravitHod(zad, func(h *HodVps) {
			h.Put, h.Parol = it.Vhod.Put, &p
			if zapisana != nil {
				h.Preduprezhdeniya = append(h.Preduprezhdeniya, OtkazVps{Kod: kodSohraneniya(zapisana), Tekst: sboi.DlyaCheloveka(zapisana)})
			}
		})
		return nil
	})
}

// svoyVpsZabyt убирает из клиента ключи группы, запись и ключ управления
// (З5). На сервере не меняется ничего: снять с него ключ - svoyVpsOtpustit.
func (s *Sluzhba) svoyVpsZabyt(k protokol.Kadr) protokol.Kadr {
	// Запор, а не проверка: захват того же сервера, начатый посреди правки,
	// завёл бы ключ управления, который она тут же сотрёт. Срок запора лишь
	// страховка: снимает его defer.
	otperet, tekst := s.zaperetVps(time.Now().Add(zaporNaZabyt), otkazZabyvaetsya)
	if otperet == nil {
		return otkaz(k.Id, k.Imya, protokol.KodVpsZanyato, tekst)
	}
	defer otperet()
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	// Ключ первым: не стёрся - не изменилось ничего, и повтор доделает.
	// Упадёт правка набора - запись останется без ключа, и её уберёт тот же
	// повтор.
	if err := s.ubratKlyuchUpravleniya(z.Id); err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	if err := s.ubratZapisSvoego(z.Id); err != nil {
		return otkazIz(k, kodSohraneniya(err), err)
	}
	return otvet(k.Id, k.Imya, map[string]string{"id": z.Id})
}

const (
	zaporNaZabyt     = 10 * time.Minute
	otkazZabyvaetsya = "клиент как раз забывает свой сервер: повтори через несколько секунд"
	otkazImenuetsya  = "клиент как раз меняет название своего сервера: повтори через несколько секунд"
)

var errSvoegoNet = errors.New("такого сервера в разделе «Свой VPN» нет")

// pishutZapis это задания, что пишут запись о сервере целиком, с названием,
// взятым в начале дела. Проверка, пароль, снос и снятие ключа правят запись
// по полям и названия не трогают.
var pishutZapis = map[string]bool{vidUstanovka: true, vidPodbor: true, vidVzyat: true, vidMaska: true}

// svoyVpsPereimenovat меняет название своего сервера (ИМ1) в записи и в
// именах ключей группы одной правкой набора. На сервер название не уходит,
// входа нет. Ключи на телефоне носят прежнее имя, пока их не добавят заново.
func (s *Sluzhba) svoyVpsPereimenovat(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id   string `json:"id"`
		Imya string `json:"imya"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	imya := obrezatImya(telo.Imya)
	if imya == "" {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "название пустое")
	}
	// Запор, а не проверка: установка, подбор и захват этого сервера пишут
	// запись и ключи с названием, взятым в начале дела, и затёрли бы новое.
	// Прочие задания идут дальше. Задание, чей сервер ещё не назван в
	// снимке, могло начаться над этим.
	otperet, tekst := s.zaperetVpsKrome(time.Now().Add(zaporNaZabyt), otkazImenuetsya, func(h HodVps) bool {
		return !pishutZapis[h.Vid] || (h.Server != "" && h.Server != telo.Id)
	})
	if otperet == nil {
		return otkaz(k.Id, k.Imya, protokol.KodVpsZanyato, tekst)
	}
	defer otperet()
	imena := map[string]string{}
	err := s.pravitNabor(func(n *Nabor) error {
		clear(imena)
		i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == telo.Id })
		if i < 0 {
			return errSvoegoNet
		}
		z := &n.SvoiServery[i]
		z.Imya = imya
		vsego := map[string]int{}
		for _, srv := range n.Servery {
			if slices.Contains(z.Servery, srv.Id) {
				vsego[srv.Transport]++
			}
		}
		for j := range n.Servery {
			if srv := &n.Servery[j]; slices.Contains(z.Servery, srv.Id) {
				srv.Imya = konfig.ImyaKlyucha(imya, srv.Transport, srv.Port, vsego[srv.Transport])
				imena[srv.Id] = srv.Imya
			}
		}
		return nil
	})
	otv := map[string]any{"id": telo.Id, "imya": imya}
	switch {
	case errors.Is(err, errSvoegoNet):
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	case errors.Is(err, errPravilaOtstali):
		// Набор записан, названия сменились. Правила от имён не зависят, а
		// отставшие человек увидит предупреждением: отказ здесь сказал бы,
		// что переименование не прошло.
		log.Printf("свой сервер %s: переименован, %v", telo.Id, err)
		otv["preduprezhdenie"] = OtkazVps{Kod: kodSohraneniya(err), Tekst: sboi.DlyaCheloveka(err)}
	case err != nil:
		return otkazIz(k, kodSohraneniya(err), err)
	}
	s.obnovitImenaYadra(imena)
	return otvet(k.Id, k.Imya, otv)
}

// svoyVpsObhodRf включает и выключает «Российские сайты напрямую» (РФ1) в
// записи о сервере. Входа на сервер нет: в простом режиме правила едут на
// телефон отдельным QR, и окно по записи показывает нужный. Дела, что пишут
// запись целиком, берут этот выбор из записи в миг записи, поэтому запор не
// нужен.
func (s *Sluzhba) svoyVpsObhodRf(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id  string `json:"id"`
		Vkl *bool  `json:"vkl"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil || telo.Vkl == nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	err := s.pravitNabor(func(n *Nabor) error {
		i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == telo.Id })
		if i < 0 {
			return errSvoegoNet
		}
		n.SvoiServery[i].RfCherezVpn = !*telo.Vkl
		return nil
	})
	otv := map[string]any{"id": telo.Id, "vkl": *telo.Vkl}
	switch {
	case errors.Is(err, errSvoegoNet):
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	case errors.Is(err, errPravilaOtstali):
		// Выбор записан, а правила брандмауэра от него не зависят.
		log.Printf("свой сервер %s: выбор о российских сайтах записан, %v", telo.Id, err)
		otv["preduprezhdenie"] = OtkazVps{Kod: kodSohraneniya(err), Tekst: sboi.DlyaCheloveka(err)}
	case err != nil:
		return otkazIz(k, kodSohraneniya(err), err)
	}
	return otvet(k.Id, k.Imya, otv)
}

// obnovitImenaYadra переносит новые имена ключей (id - имя) в снимок ядра и в
// имя несущего: при том же несущем его имя иначе не перечитывается до
// переподключения. Отпечаток ключа имени не включает, ядро не поднимается.
func (s *Sluzhba) obnovitImenaYadra(imena map[string]string) {
	s.mu.Lock()
	for i := range s.serveryYadra {
		if im, ok := imena[s.serveryYadra[i].Id]; ok {
			s.serveryYadra[i].Imya = im
		}
	}
	im, ok := imena[s.nesushchiyId]
	smenilos := ok && s.nesushchiyImya != im
	if smenilos {
		s.nesushchiyImya = im
	}
	s.mu.Unlock()
	if smenilos {
		s.izvestit("state", s.Status())
	}
}

// ubratZapisSvoego убирает запись о сервере id с её ключами одной правкой
// набора.
func (s *Sluzhba) ubratZapisSvoego(id string) error {
	return s.pravitNabor(func(n *Nabor) error {
		i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id })
		if i < 0 {
			return nil
		}
		ubratServery(n, n.SvoiServery[i].Servery)
		n.SvoiServery = slices.Delete(n.SvoiServery, i, i+1)
		return nil
	})
}

// otmetitKlyuchHosta ведёт ключ хоста, ждущий приёма, по концу задания над
// сервером id. Сервер предъявил другой ключ - запись запоминает его:
// карточка покажет отпечаток, и «Систему переустановили» примет именно
// его. Задание удалось - вход прошёл с записанным ключом (осмотр и захват
// при записи берут его же), и ждать нечего. Ключ, совпавший с записанным,
// ожидание тоже снимает. Запись правится, только если ожидание меняется:
// плановая проверка раз в час иначе переписывала бы набор зря.
func (s *Sluzhba) otmetitKlyuchHosta(id string, err error) {
	if id == "" {
		return
	}
	var tip, telo string
	if err != nil {
		var otk *ssh.Otkaz
		if !errors.As(err, &otk) || otk.Prichina != ssh.PrichinaKlyuchHostaSmenilsya || otk.Predyavlen == nil {
			return
		}
		tip, telo = otk.Predyavlen.Tip, otk.Predyavlen.TeloBase64()
	}
	zhdat := func(x *SvoyServer) (string, string) {
		if x.KlyuchHostaTip == tip && x.KlyuchHosta == telo {
			return "", ""
		}
		return tip, telo
	}
	n, e := s.nabor()
	if e != nil {
		log.Printf("свой сервер %s: ключ хоста, ждущий приёма, не отмечен: %v", id, e)
		return
	}
	i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id })
	if i < 0 {
		return
	}
	if t, b := zhdat(&n.SvoiServery[i]); t == n.SvoiServery[i].NovyyKlyuchHostaTip && b == n.SvoiServery[i].NovyyKlyuchHosta {
		return
	}
	if e := s.otmetitProverku(id, func(x *SvoyServer) {
		x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta = zhdat(x)
	}); e != nil {
		log.Printf("свой сервер %s: ключ хоста, ждущий приёма, не отмечен: %v", id, e)
	}
}

const (
	// zaporNaPriyomKlyucha лишь страховка, как у «Забыть»: запор снимает defer.
	zaporNaPriyomKlyucha    = 10 * time.Minute
	otkazPrinimaetsyaKlyuch = "клиент как раз принимает новый ключ сервера: повтори через несколько секунд"
)

// svoyVpsPrinyatKlyuchHosta это «Систему переустановили» (§4.3): запись
// берёт ключ хоста, который сервер предъявил вместо записанного. Окно шлёт
// отпечаток, показанный человеку, и принимается только ключ с ним: сервер
// мог с тех пор предъявить другой. Отметка пароля сбрасывается: на свежей
// системе нашего файла sshd нет. Прошлая проверка снимается: её отказ был
// про прежний ключ, а без отметки сервер первым попадёт в плановую.
func (s *Sluzhba) svoyVpsPrinyatKlyuchHosta(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id         string `json:"id"`
		Otpechatok string `json:"otpechatok"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil || telo.Otpechatok == "" {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	// Запор: пока ключ меняется, задание не начнётся. Задание, собравшее вход
	// до запора, войдёт с прежним ключом и упадёт; отказ проверки в запись
	// тогда не ляжет (deloProverki сверяет ключ).
	otperet, tekst := s.zaperetVps(time.Now().Add(zaporNaPriyomKlyucha), otkazPrinimaetsyaKlyuch)
	if otperet == nil {
		return otkaz(k.Id, k.Imya, protokol.KodVpsZanyato, tekst)
	}
	defer otperet()
	var (
		nePrinyat = errors.New("ключ не принят")
		kod       = protokol.KodTeloNegodno
		prichina  string
	)
	err := s.pravitNabor(func(n *Nabor) error {
		i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == telo.Id })
		if i < 0 {
			prichina = "такого сервера в разделе «Свой VPN» нет"
			return nePrinyat
		}
		x := &n.SvoiServery[i]
		if x.NovyyKlyuchHosta == "" {
			prichina = "другого ключа сервер не предъявлял: проверь сервер"
			return nePrinyat
		}
		kh, err := ssh.KlyuchHostaIzZapisi(x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta)
		if err != nil {
			prichina = "новый ключ сервера в записи не разобран: проверь сервер ещё раз"
			return nePrinyat
		}
		// Окно по этому коду перечитает список и покажет новый отпечаток.
		if kh.Otpechatok() != telo.Otpechatok {
			kod, prichina = protokol.KodVpsKlyuchHosta, "сервер с тех пор предъявил другой ключ: сверь новый отпечаток"
			return nePrinyat
		}
		x.KlyuchHostaTip, x.KlyuchHosta = x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta
		x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta = "", ""
		x.ParolVyklyuchen = false
		x.Proveren, x.ProverkaOtkaz = time.Time{}, ""
		return nil
	})
	switch {
	case errors.Is(err, nePrinyat):
		return otkaz(k.Id, k.Imya, kod, prichina)
	case err != nil:
		return otkazIz(k, kodSohraneniya(err), err)
	}
	return otvet(k.Id, k.Imya, map[string]string{"id": telo.Id})
}
