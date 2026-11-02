package main

import (
	"context"
	"encoding/json"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

const vidPodbor = "podbor"

// zaprosPodboraVps это «Подобрать порты заново» и «Задать порты» (З9): порт
// вида, заданный руками, ставится один, 0 - подобрать заново.
type zaprosPodboraVps struct {
	Id      string `json:"id"`
	Reality int    `json:"reality,omitempty"`
	Hy2     int    `json:"hy2,omitempty"`
}

// svoyVpsPodobratPorty подбирает порты своего сервера заново (З9, К4):
// провайдер начал резать выбранный порт, а сносить сервер незачем. Ключи
// группы меняются одной правкой набора, у ключа вида, чей порт остался,
// тот же id. Итог приходит в снимке так же, как у установки.
func (s *Sluzhba) svoyVpsPodobratPorty(k protokol.Kadr) protokol.Kadr {
	var p zaprosPodboraVps
	if err := json.Unmarshal(k.Telo, &p); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	for _, port := range []int{p.Reality, p.Hy2} {
		if port < 0 || port > 65535 {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "порт вне 1-65535")
		}
	}
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidPodbor, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id })
		a, err := s.fizicheskiyAdapter()
		if err != nil {
			return err
		}
		pom := pomehiKompyutera(s.obnovitPomehi())
		osn := osnovaZapisi{Imya: z.Imya, Port: z.PortSsh, Polzovatel: z.Polzovatel,
			PortyRuchnye: p.Reality != 0 || p.Hy2 != 0, Postavlen: z.Postavlen, ZapisEst: true, Pomehi: pom}
		return s.zapisatPoHodu(zad, z.Id, osn, func(klyuchi func(upravlenie.Itog) error) (upravlenie.Itog, error) {
			return s.vpsPodobratPorty(ctx, upravlenie.Podbor{
				Puti: s.putiVps(), Vhod: zapros, Imya: z.Imya,
				Ruchnye:  porty.Ruchnye{Reality: p.Reality, Hy2: p.Hy2},
				Opcii:    konfig.Opcii{Staging: s.vpsStaging},
				Proverka: proverka.Opcii{Adapter: a.Imya},
				Zapusk:   s.zapuskProverki,
				Pryamo:   ssh.PutPryamo(a.Indeks, a.Imya).Dial,
				Hod:      func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h, pom) }) },
				Klyuchi:  klyuchi,
			})
		})
	})
}
