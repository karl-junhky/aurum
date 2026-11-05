package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

func TestSvoyVpsMaska(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye, sek := postavitDlyaPodbora(t, s, kh)
	// Оба вида не проходили из сети, выбран ключ reality, он же вне авто.
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Vybran, n.VneAvto = starye[0], []string{starye[0]}
		z := &n.SvoiServery[0]
		z.PortyRuchnye = true
		z.NeIzSeti = []string{"reality-tcp", "hy2"}
		z.SetTekst, z.SetPoVidam = "оба не прошли", map[string]string{"reality-tcp": "reality глушат", "hy2": "hy2 режут"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	var p upravlenie.SmenaMaski
	s.vpsSmenitMasku = func(_ context.Context, x upravlenie.SmenaMaski) (upravlenie.Itog, error) {
		p = x
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagMaska})
		novye := sek
		novye.Maska = "sosed.example.net"
		it := itogSoSekretami(t, novye, kh, 443, 8443)
		it.MaskaIstochnik, it.Vhod.Put = upravlenie.MaskaZapas, "напрямую"
		return it, nil
	}
	if otv := s.svoyVpsMaska(kadrVps(t, "svoyVpsMaska", zaprosMaskiVps{Id: id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Vid != vidMaska || h.Server != id || h.Ustanovlen == nil || len(h.Ustanovlen.Ids) != 2 ||
		h.Maska == nil || *h.Maska != (MaskaVps{Imya: "sosed.example.net", Istochnik: upravlenie.MaskaZapas}) || h.Put != "напрямую" {
		t.Fatalf("смена маски: %+v", h)
	}
	if p.Svoya != "" || p.BrendSoglasen || p.Bylo != "dl.google.com" || p.Imya != "Дача" || p.Vhod.Port != 2222 || p.Vhod.Vhod.Polzovatel != "root" ||
		p.Vhod.Vhod.Podpis == nil || !bytes.Equal(p.Vhod.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
		p.Vhod.Izvesten == nil || !bytes.Equal(p.Vhod.Izvesten.Telo, kh.Telo) {
		t.Fatalf("смена ушла с %+v", p)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	// Ключ reality сменился вместе с маской, выбор и исключение из авто ушли
	// за ним. Ключ hysteria2 тот же.
	novye := h.Ustanovlen.Ids
	if len(n.Servery) != 2 || novye[0] == starye[0] || novye[1] != starye[1] {
		t.Fatalf("ключей %d, было %v, стало %v", len(n.Servery), starye, novye)
	}
	if n.Vybran != novye[0] || !slices.Equal(n.VneAvto, []string{novye[0]}) {
		t.Fatalf("выбор %q, вне авто %v", n.Vybran, n.VneAvto)
	}
	if i := slices.IndexFunc(n.Servery, func(x protokol.Server) bool { return x.Id == novye[0] }); i < 0 || n.Servery[i].Sni != "sosed.example.net" {
		t.Fatalf("ключ reality: %+v", n.Servery)
	}
	// hysteria2 маска не касается: его отметка о сети осталась, а reality с
	// новой маской ещё не проверялся.
	zap := n.SvoiServery[0]
	if zap.Maska != "sosed.example.net" || zap.MaskaIstochnik != upravlenie.MaskaZapas || !zap.PortyRuchnye ||
		!zap.Postavlen.Equal(postavlenDlyaTesta) || zap.PortSsh != 2222 || zap.Polzovatel != "admin" ||
		!slices.Equal(zap.NeIzSeti, []string{"hy2"}) || zap.SetTekst != "hy2 режут" || zap.SetPoVidam != nil {
		t.Fatalf("запись: %+v", zap)
	}
}

func TestSvoyVpsMaskaSvoya(t *testing.T) {
	s := sPustymNaborom(t)
	s.vpsStaging = true
	kh := klyuchHostaDlyaTesta(t)
	id, _, sek := postavitDlyaPodbora(t, s, kh)
	// Один вид не проходил: причина у него общей строкой.
	if err := s.pravitNabor(func(n *Nabor) error {
		n.SvoiServery[0].NeIzSeti, n.SvoiServery[0].SetTekst = []string{"hy2"}, "hy2 режут"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var p upravlenie.SmenaMaski
	s.vpsSmenitMasku = func(_ context.Context, x upravlenie.SmenaMaski) (upravlenie.Itog, error) {
		p = x
		novye := sek
		novye.Maska = "www.adobe.com"
		it := itogSoSekretami(t, novye, kh, 443, 8443)
		it.MaskaIstochnik = upravlenie.MaskaBrend
		return it, nil
	}
	s.svoyVpsMaska(kadrVps(t, "svoyVpsMaska", zaprosMaskiVps{Id: id, Svoya: "  www.adobe.com ", BrendSoglasen: true}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Maska == nil || h.Maska.Svoya != "www.adobe.com" || !h.Maska.BrendSoglasen || h.Maska.Istochnik != upravlenie.MaskaBrend {
		t.Fatalf("своя маска: %+v", h)
	}
	if p.Svoya != "www.adobe.com" || !p.BrendSoglasen || !p.Opcii.Staging {
		t.Fatalf("смена ушла с %+v", p)
	}
	if zap := zapisPoId(t, s, id); zap.Maska != "www.adobe.com" || !slices.Equal(zap.NeIzSeti, []string{"hy2"}) || zap.SetTekst != "hy2 режут" {
		t.Fatalf("запись: %+v", zap)
	}
}

func TestSvoyVpsMaskaOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye, _ := postavitDlyaPodbora(t, s, kh)
	vyzvan := false
	s.vpsSmenitMasku = func(context.Context, upravlenie.SmenaMaski) (upravlenie.Itog, error) {
		vyzvan = true
		return upravlenie.Itog{}, errors.New("не ждали")
	}
	for _, c := range []struct {
		imya string
		telo any
	}{
		{"длинное имя", zaprosMaskiVps{Id: id, Svoya: strings.Repeat("a", 254)}},
		{"нет сервера", zaprosMaskiVps{Id: "svoy-0000000000000000"}},
		{"тело не разобрано", "id"},
	} {
		if kod := kodOtkaza(s.svoyVpsMaska(kadrVps(t, "svoyVpsMaska", c.telo))); kod != protokol.KodTeloNegodno {
			t.Errorf("%s: код %q", c.imya, kod)
		}
	}
	if vyzvan {
		t.Fatal("смена пошла при негодной команде")
	}

	for _, c := range []struct {
		imya  string
		err   error
		kod   string
		tekst string
	}{
		{"не годится", &upravlenie.MaskaNegodna{Imya: "suckless.org", Prichina: "сайт не отвечает h2"}, protokol.KodVpsMaska, "сайт не отвечает h2"},
		{"известная", &upravlenie.MaskaIzvestnaya{Imya: "www.adobe.com"}, protokol.KodVpsMaskaIzvestnaya, "глушат"},
		{"старая программа", upravlenie.ErrStarayaProgramma, protokol.KodVpsMaska, "обнови"},
		{"другой нет", fmt.Errorf("%w: %w", upravlenie.ErrNetDrugoyMaski, upravlenie.ErrNetMaski), protokol.KodVpsMaska, "можно ввести свою"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			s.vpsSmenitMasku = func(context.Context, upravlenie.SmenaMaski) (upravlenie.Itog, error) {
				return upravlenie.Itog{}, c.err
			}
			s.svoyVpsMaska(kadrVps(t, "svoyVpsMaska", zaprosMaskiVps{Id: id, Svoya: "svoya.example.org"}))
			h := dozhdatsyaVps(t, s)
			if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != c.kod || !strings.Contains(h.Otkaz.Tekst, c.tekst) ||
				h.Maska == nil || h.Maska.Svoya != "svoya.example.org" || h.Maska.Imya != "" {
				t.Fatalf("отказ: %+v", h)
			}
			if zap := zapisPoId(t, s, id); zap.Maska != "dl.google.com" || !slices.Equal(zap.Servery, starye) {
				t.Fatalf("после отказа запись %+v", zap)
			}
		})
	}
}
