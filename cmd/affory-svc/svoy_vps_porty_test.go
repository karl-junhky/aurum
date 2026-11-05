package main

import (
	"bytes"
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

var postavlenDlyaTesta = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// postavitDlyaPodbora кладёт свой сервер на 443/tcp и 8443/udp с ключом
// управления и отдаёт секреты, с которыми подбор заново вернёт итог.
func postavitDlyaPodbora(t *testing.T, s *Sluzhba, kh ssh.KlyuchHosta) (string, []string, konfig.Sekrety) {
	t.Helper()
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	id := idSvoego(netip.MustParseAddr(adresSvoego))
	it := itogSoSekretami(t, sek, kh, 443, 8443)
	it.MaskaIstochnik = upravlenie.MaskaSosedi
	ids, err := s.zapisatSvoy(id, osnovaZapisi{Imya: "Дача", Port: 2222, Polzovatel: "admin", Postavlen: postavlenDlyaTesta}, it)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.zavestiKlyuchUpravleniya(id); err != nil {
		t.Fatal(err)
	}
	return id, ids, sek
}

func TestSvoyVpsPodobratPorty(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye, sek := postavitDlyaPodbora(t, s, kh)
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Vybran, n.VneAvto = starye[1], []string{starye[1]}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	var p upravlenie.Podbor
	s.vpsPodobratPorty = func(_ context.Context, x upravlenie.Podbor) (upravlenie.Itog, error) {
		p = x
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagPodbor, Otsev: &porty.Otsev{Proto: "udp", Port: 443, Prichina: porty.PrichinaZanyat, Process: "caddy"}})
		it := itogSoSekretami(t, sek, kh, 443, 11444)
		it.Vhod.Put, it.ParolVyklyuchen = "напрямую", true
		// Ключи ложатся в набор до перепроверки из сети: hy2 на подборе не
		// прошёл, а при повторе провёз.
		rannyy := it
		rannyy.NeProshli = []string{"hy2"}
		if err := x.Klyuchi(rannyy); err != nil {
			return upravlenie.Itog{}, err
		}
		if z := zapisPoId(t, s, id); z.Hy2 != 11444 || !slices.Equal(z.NeIzSeti, []string{"hy2"}) {
			t.Errorf("запись до перепроверки: %+v", z)
		}
		return it, nil
	}
	if otv := s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Vid != vidPodbor || h.Server != id || h.Ustanovlen == nil || len(h.Ustanovlen.Ids) != 2 ||
		h.Put != "напрямую" || len(h.Otseyany) != 1 {
		t.Fatalf("подбор: %+v", h)
	}
	if p.Vhod.Adres != adresSvoego || p.Vhod.Port != 2222 || p.Vhod.Vhod.Polzovatel != "root" || p.Vhod.Vhod.Podpis == nil ||
		!bytes.Equal(p.Vhod.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
		p.Vhod.Izvesten == nil || !bytes.Equal(p.Vhod.Izvesten.Telo, kh.Telo) {
		t.Fatalf("вход подбора: %+v", p.Vhod)
	}
	if p.Imya != "Дача" || p.Ruchnye != (porty.Ruchnye{}) || p.Proverka.Adapter != "Ethernet" || p.Zapusk == nil || p.Pryamo == nil {
		t.Fatalf("подбор ушёл с %+v", p)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
		t.Fatalf("набор: %d ключей, %d своих", len(n.Servery), len(n.SvoiServery))
	}
	// Порт reality тот же: и ключ тот же. Ключ hy2 сменился, выбор и
	// исключение из авто ушли за ним.
	novye := h.Ustanovlen.Ids
	if novye[0] != starye[0] || novye[1] == starye[1] {
		t.Fatalf("ключи было %v, стало %v", starye, novye)
	}
	if n.Vybran != novye[1] || !slices.Equal(n.VneAvto, []string{novye[1]}) || n.Rezhim != "" {
		t.Fatalf("выбор %q, вне авто %v, режим %q", n.Vybran, n.VneAvto, n.Rezhim)
	}
	zap := n.SvoiServery[0]
	if zap.Hy2 != 11444 || zap.Reality != 443 || !zap.Postavlen.Equal(postavlenDlyaTesta) || zap.PortyRuchnye || !zap.ParolVyklyuchen ||
		zap.Polzovatel != "admin" || zap.PortSsh != 2222 || !slices.Equal(zap.Servery, novye) || len(zap.NeIzSeti) != 0 {
		t.Fatalf("запись: %+v", zap)
	}
	// Подбор портов маску не подбирал: откуда она, помнит прежняя запись.
	if zap.Maska != "dl.google.com" || zap.MaskaIstochnik != upravlenie.MaskaSosedi {
		t.Fatalf("маска %q из %q", zap.Maska, zap.MaskaIstochnik)
	}
}

