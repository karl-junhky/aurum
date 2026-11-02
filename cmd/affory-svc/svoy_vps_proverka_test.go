package main

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

var chasyProverki = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// zapisPoId достаёт запись о своём сервере из набора.
func zapisPoId(t *testing.T, s *Sluzhba, id string) SvoyServer {
	t.Helper()
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	for _, z := range n.SvoiServery {
		if z.Id == id {
			return z
		}
	}
	t.Fatalf("записи %s нет", id)
	return SvoyServer{}
}

func TestSvoyVpsProverit(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	// Отказ прошлой проверки удачная снимает.
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.ProverkaOtkaz = "прошлая не вошла" }); err != nil {
		t.Fatal(err)
	}
	s.seychas = func() time.Time { return chasyProverki }
	do := chasyProverki.Add(10 * time.Hour).Unix()
	var (
		zapros ssh.Zapros
		maska  string
	)
	s.vpsProverit = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros, m string) (upravlenie.ZdorovyeServera, error) {
		zapros, maska = z, m
		o := osmotrChistyy()
		// hy2 слушает tcp вместо udp: порт есть, а входа нет.
		o.Slushayut = []server.Slushatel{
			{Proto: "tcp", Adres: "*", Port: 443, Process: "vpn-uzel"},
			{Proto: "tcp", Adres: "*", Port: 8443, Process: "vpn-uzel"},
		}
		return upravlenie.ZdorovyeServera{Vhod: upravlenie.Svedeniya{Put: "напрямую"}, Osmotr: o,
			Uzel: server.SostoyanieUzla{Versiya: "0.6.1", YadroZhivo: true,
				Sert: &server.SertUzla{Est: true, Do: do, Trevoga: true, Otkaz: "порт 80 закрыт"}},
			Maska: true, MaskaProverena: true}, nil
	}
	if otv := s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Vid != vidProverka || h.Faza != fazaGotovo || h.Proverka == nil || h.Put != "напрямую" || len(h.Preduprezhdeniya) != 0 ||
		h.Server != id || h.Planovaya {
		t.Fatalf("проверка: %+v", h)
	}
	zhdali := ProverkaVps{Versiya: "0.6.1", YadroZhivo: true,
		Porty:  []PortVps{{Vid: "reality", Port: 443, Slushaet: true}, {Vid: "hy2", Port: 8443, Slushaet: false}},
		SertDo: do, SertTrevoga: true, SertOtkaz: "порт 80 закрыт", Maska: "godna", PamyatMB: 1024, DiskMB: 20000}
	if !reflect.DeepEqual(*h.Proverka, zhdali) {
		t.Fatalf("итог проверки:\n%+v\nждали\n%+v", *h.Proverka, zhdali)
	}
	if zapros.Adres != adresSvoego || zapros.Port != 2222 || zapros.Vhod.Polzovatel != "root" ||
		zapros.Vhod.Podpis == nil || !bytes.Equal(zapros.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
		zapros.Izvesten == nil || !bytes.Equal(zapros.Izvesten.Telo, kh.Telo) {
		t.Fatalf("вход проверки: %+v", zapros)
	}
	z := zapisPoId(t, s, id)
	if maska != z.Maska || maska == "" {
		t.Fatalf("маска %q, в записи %q", maska, z.Maska)
	}
	if !z.Proveren.Equal(chasyProverki) || !z.SertProveren.Equal(chasyProverki) || z.ProverkaOtkaz != "" || z.SertDo != do ||
		!z.SertTrevoga || z.SertOtkaz != "порт 80 закрыт" || z.Versiya != "0.6.1" {
		t.Fatalf("запись после проверки: %+v", z)
	}
	tr := s.Status().SvoiTrevogi
	if len(tr) != 1 || tr[0].Id != id || tr[0].Imya != "Дача" || !strings.Contains(tr[0].Tekst, "не продлевается: порт 80 закрыт") {
		t.Fatalf("тревоги в статусе: %+v", tr)
	}
}

