package upravlenie

import (
	"context"
	"errors"
	"io"
	"io/fs"
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