func TestSvoyVpsPodobratPortyRuchnye(t *testing.T) {
	s := sPustymNaborom(t)
	s.vpsStaging = true
	kh := klyuchHostaDlyaTesta(t)
	id, _, sek := postavitDlyaPodbora(t, s, kh)
	var p upravlenie.Podbor
	s.vpsPodobratPorty = func(_ context.Context, x upravlenie.Podbor) (upravlenie.Itog, error) {
		p = x
		return itogSoSekretami(t, sek, kh, 2083, 8443), nil
	}
	s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id, Reality: 2083}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo {
		t.Fatalf("подбор: %+v", h)
	}
	if p.Ruchnye != (porty.Ruchnye{Reality: 2083}) || !p.Opcii.Staging {
		t.Fatalf("ручные %+v, опции %+v", p.Ruchnye, p.Opcii)
	}
	if zap := zapisPoId(t, s, id); !zap.PortyRuchnye || zap.Reality != 2083 {
		t.Fatalf("запись: %+v", zap)
	}
}

func TestSvoyVpsPodobratPortyOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye, _ := postavitDlyaPodbora(t, s, kh)
	if err := s.pravitNabor(func(n *Nabor) error { n.Vybran = starye[1]; return nil }); err != nil {
		t.Fatal(err)
	}
	vyzvan := false
	s.vpsPodobratPorty = func(context.Context, upravlenie.Podbor) (upravlenie.Itog, error) {
		vyzvan = true
		return upravlenie.Itog{}, porty.Otsev{Proto: "tcp", Port: 22, Prichina: porty.PrichinaSsh}
	}
	for _, c := range []struct {
		imya string
		telo any
		kod  string
	}{
		{"порт меньше нуля", zaprosPodboraVps{Id: id, Hy2: -1}, protokol.KodTeloNegodno},
		{"порт за пределом", zaprosPodboraVps{Id: id, Reality: 65536}, protokol.KodTeloNegodno},
		{"нет сервера", zaprosPodboraVps{Id: "svoy-0000000000000000"}, protokol.KodTeloNegodno},
		{"тело не разобрано", "id", protokol.KodTeloNegodno},
	} {
		if kod := kodOtkaza(s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", c.telo))); kod != c.kod {
			t.Errorf("%s: код %q", c.imya, kod)
		}
	}
	if vyzvan {
		t.Fatal("подбор пошёл при негодной команде")
	}

	// Отказ подбора: набор как был.
	s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id, Reality: 22}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsPorty {
		t.Fatalf("отказ подбора: %+v", h)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if zap := n.SvoiServery[0]; n.Vybran != starye[1] || !slices.Equal(zap.Servery, starye) || zap.Hy2 != 8443 {
		t.Fatalf("после отказа выбор %q, запись %+v", n.Vybran, zap)
	}

	// Ключа управления на этом компьютере нет: окно предлагает взять сервер.
	if err := s.ubratKlyuchUpravleniya(id); err != nil {
		t.Fatal(err)
	}
	if kod := kodOtkaza(s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id}))); kod != protokol.KodVpsVzyat {
		t.Fatalf("без ключа управления: %q", kod)
	}
}

// Сервер забыли, пока шёл подбор: запись не заводится снова.
func TestSvoyVpsPodobratPortyZabyt(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _, sek := postavitDlyaPodbora(t, s, kh)
	s.vpsPodobratPorty = func(_ context.Context, x upravlenie.Podbor) (upravlenie.Itog, error) {
		if err := s.ubratZapisSvoego(id); err != nil {
			t.Error(err)
		}
		// Отказ ранней записи останавливает подбор до перепроверки.
		if err := x.Klyuchi(itogSoSekretami(t, sek, kh, 443, 11444)); err != nil {
			return upravlenie.Itog{}, err
		}
		t.Error("ключи забытого сервера записаны")
		return upravlenie.Itog{}, nil
	}
	s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsPorty || !strings.Contains(h.Otkaz.Tekst, "забыли") {
		t.Fatalf("подбор после «забыть»: %+v", h)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.SvoiServery) != 0 || len(n.Servery) != 0 {
		t.Fatalf("забытый сервер вернулся: %d записей, %d ключей", len(n.SvoiServery), len(n.Servery))
	}
}

// Перенос выбора: ключ другого вида и чужой ключ не трогаются.
func TestPerenestiVybor(t *testing.T) {
	n := Nabor{
		Servery: []protokol.Server{
			{Id: "r1", Transport: "reality-tcp"}, {Id: "h1", Transport: "hy2"}, {Id: "chuzhoy", Transport: "hy2"},
		},
		Vybran: "r1", VneAvto: []string{"h1", "chuzhoy", "h2"},
	}
	perenestiVybor(&n, []string{"r1", "h1", "net-takogo"}, map[string]string{"hy2": "h2"})
	if n.Vybran != "r1" || !slices.Equal(n.VneAvto, []string{"chuzhoy", "h2"}) {
		t.Fatalf("выбор %q, вне авто %v", n.Vybran, n.VneAvto)
	}
	n.Vybran = "h1"
	n.VneAvto = []string{"h1"}
	perenestiVybor(&n, []string{"h1"}, map[string]string{"hy2": "h2", "reality-tcp": "r2"})
	if n.Vybran != "h2" || !slices.Equal(n.VneAvto, []string{"h2"}) {
		t.Fatalf("выбор %q, вне авто %v", n.Vybran, n.VneAvto)
	}
}

func TestOtkazNetKandidatovBezMesta(t *testing.T) {
	o := otkazVps(porty.ErrNetKandidatov)
	if strings.Contains(o.Tekst, "Дополнительно") {
		t.Fatalf("текст зовёт туда, где при подборе заново ничего нет: %q", o.Tekst)
	}
}