// Программа не ответила - сведения о сертификате с прошлой проверки
// остаются. Вход не удался - запись помнит когда и почему.
func TestSvoyVpsProveritNeudachi(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	do := chasyProverki.Add(50 * time.Hour).Unix()
	if err := s.otmetitProverku(id, func(x *SvoyServer) {
		x.SertDo, x.SertTrevoga, x.SertOtkaz, x.Versiya = do, true, "прежний отказ", "0.6.0"
	}); err != nil {
		t.Fatal(err)
	}
	prezhnyaya := func(z SvoyServer) bool {
		return z.SertDo == do && z.SertTrevoga && z.SertOtkaz == "прежний отказ" && z.Versiya == "0.6.0"
	}

	s.seychas = func() time.Time { return chasyProverki }
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{Osmotr: osmotrChistyy(),
			UzelOtkaz: errors.New("пульт молчит"), MaskaOtkaz: errors.New("openssl упал")}, nil
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	p := h.Proverka
	if h.Faza != fazaGotovo || p == nil || p.ProgrammaOtkaz == "" || p.Versiya != "" || p.SertDo != 0 ||
		p.Maska != "ne-provereno" || p.MaskaOtkaz == "" {
		t.Fatalf("проверка без программы: %+v %+v", h, p)
	}
	// Сведения о сертификате остаются со своей прежней отметкой, а почему
	// они не обновились, говорит отказ проверки.
	if z := zapisPoId(t, s, id); !prezhnyaya(z) || !z.Proveren.Equal(chasyProverki) || !z.SertProveren.Equal(z.Postavlen) ||
		z.ProverkaOtkaz != "программа сервера не ответила: "+p.ProgrammaOtkaz {
		t.Fatalf("запись после молчания программы: %+v", z)
	}

	pozzhe := chasyProverki.Add(time.Hour)
	s.seychas = func() time.Time { return pozzhe }
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{}, &upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaTishina}}
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	h = dozhdatsyaVps(t, s)
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsNeOtvechaet || h.Proverka != nil {
		t.Fatalf("неудачная проверка: %+v", h)
	}
	if z := zapisPoId(t, s, id); !prezhnyaya(z) || !z.Proveren.Equal(pozzhe) || z.ProverkaOtkaz != h.Otkaz.Tekst {
		t.Fatalf("запись после неудачи: %+v", z)
	}
}

// osmotrSoSlushatelyami это осмотр, где программа сервера слушает порты
// postavitSvoy: reality на 443 и hysteria2 на 8443.
func osmotrSoSlushatelyami(reality, hy2 bool) *server.Osmotr {
	o := osmotrChistyy()
	if reality {
		o.Slushayut = append(o.Slushayut, server.Slushatel{Proto: "tcp", Port: 443, Process: "vpn-uzel"})
	}
	if hy2 {
		o.Slushayut = append(o.Slushayut, server.Slushatel{Proto: "udp", Port: 8443, Process: "vpn-uzel"})
	}
	return o
}

