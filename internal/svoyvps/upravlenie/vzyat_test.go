package upravlenie

import (
	"bytes"
	"context"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

func TestSnesti(t *testing.T) {
	for _, c := range []struct {
		imya     string
		posle    *server.Osmotr
		errSnosa error
		zhdyom   string // chisto, ne-chisto, otkaz
	}{
		{"чисто", chistyyOsmotr(), nil, "chisto"},
		{"осталась отметка", func() *server.Osmotr { o := chistyyOsmotr(); o.Otmetka = &server.Sostav{}; return o }(), nil, "ne-chisto"},
		{"осталась программа", func() *server.Osmotr { o := chistyyOsmotr(); o.VpnUzel = "0.6.0"; return o }(), nil, "ne-chisto"},
		{"отказ сноса", chistyyOsmotr(), &server.Oshibka{Shag: "snos", Kod: 1}, "otkaz"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			root := &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "напрямую"}}
			v := &vhody{pryamo: root}
			v.podmenit(t)
			bylo := snesti
			t.Cleanup(func() { snesti = bylo })
			snyato := false
			snesti = func(_ context.Context, m server.Mashina) (server.Snos, error) {
				if m != root {
					t.Fatal("снос не тем входом")
				}
				snyato = true
				return server.Snos{ParolVernulsya: true}, c.errSnosa
			}
			podmenitOsmotr(t, func(context.Context, server.Mashina) (*server.Osmotr, error) {
				if !snyato {
					t.Fatal("осмотр раньше сноса")
				}
				return c.posle, nil
			})
			it, err := Snesti(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"})
			var nc *NeChisto
			switch c.zhdyom {
			case "chisto":
				if err != nil || !it.Snos.ParolVernulsya || it.Vhod.Put != "напрямую" {
					t.Fatalf("снос: %+v %v", it, err)
				}
			case "ne-chisto":
				if !errors.As(err, &nc) || len(nc.Chto) != 1 {
					t.Fatalf("остаток не замечен: %v", err)
				}
			case "otkaz":
				if !errors.Is(err, c.errSnosa) {
					t.Fatalf("отказ сноса: %v", err)
				}
			}
			if root.zakryt != 1 {
				t.Fatalf("вход закрыт %d раз", root.zakryt)
			}
		})
	}
}

// zahvat это подставной сервер под Affory: отметка, принятый пакет и
// выпуск сертификата.
type zahvat struct {
	parol, root *svyaz
	osm         *server.Osmotr
	paket       []byte
	sert        *server.SertUzla
	zapisan     gossh.PublicKey
}

func novyyZahvat(t *testing.T, sek konfig.Sekrety, v konfig.Vhody) *zahvat {
	t.Helper()
	p, err := konfig.Paket(sek, v, konfig.Opcii{})
	if err != nil {
		t.Fatal(err)
	}
	z := &zahvat{osm: chistyyOsmotr(), paket: p, sert: &server.SertUzla{IP: "203.0.113.10", Est: true, Sposob: "http-01"}}
	z.osm.Otmetka = &server.Sostav{Format: 1, Sostoyanie: "gotovo", VpnUzel: "vpn-uzel 0.6.0, sing-box 1.14.2", Klyuchi: []string{"chuzhoy"}}
	z.root = &svyaz{imya: "root", sv: Svedeniya{Root: true}}
	z.parol = &svyaz{imya: "parol", root: z.root, sv: Svedeniya{Put: "напрямую"}}
	(&vhody{pryamo: z.parol}).podmenit(t)
	podmenitOsmotr(t, func(_ context.Context, m server.Mashina) (*server.Osmotr, error) {
		if m != z.parol {
			t.Fatal("осмотр захвата не входом человека")
		}
		return z.osm, nil
	})
	bZ, bP, bS := zapisatKlyuch, prochitatUstanovku, sostoyanie
	t.Cleanup(func() { zapisatKlyuch, prochitatUstanovku, sostoyanie = bZ, bP, bS })
	zapisatKlyuch = func(_ context.Context, m server.Mashina, k gossh.PublicKey) error {
		if m != z.root {
			t.Fatal("ключ в отметку не входом root")
		}
		z.zapisan = k
		return nil
	}
	prochitatUstanovku = func(_ context.Context, m server.Mashina) (server.Sostav, []byte, error) {
		if m != z.root || z.zapisan == nil {
			t.Fatal("установка читается не входом root или до записи ключа")
		}
		return *z.osm.Otmetka, z.paket, nil
	}
	sostoyanie = func(_ context.Context, m server.Mashina) (server.SostoyanieUzla, error) {
		return server.SostoyanieUzla{Versiya: "0.6.0", YadroZhivo: true, Sert: z.sert}, nil
	}
	return z
}

