package upravlenie

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// mashina это подставной сервер установки: что ему прислали и что он
// отвечает.
type mashina struct {
	t      *testing.T
	vhod   *vhody
	root   *svyaz
	osm    *server.Osmotr
	pakety [][]byte
	// zanyat: на какой по счёту пакет ответить занятым портом.
	zanyat map[int]*server.Zanyat
	// serty это ответы sostoyanie по порядку, последний повторяется.
	serty     []*server.SertUzla
	sprosov   int
	vybor     porty.Vybor
	errVybor  error
	itogi     func([]protokol.Server) []proverka.Itog
	errItogi  error
	det       blok.Itog
	detOpcii  *blok.Opcii
	porty     [2][]int
	podbor    *konfig.Vhody
	proveryal []protokol.Server
	lovushka  int
	maski     []string
}

func novayaMashina(t *testing.T) *mashina {
	m := &mashina{t: t, osm: chistyyOsmotr(), zanyat: map[int]*server.Zanyat{}}
	m.root = &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "напрямую через Ethernet"}}
	m.vhod = &vhody{pryamo: &svyaz{imya: "parol", root: m.root, sv: Svedeniya{Put: "напрямую через Ethernet"}}}
	m.serty = []*server.SertUzla{nil, {IP: "203.0.113.10", Idyot: true}, {IP: "203.0.113.10", Est: true, Sposob: "http-01"}}
	m.vybor = porty.Vybor{Reality: 443, Hy2: 443}
	m.itogi = func(k []protokol.Server) []proverka.Itog {
		var it []proverka.Itog
		for _, s := range k {
			it = append(it, proverka.Itog{Server: s, Vid: proverka.Vezyot})
		}
		return it
	}
	m.vhod.podmenit(t)
	podmenitOsmotr(t, func(_ context.Context, x server.Mashina) (*server.Osmotr, error) {
		if x != m.vhod.pryamo {
			t.Fatal("осмотр установки не входом по паролю")
		}
		return m.osm, nil
	})
	bD, bU, bP, bS, bO, bL := dozhdatsya, ustanovit, primenit, sostoyanie, otkrytPorty, lovit
	bPd, bPv, bDt, bSr, bSh, bM := podobrat, proverit, detektor, srokSerta, shagSerta, proveritMasku
	t.Cleanup(func() {
		dozhdatsya, ustanovit, primenit, sostoyanie, otkrytPorty, lovit = bD, bU, bP, bS, bO, bL
		podobrat, proverit, detektor, srokSerta, shagSerta, proveritMasku = bPd, bPv, bDt, bSr, bSh, bM
	})
	shagSerta = time.Millisecond
	proveritMasku = func(_ context.Context, x server.Mashina, maska string) (bool, bool, error) {
		if x != m.root {
			t.Fatal("маска проверяется не входом root")
		}
		m.maski = append(m.maski, maska)
		return true, true, nil
	}
	rootom := func(x server.Mashina, chto string) {
		if x != m.root {
			t.Fatalf("%s не входом root по ключу", chto)
		}
	}
	dozhdatsya = func(_ context.Context, x server.Mashina) error {
		if x != m.vhod.pryamo {
			t.Fatal("ожидание настройки не входом по паролю")
		}
		return nil
	}
	ustanovit = func(_ context.Context, x server.Mashina, _ io.ReadSeeker, _ gossh.PublicKey) (server.Ustanovka, error) {
		rootom(x, "установка программы")
		return server.Ustanovka{Versiya: "vpn-uzel 0.6.0, sing-box 1.14.2"}, nil
	}
	primenit = func(_ context.Context, x server.Mashina, p []byte) (server.Kvitanciya, error) {
		rootom(x, "пакет")
		m.pakety = append(m.pakety, p)
		if z := m.zanyat[len(m.pakety)]; z != nil {
			return server.Kvitanciya{Zanyat: z}, &server.Oshibka{Shag: "primenit", Tekst: "занят"}
		}
		return server.Kvitanciya{YadroZhivo: true}, nil
	}
	sostoyanie = func(_ context.Context, x server.Mashina) (server.SostoyanieUzla, error) {
		rootom(x, "состояние")
		s := m.serty[min(m.sprosov, len(m.serty)-1)]
		m.sprosov++
		return server.SostoyanieUzla{Versiya: "0.6.0", YadroZhivo: true, Sert: s}, nil
	}
	otkrytPorty = func(_ context.Context, x server.Mashina, tcp, udp []int) (server.Porty, error) {
		rootom(x, "порты")
		m.porty = [2][]int{tcp, udp}
		return server.Porty{UFW: "active"}, nil
	}
	lovit = func(_ context.Context, x server.Mashina, _ string, _, _ int) (int64, error) {
		rootom(x, "ловушка")
		m.lovushka++
		return 3, nil
	}
	podobrat = func(_ context.Context, s konfig.Sekrety, v konfig.Vhody, o proverka.Opcii, _ proverka.Zapusk) (porty.Vybor, error) {
		m.podbor = &v
		if len(o.Vyhody) != 2 || o.Hod == nil || o.Adapter != "Ethernet" {
			t.Fatalf("опции подбора %+v", o)
		}
		return m.vybor, m.errVybor
	}
	proverit = func(_ context.Context, k []protokol.Server, o proverka.Opcii, _ proverka.Zapusk) ([]proverka.Itog, error) {
		if o.PervyyNaVid {
			t.Fatal("окончательная проверка по первому на вид")
		}
		m.proveryal = k
		return m.itogi(k), m.errItogi
	}
	detektor = func(ctx context.Context, o blok.Opcii) (blok.Itog, error) {
		m.detOpcii = &o
		if _, err := o.Lovushka(ctx, "tcp", 25000, 15); err != nil {
			t.Fatal(err)
		}
		return m.det, nil
	}
	return m
}