// Ключи, что не проходили из сети, проверка гоняет заново: провёзший вид
// уходит из отметки, а без провёзших отметка остаётся как была.
func TestSvoyVpsProveritIzSeti(t *testing.T) {
	s := sPustymNaborom(t)
	id, ids := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	poVidam := map[string]string{"reality-tcp": "открой у хостера порт 443 (TCP)", "hy2": "проверь hysteria2 с телефона"}
	otmetit := func() {
		t.Helper()
		if err := s.otmetitProverku(id, func(x *SvoyServer) {
			x.NeIzSeti, x.SetTekst, x.SetPoVidam = []string{"reality-tcp", "hy2"}, "открой у хостера порты 443 (TCP) и 8443 (UDP)", poVidam
		}); err != nil {
			t.Fatal(err)
		}
	}
	otmetit()
	slushaetReality := true
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{Osmotr: osmotrSoSlushatelyami(slushaetReality, true), Uzel: server.SostoyanieUzla{Versiya: "0.6.1", YadroZhivo: true}}, nil
	}
	var (
		proverennye []string
		adapter     string
		otkaz       error
	)
	vezyot := "hy2"
	s.vpsProveritKlyuchi = func(_ context.Context, k []protokol.Server, o *server.Osmotr, op proverka.Opcii, _ proverka.Zapusk) ([]proverka.Itog, error) {
		proverennye, adapter = nil, op.Adapter
		if o == nil {
			t.Error("ключи проверялись без осмотра")
		}
		var itogi []proverka.Itog
		for _, x := range k {
			proverennye = append(proverennye, x.Id)
			v := proverka.NetOtveta
			if x.Transport == vezyot {
				v = proverka.Vezyot
			}
			itogi = append(itogi, proverka.Itog{Server: x, Vid: v})
		}
		return itogi, otkaz
	}
	proverit := func(shag string) SvoyServer {
		t.Helper()
		if otv := s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id})); otv.Oshib != nil {
			t.Fatal(otv.Oshib)
		}
		if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || h.Shag != shag {
			t.Fatalf("проверка: %+v", h)
		}
		return zapisPoId(t, s, id)
	}
	// Общая причина называла оба вида: оставшемуся остаётся своя.
	if z := proverit(string(upravlenie.ShagProverka)); !slices.Equal(z.NeIzSeti, []string{"reality-tcp"}) ||
		z.SetTekst != poVidam["reality-tcp"] || z.SetPoVidam != nil {
		t.Fatalf("hysteria2 провёз: %+v", z)
	}
	if !slices.Equal(proverennye, ids) || adapter != "Ethernet" {
		t.Fatalf("проверены %v через %q", proverennye, adapter)
	}
	// Не провёз никто или проверка ключей не состоялась: отметка как была.
	vezyot = ""
	if z := proverit(string(upravlenie.ShagProverka)); !slices.Equal(z.NeIzSeti, []string{"reality-tcp"}) || !slices.Equal(proverennye, ids[:1]) {
		t.Fatalf("никто не провёз: %+v, проверены %v", z, proverennye)
	}
	vezyot, otkaz = "reality-tcp", errors.New("ядро проверки не поднялось")
	if z := proverit(string(upravlenie.ShagProverka)); !slices.Equal(z.NeIzSeti, []string{"reality-tcp"}) {
		t.Fatalf("проверка не состоялась: %+v", z)
	}
	// Порт reality на сервере не слушает: его ключ не гоняется.
	otkaz, proverennye, slushaetReality = nil, nil, false
	if z := proverit(string(upravlenie.ShagVhod)); !slices.Equal(z.NeIzSeti, []string{"reality-tcp"}) || proverennye != nil {
		t.Fatalf("мёртвый порт: %+v, проверены %v", z, proverennye)
	}
	// Плановая проверка ключей не гоняет: команды человека ждали бы минутами.
	slushaetReality = true
	if _, tekst := s.nachatVps(vidProverka, true, s.deloProverki(zapisPoId(t, s, id), ssh.Zapros{})); tekst != "" {
		t.Fatalf("плановая не началась: %s", tekst)
	}
	dozhdatsyaPlanovoy(t, s)
	if z := zapisPoId(t, s, id); !slices.Equal(z.NeIzSeti, []string{"reality-tcp"}) || proverennye != nil {
		t.Fatalf("плановая: %+v, проверены %v", z, proverennye)
	}
	// Ключа reality в списке больше нет: вид снимается без проверки, а
	// hysteria2 гоняется и не провозит.
	otmetit()
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = slices.DeleteFunc(n.Servery, func(k protokol.Server) bool { return k.Id == ids[0] })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	vezyot = ""
	if z := proverit(string(upravlenie.ShagProverka)); !slices.Equal(z.NeIzSeti, []string{"hy2"}) ||
		z.SetTekst != poVidam["hy2"] || !slices.Equal(proverennye, ids[1:]) {
		t.Fatalf("ключа нет: %+v, проверены %v", z, proverennye)
	}
	// Отметки нет: ключи не гоняются.
	proverennye = nil
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.NeIzSeti = nil }); err != nil {
		t.Fatal(err)
	}
	if proverit(string(upravlenie.ShagVhod)); proverennye != nil {
		t.Fatalf("ключи без отметки проверялись: %v", proverennye)
	}
}

// Отмена посреди проверки ключей не пишет ничего, даже провёзший вид.
func TestSvoyVpsProveritIzSetiOtmena(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.NeIzSeti = []string{"hy2"} }); err != nil {
		t.Fatal(err)
	}
	bylo := zapisPoId(t, s, id)
	s.seychas = func() time.Time { return chasyProverki }
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{Osmotr: osmotrSoSlushatelyami(true, true), Uzel: server.SostoyanieUzla{Versiya: "0.6.1"}}, nil
	}
	s.vpsProveritKlyuchi = func(ctx context.Context, k []protokol.Server, _ *server.Osmotr, _ proverka.Opcii, _ proverka.Zapusk) ([]proverka.Itog, error) {
		s.svoyVpsOtmenit(kadrVps(t, "svoyVpsOtmenit", nil))
		<-ctx.Done()
		return []proverka.Itog{{Server: k[0], Vid: proverka.Vezyot}}, nil
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaOtmeneno {
		t.Fatalf("отмена: %+v", h)
	}
	if z := zapisPoId(t, s, id); !z.Proveren.Equal(bylo.Proveren) || !slices.Equal(z.NeIzSeti, []string{"hy2"}) {
		t.Fatalf("отменённая проверка записала: %+v", z)
	}
}

