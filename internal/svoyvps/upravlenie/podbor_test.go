package upravlenie

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// postavlennyy это поставленный сервер для подбора заново: своя отметка,
// принятый пакет с рабочими портами 50443/tcp и 40443/udp вне списков
// кандидатов, выпуск через 80 из тестового каталога. Вход сразу root по
// ключу управления.
type postavlennyy struct {
	*mashina
	sek   konfig.Sekrety
	paket []byte
}

func novyyPostavlennyy(t *testing.T, vh konfig.Vhody) (*postavlennyy, Podbor) {
	t.Helper()
	m := novayaMashina(t)
	m.vhod.pryamo = m.root
	k := klyuchUpravleniya(t)
	m.osm.Otmetka = &server.Sostav{Format: 1, Sostoyanie: "gotovo", VpnUzel: "0.6.0",
		Klyuchi: []string{base64.StdEncoding.EncodeToString(k.PublicKey().Marshal())}}
	m.serty = []*server.SertUzla{{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true}}
	sek, err := konfig.NovyeSekrety("www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	b, err := konfig.Paket(sek, vh, konfig.Opcii{Staging: true})
	if err != nil {
		t.Fatal(err)
	}
	ps := &postavlennyy{mashina: m, sek: sek, paket: b}
	bP := prochitatUstanovku
	t.Cleanup(func() { prochitatUstanovku = bP })
	prochitatUstanovku = func(_ context.Context, x server.Mashina) (server.Sostav, []byte, error) {
		if x != m.root {
			t.Fatal("установка читается не входом root")
		}
		return *m.osm.Otmetka, ps.paket, nil
	}
	p := Podbor{Puti: puti(false), Imya: "Мой сервер",
		Vhod:     ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: k}},
		Proverka: proverka.Opcii{Adapter: "Ethernet"},
		Zapusk:   func(context.Context, []byte) (func(), error) { return func() {}, nil },
		Pryamo:   (&blokDial{}).dial,
	}
	return ps, p
}

var rabochiyServer = konfig.Vhody{Imya: "Мой сервер", Adres: "203.0.113.10", Reality: []int{50443}, Hy2: []int{40443}}

