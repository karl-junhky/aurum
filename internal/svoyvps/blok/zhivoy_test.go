//go:build zhivoy

package blok

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// Живой детектор на настоящем сервере и настоящей сети (Р4 плана):
//
//	AFFORY_BLOK_ADRES=<адрес> AFFORY_BLOK_ZHDU=<вердикт> [AFFORY_BLOK_TUNNEL=127.0.0.1:10809] \
//	  go test -tags zhivoy -run TestZhivoy -v ./internal/svoyvps/blok/
//
// Туннель судится, как его судила бы служба: приветствием SSH через вход
// ядра. Ловушки здесь нет: она требует программы сервера. Повторы короче
// боевых, их сроки проверяют тесты без сети.
func TestZhivoy(t *testing.T) {
	adres := os.Getenv("AFFORY_BLOK_ADRES")
	if adres == "" {
		t.Skip("не задан AFFORY_BLOK_ADRES")
	}
	vernutBoevye()
	povtory = []time.Duration{20 * time.Second, 40 * time.Second}
	a, err := ssh.FizicheskiyAdapter()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("адаптер %s", a.Imya)
	o := Opcii{
		Adres: netip.MustParseAddr(adres), PortSsh: 22,
		TCP:    []PortTCP{{Port: 80}, {Port: 443, TLS: "dl.google.com"}},
		Pryamo: ssh.PutPryamo(a.Indeks, a.Imya).Dial,
		Hod: func(k Krug) {
			t.Logf("круг %d: %s, следующий через %s", k.Nomer, k.Vid, k.Sleduyushchiy)
		},
	}
	if vhod := os.Getenv("AFFORY_BLOK_TUNNEL"); vhod != "" {
		o.Tunnel = tunnelSSH(t, ssh.PutCherezTunnel(vhod).Dial, adres)
		t.Logf("через туннель: %q", o.Tunnel)
	}
	ctx, otm := context.WithTimeout(context.Background(), 5*time.Minute)
	defer otm()
	// При живом туннеле имена находит система, и до публичного DNS мимо
	// туннеля очередь не доходит. Поэтому он судится отдельно: системный
	// поиск выключен, сеть обязана оказаться живой.
	bylo := iskat
	iskat = func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, errors.New("системный поиск выключен живым тестом")
	}
	set := proveritSet(ctx, o)
	iskat = bylo
	if set != SetZhivaya {
		t.Fatalf("через публичный DNS мимо туннеля сеть %s", set)
	}
	itog, err := Proverit(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range itog.Stuki {
		t.Logf("%d: %s %s %s", s.Port, s.Ishod, s.Otvet, s.Podrobno)
	}
	t.Logf("сеть %s, вердикт %s, кругов %d", itog.Set, itog.Vid, itog.Krugov)
	if zhdu := os.Getenv("AFFORY_BLOK_ZHDU"); zhdu != "" && string(itog.Vid) != zhdu {
		t.Fatalf("вердикт %s, ждала %s", itog.Vid, zhdu)
	}
}

func tunnelSSH(t *testing.T, dial Dial, adres string) Tunnel {
	ctx, otm := context.WithTimeout(context.Background(), 15*time.Second)
	defer otm()
	c, err := dial(ctx, "tcp", net.JoinHostPort(adres, "22"))
	if err != nil {
		t.Logf("туннель: %v", err)
		return TunnelMolchit
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(15 * time.Second))
	stroka, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || !strings.HasPrefix(stroka, "SSH-") {
		t.Logf("туннель: %q %v", stroka, err)
		return TunnelMolchit
	}
	return TunnelOtvetil
}
