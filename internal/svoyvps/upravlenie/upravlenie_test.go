package upravlenie

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"net/netip"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// svyaz это подставной вход: операции на сервере подменяются целиком, а
// здесь видно, каким входом их позвали и закрыт ли он.
type svyaz struct {
	imya    string
	sv      Svedeniya
	zakryt  int
	root    *svyaz
	errRoot error
	// Новый вход тем же путём (Perevoyti): errNovyy на первые errNovyyRaz
	// входов, а 0 - на все.
	novyy       *svyaz
	errNovyy    error
	errNovyyRaz int
	novyhVhodov int
	klyuchom    struct {
		bylo   bool
		pustoy bool
		komm   string
	}
}

func (s *svyaz) Vypolnit(context.Context, string, io.Reader) (ssh.Itog, error) {
	return ssh.Itog{}, errors.New("подставной вход: команды не ждали")
}

func (s *svyaz) Skript(context.Context, string, ...string) (ssh.Itog, error) {
	return ssh.Itog{}, errors.New("подставной вход: скрипта не ждали")
}

func (s *svyaz) OtpravitFayl(context.Context, io.ReadSeeker, string, fs.FileMode) error {
	return errors.New("подставной вход: файла не ждали")
}

func (s *svyaz) Svedeniya() Svedeniya { return s.sv }

func (s *svyaz) KlyuchRoot(_ context.Context, _ gossh.Signer, komm string, pustoy bool) (Svyaz, error) {
	s.klyuchom.bylo, s.klyuchom.pustoy, s.klyuchom.komm = true, pustoy, komm
	if s.errRoot != nil {
		return nil, s.errRoot
	}
	return s.root, nil
}

func (s *svyaz) Perevoyti(context.Context, gossh.Signer) (Svyaz, error) {
	s.novyhVhodov++
	if s.errNovyy != nil && (s.errNovyyRaz == 0 || s.novyhVhodov <= s.errNovyyRaz) {
		return nil, s.errNovyy
	}
	if s.novyy == nil {
		return nil, errors.New("подставной вход: нового входа не ждали")
	}
	return s.novyy, nil
}

func (s *svyaz) Zakryt() { s.zakryt++ }

// vhody подставляет podklyuchit: на каждую дорогу свой ответ.
type vhody struct {
	pryamo, tunnel *svyaz
	errPryamo      error
	errTunnel      error
	zaprosy        []ssh.Zapros
}

func (v *vhody) podmenit(t *testing.T) {
	t.Helper()
	bylo := podklyuchit
	t.Cleanup(func() { podklyuchit = bylo })
	podklyuchit = func(_ context.Context, z ssh.Zapros) (Svyaz, error) {
		v.zaprosy = append(v.zaprosy, z)
		if z.Put.Tunnel {
			if v.errTunnel != nil {
				return nil, v.errTunnel
			}
			return v.tunnel, nil
		}
		if v.errPryamo != nil {
			return nil, v.errPryamo
		}
		return v.pryamo, nil
	}
}

func puti(tunnel bool) Puti {
	return Puti{
		Pryamo: func() (ssh.Put, error) { return ssh.Put{Imya: "напрямую через Ethernet"}, nil },
		Tunnel: func() (ssh.Put, bool) { return ssh.Put{Imya: "через туннель", Tunnel: true}, tunnel },
	}
}

func otkaz(p ssh.Prichina) error { return &ssh.Otkaz{Prichina: p} }

