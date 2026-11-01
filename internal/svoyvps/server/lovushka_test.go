package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

const komandaLovushki = "/usr/local/sbin/vpn-uzel pult lovushka"

func TestLovit(t *testing.T) {
	m := novaya()
	m.otvety[komandaLovushki] = ssh.Itog{Vyvod: []byte(`{"proto":"tcp","port":47123,"sekund":8,"vsego":3,"adresa":{"203.0.113.5":3}}` + "\n")}
	vsego, err := Lovit(context.Background(), m, "tcp", 47123, 8)
	if err != nil || vsego != 3 {
		t.Fatalf("поймано %d, ошибка %v", vsego, err)
	}
	if len(m.vyzovy) != 1 || m.vyzovy[0] != "cat "+komandaLovushki {
		t.Fatalf("вызовы %q", m.vyzovy)
	}
	// Ноль это ответ, а не отказ: стук не дошёл.
	m.otvety[komandaLovushki] = ssh.Itog{Vyvod: []byte(`{"proto":"udp","port":443,"sekund":8,"vsego":0,"adresa":{}}`)}
	if vsego, err := Lovit(context.Background(), m, "udp", 443, 8); err != nil || vsego != 0 {
		t.Fatalf("поймано %d, ошибка %v", vsego, err)
	}
}

// Отказ ловушки это отказ, а не «ничего не дошло».
func TestLovitOtkazy(t *testing.T) {
	for _, c := range []struct {
		imya  string
		itog  ssh.Itog
		tekst string
	}{
		{"ошибка пульта", ssh.Itog{Kod: 1, Vyvod: []byte(`{"oshibka":"программа сервера не отвечает: dial unix"}`)}, "программа сервера не отвечает"},
		{"программы нет", ssh.Itog{Kod: 127, Oshibki: "bash: /usr/local/sbin/vpn-uzel: No such file or directory"}, "No such file"},
		{"без счёта", ssh.Itog{Vyvod: []byte(`{"proto":"tcp"}`)}, "нет счёта"},
		{"счёт меньше нуля", ssh.Itog{Vyvod: []byte(`{"proto":"tcp","vsego":-1}`)}, "насчитала -1"},
		{"не JSON", ssh.Itog{Vyvod: []byte("Segmentation fault")}, "не разобран"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := novaya()
			m.otvety[komandaLovushki] = c.itog
			vsego, err := Lovit(context.Background(), m, "tcp", 47123, 8)
			var o *Oshibka
			if !errors.As(err, &o) || o.Shag != "lovushka" || !strings.Contains(err.Error(), c.tekst) || vsego != 0 {
				t.Fatalf("поймано %d, ошибка %v", vsego, err)
			}
		})
	}
	m := novaya()
	m.oshibka = errors.New("соединение оборвалось")
	if _, err := Lovit(context.Background(), m, "tcp", 47123, 8); !errors.Is(err, m.oshibka) {
		t.Fatal(err)
	}
	for _, c := range []struct {
		proto        string
		port, sekund int
	}{{"icmp", 1, 1}, {"tcp", 0, 1}, {"udp", 65536, 1}, {"tcp", 22, 0}, {"tcp", 22, PredelLovushki + 1}} {
		m := novaya()
		if _, err := Lovit(context.Background(), m, c.proto, c.port, c.sekund); err == nil || len(m.vyzovy) != 0 {
			t.Fatalf("%+v: ошибка %v, вызовы %q", c, err, m.vyzovy)
		}
	}
}