func vzyatie(t *testing.T) Vzyatie {
	return Vzyatie{Puti: puti(false), Imya: "Дача", Upravlenie: klyuchUpravleniya(t), Kommentariy: "affory PC 2026-11-01",
		Vhod: ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Parol: "p"}}}
}

func TestVzyat(t *testing.T) {
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	vh := konfig.Vhody{Imya: "Дача", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{8443}}
	z := novyyZahvat(t, sek, vh)
	v := vzyatie(t)
	it, err := Vzyat(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	if !z.parol.klyuchom.bylo || !z.parol.klyuchom.pustoy || z.parol.klyuchom.komm != v.Kommentariy {
		t.Fatalf("ключ root: %+v", z.parol.klyuchom)
	}
	if z.zapisan == nil || !bytes.Equal(z.zapisan.Marshal(), v.Upravlenie.PublicKey().Marshal()) {
		t.Fatal("в отметку лёг не ключ этого компьютера")
	}
	if z.parol.zakryt != 1 || z.root.zakryt != 1 {
		t.Fatalf("входы закрыты %d и %d раз", z.parol.zakryt, z.root.zakryt)
	}
	nado, err := konfig.Klyuchi(sek, vh)
	if err != nil {
		t.Fatal(err)
	}
	if len(it.Klyuchi) != len(nado) {
		t.Fatalf("ключей %d, а ставилось %d", len(it.Klyuchi), len(nado))
	}
	for i := range nado {
		if it.Klyuchi[i].Ssylka != nado[i].Ssylka {
			t.Fatalf("ключ %d перевыпущен", i)
		}
	}
	if it.Prodlenie != porty.Cherez80 || it.Sert != z.sert || it.Versiya != "0.6.0" || it.Adres != "203.0.113.10" || it.ParolVyklyuchen {
		t.Fatalf("итог захвата: %+v", it)
	}
	// Наш файл sshd на сервере: пароль выключали из программы.
	z = novyyZahvat(t, sek, vh)
	z.osm.SshdFayly = []string{"00-billpanel.conf", server.FaylParolya}
	if it, err := Vzyat(context.Background(), vzyatie(t)); err != nil || !it.ParolVyklyuchen {
		t.Fatalf("наш файл sshd: %v %v", it.ParolVyklyuchen, err)
	}
}

func TestVzyatProdlenie(t *testing.T) {
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		imya string
		hy2  []int
		sert *server.SertUzla
		nado porty.Prodlenie
	}{
		{"без hy2", nil, nil, porty.BezSerta},
		{"программа молчит", []int{8443}, nil, porty.Cherez443},
		{"через 443", []int{8443}, &server.SertUzla{Est: true, Sposob: "tls-alpn-01"}, porty.Cherez443},
	} {
		t.Run(c.imya, func(t *testing.T) {
			z := novyyZahvat(t, sek, konfig.Vhody{Imya: "x", Adres: "203.0.113.10", Reality: []int{443}, Hy2: c.hy2})
			z.sert = c.sert
			it, err := Vzyat(context.Background(), vzyatie(t))
			if err != nil || it.Prodlenie != c.nado {
				t.Fatalf("продление %v, ждали %v: %v", it.Prodlenie, c.nado, err)
			}
		})
	}
}

func TestVzyatOtkazy(t *testing.T) {
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	vh := konfig.Vhody{Imya: "x", Adres: "203.0.113.10", Reality: []int{443}}
	for _, c := range []struct {
		imya     string
		izmenit  func(*zahvat)
		klyuchom bool
		test     func(error) bool
	}{
		{"нет установки", func(z *zahvat) { z.osm.Otmetka = nil }, false,
			func(err error) bool { return errors.Is(err, server.ErrNetUstanovki) }},
		{"под центром", func(z *zahvat) { z.osm.Centr = []string{"vpn-centr"} }, false,
			func(err error) bool {
				var o *Ostanovka
				return errors.As(err, &o) && o.Zamechaniya[0].Kod == server.KodPodCentrom
			}},
		{"не дошла", func(z *zahvat) { z.paket = nil }, true,
			func(err error) bool { return errors.Is(err, ErrNeDoshla) }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			z := novyyZahvat(t, sek, vh)
			c.izmenit(z)
			_, err := Vzyat(context.Background(), vzyatie(t))
			if !c.test(err) {
				t.Fatalf("отказ: %v", err)
			}
			if z.parol.klyuchom.bylo != c.klyuchom {
				t.Fatalf("ключ root положен: %v", z.parol.klyuchom.bylo)
			}
		})
	}
	v := vzyatie(t)
	v.Vhod.Adres = "vps.example.net"
	if _, err := Vzyat(context.Background(), v); err == nil {
		t.Fatal("захват по имени хоста")
	}
}
