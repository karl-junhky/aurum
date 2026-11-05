package main

import (
	"context"
	"encoding/json"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/domen"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/happ"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// Свой домен (ДМ1-3) и подписка на сайте сервера (ПД1).

const (
	vidDomen    = "domen"
	vidPodpiska = "podpiska"
)

// Зачем пересобирается подписка: новая ссылка, правила о российских сайтах
// или новое название сервера.
const (
	zachemPut  = "put"
	zachemRf   = "rf"
	zachemImya = "imya"
)

// DomenVps это дело своего домена в снимке. У vidDomen Domen - что просил
// человек, пусто значит вернуться к простым ключам; у установки - домен,
// на который она идёт. У vidPodpiska Zachem говорит, зачем она, а Vkl - что
// просили о российских сайтах (true - напрямую).
type DomenVps struct {
	Domen  string `json:"domen,omitempty"`
	Zachem string `json:"zachem,omitempty"`
	Vkl    *bool  `json:"vkl,omitempty"`
}

// srokProverkiDomena укладывает проверку домена в срок ответа канала (5 с).
const srokProverkiDomena = 3500 * time.Millisecond

// svoyVpsDomenProverit спрашивает Google и Cloudflare, ведёт ли домен на
// сервер. Сервер - запись по id (карточка) или адрес из мастера. Сразу, без
// задания: на сервер никто не ходит.
func (s *Sluzhba) svoyVpsDomenProverit(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var p struct {
		Domen string `json:"domen"`
		Id    string `json:"id,omitempty"`
		Adres string `json:"adres,omitempty"`
	}
	if err := json.Unmarshal(k.Telo, &p); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	d, err := domen.Normalizovat(p.Domen)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	}
	adres := strings.TrimSpace(p.Adres)
	if p.Id != "" {
		z, o := s.zapisSvoego(k)
		if o != nil {
			return *o
		}
		adres = z.Adres
	}
	a, err := netip.ParseAddr(adres)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "адрес сервера не IP")
	}
	ctx, otmena := context.WithTimeout(ctx, srokProverkiDomena)
	defer otmena()
	it := s.vpsProveritDomen(ctx, d, a.Unmap())
	if it.Oshibka != "" {
		log.Printf("проверка домена %s: не ответили %s: %s", d, strings.Join(it.NeOtvetili, ", "), it.Oshibka)
	}
	return otvet(k.Id, k.Imya, it)
}

// svoyVpsDomen переводит свой сервер на свой домен или возвращает к простым
// ключам (пустой домен). Порты и пароли прежние, ключи в наборе меняются.
func (s *Sluzhba) svoyVpsDomen(k protokol.Kadr) protokol.Kadr {
	var p struct {
		Domen string `json:"domen"`
	}
	if err := json.Unmarshal(k.Telo, &p); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	if strings.TrimSpace(p.Domen) != "" {
		d, err := domen.Normalizovat(p.Domen)
		if err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
		}
		p.Domen = d
	} else {
		p.Domen = ""
	}
	z, svoi, o := s.zapisISvoi(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	routing, err := routingDlya(z.RfCherezVpn)
	if err != nil {
		return otkazIz(k, protokol.KodVpsDomen, err)
	}
	// Домен переезжает с другого своего сервера: ссылка на телефонах та же.
	put := putPodpiskiDomena(svoi, z.Id, p.Domen)
	return s.zapustitVps(k, vidDomen, func(ctx context.Context, zad *zadanieVps) error {
		prosil := DomenVps{Domen: p.Domen}
		s.pravitHod(zad, func(h *HodVps) {
			h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id
			d := prosil
			h.Domen = &d
		})
		itog, err := s.vpsSmenitRezhim(ctx, upravlenie.SmenaRezhima{
			Puti: s.putiVps(), Vhod: zapros, Imya: z.Imya, Domen: p.Domen, Routing: routing, Put: put,
			Opcii: konfig.Opcii{Staging: s.vpsStaging},
			Hod:   func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h, nil) }) },
		})
		if err != nil {
			return err
		}
		// Порт hysteria2 тот же, и отметка, что он не проходит из сети
		// человека, остаётся. Ключ reality с новой маской не проверялся.
		osn := osnovaZapisi{Imya: z.Imya, Port: z.PortSsh, Polzovatel: z.Polzovatel, PortyRuchnye: z.PortyRuchnye,
			Postavlen: z.Postavlen, ZapisEst: true, PrezhnyayaSet: []string{"hy2"}}
		if sayt := itog.Vhody.Sayt; sayt != nil {
			osn.RfCherezVpn = rfIzRoutinga(sayt.Routing)
		}
		return s.zapisatIPokazat(zad, z.Id, osn, itog)
	})
}