func TestPodobratPorty(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	var shagi []Shag
	p.Hod = func(h Hod) {
		if len(shagi) == 0 || shagi[len(shagi)-1] != h.Shag {
			shagi = append(shagi, h.Shag)
		}
	}
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Shag{ShagVhod, ShagOsmotr, ShagPodbor, ShagPaket, ShagPorty, ShagProverka}; !slices.Equal(shagi, want) {
		t.Fatalf("шаги %v", shagi)
	}
	if m.root.klyuchom.bylo || m.root.zakryt != 1 || m.maski != nil {
		t.Fatalf("ключ root %v, закрыт %d раз, маски %v", m.root.klyuchom.bylo, m.root.zakryt, m.maski)
	}
	if len(m.pakety) != 2 {
		t.Fatalf("пакетов %d: проба и окончательный, без выпуска", len(m.pakety))
	}
	// Рабочие порты идут в пробу последними: пока она идёт, ключи живут.
	if r, h, proba, sert := paket(t, m.pakety[0]); !slices.Equal(r, []int{443, 8443, 465, 993, 2053, 9443, 50443}) ||
		!slices.Equal(h, []int{443, 8443, 11444, 5443, 2053, 40443}) || proba < 10 || !sert {
		t.Fatalf("проба: reality %v, hy2 %v, срок %d, выпуск %v", r, h, proba, sert)
	}
	if r, h, proba, _ := paket(t, m.pakety[1]); !slices.Equal(r, []int{443}) || !slices.Equal(h, []int{443}) || proba != 0 {
		t.Fatalf("окончательный: reality %v, hy2 %v, проба %d", r, h, proba)
	}
	// Каталог сертификата тот, что у программы сервера: иначе перевыпуск.
	for i, b := range m.pakety {
		if !strings.Contains(string(b), `"staging":true`) {
			t.Fatalf("пакет %d сменил каталог сертификата", i)
		}
	}
	if !slices.Equal(m.porty[0], []int{443}) || !slices.Equal(m.porty[1], []int{443}) {
		t.Fatalf("порты ufw %v", m.porty)
	}
	if itog.Sekrety != m.sek || itog.Prodlenie != porty.Cherez80 || itog.Versiya != "0.6.0" || len(itog.Klyuchi) != 2 ||
		itog.Adres != "203.0.113.10" || len(itog.Preduprezhdeniya) != 0 || itog.Sert == nil || !itog.Sert.Staging || itog.ParolVyklyuchen {
		t.Fatalf("итог %+v", itog)
	}
	if itog.Klyuchi[0].Server.Port != 443 {
		t.Fatalf("ключ reality на порту %d", itog.Klyuchi[0].Server.Port)
	}
	// Рабочий порт уже в списке кандидатов: второй раз он туда не идёт.
	m, p = novyyPostavlennyy(t, konfig.Vhody{Imya: "Мой сервер", Adres: "203.0.113.10", Reality: []int{8443}, Hy2: []int{5443}})
	if _, err := PodobratPorty(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if r, h, _, _ := paket(t, m.pakety[0]); !slices.Equal(r, []int{443, 8443, 465, 993, 2053, 9443}) ||
		!slices.Equal(h, []int{443, 8443, 11444, 5443, 2053}) {
		t.Fatalf("проба: reality %v, hy2 %v", r, h)
	}
	// Наш файл sshd на сервере: пароль выключали из программы.
	m, p = novyyPostavlennyy(t, rabochiyServer)
	m.osm.SshdFayly = []string{server.FaylParolya}
	if itog, err := PodobratPorty(context.Background(), p); err != nil || !itog.ParolVyklyuchen {
		t.Fatalf("наш файл sshd: %v %v", itog.ParolVyklyuchen, err)
	}
}

// Окончательный пакет принят: отказ портов в ufw и отмена посреди детектора
// не теряют ключей, а только предупреждают.
func TestPodobratPortyPosleOkonchatelnogo(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	errUfw := errors.New("ufw: отказ")
	otkrytPorty = func(context.Context, server.Mashina, []int, []int) (server.Porty, error) {
		return server.Porty{}, errUfw
	}
	m.vybor = porty.Vybor{Reality: 443}
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	detektor = func(ctx context.Context, _ blok.Opcii) (blok.Itog, error) {
		otmena()
		return blok.Itog{}, ctx.Err()
	}
	m.itogi = func([]protokol.Server) []proverka.Itog { return nil }
	m.errItogi = context.Canceled
	itog, err := PodobratPorty(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(itog.Klyuchi) != 2 || itog.Sekrety != m.sek || !slices.Equal(itog.Vhody.Reality, []int{443}) || !slices.Equal(itog.Vhody.Hy2, []int{40443}) {
		t.Fatalf("итог %+v", itog)
	}
	var vidy []VidPreduprezhdeniya
	for _, pr := range itog.Preduprezhdeniya {
		vidy = append(vidy, pr.Vid)
		if pr.Vid == NeProshyol && (pr.Detektor != nil || pr.Otkaz == "" || pr.Port != 40443) {
			t.Fatalf("предупреждение детектора %+v", pr)
		}
		if pr.Vid == PortyNeOtkryty && !strings.Contains(pr.Otkaz, "ufw") {
			t.Fatalf("предупреждение портов %+v", pr)
		}
	}
	if !slices.Equal(vidy, []VidPreduprezhdeniya{PortyNeOtkryty, NeProshyol, NeProvereno}) {
		t.Fatalf("предупреждения %v", vidy)
	}
	// Окончательный пакет не принят: ключей нет, это отказ.
	m, p = novyyPostavlennyy(t, rabochiyServer)
	m.zanyat[2] = &server.Zanyat{Proto: "udp", Port: 443}
	if _, err := PodobratPorty(context.Background(), p); err == nil {
		t.Fatal("непринятый окончательный пакет прошёл удачей")
	}
}

// Не прошёл ни один кандидат вида: остаётся рабочий порт, а детектор
// говорит почему.
func TestPodobratPortyNeProshyol(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.vybor = porty.Vybor{Hy2: 8443}
	m.det = blok.Itog{Vid: blok.NeDohodit}
	p.Vhod.Port = 2222
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if m.detOpcii == nil || m.detOpcii.PortSsh != 2222 {
		t.Fatalf("детектор стучит не в порт SSH записи: %+v", m.detOpcii)
	}
	if !slices.Equal(itog.Vhody.Reality, []int{50443}) || !slices.Equal(itog.Vhody.Hy2, []int{8443}) {
		t.Fatalf("входы %+v", itog.Vhody)
	}
	if len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NeProshyol || itog.Preduprezhdeniya[0].Port != 50443 {
		t.Fatalf("предупреждения %+v", itog.Preduprezhdeniya)
	}
	if !slices.Equal(m.porty[0], []int{50443}) || !slices.Equal(m.porty[1], []int{8443}) {
		t.Fatalf("порты ufw %v", m.porty)
	}
}

func TestPodobratPortyRuchnye(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	p.Ruchnye = porty.Ruchnye{Reality: 2083}
	m.vybor = porty.Vybor{}
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	// Заданный руками идёт один, рабочий порт вида в пробу не попадает.
	if r, h, _, _ := paket(t, m.pakety[0]); !slices.Equal(r, []int{2083}) || !slices.Equal(h, []int{443, 8443, 11444, 5443, 2053, 40443}) {
		t.Fatalf("проба: reality %v, hy2 %v", r, h)
	}
	if !slices.Equal(itog.Vhody.Reality, []int{2083}) || !slices.Equal(itog.Vhody.Hy2, []int{40443}) {
		t.Fatalf("входы %+v", itog.Vhody)
	}
	// Негодный ручной порт это отказ до любой правки сервера.
	m, p = novyyPostavlennyy(t, rabochiyServer)
	p.Ruchnye = porty.Ruchnye{Reality: 22}
	var ots porty.Otsev
	if _, err := PodobratPorty(context.Background(), p); !errors.As(err, &ots) || ots.Prichina != porty.PrichinaSsh || len(m.pakety) != 0 {
		t.Fatalf("ошибка %v, пакетов %d", err, len(m.pakety))
	}
}

// Рабочий порт, который с тех пор заняла чужая программа, запасным не идёт.
func TestPodobratPortyRabochiyZanyat(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.osm.Slushayut = []server.Slushatel{{Proto: "tcp", Port: 50443, Process: "nginx"}, {Proto: "udp", Port: 40443, Process: "vpn-uzel"}}
	m.vybor = porty.Vybor{}
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r, h, _, _ := paket(t, m.pakety[0]); slices.Contains(r, 50443) || !slices.Contains(h, 40443) {
		t.Fatalf("проба: reality %v, hy2 %v", r, h)
	}
	if !slices.Equal(itog.Vhody.Reality, []int{443}) || !slices.Equal(itog.Vhody.Hy2, []int{40443}) {
		t.Fatalf("входы %+v", itog.Vhody)
	}
}

var bezHy2 = konfig.Vhody{Imya: "Мой сервер", Adres: "203.0.113.10", Reality: []int{50443}}

// Сервер без hy2: сертификат при установке не вышел, а теперь выходит, и
// встаёт hy2. Выпуск идёт пробой с прежними входами.
func TestPodobratPortyBezHy2Vypusk(t *testing.T) {
	m, p := novyyPostavlennyy(t, bezHy2)
	m.serty = []*server.SertUzla{nil, nil, {IP: "203.0.113.10", Idyot: true}, {IP: "203.0.113.10", Est: true, Sposob: "http-01"}}
	var shagi []Shag
	p.Hod = func(h Hod) {
		if len(shagi) == 0 || shagi[len(shagi)-1] != h.Shag {
			shagi = append(shagi, h.Shag)
		}
	}
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Shag{ShagVhod, ShagOsmotr, ShagSertifikat, ShagPodbor, ShagPaket, ShagPorty, ShagProverka}; !slices.Equal(shagi, want) {
		t.Fatalf("шаги %v", shagi)
	}
	if len(m.pakety) != 3 {
		t.Fatalf("пакетов %d: выпуск, проба, окончательный", len(m.pakety))
	}
	if r, h, proba, sert := paket(t, m.pakety[0]); !slices.Equal(r, []int{50443}) || len(h) != 0 || !sert || proba != 360 {
		t.Fatalf("выпуск: reality %v, hy2 %v, проба %d, выпуск %v", r, h, proba, sert)
	}
	if _, h, _, sert := paket(t, m.pakety[1]); !slices.Equal(h, []int{443, 8443, 11444, 5443, 2053}) || !sert {
		t.Fatalf("проба: hy2 %v, выпуск %v", h, sert)
	}
	if itog.Prodlenie != porty.Cherez80 || !slices.Equal(itog.Vhody.Hy2, []int{443}) || len(itog.Klyuchi) != 2 ||
		len(itog.Preduprezhdeniya) != 0 || itog.Sert == nil || !itog.Sert.Est {
		t.Fatalf("итог %+v", itog)
	}
}

// Выпуск снова не вышел: hy2 не встаёт, подбор только reality. Старый отказ
// из учёта программы, видный до новой попытки, свежим не считается.
func TestPodobratPortyBezHy2NeVyshel(t *testing.T) {
	m, p := novyyPostavlennyy(t, bezHy2)
	m.serty = []*server.SertUzla{nil, nil, {IP: "203.0.113.10", Popytka: 100, Otkaz: "прошлый"},
		{IP: "203.0.113.10", Idyot: true, Popytka: 100}, {IP: "203.0.113.10", Popytka: 200, Otkaz: "Timeout during connect"}}
	m.vybor = porty.Vybor{Reality: 8443}
	itog, err := PodobratPorty(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r, h, _, sert := paket(t, m.pakety[1]); len(h) != 0 || sert || !slices.Contains(r, 50443) {
		t.Fatalf("проба: reality %v, hy2 %v, выпуск %v", r, h, sert)
	}
	if itog.Prodlenie != porty.BezSerta || !slices.Equal(itog.Vhody.Reality, []int{8443}) || len(itog.Vhody.Hy2) != 0 || len(itog.Klyuchi) != 1 {
		t.Fatalf("итог %+v", itog)
	}
	if len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NetSerta || itog.Preduprezhdeniya[0].Otkaz != "Timeout during connect" {
		t.Fatalf("предупреждения %+v", itog.Preduprezhdeniya)
	}
	// Новой попытки на глазах не было: ждём до срока, а не верим старому.
	m, p = novyyPostavlennyy(t, bezHy2)
	m.serty = []*server.SertUzla{nil, nil, {IP: "203.0.113.10", Popytka: 100, Otkaz: "прошлый"}}
	srokSerta = 20 * time.Millisecond
	itog, err = PodobratPorty(context.Background(), p)
	if err != nil || len(itog.Preduprezhdeniya) != 1 || !strings.Contains(itog.Preduprezhdeniya[0].Otkaz, "не выпущен") {
		t.Fatalf("итог %+v, ошибка %v", itog.Preduprezhdeniya, err)
	}
}

func TestPodobratPortyOtkazy(t *testing.T) {
	chuzhoy := base64.StdEncoding.EncodeToString(klyuchUpravleniya(t).PublicKey().Marshal())
	for _, c := range []struct {
		imya   string
		izmeni func(*postavlennyy)
		test   func(error) bool
	}{
		{"нет установки", func(m *postavlennyy) { m.osm.Otmetka = nil },
			func(err error) bool { return errors.Is(err, server.ErrNetUstanovki) }},
		{"чужая отметка", func(m *postavlennyy) { m.osm.Otmetka.Klyuchi = []string{chuzhoy} },
			func(err error) bool { return errors.Is(err, ErrChuzhayaUstanovka) }},
		{"под центром", func(m *postavlennyy) { m.osm.Centr = []string{"klyuch"} },
			func(err error) bool {
				var o *Ostanovka
				return errors.As(err, &o) && o.Zamechaniya[0].Kod == server.KodPodCentrom
			}},
		{"не дошла", func(m *postavlennyy) { m.paket = nil },
			func(err error) bool { return errors.Is(err, ErrNeDoshla) }},
		{"идёт проба", func(m *postavlennyy) {
			b, err := konfig.Paket(m.sek, rabochiyServer, konfig.Opcii{Proba: 300})
			if err != nil {
				t.Fatal(err)
			}
			m.paket = b
		}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "пробный пакет") }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m, p := novyyPostavlennyy(t, rabochiyServer)
			c.izmeni(m)
			if _, err := PodobratPorty(context.Background(), p); !c.test(err) {
				t.Fatalf("отказ: %v", err)
			}
			if len(m.pakety) != 0 || m.root.zakryt != 1 {
				t.Fatalf("пакетов %d, вход закрыт %d раз", len(m.pakety), m.root.zakryt)
			}
		})
	}
	for _, c := range []struct {
		imya  string
		pravo func(*Podbor)
	}{
		{"имя вместо IP", func(p *Podbor) { p.Vhod.Adres = "vps.example.net" }},
		{"без ключа управления", func(p *Podbor) { p.Vhod.Vhod.Podpis = nil }},
		{"без имени", func(p *Podbor) { p.Imya = "" }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m, p := novyyPostavlennyy(t, rabochiyServer)
			c.pravo(&p)
			if _, err := PodobratPorty(context.Background(), p); err == nil || len(m.vhod.zaprosy) != 0 {
				t.Fatalf("ошибка %v, входов %d", err, len(m.vhod.zaprosy))
			}
		})
	}
}