func (m *mashina) ustanovka() Ustanovka {
	return Ustanovka{
		Puti: puti(false), Vhod: ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Parol: "p"}},
		Pustoy: true, Imya: "Мой сервер", Upravlenie: klyuchUpravleniya(m.t), Kommentariy: "affory DESKTOP 2026-11-01",
		Programma: bytes.NewReader([]byte("vpn-uzel")), Maski: []string{"www.example.com"},
		Proverka: proverka.Opcii{Adapter: "Ethernet"},
		Zapusk:   func(context.Context, []byte) (func(), error) { return func() {}, nil },
		Pryamo:   (&blokDial{}).dial,
	}
}

type blokDial struct{}

func (*blokDial) dial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("не ждали")
}

// paket разбирает пакет программы сервера: порты входов и срок пробы.
func paket(t *testing.T, p []byte) (reality, hy2 []int, proba int, sert bool) {
	t.Helper()
	var v struct {
		Yadro struct {
			Inbounds []struct {
				Type string `json:"type"`
				Port int    `json:"listen_port"`
			} `json:"inbounds"`
		} `json:"yadro"`
		Proba *struct {
			Srok int `json:"srok"`
		} `json:"proba"`
		SertIPSam *struct{} `json:"sert_ip_sam"`
	}
	if err := json.Unmarshal(p, &v); err != nil {
		t.Fatal(err)
	}
	for _, i := range v.Yadro.Inbounds {
		if i.Type == "vless" {
			reality = append(reality, i.Port)
		} else {
			hy2 = append(hy2, i.Port)
		}
	}
	if v.Proba != nil {
		proba = v.Proba.Srok
	}
	return reality, hy2, proba, v.SertIPSam != nil
}

