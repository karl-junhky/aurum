package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

const (
	komandaPrimenit   = "/usr/local/sbin/vpn-uzel pult primenit"
	komandaSostoyanie = "/usr/local/sbin/vpn-uzel pult sostoyanie"
)

var probnyyPaket = []byte(`{"versiya":3,"yadro":{}}`)

func kvitanciya(hesh string, zhivo bool, dalshe string) []byte {
	return fmt.Appendf(nil, `{"hesh":%q,"kak":"перезапуск","zakryto":0,"yadro_zhivo":%t%s}`+"\n", hesh, zhivo, dalshe)
}

func TestPrimenit(t *testing.T) {
	m := novaya()
	m.otvety[komandaPrimenit] = ssh.Itog{Vyvod: kvitanciya(heshPaketa(probnyyPaket), true, "")}
	k, err := Primenit(context.Background(), m, probnyyPaket)
	if err != nil || k.Kak != "перезапуск" || !k.YadroZhivo {
		t.Fatalf("квитанция %+v, ошибка %v", k, err)
	}
	if !bytes.Equal(m.vhod, probnyyPaket) || len(m.vyzovy) != 1 || m.vyzovy[0] != "cat "+komandaPrimenit {
		t.Fatalf("в stdin %q, вызовы %q", m.vhod, m.vyzovy)
	}
}

func TestPrimenitOtkazy(t *testing.T) {
	svoy := heshPaketa(probnyyPaket)
	for _, c := range []struct {
		imya   string
		itog   ssh.Itog
		tekst  string
		zanyat int
	}{
		{"порт занят", ssh.Itog{Kod: 1, Vyvod: kvitanciya("prezhniy", true,
			`,"oshibka":"listen tcp 0.0.0.0:443: bind: address already in use","zanyat":{"port":443,"proto":"tcp","vhod":"reality-443","prichina":"address already in use"}`)},
			"address already in use", 443},
		{"пульт молчит", ssh.Itog{Kod: 1, Vyvod: []byte(`{"oshibka":"программа сервера не отвечает: dial unix"}`)}, "не отвечает", 0},
		{"программы нет", ssh.Itog{Kod: 127, Oshibki: "bash: /usr/local/sbin/vpn-uzel: No such file or directory"}, "No such file", 0},
		{"не JSON", ssh.Itog{Vyvod: []byte("Segmentation fault")}, "не разобрана", 0},
		{"работает другой пакет", ssh.Itog{Vyvod: kvitanciya("chuzhoy", true, "")}, "не с присланным", 0},
		{"ядро не живо", ssh.Itog{Vyvod: kvitanciya(svoy, false, "")}, "не поднялось", 0},
		{"код без ошибки", ssh.Itog{Kod: 1, Vyvod: kvitanciya(svoy, true, ""), Oshibki: "сбой"}, "сбой", 0},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := novaya()
			m.otvety[komandaPrimenit] = c.itog
			k, err := Primenit(context.Background(), m, probnyyPaket)
			var o *Oshibka
			if !errors.As(err, &o) || o.Shag != "primenit" || !strings.Contains(err.Error(), c.tekst) {
				t.Fatalf("ошибка %v", err)
			}
			if (c.zanyat == 0) != (k.Zanyat == nil) || k.Zanyat != nil && (k.Zanyat.Port != c.zanyat || k.Zanyat.Vhod != "reality-443") {
				t.Fatalf("квитанция %+v", k)
			}
		})
	}
	m := novaya()
	if _, err := Primenit(context.Background(), m, nil); err == nil || len(m.vyzovy) != 0 {
		t.Fatalf("пустой пакет: ошибка %v, вызовы %q", err, m.vyzovy)
	}
	m.oshibka = errors.New("соединение оборвалось")
	if _, err := Primenit(context.Background(), m, probnyyPaket); !errors.Is(err, m.oshibka) {
		t.Fatal(err)
	}
}

func TestSostoyanie(t *testing.T) {
	m := novaya()
	m.otvety[komandaSostoyanie] = ssh.Itog{Vyvod: []byte(`{"versiya":"0.6.0","sing_box":"1.14.2","hesh":"abc","yadro_zhivo":true,` +
		`"oshibka":"прошлый отказ","vhody":[],"sert_ip_sam":{"ip":"203.0.113.10","est":true,"ot":100,"do":200,"sposob":"http-01"}}`)}
	s, err := Sostoyanie(context.Background(), m)
	if err != nil || s.Versiya != "0.6.0" || s.Hesh != "abc" || !s.YadroZhivo || s.Oshibka != "прошлый отказ" ||
		s.Sert == nil || !s.Sert.Est || s.Sert.Do != 200 || s.Sert.Sposob != "http-01" {
		t.Fatalf("состояние %+v, ошибка %v", s, err)
	}
	// Без выпуска в пакете поля нет вовсе.
	m.otvety[komandaSostoyanie] = ssh.Itog{Vyvod: []byte(`{"versiya":"0.6.0","hesh":"","yadro_zhivo":false,"vhody":null}`)}
	if s, err := Sostoyanie(context.Background(), m); err != nil || s.Sert != nil || s.YadroZhivo {
		t.Fatalf("состояние %+v, ошибка %v", s, err)
	}
}

func TestSostoyanieOtkazy(t *testing.T) {
	for _, c := range []struct {
		imya  string
		itog  ssh.Itog
		tekst string
	}{
		{"пульт молчит", ssh.Itog{Kod: 1, Vyvod: []byte(`{"oshibka":"программа сервера не отвечает: dial unix"}`)}, "не отвечает"},
		{"программы нет", ssh.Itog{Kod: 127, Oshibki: "bash: /usr/local/sbin/vpn-uzel: No such file or directory"}, "No such file"},
		{"не JSON", ssh.Itog{Vyvod: []byte("Segmentation fault")}, "не разобран"},
		{"без версии", ssh.Itog{Vyvod: []byte(`{"hesh":"abc"}`)}, "нет версии"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m := novaya()
			m.otvety[komandaSostoyanie] = c.itog
			_, err := Sostoyanie(context.Background(), m)
			var o *Oshibka
			if !errors.As(err, &o) || o.Shag != "sostoyanie" || !strings.Contains(err.Error(), c.tekst) {
				t.Fatalf("ошибка %v", err)
			}
		})
	}
}
