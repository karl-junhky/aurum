package sayt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	domen = "kot.example.net"
	put   = "0123456789abcdef0123456789abcdef"
)

type podstavnoyVypusk struct {
	mu      sync.Mutex
	sert    *tls.Certificate
	derzhit atomic.Bool
	otvet   string // ответ на проверку; пусто - проверки нет
}

func (v *podstavnoyVypusk) Tekushchiy() *tls.Certificate {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.sert
}

func (v *podstavnoyVypusk) postavit(c *tls.Certificate) {
	v.mu.Lock()
	v.sert = c
	v.mu.Unlock()
}

func (v *podstavnoyVypusk) OtvetitProverke(w http.ResponseWriter, r *http.Request) bool {
	if v.otvet == "" || r.URL.Path != "/.well-known/acme-challenge/tok" {
		return false
	}
	_, _ = io.WriteString(w, v.otvet)
	return true
}

func (v *podstavnoyVypusk) Derzhat80(da bool) { v.derzhit.Store(da) }

type podstavnoyBrandmauer struct {
	otkryto, ubrano atomic.Int32
}

func (b *podstavnoyBrandmauer) Otkryt80() error { b.otkryto.Add(1); return nil }
func (b *podstavnoyBrandmauer) Ubrat() error    { b.ubrano.Add(1); return nil }

type stend struct {
	sayt       *Sayt
	vypusk     *podstavnoyVypusk
	brandmauer *podstavnoyBrandmauer
	koren      *x509.CertPool
	port80     int
	portHttps  int
}

func novyyStend(t *testing.T) *stend {
	t.Helper()
	sert, koren := samopodpisannyy(t, domen)
	st := &stend{vypusk: &podstavnoyVypusk{sert: sert}, brandmauer: &podstavnoyBrandmauer{}, koren: koren,
		port80: svobodnyyPort(t), portHttps: svobodnyyPort(t)}
	st.sayt = Novyy(slog.New(slog.NewTextHandler(io.Discard, nil)), st.vypusk,
		Opcii{Port80: "127.0.0.1:" + strconv.Itoa(st.port80), Brandmauer: st.brandmauer})
	t.Cleanup(st.sayt.Ostanovit)
	return st
}

func (st *stend) nastroyka() *Nastroyka {
	return &Nastroyka{Domen: domen, Port: 8443, Slushat: "127.0.0.1:" + strconv.Itoa(st.portHttps),
		Zerno: "fedcba9876543210fedcba9876543210",
		Podpiska: Podpiska{Put: put, Telo: "dmxlc3M6Ly8=",
			Zagolovki: map[string]string{"profile-update-interval": "12", "routing": "happ://routing/onadd/abc"}}}
}

func (st *stend) klientHttps() *http.Client {
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: st.koren, ServerName: domen},
		ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+strconv.Itoa(st.portHttps))
		},
	}}
}