func TestUstanovit(t *testing.T) {
	m := novayaMashina(t)
	u := m.ustanovka()
	// Окончательная проверка судит каждый ключ, что бы ни пришло в опциях.
	u.Proverka.PervyyNaVid = true
	var shagi []Shag
	u.Hod = func(h Hod) {
		if len(shagi) == 0 || shagi[len(shagi)-1] != h.Shag {
			shagi = append(shagi, h.Shag)
		}
	}
	itog, err := Ustanovit(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Shag{ShagVhod, ShagNastroyka, ShagOsmotr, ShagKlyuch, ShagProgramma, ShagMaska, ShagSertifikat, ShagPodbor,
		ShagPaket, ShagPorty, ShagProverka}; !slices.Equal(shagi, want) {
		t.Fatalf("шаги %v", shagi)
	}
	pr := m.vhod.pryamo
	if !pr.klyuchom.bylo || !pr.klyuchom.pustoy || pr.klyuchom.komm != "affory DESKTOP 2026-11-01" {
		t.Fatalf("ключ root %+v", pr.klyuchom)
	}
	if pr.zakryt != 1 || m.root.zakryt != 1 {
		t.Fatalf("закрыты: по паролю %d, root %d", pr.zakryt, m.root.zakryt)
	}
	if len(m.pakety) != 3 {
		t.Fatalf("пакетов %d: сертификат, проба, окончательный", len(m.pakety))
	}
	if r, h, proba, sert := paket(t, m.pakety[0]); len(r)+len(h) != 0 || proba != 0 || !sert {
		t.Fatalf("пакет сертификата: reality %v, hy2 %v, проба %d, выпуск %v", r, h, proba, sert)
	}
	// Выпуск через 80: reality может взять 443/tcp.
	if r, h, proba, _ := paket(t, m.pakety[1]); !slices.Equal(r, []int{443, 8443, 465, 993, 2053, 9443}) ||
		!slices.Equal(h, []int{443, 8443, 11444, 5443, 2053}) || proba < 10 {
		t.Fatalf("проба: reality %v, hy2 %v, срок %d", r, h, proba)
	}
	if r, h, proba, sert := paket(t, m.pakety[2]); !slices.Equal(r, []int{443}) || !slices.Equal(h, []int{443}) || proba != 0 || !sert {
		t.Fatalf("окончательный: reality %v, hy2 %v, проба %d", r, h, proba)
	}
	if !slices.Equal(m.porty[0], []int{443}) || !slices.Equal(m.porty[1], []int{443}) {
		t.Fatalf("порты ufw %v", m.porty)
	}
	if len(m.proveryal) != 2 || !m.proveryal[0].Svoy {
		t.Fatalf("проверены %+v", m.proveryal)
	}
	if itog.Adres != "203.0.113.10" || itog.Prodlenie != porty.Cherez80 || itog.Versiya != "0.6.0" || len(itog.Klyuchi) != 2 ||
		len(itog.Proverki) != 2 || len(itog.Preduprezhdeniya) != 0 || itog.Vhod.Put != "напрямую через Ethernet" ||
		itog.Sekrety.Maska != "www.example.com" || itog.Porty.UFW != "active" {
		t.Fatalf("итог %+v", itog)
	}
	if m.detOpcii != nil {
		t.Fatal("детектор без нужды: все виды прошли")
	}
	if itog.Sert == nil || !itog.Sert.Est || itog.Sert.Sposob != "http-01" {
		t.Fatalf("выпуск в итоге %+v", itog.Sert)
	}
	if itog.ParolVyklyuchen {
		t.Fatal("пароль выключен на сервере без нашего файла sshd")
	}
	// Установка поверх своей, где пароль выключали из программы.
	m = novayaMashina(t)
	m.osm.SshdFayly = []string{server.FaylParolya}
	if itog, err := Ustanovit(context.Background(), m.ustanovka()); err != nil || !itog.ParolVyklyuchen {
		t.Fatalf("наш файл sshd: %v %v", itog.ParolVyklyuchen, err)
	}
}

// Без ключа управления на этом компьютере любая отметка чужая.
func TestPrepyatstviyaBezKlyucha(t *testing.T) {
	o := chistyyOsmotr()
	if p := Prepyatstviya(o, nil); p != nil {
		t.Fatalf("чистый сервер: %v", p)
	}
	o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: []string{"AAAA"}}
	if p := Prepyatstviya(o, nil); p == nil || p.Zamechaniya[0].Kod != server.KodUzheStoit || !p.Zamechaniya[0].Stop {
		t.Fatalf("отметка без ключа: %v", p)
	}
}

// Выпуск через TLS-ALPN-01 оставляет 443/tcp продлению.
func TestUstanovitSertCherez443(t *testing.T) {
	m := novayaMashina(t)
	m.serty = []*server.SertUzla{nil, {IP: "203.0.113.10", Est: true, Sposob: "tls-alpn-01"}}
	m.vybor = porty.Vybor{Reality: 8443, Hy2: 443}
	itog, err := Ustanovit(context.Background(), m.ustanovka())
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := paket(t, m.pakety[1]); slices.Contains(r, 443) || itog.Prodlenie != porty.Cherez443 {
		t.Fatalf("reality %v на 443 при продлении через 443", r)
	}
}

