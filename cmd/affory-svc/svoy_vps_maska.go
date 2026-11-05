package main

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

const vidMaska = "maska"

// dlinaMaski это предел имени в DNS: длиннее имя сайта не бывает.
const dlinaMaski = 253

// zaprosMaskiVps это «Сменить маску» в карточке (МА2): svoya пусто -
// подобрать другую у соседей сервера.
type zaprosMaskiVps struct {
	Id    string `json:"id"`
	Svoya string `json:"svoya,omitempty"`
	// BrendSoglasen: человек видел, что известное имя глушат на мобильном.
	BrendSoglasen bool `json:"brend_soglasen,omitempty"`
}

// MaskaVps это смена маски в снимке. Svoya и BrendSoglasen - что просил
// человек: по ним окно повторяет дело и ставит известное имя после
// согласия. Imya и Istochnik - что встало.
type MaskaVps struct {
	Svoya         string `json:"svoya,omitempty"`
	BrendSoglasen bool   `json:"brend_soglasen,omitempty"`
	Imya          string `json:"imya,omitempty"`
	Istochnik     string `json:"istochnik,omitempty"`
}

// svoyVpsMaska ставит на свой сервер другую маску reality. Порты, пароли и
// ключ hysteria2 те же, ключ reality в наборе меняется, и на телефоне его
// надо добавить заново.
func (s *Sluzhba) svoyVpsMaska(k protokol.Kadr) protokol.Kadr {
	var p zaprosMaskiVps
	if err := json.Unmarshal(k.Telo, &p); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разобрано")
	}
	p.Svoya = strings.TrimSpace(p.Svoya)
	if utf8.RuneCountInString(p.Svoya) > dlinaMaski {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "имя маски длиннее 253 знаков")
	}
	z, o := s.zapisSvoego(k)
	if o != nil {
		return *o
	}
	if z.Domen != "" {
		return otkaz(k.Id, k.Imya, protokol.KodVpsMaska, tekstMaskaDomen)
	}
	zapros, kod, err := s.vhodUpravleniya(z)
	if err != nil {
		return otkazIz(k, kod, err)
	}
	return s.zapustitVps(k, vidMaska, func(ctx context.Context, zad *zadanieVps) error {
		prosil := MaskaVps{Svoya: p.Svoya, BrendSoglasen: p.BrendSoglasen}
		s.pravitHod(zad, func(h *HodVps) {
			h.Shag, h.Server = string(upravlenie.ShagVhod), z.Id
			m := prosil
			h.Maska = &m
		})
		itog, err := s.vpsSmenitMasku(ctx, upravlenie.SmenaMaski{
			Puti: s.putiVps(), Vhod: zapros, Imya: z.Imya, Svoya: p.Svoya, BrendSoglasen: p.BrendSoglasen, Bylo: z.Maska,
			Opcii: konfig.Opcii{Staging: s.vpsStaging},
			Hod:   func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h, nil) }) },
		})
		if err != nil {
			return err
		}
		// hysteria2 маска не касается: отметка, что его ключ не проходит из
		// сети человека, остаётся. Ключ reality с новой маской ещё не
		// проверялся.
		osn := osnovaZapisi{Imya: z.Imya, Port: z.PortSsh, Polzovatel: z.Polzovatel, PortyRuchnye: z.PortyRuchnye,
			Postavlen: z.Postavlen, ZapisEst: true, PrezhnyayaSet: []string{"hy2"}}
		if err := s.zapisatIPokazat(zad, z.Id, osn, itog); err != nil {
			return err
		}
		s.pravitHod(zad, func(h *HodVps) {
			m := prosil
			m.Imya, m.Istochnik = itog.Sekrety.Maska, itog.MaskaIstochnik
			h.Maska = &m
		})
		return nil
	})
}