func TestSaytIPodpiska(t *testing.T) {
	st := novyyStend(t)
	n := st.nastroyka()
	if err := n.Proverit(); err != nil {
		t.Fatal(err)
	}
	st.sayt.Primenit(n)
	k := st.klientHttps()

	o, err := k.Get("https://" + domen + "/")
	if err != nil {
		t.Fatal(err)
	}
	telo, _ := io.ReadAll(o.Body)
	o.Body.Close()
	if o.StatusCode != http.StatusOK || !strings.Contains(string(telo), "<html") {
		t.Fatalf("главная заглушки: %d %.200s", o.StatusCode, telo)
	}
	if o.ProtoMajor != 2 || o.TLS.CurveID != tls.X25519 {
		t.Fatalf("сайт не похож на обычный: %s, кривая %v", o.Proto, o.TLS.CurveID)
	}
	if st.sayt.Sostoyanie().Zabrana != 0 {
		t.Fatal("главная засчитана как забор подписки")
	}

	o, err = k.Get("https://" + domen + "/" + put)
	if err != nil {
		t.Fatal(err)
	}
	telo, _ = io.ReadAll(o.Body)
	o.Body.Close()
	if o.StatusCode != http.StatusOK || string(telo) != "dmxlc3M6Ly8=" ||
		o.Header.Get("routing") != "happ://routing/onadd/abc" || o.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("подписка: %d %q %v", o.StatusCode, telo, o.Header)
	}
	if st.sayt.Sostoyanie().Zabrana == 0 {
		t.Fatal("забор подписки не отмечен")
	}

	for _, chuzhoy := range []string{"/" + strings.ToUpper(put), "/" + put + "/", "/sub/" + put, "/ffffffffffffffffffffffffffffffff"} {
		o, err = k.Get("https://" + domen + chuzhoy)
		if err != nil {
			t.Fatal(err)
		}
		telo, _ = io.ReadAll(o.Body)
		o.Body.Close()
		if o.StatusCode != http.StatusNotFound || strings.Contains(string(telo), "dmxlc3M6Ly8=") {
			t.Fatalf("%s: %d %.100s", chuzhoy, o.StatusCode, telo)
		}
	}
	zapros, _ := http.NewRequest(http.MethodPost, "https://"+domen+"/"+put, nil)
	if o, err = k.Do(zapros); err != nil {
		t.Fatal(err)
	}
	o.Body.Close()
	if o.StatusCode == http.StatusOK {
		t.Fatal("подписка отдана на POST")
	}
}

func TestPort80(t *testing.T) {
	st := novyyStend(t)
	st.sayt.Primenit(st.nastroyka())
	if !st.vypusk.derzhit.Load() || st.brandmauer.otkryto.Load() != 1 {
		t.Fatalf("80 не отдан выпуску: держит %v, открыт %d", st.vypusk.derzhit.Load(), st.brandmauer.otkryto.Load())
	}
	k := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	adres := "http://127.0.0.1:" + strconv.Itoa(st.port80)

	zapros, _ := http.NewRequest(http.MethodGet, adres+"/x/y?z=1", nil)
	zapros.Host = "chuzhoy.example.org"
	o, err := k.Do(zapros)
	if err != nil {
		t.Fatal(err)
	}
	o.Body.Close()
	if o.StatusCode != http.StatusMovedPermanently || o.Header.Get("Location") != "https://"+domen+":8443/x/y?z=1" {
		t.Fatalf("переадресация: %d %q", o.StatusCode, o.Header.Get("Location"))
	}

	st.vypusk.otvet = "tok.klyuch"
	o, err = k.Get(adres + "/.well-known/acme-challenge/tok")
	if err != nil {
		t.Fatal(err)
	}
	telo, _ := io.ReadAll(o.Body)
	o.Body.Close()
	if o.StatusCode != http.StatusOK || string(telo) != "tok.klyuch" {
		t.Fatalf("проверка Let's Encrypt: %d %q", o.StatusCode, telo)
	}
	o, err = k.Get(adres + "/.well-known/acme-challenge/drugoy")
	if err != nil {
		t.Fatal(err)
	}
	o.Body.Close()
	if o.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("чужая проверка: %d", o.StatusCode)
	}

	st.sayt.Primenit(nil)
	if st.vypusk.derzhit.Load() || st.brandmauer.ubrano.Load() == 0 || st.sayt.Sostoyanie() != nil {
		t.Fatal("выключенный сайт держит 80")
	}
	if _, err := k.Get(adres + "/"); err == nil {
		t.Fatal("80 слушается после выключения")
	}
	if _, err := st.klientHttps().Get("https://" + domen + "/"); err == nil {
		t.Fatal("https слушается после выключения")
	}
}