// Сертификат на другой адрес (прежний пакет до переезда) не годится: ждём
// выпуска на этот.
func TestUstanovitSertNaDrugoyAdres(t *testing.T) {
	m := novayaMashina(t)
	m.serty = []*server.SertUzla{nil, {IP: "198.51.100.7", Est: true, Sposob: "tls-alpn-01"},
		{IP: "203.0.113.10", Est: true, Sposob: "http-01"}}
	itog, err := Ustanovit(context.Background(), m.ustanovka())
	if err != nil || itog.Prodlenie != porty.Cherez80 {
		t.Fatalf("продление %v, ошибка %v", itog.Prodlenie, err)
	}
}

func TestUstanovitBezSerta(t *testing.T) {
	m := novayaMashina(t)
	m.serty = []*server.SertUzla{{IP: "203.0.113.10", Popytka: 100, Otkaz: "прошлый"},
		{IP: "203.0.113.10", Idyot: true, Popytka: 100}, {IP: "203.0.113.10", Popytka: 200, Otkaz: "Timeout during connect"}}
	m.vybor = porty.Vybor{Reality: 8443}
	itog, err := Ustanovit(context.Background(), m.ustanovka())
	if err != nil {
		t.Fatal(err)
	}
	if itog.Prodlenie != porty.BezSerta || len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NetSerta ||
		itog.Preduprezhdeniya[0].Otkaz != "Timeout during connect" {
		t.Fatalf("итог %+v", itog)
	}
	if _, h, _, sert := paket(t, m.pakety[2]); len(h) != 0 || sert || len(itog.Vhody.Hy2) != 0 || len(itog.Klyuchi) != 1 {
		t.Fatalf("hy2 %v без сертификата, выпуск %v", h, sert)
	}
	// Без сертификата 443/tcp свободен для reality: продлевать нечего.
	if r, _, _, _ := paket(t, m.pakety[1]); r[0] != 443 {
		t.Fatalf("кандидаты reality %v", r)
	}
}

// Прошлый отказ не считается, даже если он был на другой адрес: ждём новую
// попытку.
func TestUstanovitSertSrok(t *testing.T) {
	m := novayaMashina(t)
	m.serty = []*server.SertUzla{{IP: "198.51.100.7", Popytka: 100, Otkaz: "прошлый"},
		{IP: "203.0.113.10", Popytka: 100, Otkaz: "прошлый"}}
	srokSerta = 20 * time.Millisecond
	itog, err := Ustanovit(context.Background(), m.ustanovka())
	if err != nil {
		t.Fatal(err)
	}
	if itog.Prodlenie != porty.BezSerta || !strings.Contains(itog.Preduprezhdeniya[0].Otkaz, "не выпущен") {
		t.Fatalf("итог %+v", itog)
	}
	if m.sprosov < 3 {
		t.Fatalf("спрошено %d раз: не дождались", m.sprosov)
	}
}

