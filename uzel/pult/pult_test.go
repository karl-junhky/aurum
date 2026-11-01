package pult

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRazobrat(t *testing.T) {
	horoshie := map[string]Komanda{
		"sostoyanie":          {Imya: "sostoyanie"},
		"zaprosy":             {Imya: "zaprosy"},
		"sert":                {Imya: "sert"},
		"lovushka tcp 443 10": {Imya: "lovushka", Proto: "tcp", Port: 443, Sekund: 10},
		"primenit\n":          {Imya: "primenit"},
		"zhurnal 17":          {Imya: "zhurnal", Chislo: 17},
		"  podtverdit 3 ":     {Imya: "podtverdit", Chislo: 3},
	}
	for stroka, zhdali := range horoshie {
		k, err := Razobrat(stroka)
		if err != nil || k != zhdali {
			t.Fatalf("%q: %+v, %v", stroka, k, err)
		}
	}
	for _, plohaya := range []string{"", "rm -rf /", "zhurnal", "zhurnal -1", "zhurnal 1 2", "sostoyanie lishnee", "zaprosy 1", "sert seychas", "podtverdit x",
		"lovushka", "lovushka tcp 443", "lovushka icmp 443 10", "lovushka tcp x 10", "lovushka udp 443 31", "lovushka tcp 443 10 lishnee"} {
		if _, err := Razobrat(plohaya); err == nil {
			t.Fatalf("%q прошла", plohaya)
		}
	}
}

type poddelka struct {
	paket         []byte
	podtverzhdeno uint64
}

func (p *poddelka) Sostoyanie() any { return map[string]string{"hesh": "abc"} }
func (p *poddelka) Primenit(paket []byte) any {
	p.paket = paket
	if bytes.Contains(paket, []byte("plohoy")) {
		return map[string]string{"oshibka": "пакет плохой"}
	}
	return map[string]string{"hesh": "novyy"}
}
func (p *poddelka) Zhurnal(posle uint64, predel int) (any, error) {
	return []map[string]uint64{{"n": posle + 1}}, nil
}
func (p *poddelka) Zaprosy() any { return map[string]int64{"emp-ab12": 1760500000} }
func (p *poddelka) Sert() any    { return map[string]bool{"est": true} }
func (p *poddelka) Lovushka(proto string, port, sekund int) any {
	return map[string]any{"proto": proto, "port": port, "vsego": sekund}
}
func (p *poddelka) Podtverdit(n uint64) error {
	if n == 13 {
		return errors.New("несчастливое")
	}
	p.podtverzhdeno = n
	return nil
}

func TestPultTudaObratno(t *testing.T) {
	put := filepath.Join(t.TempDir(), "p.sock")
	p := &poddelka{}
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	gotov := make(chan error, 1)
	go func() { gotov <- Slushat(ctx, put, p, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(put); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	sprosit := func(stroka, vhod string) (string, int) {
		var vyhod bytes.Buffer
		kod := Sprosit(put, stroka, strings.NewReader(vhod), &vyhod)
		return vyhod.String(), kod
	}
	if otvet, kod := sprosit("sostoyanie", ""); kod != 0 || !strings.Contains(otvet, `"hesh":"abc"`) {
		t.Fatalf("состояние: %d %s", kod, otvet)
	}
	if otvet, kod := sprosit("primenit", `{"versiya":1}`); kod != 0 || !strings.Contains(otvet, "novyy") || string(p.paket) != `{"versiya":1}` {
		t.Fatalf("применение: %d %s, дошло %q", kod, otvet, p.paket)
	}
	if otvet, kod := sprosit("primenit", "plohoy"); kod != 1 || !strings.Contains(otvet, "пакет плохой") {
		t.Fatalf("плохой пакет должен дать код 1: %d %s", kod, otvet)
	}
	if otvet, kod := sprosit("zhurnal 41", ""); kod != 0 {
		t.Fatalf("журнал: %d %s", kod, otvet)
	} else {
		var zapisi []map[string]uint64
		if err := json.Unmarshal([]byte(otvet), &zapisi); err != nil || zapisi[0]["n"] != 42 {
			t.Fatalf("журнал: %s %v", otvet, err)
		}
	}
	if otvet, kod := sprosit("zaprosy", ""); kod != 0 || !strings.Contains(otvet, `"emp-ab12":1760500000`) {
		t.Fatalf("запросы подписки: %d %s", kod, otvet)
	}
	if otvet, kod := sprosit("lovushka udp 8443 3", ""); kod != 0 || !strings.Contains(otvet, `"port":8443`) || !strings.Contains(otvet, `"proto":"udp"`) {
		t.Fatalf("ловушка: %d %s", kod, otvet)
	}
	if _, kod := sprosit("podtverdit 7", ""); kod != 0 || p.podtverzhdeno != 7 {
		t.Fatalf("подтверждение: %d %d", kod, p.podtverzhdeno)
	}
	if otvet, kod := sprosit("podtverdit 13", ""); kod != 1 || !strings.Contains(otvet, "несчастливое") {
		t.Fatalf("ошибка подтверждения: %d %s", kod, otvet)
	}
	if otvet, kod := sprosit("cat /etc/shadow", ""); kod != 1 || !strings.Contains(otvet, "неизвестная команда") {
		t.Fatalf("чужая команда: %d %s", kod, otvet)
	}

	otmena()
	select {
	case err := <-gotov:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("пульт не остановился")
	}
}
