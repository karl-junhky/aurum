package upravlenie

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/netip"
	"slices"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// skriptovaya отвечает на скрипт маски заданным итогом.
type skriptovaya struct {
	it         ssh.Itog
	argumenty  []string
	tekstOtvet string
}

func (s *skriptovaya) Vypolnit(context.Context, string, io.Reader) (ssh.Itog, error) {
	return ssh.Itog{}, errors.New("не ждали")
}

func (s *skriptovaya) Skript(_ context.Context, tekst string, a ...string) (ssh.Itog, error) {
	s.argumenty, s.tekstOtvet = a, tekst
	return s.it, nil
}

func (s *skriptovaya) OtpravitFayl(context.Context, io.ReadSeeker, string, fs.FileMode) error {
	return errors.New("не ждали")
}

func TestProveritMasku(t *testing.T) {
	for _, c := range []struct {
		imya               string
		it                 ssh.Itog
		godna, proveryaemo bool
		oshibka            bool
	}{
		{"годна", ssh.Itog{Vyvod: []byte("godna\n")}, true, true, false},
		{"негодна", ssh.Itog{Vyvod: []byte("negodna\n")}, false, true, false},
		{"нет openssl", ssh.Itog{Vyvod: []byte("net-openssl\n")}, false, false, false},
		{"скрипт упал", ssh.Itog{Kod: 2, Oshibki: "bash: сбой"}, false, false, true},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := &skriptovaya{it: c.it}
			godna, proveryaemo, err := proveritMasku(context.Background(), m, "dl.google.com")
			if godna != c.godna || proveryaemo != c.proveryaemo || (err != nil) != c.oshibka {
				t.Fatalf("годна %v, проверяемо %v, ошибка %v", godna, proveryaemo, err)
			}
			if !slices.Equal(m.argumenty, []string{"dl.google.com"}) || m.tekstOtvet != skriptMaski {
				t.Fatalf("аргументы %q", m.argumenty)
			}
		})
	}
}

func TestVybratMasku(t *testing.T) {
	bylo := proveritMasku
	t.Cleanup(func() { proveritMasku = bylo })
	spisok := []string{"a.example", "b.example", "c.example"}
	otvety := map[string][2]bool{}
	var sprosheny []string
	proveritMasku = func(_ context.Context, _ server.Mashina, maska string) (bool, bool, error) {
		sprosheny = append(sprosheny, maska)
		o := otvety[maska]
		return o[0], o[1], nil
	}

	otvety = map[string][2]bool{"a.example": {false, true}, "b.example": {true, true}, "c.example": {false, true}}
	for range 20 {
		sprosheny = nil
		if m, err := vybratMasku(context.Background(), nil, spisok); err != nil || m != "b.example" {
			t.Fatalf("маска %q, ошибка %v", m, err)
		}
	}
	// Порядок случайный: каждая маска хоть раз спрошена первой.
	pervye := map[string]bool{}
	for range 200 {
		sprosheny = nil
		if _, err := vybratMasku(context.Background(), nil, spisok); err != nil {
			t.Fatal(err)
		}
		pervye[sprosheny[0]] = true
	}
	if len(pervye) != 3 {
		t.Fatalf("первыми бывали %v", pervye)
	}

	otvety = map[string][2]bool{"a.example": {false, true}, "b.example": {false, true}, "c.example": {false, true}}
	if _, err := vybratMasku(context.Background(), nil, spisok); !errors.Is(err, ErrNetMaski) {
		t.Fatal(err)
	}
	otvety = map[string][2]bool{}
	sprosheny = nil
	if m, err := vybratMasku(context.Background(), nil, spisok); err != nil || len(sprosheny) != 1 || m != sprosheny[0] {
		t.Fatalf("без openssl маска %q, спрошены %v, ошибка %v", m, sprosheny, err)
	}
	if _, err := vybratMasku(context.Background(), nil, nil); err == nil {
		t.Fatal("пустой список")
	}
	sboy := errors.New("оборвалось")
	proveritMasku = func(context.Context, server.Mashina, string) (bool, bool, error) { return false, false, sboy }
	if _, err := vybratMasku(context.Background(), nil, spisok); !errors.Is(err, sboy) {
		t.Fatal(err)
	}
}

func TestPodobratMasku(t *testing.T) {
	adres := netip.MustParseAddr("203.0.113.10")
	for _, c := range []struct {
		imya    string
		it      ssh.Itog
		maska   Maska
		oshibka error
	}{
		{"соседи", ssh.Itog{Vyvod: []byte("maska\tsosed.example.net\r\nistochnik\tsosedi\nprosmotreno\t40\n")},
			Maska{Imya: "sosed.example.net", Istochnik: MaskaSosedi}, nil},
		{"запас", ssh.Itog{Vyvod: []byte("maska\tzapas.example.org\nistochnik\tzapas\n")},
			Maska{Imya: "zapas.example.org", Istochnik: MaskaZapas}, nil},
		{"неизвестный источник считается известной маской", ssh.Itog{Vyvod: []byte("maska\tx.example.org\nistochnik\tnovoe\n")},
			Maska{Imya: "x.example.org", Istochnik: MaskaBrend}, nil},
		{"не нашлось", ssh.Itog{Kod: 1, Oshibki: "ни один сайт не годится в маску\n"}, Maska{}, ErrNetMaski},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := &skriptovaya{it: c.it}
			mk, err := podobratMasku(context.Background(), m, adres)
			if mk != c.maska || !errors.Is(err, c.oshibka) {
				t.Fatalf("маска %+v, ошибка %v", mk, err)
			}
			if !slices.Equal(m.argumenty, []string{"203.0.113.10"}) || m.tekstOtvet != skriptPodbora {
				t.Fatalf("аргументы %q", m.argumenty)
			}
		})
	}
	for imya, it := range map[string]ssh.Itog{
		"негодное имя": {Vyvod: []byte("maska\tНе имя\nistochnik\tsosedi\n")},
		"пусто":        {Vyvod: []byte("istochnik\tsosedi\n")},
		"скрипт упал":  {Kod: 127, Oshibki: "vpn-uzel: not found\n"},
		// Код 2 без «неизвестная команда» - отказ новой программы, а не
		// старая программа: известная маска молча не ставится.
		"негодный адрес": {Kod: 2, Oshibki: "адрес сервера: ParseAddr: unable to parse IP\n"},
	} {
		if _, err := podobratMasku(context.Background(), &skriptovaya{it: it}, adres); err == nil || errors.Is(err, ErrNetMaski) {
			t.Errorf("%s: %v", imya, err)
		}
	}
}

// Программа сервера без подбора (старше 0.7.0) отвечает кодом 2: маска из
// прежнего списка, помеченная как известная.
func TestPodobratMaskuStarayaProgramma(t *testing.T) {
	bylo := proveritMasku
	t.Cleanup(func() { proveritMasku = bylo })
	proveritMasku = func(context.Context, server.Mashina, string) (bool, bool, error) { return true, true, nil }
	mk, err := podobratMasku(context.Background(), &skriptovaya{it: ssh.Itog{Kod: 2, Oshibki: "неизвестная команда \"maska\"\n"}},
		netip.MustParseAddr("203.0.113.10"))
	if err != nil || !slices.Contains(Maski, mk.Imya) || mk.Istochnik != MaskaBrend {
		t.Fatalf("%+v, %v", mk, err)
	}
}