func TestUstanovitSertOtmena(t *testing.T) {
	m := novayaMashina(t)
	m.serty = []*server.SertUzla{nil, {IP: "203.0.113.10", Idyot: true}}
	ctx, otmena := context.WithCancel(context.Background())
	shagSerta = time.Hour
	time.AfterFunc(20*time.Millisecond, otmena)
	if _, err := Ustanovit(ctx, m.ustanovka()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if m.root.zakryt != 1 {
		t.Fatal("вход root не закрыт после отмены")
	}
}

// Порт, который заняли после осмотра, уходит из кандидатов, и проба
// ставится снова.
func TestUstanovitZanyat(t *testing.T) {
	m := novayaMashina(t)
	m.zanyat[2] = &server.Zanyat{Port: 443, Proto: "tcp", Vhod: "reality-443"}
	m.zanyat[3] = &server.Zanyat{Port: 8443, Proto: "udp", Vhod: "hy2-8443"}
	m.vybor = porty.Vybor{Reality: 8443, Hy2: 443}
	u := m.ustanovka()
	var otsev []porty.Otsev
	u.Hod = func(h Hod) {
		if h.Otsev != nil {
			otsev = append(otsev, *h.Otsev)
		}
	}
	if _, err := Ustanovit(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if r, h, _, _ := paket(t, m.pakety[3]); slices.Contains(r, 443) || slices.Contains(h, 8443) || !slices.Contains(h, 443) {
		t.Fatalf("третья проба: reality %v, hy2 %v", r, h)
	}
	if len(otsev) != 2 || otsev[0].Port != 443 || otsev[0].Prichina != porty.PrichinaZanyat || otsev[1].Proto != "udp" {
		t.Fatalf("отсев %+v", otsev)
	}
	if slices.Contains(m.podbor.Reality, 443) {
		t.Fatalf("подбор с занятым портом %v", m.podbor.Reality)
	}
}

func TestUstanovitZanyatVse(t *testing.T) {
	m := novayaMashina(t)
	u := m.ustanovka()
	u.Ruchnye = porty.Ruchnye{Reality: 8443}
	m.zanyat[2] = &server.Zanyat{Port: 8443, Proto: "tcp"}
	if _, err := Ustanovit(context.Background(), u); !errors.Is(err, porty.ErrNetKandidatov) {
		t.Fatal(err)
	}
	// Занятым назван порт не из пакета: отказ как есть, а не тихая правка.
	m = novayaMashina(t)
	m.zanyat[2] = &server.Zanyat{Port: 1, Proto: "tcp"}
	if _, err := Ustanovit(context.Background(), m.ustanovka()); err == nil || errors.Is(err, porty.ErrNetKandidatov) {
		t.Fatal(err)
	}
}

// Не прошёл ни один кандидат вида: ставится первый, детектор говорит почему.
func TestUstanovitNeProshyol(t *testing.T) {
	m := novayaMashina(t)
	m.vhod.pryamo.sv.Tunnel = true
	m.vybor = porty.Vybor{}
	m.det = blok.Itog{Vid: blok.NeDohodit}
	nikto := func(k []protokol.Server) []proverka.Itog {
		var it []proverka.Itog
		for _, s := range k {
			it = append(it, proverka.Itog{Server: s, Vid: proverka.NetOtveta})
		}
		return it
	}
	m.itogi = nikto
	// Ключи уходят службе до детектора: перепроверки можно не ждать.
	u := m.ustanovka()
	var shagi []Shag
	var rano *Itog
	u.Hod = func(h Hod) { shagi = append(shagi, h.Shag) }
	u.Klyuchi = func(it Itog) error {
		if rano != nil {
			t.Fatal("ключи отданы дважды")
		}
		rano = &it
		shagi = append(shagi, "klyuchi")
		return nil
	}
	itog, err := Ustanovit(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if rano == nil || len(rano.Klyuchi) != 2 || !slices.Equal(rano.NeProshli, []string{"reality-tcp", "hy2"}) ||
		len(rano.Preduprezhdeniya) != 0 || rano.Adres != "203.0.113.10" {
		t.Fatalf("ранний итог %+v", rano)
	}
	if i, j := slices.Index(shagi, "klyuchi"), slices.Index(shagi, ShagDetektor); i < 0 || j < i || slices.Index(shagi, ShagPorty) > i {
		t.Fatalf("шаги %v", shagi)
	}
	if !slices.Equal(itog.Vhody.Reality, []int{443}) || !slices.Equal(itog.Vhody.Hy2, []int{443}) {
		t.Fatalf("входы %+v", itog.Vhody)
	}
	if !slices.Equal(itog.NeProshli, []string{"reality-tcp", "hy2"}) {
		t.Fatalf("не прошли %v", itog.NeProshli)
	}
	o := m.detOpcii
	if o == nil || o.Adres.String() != "203.0.113.10" || o.PortSsh != 22 || o.Tunnel != blok.TunnelOtvetil ||
		!slices.Equal(o.UDP, []int{443}) || o.Pryamo == nil || m.lovushka != 1 {
		t.Fatalf("опции детектора %+v", o)
	}
	if want := []blok.PortTCP{{Port: 80}, {Port: 443, TLS: "www.example.com"}}; !slices.Equal(o.TCP, want) {
		t.Fatalf("стук %+v", o.TCP)
	}
	if len(itog.Preduprezhdeniya) != 2 {
		t.Fatalf("предупреждения %+v", itog.Preduprezhdeniya)
	}
	for _, p := range itog.Preduprezhdeniya {
		if p.Vid != NeProshyol || p.Detektor == nil || p.Detektor.Vid != blok.NeDohodit || p.Port != 443 || !slices.Contains(p.Kandidaty, 443) {
			t.Fatalf("предупреждение %+v", p)
		}
	}
	// Окончательная проверка тех же ключей второго предупреждения не даёт.
	m = novayaMashina(t)
	m.vybor = porty.Vybor{Reality: 8443}
	m.itogi = func(k []protokol.Server) []proverka.Itog {
		return []proverka.Itog{{Server: k[0], Vid: proverka.Vezyot}, {Server: k[1], Vid: proverka.NetOtveta}}
	}
	itog, err = Ustanovit(context.Background(), m.ustanovka())
	if err != nil || len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Transport != "hy2" || !slices.Equal(itog.NeProshli, []string{"hy2"}) {
		t.Fatalf("итог %+v, ошибка %v", itog.Preduprezhdeniya, err)
	}
	if m.detOpcii.Tunnel != blok.TunnelNeProveren || !slices.Equal(m.detOpcii.TCP,
		[]blok.PortTCP{{Port: 80}, {Port: 443}, {Port: 8443, TLS: "www.example.com"}}) {
		t.Fatalf("опции детектора %+v", m.detOpcii)
	}
	// Адрес закрывали ненадолго: на подборе не прошёл никто, а окончательная
	// проверка после детектора провезла оба ключа. Предупреждать не о чем.
	m = novayaMashina(t)
	m.vybor = porty.Vybor{}
	m.det = blok.Itog{Vid: blok.Dostupen, Krugov: 2}
	itog, err = Ustanovit(context.Background(), m.ustanovka())
	if err != nil || len(itog.Preduprezhdeniya) != 0 || len(itog.NeProshli) != 0 || m.detOpcii == nil {
		t.Fatalf("после повтора итог %+v, не прошли %v, ошибка %v", itog.Preduprezhdeniya, itog.NeProshli, err)
	}
	// Отказ записи ключей останавливает дело: детектор и проверка не идут.
	m = novayaMashina(t)
	m.vybor = porty.Vybor{}
	u = m.ustanovka()
	u.Klyuchi = func(Itog) error { return errors.New("набор не записан") }
	if _, err := Ustanovit(context.Background(), u); err == nil || m.detOpcii != nil || m.proveryal != nil {
		t.Fatalf("отказ записи: %v, детектор %v, проверка %v", err, m.detOpcii, m.proveryal)
	}
}

func TestUstanovitNeVezyot(t *testing.T) {
	m := novayaMashina(t)
	m.itogi = func(k []protokol.Server) []proverka.Itog {
		return []proverka.Itog{{Server: k[0], Vid: proverka.Zamerz, Otkaz: "встал на 16 КБ"}, {Server: k[1], Vid: proverka.Vezyot}}
	}
	itog, err := Ustanovit(context.Background(), m.ustanovka())
	if err != nil || len(itog.Preduprezhdeniya) != 1 {
		t.Fatalf("итог %+v, ошибка %v", itog, err)
	}
	if p := itog.Preduprezhdeniya[0]; p.Vid != NeVezyot || p.Transport != "reality-tcp" || p.Port != 443 || p.Otkaz != "встал на 16 КБ" || p.Proba != proverka.Zamerz {
		t.Fatalf("предупреждение %+v", p)
	}
	if !slices.Equal(itog.NeProshli, []string{"reality-tcp"}) {
		t.Fatalf("не прошли %v", itog.NeProshli)
	}
	// Проверка не состоялась: сервер поставлен, ключи отдаются.
	m = novayaMashina(t)
	m.itogi = func([]protokol.Server) []proverka.Itog { return nil }
	m.errItogi = proverka.ErrNegoden
	itog, err = Ustanovit(context.Background(), m.ustanovka())
	if err != nil || len(itog.Klyuchi) != 2 || len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NeProvereno {
		t.Fatalf("итог %+v, ошибка %v", itog, err)
	}
	// Отмена на проверке: сервер уже раздаёт ключи, и они не теряются.
	m = novayaMashina(t)
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	m.itogi = func([]protokol.Server) []proverka.Itog {
		otmena()
		return nil
	}
	m.errItogi = context.Canceled
	itog, err = Ustanovit(ctx, m.ustanovka())
	if err != nil || len(itog.Klyuchi) != 2 || len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NeProvereno {
		t.Fatalf("после отмены итог %+v, ошибка %v", itog, err)
	}
}

func TestUstanovitOstanovka(t *testing.T) {
	chuzhoy := base64.StdEncoding.EncodeToString(klyuchUpravleniya(t).PublicKey().Marshal())
	for _, c := range []struct {
		imya  string
		pravo func(*server.Osmotr, string)
		kod   server.Kod
	}{
		{"чужое", func(o *server.Osmotr, _ string) { o.Chuzhoe = []string{"programma nginx"} }, server.KodNePustoy},
		{"под центром", func(o *server.Osmotr, _ string) { o.Centr = []string{"klyuch"} }, server.KodPodCentrom},
		{"чужая отметка", func(o *server.Osmotr, _ string) {
			o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: []string{chuzhoy}}
		}, server.KodUzheStoit},
		{"испорченная отметка", func(o *server.Osmotr, _ string) { o.Otmetka = &server.Sostav{Sostoyanie: "isporchena"} }, server.KodUzheStoit},
		{"контейнер", func(o *server.Osmotr, _ string) { o.Virt = "openvz" }, server.KodKonteyner},
		{"своя незаконченная", func(o *server.Osmotr, nash string) {
			o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: []string{chuzhoy, nash}}
		}, ""},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := novayaMashina(t)
			u := m.ustanovka()
			c.pravo(m.osm, base64.StdEncoding.EncodeToString(u.Upravlenie.PublicKey().Marshal()))
			_, err := Ustanovit(context.Background(), u)
			var o *Ostanovka
			if c.kod == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.As(err, &o) || !slices.ContainsFunc(o.Zamechaniya, func(z server.Zamechanie) bool { return z.Kod == c.kod && z.Stop }) {
				t.Fatalf("ошибка %v", err)
			}
			if m.vhod.pryamo.klyuchom.bylo || len(m.pakety) != 0 {
				t.Fatal("после остановки на машине что-то менялось")
			}
			if m.vhod.pryamo.zakryt != 1 {
				t.Fatal("вход не закрыт")
			}
		})
	}
}