func TestVoyti(t *testing.T) {
	for _, c := range []struct {
		imya      string
		errPryamo error
		errTunnel error
		tunnel    bool
		zhdyom    string // pryamo, tunnel, molchanie, otkaz
		dorog     int
	}{
		{"напрямую", nil, nil, true, "pryamo", 1},
		{"прямой молчит, туннель отвечает", otkaz(ssh.PrichinaTishina), nil, true, "tunnel", 2},
		{"прямой рвётся", otkaz(ssh.PrichinaObryv), nil, true, "tunnel", 2},
		{"сброс на прямом", otkaz(ssh.PrichinaPortZakryt), nil, true, "tunnel", 2},
		{"молчат обе", otkaz(ssh.PrichinaTishina), otkaz(ssh.PrichinaTishina), true, "molchanie", 2},
		{"туннеля нет", otkaz(ssh.PrichinaTishina), nil, false, "molchanie", 1},
		{"пароль не принят второй дорогой не проверяется", otkaz(ssh.PrichinaParolNePrinyat), nil, true, "otkaz", 1},
		{"туннель ответил отказом", otkaz(ssh.PrichinaTishina), otkaz(ssh.PrichinaKlyuchHostaSmenilsya), true, "otkaz", 2},
	} {
		t.Run(c.imya, func(t *testing.T) {
			v := &vhody{pryamo: &svyaz{imya: "pryamo"}, tunnel: &svyaz{imya: "tunnel"}, errPryamo: c.errPryamo, errTunnel: c.errTunnel}
			v.podmenit(t)
			s, err := Voyti(context.Background(), puti(c.tunnel), ssh.Zapros{Adres: "203.0.113.10"})
			if len(v.zaprosy) != c.dorog {
				t.Fatalf("дорог %d, ждали %d", len(v.zaprosy), c.dorog)
			}
			var m *Molchanie
			switch c.zhdyom {
			case "pryamo", "tunnel":
				if err != nil || s.(*svyaz).imya != c.zhdyom {
					t.Fatalf("вход %v, ошибка %v", s, err)
				}
			case "molchanie":
				if !errors.As(err, &m) || m.Pryamo != c.errPryamo || m.Tunnel != c.errTunnel {
					t.Fatalf("ошибка %v", err)
				}
				// Причина достаётся из молчания: служба пишет текст по ней.
				if ssh.PrichinaIz(err) != ssh.PrichinaTishina {
					t.Fatalf("причина %q", ssh.PrichinaIz(err))
				}
			case "otkaz":
				if errors.As(err, &m) || err == nil {
					t.Fatalf("ошибка %v", err)
				}
			}
		})
	}
	bezAdaptera := errors.New("нет адаптера")
	_, err := Voyti(context.Background(), Puti{
		Pryamo: func() (ssh.Put, error) { return ssh.Put{}, bezAdaptera },
		Tunnel: func() (ssh.Put, bool) {
			t.Fatal("без адаптера туннель не спрашивают")
			return ssh.Put{}, false
		},
	}, ssh.Zapros{Adres: "203.0.113.10"})
	if !errors.Is(err, bezAdaptera) {
		t.Fatal(err)
	}
	if _, err := Voyti(context.Background(), Puti{}, ssh.Zapros{}); err == nil {
		t.Fatal("вход без дорог")
	}
}

func chistyyOsmotr() *server.Osmotr {
	return &server.Osmotr{
		OS: "ubuntu", OSVersiya: "24.04", OSImya: "Ubuntu 24.04", Arch: "x86_64", PID1: "systemd", Virt: "kvm",
		PamyatMB: 900, DiskMB: 9000, BBR: "zagruzhen", UFW: "active",
		IPv4: []server.Adres{{Interfeys: "eth0", Set: netip.MustParsePrefix("203.0.113.10/24")}},
		IPv6: []server.Adres{{Interfeys: "eth0", Set: netip.MustParsePrefix("2001:db8::10/64")}},
		Sshd: map[string]string{"port": "22"},
	}
}

func podmenitOsmotr(t *testing.T, fn func(context.Context, server.Mashina) (*server.Osmotr, error)) {
	t.Helper()
	bylo := osmotret
	t.Cleanup(func() { osmotret = bylo })
	osmotret = fn
}

func TestOsmotret(t *testing.T) {
	v := &vhody{pryamo: &svyaz{imya: "pryamo", sv: Svedeniya{Put: "напрямую через Ethernet", Root: true}}}
	v.podmenit(t)
	o := chistyyOsmotr()
	o.Chuzhoe = []string{"programma nginx"}
	podmenitOsmotr(t, func(_ context.Context, m server.Mashina) (*server.Osmotr, error) {
		if m != v.pryamo {
			t.Fatal("осмотр не тем входом")
		}
		return o, nil
	})
	otch, err := Osmotret(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"})
	if err != nil || otch.Osmotr != o || !otch.Vhod.Root || otch.Vhod.Put != "напрямую через Ethernet" {
		t.Fatalf("отчёт %+v, ошибка %v", otch, err)
	}
	if len(otch.Zamechaniya) != 1 || otch.Zamechaniya[0].Kod != server.KodNePustoy {
		t.Fatalf("замечания %+v", otch.Zamechaniya)
	}
	if v.pryamo.zakryt != 1 {
		t.Fatal("вход осмотра не закрыт")
	}
	// Отказ осмотра тоже закрывает вход.
	podmenitOsmotr(t, func(context.Context, server.Mashina) (*server.Osmotr, error) {
		return nil, errors.New("оборван")
	})
	if _, err := Osmotret(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"}); err == nil || v.pryamo.zakryt != 2 {
		t.Fatalf("ошибка %v, закрыт %d раз", err, v.pryamo.zakryt)
	}
}

func klyuchUpravleniya(t *testing.T) gossh.Signer {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := gossh.NewSignerFromKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
