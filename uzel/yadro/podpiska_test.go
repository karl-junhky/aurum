package yadro_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/podpiska"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/trafik"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/yadro"
)

var tokenBorisa = strings.Repeat("b2", 16)

func (s *stend) sertIzFaylov() podpiska.Sert {
	s.t.Helper()
	cert, err := os.ReadFile(s.sert)
	if err != nil {
		s.t.Fatal(err)
	}
	klyuch, err := os.ReadFile(s.klyuch)
	if err != nil {
		s.t.Fatal(err)
	}
	return podpiska.Sert{Cert: string(cert), Klyuch: string(klyuch)}
}

func (s *stend) paket2(lyudi []chelovek, adresPodpiski, telo string) []byte {
	s.t.Helper()
	sert := s.sertIzFaylov()
	p, err := json.Marshal(map[string]any{
		"versiya": 2,
		"yadro":   s.konfigServera(lyudi, s.porty["hy2"]),
		"sert":    sert,
		"podpiska": podpiska.Nabor{Slushat: adresPodpiski, Lyudi: map[string]podpiska.Otvet{
			tokenBorisa: {Imya: "boris", Telo: telo, Zagolovki: map[string]string{"profile-title": "Affory"}},
		}},
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return p
}

// zabrat забирает подписку Бориса по TLS, проверяя сертификат стенда.
func (s *stend) zabrat(adres string) (int, string) {
	s.t.Helper()
	doverie := x509.NewCertPool()
	doverie.AppendCertsFromPEM([]byte(s.sertIzFaylov().Cert))
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: doverie, ServerName: "localhost"}}
	defer tr.CloseIdleConnections()
	otvet, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get("https://" + adres + "/sub/" + tokenBorisa)
	if err != nil {
		s.t.Fatal(err)
	}
	defer otvet.Body.Close()
	telo, err := io.ReadAll(otvet.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return otvet.StatusCode, string(telo)
}

// Пакет версии 2: сертификат ложится в каталог, подписка раздаётся, смена
// одной подписки ядро не трогает, после перезапуска подписка поднимается с
// диска, пакет версии 1 подписку гасит.
func TestPodpiskaVPakete(t *testing.T) {
	s := novyyStend(t)
	tihiy := slog.New(slog.NewTextHandler(io.Discard, nil))
	raz := podpiska.Novyy(tihiy)
	t.Cleanup(raz.Ostanovit)
	s.y.PodklyuchitPodpisku(raz)
	adres := "127.0.0.1:" + strconv.Itoa(svobodnyyPort(t, false))

	k := s.y.Primenit(s.paket2([]chelovek{lyudB}, adres, "pervoe"))
	if k.Oshibka != "" || k.Kak != yadro.Perezapusk {
		t.Fatalf("первый пакет: %+v", k)
	}
	for imya, zhdali := range map[string]string{"fullchain.pem": s.sertIzFaylov().Cert, "privkey.pem": s.sertIzFaylov().Klyuch} {
		bylo, err := os.ReadFile(filepath.Join(s.katalog, "sert", imya))
		if err != nil || string(bylo) != zhdali {
			t.Fatalf("%s в каталоге состояния: %v", imya, err)
		}
	}
	if kod, telo := s.zabrat(adres); kod != http.StatusOK || telo != "pervoe" {
		t.Fatalf("подписка: %d %q", kod, telo)
	}

	klient := s.klient([]chelovek{lyudB}, s.porty["hy2"])
	c, err := s.soedinit(klient, "trojan-boris")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := pereklichka(c, "do", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	k = s.y.Primenit(s.paket2([]chelovek{lyudB}, adres, "vtoroe"))
	if k.Oshibka != "" || k.Kak != yadro.YadroPrezhnee || k.Zakryto != 0 {
		t.Fatalf("смена одной подписки: %+v", k)
	}
	if err := pereklichka(c, "posle", 5*time.Second); err != nil {
		t.Fatalf("смена подписки порвала соединение: %v", err)
	}
	if kod, telo := s.zabrat(adres); kod != http.StatusOK || telo != "vtoroe" {
		t.Fatalf("подписка после смены: %d %q", kod, telo)
	}
	hesh := k.Hesh

	// Перезапуск программы: подписка встаёт из файла состояния.
	raz.Ostanovit()
	s.y.Ostanovit()
	raz2 := podpiska.Novyy(tihiy)
	t.Cleanup(raz2.Ostanovit)
	y2 := yadro.Novoe(s.katalog, trafik.NovyySchyotchik(), tihiy)
	t.Cleanup(y2.Ostanovit)
	y2.PodklyuchitPodpisku(raz2)
	if err := y2.Zapustit(); err != nil {
		t.Fatal(err)
	}
	if z := y2.Zdorove(); !z.YadroZhivo || z.Hesh != hesh {
		t.Fatalf("с диска: %+v", z)
	}
	if kod, telo := s.zabrat(adres); kod != http.StatusOK || telo != "vtoroe" {
		t.Fatalf("подписка с диска: %d %q", kod, telo)
	}

	// Версия 1 с тем же ядром: ядро прежнее, подписка снята.
	k = y2.Primenit(paket(t, s.konfigServera([]chelovek{lyudB}, s.porty["hy2"])))
	if k.Oshibka != "" || k.Kak != yadro.YadroPrezhnee {
		t.Fatalf("пакет версии 1: %+v", k)
	}
	if raz2.Sostoyanie() != nil {
		t.Fatal("пакет версии 1 не снял подписку")
	}
}

// Негодная подписка отбивается целиком, до того как трогать ядро.
func TestNegodnayaPodpiskaNeTrogaetYadro(t *testing.T) {
	s := novyyStend(t)
	k := s.primenit([]chelovek{lyudB}, s.porty["hy2"])
	if k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	prezhniy := k.Hesh

	var p map[string]any
	if err := json.Unmarshal(s.paket2([]chelovek{lyudA, lyudB}, "127.0.0.1:0", "x"), &p); err != nil {
		t.Fatal(err)
	}
	p["podpiska"].(map[string]any)["lyudi"].(map[string]any)[tokenBorisa].(map[string]any)["zagolovki"] = map[string]string{"x": "a\nb"}
	plohoy, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	k = s.y.Primenit(plohoy)
	if k.Oshibka == "" || k.Hesh != prezhniy {
		t.Fatalf("негодная подписка прошла: %+v", k)
	}
	if err := s.mozhet(s.klient([]chelovek{lyudA}, s.porty["hy2"]), "trojan-anna"); err == nil {
		t.Fatal("ядро сменилось по пакету с негодной подпиской")
	}
	if _, err := os.Stat(filepath.Join(s.katalog, "sert")); err == nil {
		t.Fatal("сертификат из отвергнутого пакета лёг на диск")
	}

	// Подписка в пакете версии 1 тоже отказ.
	p["versiya"] = 1
	plohoy, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if k = s.y.Primenit(plohoy); k.Oshibka == "" {
		t.Fatal("подписка в пакете версии 1 прошла")
	}
}