func TestUstanovitOtkazy(t *testing.T) {
	for _, c := range []struct {
		imya  string
		pravo func(*Ustanovka)
	}{
		{"имя вместо IP", func(u *Ustanovka) { u.Vhod.Adres = "vps.example.net" }},
		{"без галочки", func(u *Ustanovka) { u.Pustoy = false }},
		{"без ключа управления", func(u *Ustanovka) { u.Upravlenie = nil }},
		{"без имени", func(u *Ustanovka) { u.Imya = "" }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := novayaMashina(t)
			u := m.ustanovka()
			c.pravo(&u)
			if _, err := Ustanovit(context.Background(), u); err == nil || len(m.vhod.zaprosy) != 0 {
				t.Fatalf("ошибка %v, входов %d", err, len(m.vhod.zaprosy))
			}
		})
	}
	// Отказ укладки ключа: вход по паролю закрыт, дальше не идёт.
	m := novayaMashina(t)
	m.vhod.pryamo.errRoot = &ssh.Otkaz{Prichina: ssh.PrichinaRootZapreshchyon, KlyuchOstalsya: true}
	_, err := Ustanovit(context.Background(), m.ustanovka())
	var o *ssh.Otkaz
	if !errors.As(err, &o) || !o.KlyuchOstalsya || m.vhod.pryamo.zakryt != 1 || len(m.pakety) != 0 {
		t.Fatalf("ошибка %v", err)
	}
	// Подбор не решил: установка не выдаёт непроверенное за выбор.
	m = novayaMashina(t)
	m.errVybor = porty.ErrNeReshil
	if _, err := Ustanovit(context.Background(), m.ustanovka()); !errors.Is(err, porty.ErrNeReshil) {
		t.Fatal(err)
	}
}