// Итог не записался: проверка всё равно показывает, что видела, и
// предупреждает, что запись отстала.
func TestSvoyVpsProveritZapisNeProshla(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		s.sekretyPisat = func([]byte) error { return errors.New("тест: диск полон") }
		return upravlenie.ZdorovyeServera{Osmotr: osmotrChistyy(), Uzel: server.SostoyanieUzla{Versiya: "0.6.1"}}, nil
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Proverka == nil || h.Proverka.Versiya != "0.6.1" ||
		len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Tekst == "" {
		t.Fatalf("проверка с незаписанным итогом: %+v", h)
	}
}

// Отметку пароля проверка берёт с сервера: его могли переключить с другого
// компьютера или руками.
func TestSvoyVpsProveritParol(t *testing.T) {
	for _, vyklyuchen := range []bool{true, false} {
		s := sPustymNaborom(t)
		id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
		if err := s.otmetitProverku(id, func(x *SvoyServer) { x.ParolVyklyuchen = !vyklyuchen }); err != nil {
			t.Fatal(err)
		}
		s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
			o := osmotrChistyy()
			o.SshdFayly = []string{"50-cloud-init.conf"}
			if vyklyuchen {
				o.SshdFayly = append(o.SshdFayly, server.FaylParolya)
			}
			return upravlenie.ZdorovyeServera{Osmotr: o, Uzel: server.SostoyanieUzla{Versiya: "0.6.1"}}, nil
		}
		s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
		if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo {
			t.Fatalf("проверка: %+v", h)
		}
		if z := zapisPoId(t, s, id); z.ParolVyklyuchen != vyklyuchen {
			t.Fatalf("наш файл sshd %v, отметка %v", vyklyuchen, z.ParolVyklyuchen)
		}
	}
}

// Пароль выключен, а отметка не записалась: дело удалось, запись поправит
// следующая проверка.
func TestSvoyVpsParolZapisNeProshla(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	s.vpsVyklyuchitParol = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		s.sekretyPisat = func([]byte) error { return errors.New("тест: диск полон") }
		return upravlenie.ParolItog{}, nil
	}
	s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": true}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Parol == nil || !h.Parol.Vyklyuchen ||
		len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Tekst == "" {
		t.Fatalf("выключение с незаписанной отметкой: %+v", h)
	}
}

// Установка и захват записывают сертификат, каким его видела программа, и
// считаются визитом: плановая проверка не идёт следом.
func TestZapisatSvoySert(t *testing.T) {
	s := sPustymNaborom(t)
	id := idSvoego(netip.MustParseAddr(adresSvoego))
	itog := itogSvoego(t, klyuchHostaDlyaTesta(t), 443, 8443)
	itog.Sert = &server.SertUzla{Est: true, Do: 1_800_000_000, Trevoga: true, Otkaz: "порт 80 закрыт"}
	if _, err := s.zapisatSvoy(id, osnovaZapisi{Imya: "Дача"}, itog); err != nil {
		t.Fatal(err)
	}
	z := zapisPoId(t, s, id)
	if z.SertDo != 1_800_000_000 || !z.SertTrevoga || z.SertOtkaz != "порт 80 закрыт" ||
		z.Proveren.IsZero() || !z.Proveren.Equal(z.Postavlen) {
		t.Fatalf("запись после установки: %+v", z)
	}
	itog.Sert = &server.SertUzla{Est: false, Do: 1_800_000_000, Trevoga: true, Otkaz: "порт 80 закрыт"}
	if _, err := s.zapisatSvoy(id, osnovaZapisi{Imya: "Дача"}, itog); err != nil {
		t.Fatal(err)
	}
	if z := zapisPoId(t, s, id); z.SertDo != 0 || !z.SertTrevoga {
		t.Fatalf("сертификата на диске нет, а срок записан: %+v", z)
	}
}

