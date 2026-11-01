package upravlenie

import (
	"bytes"
	"context"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

func TestOtpustit(t *testing.T) {
	nash := klyuchUpravleniya(t)
	for _, c := range []struct {
		imya   string
		snyat  bool
		zhdyom error
	}{
		{imya: "снят", snyat: true},
		{imya: "строки не было"},
		{imya: "последний вход", zhdyom: server.ErrPosledniyVhod},
	} {
		t.Run(c.imya, func(t *testing.T) {
			root := &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "через туннель", Tunnel: true}}
			v := &vhody{tunnel: root, errPryamo: otkaz(ssh.PrichinaTishina)}
			v.podmenit(t)
			bylo := snyatKlyuch
			t.Cleanup(func() { snyatKlyuch = bylo })
			var snimali []gossh.PublicKey
			snyatKlyuch = func(_ context.Context, m server.Mashina, k gossh.PublicKey) (bool, error) {
				if m != root {
					t.Fatal("ключ снимали не тем входом")
				}
				snimali = append(snimali, k)
				return c.snyat, c.zhdyom
			}
			it, err := Otpustit(context.Background(), puti(true), ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: nash}})
			if root.zakryt != 1 {
				t.Fatalf("вход закрыт %d раз", root.zakryt)
			}
			// Прямой путь молчит, туннель через этот же сервер годится.
			if len(v.zaprosy) != 2 || !v.zaprosy[1].BezPerezapuska || !v.zaprosy[1].Put.Tunnel {
				t.Fatalf("входы %+v", v.zaprosy)
			}
			if len(snimali) != 1 || !bytes.Equal(snimali[0].Marshal(), nash.PublicKey().Marshal()) {
				t.Fatal("снимали не наш ключ")
			}
			if c.zhdyom != nil {
				if !errors.Is(err, c.zhdyom) || it.Snyat {
					t.Fatalf("итог %+v, ошибка %v", it, err)
				}
				return
			}
			if err != nil || it.Snyat != c.snyat || it.Vhod.Put != "через туннель" {
				t.Fatalf("итог %+v, ошибка %v", it, err)
			}
		})
	}

	// Без ключа управления не входят вовсе.
	v := &vhody{}
	v.podmenit(t)
	if _, err := Otpustit(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"}); err == nil || len(v.zaprosy) != 0 {
		t.Fatalf("без ключа: %v, входы %d", err, len(v.zaprosy))
	}
	// Вход не открылся: ошибка входа наружу.
	v = &vhody{errPryamo: otkaz(ssh.PrichinaKlyuchNePrinyat)}
	v.podmenit(t)
	if _, err := Otpustit(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Podpis: nash}}); ssh.PrichinaIz(err) != ssh.PrichinaKlyuchNePrinyat {
		t.Fatal(err)
	}
}
