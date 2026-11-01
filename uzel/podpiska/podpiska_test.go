package podpiska

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var (
	tokenAnny   = strings.Repeat("a1", 16)
	tokenBorisa = strings.Repeat("b2", 16)
)

func tihiy() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func sertDlya(t *testing.T, imya string, do time.Time) *Sert {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: imya},
		DNSNames:     []string{imya},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     do,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	klyuchDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		t.Fatal(err)
	}
	return &Sert{
		Cert:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		Klyuch: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: klyuchDer})),
	}
}

func nabor(slushat string) *Nabor {
	return &Nabor{Slushat: slushat, Lyudi: map[string]Otvet{
		tokenAnny: {Imya: "emp-a1", Telo: "c3NzOi8v", Zagolovki: map[string]string{
			"content-type":          "text/plain; charset=utf-8",
			"subscription-userinfo": "upload=1; download=2; total=0",
		}},
		// Отозванный: заголовки с причиной, тело пустое.
		tokenBorisa: {Imya: "emp-b2", Zagolovki: map[string]string{"announce": "base64:0J7RgtC+0LfQstCw0L0="}},
	}}
}

func postavlen(t *testing.T, n *Nabor) *Server {
	t.Helper()
	s := Novyy(tihiy())
	sert, err := Proverit(n, sertDlya(t, "hi.example.net", time.Now().Add(24*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	s.tek.Store(&tekushchee{nabor: n, sert: sert})
	return s
}

func zapros(s *Server, metod, put string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(metod, put, nil)
	r.RemoteAddr = "198.51.100.7:40000"
	s.ServeHTTP(w, r)
	return w
}

func TestOtdacha(t *testing.T) {
	s := postavlen(t, nabor(":0"))
	s.chasy = func() time.Time { return time.Unix(1760500000, 0) }

	w := zapros(s, http.MethodGet, "/sub/"+tokenAnny)
	if w.Code != http.StatusOK || w.Body.String() != "c3NzOi8v" || w.Header().Get("subscription-userinfo") != "upload=1; download=2; total=0" {
		t.Fatalf("живой: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if z := s.Zaprosy(); z["emp-a1"] != 1760500000 {
		t.Fatalf("запрос живого не отмечен: %v", z)
	}

	w = zapros(s, http.MethodHead, "/sub/"+tokenAnny)
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("content-type") == "" {
		t.Fatalf("HEAD: %d %q", w.Code, w.Body.String())
	}

	w = zapros(s, http.MethodGet, "/sub/"+tokenBorisa)
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("announce") == "" {
		t.Fatalf("отозванный: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if _, est := s.Zaprosy()["emp-b2"]; est {
		t.Fatal("отозванному отмечен запрос")
	}

	// Чужой, кривой, не тот путь, не тот метод: всё одинаковый 404.
	for _, sl := range [][2]string{
		{http.MethodGet, "/sub/" + strings.Repeat("c", 32)},
		{http.MethodGet, "/sub/korotkiy"},
		{http.MethodGet, "/sub/" + strings.ToUpper(tokenAnny)},
		{http.MethodGet, "/admin/spisok"},
		{http.MethodPost, "/sub/" + tokenAnny},
	} {
		if w := zapros(s, sl[0], sl[1]); w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d", sl[0], sl[1], w.Code)
		}
	}
}

func TestBezNaboraVse404(t *testing.T) {
	s := Novyy(tihiy())
	if w := zapros(s, http.MethodGet, "/sub/"+tokenAnny); w.Code != http.StatusNotFound {
		t.Fatalf("без набора: %d", w.Code)
	}
	if s.Sostoyanie() != nil {
		t.Fatal("без набора есть состояние")
	}
}

func TestLimit(t *testing.T) {
	s := postavlen(t, nabor(":0"))
	for i := 0; i < potolok; i++ {
		if w := zapros(s, http.MethodGet, "/sub/"+tokenAnny); w.Code != http.StatusOK {
			t.Fatalf("лимит сработал раньше времени, на %d-м: %d", i, w.Code)
		}
	}
	w := zapros(s, http.MethodGet, "/sub/"+strings.Repeat("c", 32))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("retry-after") == "" {
		t.Fatalf("перебор не упёрся: %d", w.Code)
	}
}

func TestProverit(t *testing.T) {
	sert := sertDlya(t, "hi.example.net", time.Now().Add(time.Hour))
	if _, err := Proverit(nil, nil); err != nil {
		t.Fatal(err)
	}
	if s, err := Proverit(nil, sert); err != nil || s == nil {
		t.Fatalf("сертификат без подписки: %v", err)
	}
	if _, err := Proverit(nabor(":8443"), nil); err == nil {
		t.Fatal("подписка без сертификата прошла")
	}
	if _, err := Proverit(nil, &Sert{Cert: sert.Cert, Klyuch: sertDlya(t, "x", time.Now().Add(time.Hour)).Klyuch}); err == nil {
		t.Fatal("чужой ключ прошёл")
	}
	plohie := map[string]func(*Nabor){
		"адрес без порта":    func(n *Nabor) { n.Slushat = "8443" },
		"кривой токен":       func(n *Nabor) { n.Lyudi["ZZ"] = Otvet{Imya: "emp-z"} },
		"кривое имя":         func(n *Nabor) { n.Lyudi[tokenAnny] = Otvet{Imya: "emp a1"} },
		"перевод в значении": func(n *Nabor) { n.Lyudi[tokenAnny].Zagolovki["x"] = "a\r\nset-cookie: b" },
		"перевод в имени":    func(n *Nabor) { n.Lyudi[tokenAnny].Zagolovki["x\ny"] = "a" },
		"пустое имя":         func(n *Nabor) { n.Lyudi[tokenAnny].Zagolovki[""] = "a" },
	}
	for chto, isportit := range plohie {
		n := nabor(":8443")
		isportit(n)
		if _, err := Proverit(n, sert); err == nil {
			t.Fatalf("%s: прошло", chto)
		}
	}
}

// Живой слушатель: подписка по TLS, смена набора и сертификата на лету без
// перезапуска, занятый порт не роняет и отпускается сам.
func TestSlushatel(t *testing.T) {
	zanyat, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	adres := zanyat.Addr().String()

	s := Novyy(tihiy())
	defer s.Ostanovit()
	sert1 := sertDlya(t, "hi.example.net", time.Now().Add(24*time.Hour))
	tls1, err := Proverit(nabor(adres), sert1)
	if err != nil {
		t.Fatal(err)
	}
	s.Primenit(nabor(adres), tls1)
	if s.Oshibka() == "" || s.Sostoyanie().Slushaet != "" {
		t.Fatalf("занятый порт не виден: %+v", s.Sostoyanie())
	}

	zanyat.Close()
	ctx, otmena := contextNaSrok(t)
	defer otmena()
	go s.Podderzhivat(ctx, 20*time.Millisecond)
	zhdat(t, func() bool { return s.Oshibka() == "" && s.Sostoyanie().Slushaet == adres })

	doverie := x509.NewCertPool()
	doverie.AppendCertsFromPEM([]byte(sert1.Cert))
	klient := func() (*http.Response, *x509.Certificate) {
		t.Helper()
		var uvidel *x509.Certificate
		tr := &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    doverie,
			ServerName: "hi.example.net",
			VerifyConnection: func(cs tls.ConnectionState) error {
				uvidel = cs.PeerCertificates[0]
				return nil
			},
		}}
		defer tr.CloseIdleConnections()
		otvet, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get("https://" + adres + "/sub/" + tokenAnny)
		if err != nil {
			t.Fatal(err)
		}
		return otvet, uvidel
	}
	otvet, uvidel := klient()
	telo, _ := io.ReadAll(otvet.Body)
	otvet.Body.Close()
	if otvet.StatusCode != http.StatusOK || string(telo) != "c3NzOi8v" || uvidel.Subject.CommonName != "hi.example.net" {
		t.Fatalf("по TLS: %d %q", otvet.StatusCode, telo)
	}

	// Новый сертификат и новый состав: слушатель тот же.
	srv := s.srv
	sert2 := sertDlya(t, "hi.example.net", time.Now().Add(48*time.Hour))
	doverie.AppendCertsFromPEM([]byte(sert2.Cert))
	tls2, err := Proverit(nabor(adres), sert2)
	if err != nil {
		t.Fatal(err)
	}
	n2 := nabor(adres)
	delete(n2.Lyudi, tokenAnny)
	s.Primenit(n2, tls2)
	if s.srv != srv {
		t.Fatal("смена набора перезапустила слушатель")
	}
	otvet, uvidel = klient()
	otvet.Body.Close()
	if otvet.StatusCode != http.StatusNotFound || !uvidel.NotAfter.Equal(tls2.Leaf.NotAfter) {
		t.Fatalf("после смены: %d, сертификат до %v", otvet.StatusCode, uvidel.NotAfter)
	}
	if st := s.Sostoyanie(); st.Lyudey != 1 || st.SertDo != tls2.Leaf.NotAfter.Unix() {
		t.Fatalf("состояние: %+v", st)
	}

	s.Primenit(nil, nil)
	if s.Sostoyanie() != nil {
		t.Fatal("после снятия подписки есть состояние")
	}
	if c, err := net.DialTimeout("tcp", adres, time.Second); err == nil {
		c.Close()
		t.Fatal("после снятия подписки порт слушается")
	}
}

func contextNaSrok(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func zhdat(t *testing.T, gotovo func() bool) {
	t.Helper()
	konec := time.Now().Add(5 * time.Second)
	for time.Now().Before(konec) {
		if gotovo() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("не дождалась")
}