func TestProverkaDlyaEkrana(t *testing.T) {
	z := SvoyServer{Id: "svoy-1", Reality: 443}
	zd := upravlenie.ZdorovyeServera{Osmotr: osmotrChistyy(),
		Uzel:           server.SostoyanieUzla{Versiya: "0.6.1", Oshibka: "ядро упало", Sert: &server.SertUzla{Est: false, Do: 1, Trevoga: true, Otkaz: "нет 80"}},
		MaskaProverena: true}
	p := proverkaDlyaEkrana(z, zd)
	// Сертификата на диске нет: срока нет, тревога и отказ остаются.
	if p.SertDo != 0 || !p.SertTrevoga || p.SertOtkaz != "нет 80" || p.OshibkaYadra != "ядро упало" || p.YadroZhivo {
		t.Fatalf("сертификат: %+v", p)
	}
	if len(p.Porty) != 1 || p.Porty[0].Vid != "reality" || p.Maska != "negodna" {
		t.Fatalf("порты %+v, маска %q", p.Porty, p.Maska)
	}
	zd.MaskaProverena = false
	if p := proverkaDlyaEkrana(z, zd); p.Maska != "ne-provereno" || p.MaskaOtkaz != "" {
		t.Fatalf("без openssl: %q %q", p.Maska, p.MaskaOtkaz)
	}
	zd.Uzel.Sert = nil
	if p := proverkaDlyaEkrana(z, zd); p.SertTrevoga || p.SertOtkaz != "" || p.Versiya != "0.6.1" {
		t.Fatalf("без выпуска в пакете: %+v", p)
	}
}

// Проверка, вышедшая за срок, это неудача с понятной причиной, а не отмена.
func TestSvoyVpsProveritSrok(t *testing.T) {
	bylo := srokProverki
	srokProverki = 20 * time.Millisecond
	t.Cleanup(func() { srokProverki = bylo })
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	s.seychas = func() time.Time { return chasyProverki }
	s.vpsProverit = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		<-ctx.Done()
		return upravlenie.ZdorovyeServera{}, &ssh.Otkaz{Prichina: ssh.PrichinaSrok}
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsNeOtvechaet || h.Otkaz.Tekst != errProverkaDolgo.Error() {
		t.Fatalf("проверка за сроком: %+v %+v", h, h.Otkaz)
	}
	if z := zapisPoId(t, s, id); !z.Proveren.Equal(chasyProverki) || z.ProverkaOtkaz != errProverkaDolgo.Error() {
		t.Fatalf("запись после срока: %+v", z)
	}
}

func TestSvoyVpsProveritOtkazy(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), false)
	if kod := kodOtkaza(s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))); kod != protokol.KodVpsVzyat {
		t.Fatalf("без ключа управления: %q", kod)
	}
	if kod := kodOtkaza(s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": "svoy-net"}))); kod != protokol.KodTeloNegodno {
		t.Fatalf("без записи: %q", kod)
	}
	// Сервер переставлял другой компьютер: окно ведёт к захвату.
	if _, err := s.zavestiKlyuchUpravleniya(id); err != nil {
		t.Fatal(err)
	}
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{}, upravlenie.ErrChuzhayaUstanovka
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsVzyat {
		t.Fatalf("чужая установка: %+v", h.Otkaz)
	}
}

// Отменённая проверка не пишет ни отметки, ни отказа.
func TestSvoyVpsProveritOtmena(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	bylo := zapisPoId(t, s, id)
	s.seychas = func() time.Time { return chasyProverki }
	nachalo := make(chan struct{})
	s.vpsProverit = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		close(nachalo)
		<-ctx.Done()
		return upravlenie.ZdorovyeServera{}, ctx.Err()
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	<-nachalo
	s.svoyVpsOtmenit(kadrVps(t, "svoyVpsOtmenit", nil))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaOtmeneno {
		t.Fatalf("отмена: %+v", h)
	}
	if z := zapisPoId(t, s, id); !z.Proveren.Equal(bylo.Proveren) || z.ProverkaOtkaz != "" {
		t.Fatalf("отменённая проверка записала: %+v", z)
	}
}