// Пока выпуск переходит на другое имя или ещё не выпустил, сайт отвечает
// последним годным сертификатом, а без него честно не слушает имя.
func TestSertifikatPokaVypuskMenyaetImya(t *testing.T) {
	st := novyyStend(t)
	godnyy := st.vypusk.Tekushchiy()
	st.sayt.Primenit(st.nastroyka())
	if !st.sayt.Sostoyanie().Sert {
		t.Fatal("сертификат выпуска не виден")
	}
	chuzhoy, _ := samopodpisannyy(t, "drugoy.example.net")
	st.vypusk.postavit(chuzhoy)
	if c, err := st.sayt.sertifikat(domen); err != nil || c != godnyy {
		t.Fatalf("при сертификате на другое имя: %v", err)
	}
	st.vypusk.postavit(nil)
	if c, err := st.sayt.sertifikat(domen); err != nil || c != godnyy {
		t.Fatalf("без сертификата у выпуска: %v", err)
	}

	pustoy := novyyStend(t)
	pustoy.vypusk.postavit(nil)
	pustoy.sayt.Primenit(pustoy.nastroyka())
	if s := pustoy.sayt.Sostoyanie(); s.Sert || !s.Https {
		t.Fatalf("без сертификата: %+v", s)
	}
}

func TestZanyatyyPort(t *testing.T) {
	st := novyyStend(t)
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(st.portHttps))
	if err != nil {
		t.Fatal(err)
	}
	st.sayt.Primenit(st.nastroyka())
	if s := st.sayt.Sostoyanie(); s.Https || !strings.Contains(s.Oshibka, "не слушает") {
		t.Fatalf("при занятом порте: %+v", s)
	}
	l.Close()
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	go st.sayt.Podderzhivat(ctx, 10*time.Millisecond)
	srok := time.Now().Add(5 * time.Second)
	for !st.sayt.Sostoyanie().Https {
		if time.Now().After(srok) {
			t.Fatal("освободившийся порт не занят повтором")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s := st.sayt.Sostoyanie(); s.Oshibka != "" {
		t.Fatalf("ошибка осталась: %q", s.Oshibka)
	}
}

func TestProverit(t *testing.T) {
	godnaya := Nastroyka{Domen: domen, Port: 443, Slushat: "127.0.0.1:10443", Zerno: put,
		Podpiska: Podpiska{Put: put, Zagolovki: map[string]string{"profile-title": "base64:0JzQvtC5"}}}
	if err := godnaya.Proverit(); err != nil {
		t.Fatal(err)
	}
	plohie := map[string]func(*Nastroyka){
		"без домена":           func(n *Nastroyka) { n.Domen = "" },
		"порт 0":               func(n *Nastroyka) { n.Port = 0 },
		"не петля":             func(n *Nastroyka) { n.Slushat = "0.0.0.0:10443" },
		"имя вместо адреса":    func(n *Nastroyka) { n.Slushat = "localhost:10443" },
		"короткое зерно":       func(n *Nastroyka) { n.Zerno = "abc" },
		"путь заглавными":      func(n *Nastroyka) { n.Podpiska.Put = strings.ToUpper(put) },
		"перевод строки":       func(n *Nastroyka) { n.Podpiska.Zagolovki = map[string]string{"x": "a\r\nSet-Cookie: b"} },
		"двоеточие в имени":    func(n *Nastroyka) { n.Podpiska.Zagolovki = map[string]string{"x:y": "a"} },
		"пустое имя заголовка": func(n *Nastroyka) { n.Podpiska.Zagolovki = map[string]string{"": "a"} },
	}
	for imya, isportit := range plohie {
		n := godnaya
		n.Podpiska.Zagolovki = map[string]string{}
		isportit(&n)
		if err := n.Proverit(); err == nil {
			t.Errorf("%s: принято", imya)
		}
	}
}

func samopodpisannyy(t *testing.T, imya string) (*tls.Certificate, *x509.CertPool) {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), DNSNames: []string{imya},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	koren := x509.NewCertPool()
	koren.AddCert(leaf)
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: klyuch, Leaf: leaf}, koren
}

func svobodnyyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