// svoyVpsPodpiska выдаёт новую ссылку подписки: прежняя перестаёт отвечать,
// телефоны с ней надо подписать заново.
func (s *Sluzhba) svoyVpsPodpiska(k protokol.Kadr) protokol.Kadr {
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	if z.Domen == "" {
		return otkaz(k.Id, k.Imya, protokol.KodVpsDomen, tekstNetPodpiski)
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidPodpiska, s.deloPodpiski(z, zapros, zachemPut, nil))
}

// deloPodpiski пересобирает подписку на сервере режима домена и пишет итог
// в запись. vkl - новый выбор о российских сайтах (true - напрямую), nil -
// прежний.
func (s *Sluzhba) deloPodpiski(z SvoyServer, zapros ssh.Zapros, zachem string, vkl *bool) func(context.Context, *zadanieVps) error {
	return func(ctx context.Context, zad *zadanieVps) error {
		prosil := DomenVps{Domen: z.Domen, Zachem: zachem, Vkl: vkl}
		s.pravitHod(zad, func(h *HodVps) {
			h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id
			d := prosil
			h.Domen = &d
		})
		p := upravlenie.SmenaPodpiski{
			Puti: s.putiVps(), Vhod: zapros, Imya: z.Imya, NovyyPut: zachem == zachemPut,
			Opcii: konfig.Opcii{Staging: s.vpsStaging},
			Hod:   func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h, nil) }) },
		}
		var rf *bool
		if vkl != nil {
			r := !*vkl
			routing, err := routingDlya(r)
			if err != nil {
				return err
			}
			p.Routing, rf = &routing, &r
		}
		itog, err := s.vpsObnovitPodpisku(ctx, p)
		if err != nil {
			return err
		}
		// Ключи те же: отметки о сети переходят все.
		osn := osnovaZapisi{Imya: z.Imya, Port: z.PortSsh, Polzovatel: z.Polzovatel, PortyRuchnye: z.PortyRuchnye,
			Postavlen: z.Postavlen, ZapisEst: true, PrezhnyayaSet: []string{"reality-tcp", "hy2"}, RfCherezVpn: rf}
		return s.zapisatIPokazat(zad, z.Id, osn, itog)
	}
}

// routingDlya это ссылка профиля Happ для заголовка подписки: российские
// сайты напрямую или всё через VPN (РФ1).
func routingDlya(rfCherezVpn bool) (string, error) {
	if rfCherezVpn {
		return happ.Vyklyuchit()
	}
	return happ.Vklyuchit()
}

// rfIzRoutinga читает выбор о российских сайтах по профилю Happ в подписке
// сервера: true - всё через VPN. nil - профиль не узнан: его собрала другая
// версия программы со своими списками, или правил в подписке нет.
func rfIzRoutinga(routing string) *bool {
	if routing == "" {
		return nil
	}
	vkl, err := happ.Vklyuchit()
	vykl, err2 := happ.Vyklyuchit()
	var rf bool
	switch {
	case err != nil || err2 != nil:
		return nil
	case routing == vkl:
		rf = false
	case routing == vykl:
		rf = true
	default:
		return nil
	}
	return &rf
}