func TestTrevogiSvoih(t *testing.T) {
	ch := func(d time.Duration) int64 { return chasyProverki.Add(d).Unix() }
	svezho := chasyProverki.Add(-time.Hour)
	staro := chasyProverki.Add(-3 * 24 * time.Hour)
	n := Nabor{SvoiServery: []SvoyServer{
		{Id: "bez-hy2", Imya: "A", Prodlenie: "net", SertTrevoga: true, SertDo: ch(-time.Hour), SertProveren: svezho},
		{Id: "istyok", Imya: "B", Prodlenie: "80", SertDo: ch(-time.Minute), SertProveren: svezho},
		{Id: "ne-prodlevaetsya", Imya: "C", Prodlenie: "443", SertDo: ch(70 * time.Hour), SertTrevoga: true, SertOtkaz: "порт 80 закрыт",
			SertProveren: svezho, ProverkaOtkaz: "вчерашний сбой"},
		{Id: "konchaetsya", Imya: "D", Prodlenie: "80", SertDo: ch(5 * time.Hour), SertProveren: svezho},
		{Id: "zdorov", Imya: "E", Prodlenie: "80", SertDo: ch(100 * time.Hour), SertOtkaz: "разовый отказ", SertProveren: svezho},
		{Id: "ne-znaem", Imya: "F", Prodlenie: "80", SertProveren: svezho},
		// Старые сведения: без отказа проверки молчим, с отказом - только
		// если срок мог выйти.
		{Id: "staroe-bez-otkaza", Imya: "G", Prodlenie: "80", SertDo: ch(-time.Hour), SertTrevoga: true, SertProveren: staro},
		{Id: "staroe-daleko", Imya: "H", Prodlenie: "80", SertDo: ch(50 * time.Hour), SertProveren: staro, ProverkaOtkaz: "сервер молчит"},
		{Id: "staroe-istyok", Imya: "I", Prodlenie: "80", SertDo: ch(-time.Hour), SertOtkaz: "старый отказ",
			SertProveren: staro, ProverkaOtkaz: "сервер молчит"},
		// Отказ проверки пришёл импортом, а ключа здесь нет: отсюда сервер не
		// проверяется, и чужой отказ тревогой не становится.
		{Id: "staroe-chuzhoe", Imya: "J", Prodlenie: "80", SertDo: ch(-time.Hour), SertProveren: staro, ProverkaOtkaz: "сервер молчит"},
		// Сведений от программы не было ни разу: без даты.
		{Id: "bez-otmetki", Imya: "K", Prodlenie: "80", SertDo: ch(-time.Hour), ProverkaOtkaz: "сервер молчит"},
	}}
	tr := trevogiSvoih(sertySvoih(n, func(id string) bool { return id != "staroe-chuzhoe" }), chasyProverki)
	zhdali := map[string]string{
		"istyok":           "срок сертификата вышел, hysteria2 может не подключаться",
		"ne-prodlevaetsya": "сертификат не продлевается: порт 80 закрыт",
		"konchaetsya":      "сертификат кончается меньше чем через сутки",
		"staroe-istyok": "о сертификате нет свежих сведений с " + staro.Local().Format("02.01") +
			", а срок его мог выйти. Проверка не проходит: сервер молчит",
		"bez-otmetki": "о сертификате нет свежих сведений, а срок его мог выйти. Проверка не проходит: сервер молчит",
	}
	if len(tr) != len(zhdali) {
		t.Fatalf("тревоги: %+v", tr)
	}
	for _, x := range tr {
		if x.Tekst != zhdali[x.Id]+". Проверь сервер в разделе «Свой VPN»" || x.Imya == "" {
			t.Fatalf("тревога %+v", x)
		}
	}
}

func idPoAdresu(adres string) string { return idSvoego(netip.MustParseAddr(adres)) }

// svoiDlyaRaspisaniya кладёт в набор свои серверы по адресам с отметками
// проверки: ключ управления есть у адресов из sKlyuchom.
func svoiDlyaRaspisaniya(t *testing.T, s *Sluzhba, proveren map[string]time.Time, sKlyuchom ...string) {
	t.Helper()
	kh := klyuchHostaDlyaTesta(t)
	if err := s.pravitNabor(func(n *Nabor) error {
		// По порядку адресов: выбор самого давнего не должен зависеть от
		// того, кто стоит в наборе последним.
		for _, adres := range slices.Sorted(maps.Keys(proveren)) {
			kogda := proveren[adres]
			n.SvoiServery = append(n.SvoiServery, SvoyServer{Id: idPoAdresu(adres), Imya: adres, Adres: adres, PortSsh: 22,
				Polzovatel: "root", KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64(), Maska: "dl.google.com",
				Prodlenie: "80", Proveren: kogda})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, adres := range sKlyuchom {
		if _, err := s.zavestiKlyuchUpravleniya(idPoAdresu(adres)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProveritDavnego(t *testing.T) {
	const (
		svezhiy       = "203.0.113.11"
		davneeVseh    = "203.0.113.12"
		davniy        = "203.0.113.13"
		bezKlyucha    = "203.0.113.14"
		izBudushchego = "203.0.113.15"
		slomannyy     = "203.0.113.16"
	)
	s := sPustymNaborom(t)
	s.seychas = func() time.Time { return chasyProverki }
	svoiDlyaRaspisaniya(t, s, map[string]time.Time{
		svezhiy:       chasyProverki.Add(-time.Hour),
		davneeVseh:    chasyProverki.Add(-48 * time.Hour),
		davniy:        chasyProverki.Add(-25 * time.Hour),
		bezKlyucha:    chasyProverki.Add(-72 * time.Hour),
		izBudushchego: chasyProverki.Add(30 * 24 * time.Hour),
		slomannyy:     chasyProverki.Add(-96 * time.Hour),
	}, svezhiy, davniy, davneeVseh, izBudushchego, slomannyy)
	// Ключ управления есть, а войти нельзя: запись о сервере испорчена.
	if err := s.otmetitProverku(idPoAdresu(slomannyy), func(x *SvoyServer) { x.KlyuchHosta = "!!!" }); err != nil {
		t.Fatal(err)
	}
	// Дело человека кончилось раньше: плановая его итог не заслоняет.
	s.nachatVps(vidOsmotr, false, func(context.Context, *zadanieVps) error { return nil })
	dozhdatsyaVps(t, s)
	s.muVps.Lock()
	delo := s.vps
	s.muVps.Unlock()

	var adresa []string
	s.vpsProverit = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		adresa = append(adresa, z.Adres)
		return upravlenie.ZdorovyeServera{}, errors.New("сервер молчит")
	}
	// Три давних по одному за заход, самый давний первым; четвёртый заход не
	// находит ничего: неудача тоже отметка.
	for range 4 {
		s.proveritDavnego()
		dozhdatsyaPlanovoy(t, s)
	}
	if !reflect.DeepEqual(adresa, []string{davneeVseh, davniy, izBudushchego}) {
		t.Fatalf("плановая проверка ходила на %v", adresa)
	}
	if z := zapisPoId(t, s, idPoAdresu(davneeVseh)); !z.Proveren.Equal(chasyProverki) || z.ProverkaOtkaz == "" {
		t.Fatalf("плановая неудача не записана: %+v", z)
	}
	// Неудача входа тоже отметка: иначе сервер выбирался бы каждый час.
	if z := zapisPoId(t, s, idPoAdresu(slomannyy)); !z.Proveren.Equal(chasyProverki) || z.ProverkaOtkaz == "" {
		t.Fatalf("неудача входа плановой не записана: %+v", z)
	}
	if z := zapisPoId(t, s, idPoAdresu(bezKlyucha)); !z.Proveren.Equal(chasyProverki.Add(-72*time.Hour)) || z.ProverkaOtkaz != "" {
		t.Fatalf("сервер без ключа отмечен: %+v", z)
	}
	s.muVps.Lock()
	vernulos := s.vps == delo
	s.muVps.Unlock()
	if !vernulos {
		t.Fatal("после плановой проверки ход отдаёт не дело человека")
	}

	// Пока идёт плановая, команды отказывают своим текстом.
	s.seychas = func() time.Time { return chasyProverki.Add(48 * time.Hour) }
	voshla, otpustit := make(chan struct{}), make(chan struct{})
	// Упавшая проверка ниже не должна оставить задание висеть до срока теста.
	otpustitOdin := sync.OnceFunc(func() { close(otpustit) })
	t.Cleanup(otpustitOdin)
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		close(voshla)
		<-otpustit
		return upravlenie.ZdorovyeServera{}, errors.New("сервер молчит")
	}
	podpiska, sobytiya := s.Podpisatsya()
	defer s.Otpisatsya(podpiska)
	s.proveritDavnego()
	<-voshla
	// Ход плановой называет сервер и то, что начала её служба, и в снимке,
	// и в событии. Через двое суток самый давний - svezhiy: остальные
	// отмечены проверками выше.
	s.muVps.Lock()
	hod := s.vps.snimok
	s.muVps.Unlock()
	if !hod.Planovaya || hod.Server != idPoAdresu(svezhiy) {
		t.Fatalf("ход плановой: %+v", hod)
	}
	vSobytii := false
	for !vSobytii {
		select {
		case k := <-sobytiya:
			vSobytii = k.Imya == "svoy-vps-hod" && bytes.Contains(k.Telo, []byte(`"planovaya":true`)) &&
				bytes.Contains(k.Telo, []byte(`"server":"`+idPoAdresu(svezhiy)+`"`))
		case <-time.After(5 * time.Second):
			t.Fatal("событие хода плановой без сервера или признака")
		}
	}
	otv := s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": idPoAdresu(davniy)}))
	if kodOtkaza(otv) != protokol.KodVpsZanyato || otv.Oshib.Tekst != otkazIdyotProverka || s.svoyVpsIdyot() != otkazIdyotProverka {
		t.Fatalf("отказ во время плановой: %+v", otv.Oshib)
	}
	otpustitOdin()
	dozhdatsyaPlanovoy(t, s)

	// Занято делом человека: плановая ждёт следующего часа.
	otpustit = make(chan struct{})
	s.nachatVps(vidOsmotr, false, func(context.Context, *zadanieVps) error { <-otpustit; return nil })
	byloAdresov := len(adresa)
	s.proveritDavnego()
	close(otpustit)
	dozhdatsyaVps(t, s)
	if len(adresa) != byloAdresov {
		t.Fatalf("плановая проверка пошла мимо занятого задания: %v", adresa)
	}
}

// Запись переписали установкой между чтением набора и отметкой: отказ входа
// был про прежнюю запись и на новую не ложится.
func TestProveritDavnegoZapisPerepisana(t *testing.T) {
	const slomannyy = "203.0.113.16"
	s := sPustymNaborom(t)
	s.seychas = func() time.Time { return chasyProverki }
	postavlen := chasyProverki.Add(-time.Minute)
	svoiDlyaRaspisaniya(t, s, map[string]time.Time{slomannyy: postavlen}, slomannyy)
	id := idPoAdresu(slomannyy)
	nastoyashchiy := s.nabor
	var chteniy atomic.Int32
	s.nabor = func() (Nabor, error) {
		n, err := nastoyashchiy()
		// Плановая видит прежнюю запись: давнюю и с испорченным ключом хоста.
		if chteniy.Add(1) == 1 && err == nil {
			for i := range n.SvoiServery {
				if n.SvoiServery[i].Id == id {
					n.SvoiServery[i].Proveren, n.SvoiServery[i].KlyuchHosta = chasyProverki.Add(-48*time.Hour), "!!!"
				}
			}
		}
		return n, err
	}
	s.proveritDavnego()
	if z := zapisPoId(t, s, id); !z.Proveren.Equal(postavlen) || z.ProverkaOtkaz != "" {
		t.Fatalf("отказ по прежней записи лёг на новую: %+v", z)
	}
}

// dozhdatsyaPlanovoy ждёт конца плановой проверки: по концу она возвращает
// прежнее задание, поэтому ждать gotovo у s.vps нельзя.
func dozhdatsyaPlanovoy(t *testing.T, s *Sluzhba) {
	t.Helper()
	srok := time.Now().Add(5 * time.Second)
	for time.Now().Before(srok) {
		s.muVps.Lock()
		konchilas := s.vps == nil || !s.vps.fonovoe
		s.muVps.Unlock()
		if konchilas {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("плановая проверка не кончилась за 5 с")
}

func TestRaspisanieSvoih(t *testing.T) {
	s := sPustymNaborom(t)
	s.seychas = func() time.Time { return chasyProverki }
	svoiDlyaRaspisaniya(t, s, map[string]time.Time{adresSvoego: {}}, adresSvoego)
	var pauzy []time.Duration
	s.zhdat = func(_ context.Context, d time.Duration) bool {
		pauzy = append(pauzy, d)
		dozhdatsyaPlanovoy(t, s)
		return len(pauzy) < 3
	}
	var proverok atomic.Int32
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		proverok.Add(1)
		return upravlenie.ZdorovyeServera{}, errors.New("сервер молчит")
	}
	s.raspisanieSvoih(context.Background())
	if !reflect.DeepEqual(pauzy, []time.Duration{pervayaProverkaSvoih, periodProverkiSvoih, periodProverkiSvoih}) || proverok.Load() != 1 {
		t.Fatalf("паузы %v, проверок %d", pauzy, proverok.Load())
	}
}

// Хранилище, дочитанное в фоне, приносит и тревоги своих серверов.
func TestSvoiTrevogiPosleDochityvaniya(t *testing.T) {
	prezhnie := pauzyDochityvaniya
	pauzyDochityvaniya = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { pauzyDochityvaniya = prezhnie })
	s := podstavnaya(t, nil)
	var chteniy atomic.Int32
	s.nabor = func() (Nabor, error) {
		if chteniy.Add(1) == 1 {
			return Nabor{}, errors.New("тест: шифрование Windows не отвечает")
		}
		return Nabor{SvoiServery: []SvoyServer{{Id: "svoy-1", Imya: "Дача", Prodlenie: "80", SertTrevoga: true, SertProveren: time.Now()}}}, nil
	}
	s.mu.Lock()
	s.rezhimNeProchitan = true
	s.mu.Unlock()
	s.DochitatKhranilishche()
	dozhdatsya(t, "тревога своего сервера", func() bool {
		return len(s.Status().SvoiTrevogi) == 1
	})
}
