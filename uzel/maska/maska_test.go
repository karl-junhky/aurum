package maska

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

// stend это сеть соседей на петле: у каждого адреса свой TLS-сервер, свой
// сертификат от общего удостоверяющего центра и свои A-записи.
type stend struct {
	t      *testing.T
	uc     *x509.Certificate
	ucKl   *ecdsa.PrivateKey
	korni  *x509.CertPool
	adresa map[string]string // "a.b.c.d:443" -> адрес слушателя
	imena  map[string][]netip.Addr
}

func novyyStend(t *testing.T) *stend {
	t.Helper()
	kl, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "проба"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &kl.PublicKey, kl)
	if err != nil {
		t.Fatal(err)
	}
	uc, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	korni := x509.NewCertPool()
	korni.AddCert(uc)
	return &stend{t: t, uc: uc, ucKl: kl, korni: korni, adresa: map[string]string{}, imena: map[string][]netip.Addr{}}
}

type sosed struct {
	imena     []string
	protokoly []string
	tls12     bool
}

// postavit поднимает соседа на адресе a.
func (s *stend) postavit(a string, sd sosed) {
	s.t.Helper()
	kl, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		s.t.Fatal(err)
	}
	shablon := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), DNSNames: sd.imena,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, shablon, s.uc, &kl.PublicKey, s.ucKl)
	if err != nil {
		s.t.Fatal(err)
	}
	conf := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: kl}}, NextProtos: sd.protokoly}
	if sd.tls12 {
		conf.MaxVersion = tls.VersionTLS12
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", conf)
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.(*tls.Conn).Handshake()
				_, _ = c.Read(make([]byte, 1))
			}()
		}
	}()
	s.adresa[a+":443"] = ln.Addr().String()
}

func (s *stend) zapis(imya string, adresa ...string) {
	for _, a := range adresa {
		s.imena[imya] = append(s.imena[imya], netip.MustParseAddr(a))
	}
}

func (s *stend) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := s.imena[host]; ok {
		return append([]netip.Addr(nil), a...), nil
	}
	return nil, errors.New("no such host")
}

func (s *stend) opcii() Opcii {
	return Opcii{
		Shag: time.Millisecond, Srok: 10 * time.Second, Nuzhno: 1, Resolver: s, Korni: s.korni,
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if k, ok := s.adresa[addr]; ok {
				var d net.Dialer
				return d.DialContext(ctx, network, k)
			}
			return nil, errors.New("connection refused")
		},
		Zapasnye: []string{}, Prezhnie: []string{},
	}
}

func TestPodobratSoseda(t *testing.T) {
	s := novyyStend(t)
	s.postavit("198.51.100.7", sosed{imena: []string{"sosed.example.net"}, protokoly: []string{"h2", "http/1.1"}})
	s.zapis("sosed.example.net", "198.51.100.7")
	it, err := Podobrat(context.Background(), netip.MustParseAddr("198.51.100.20"), s.opcii())
	if err != nil {
		t.Fatal(err)
	}
	if it.Maska != "sosed.example.net" || it.Istochnik != Sosedi || it.Prosmotreno == 0 {
		t.Errorf("%+v", it)
	}
}

// Каждый негодный сосед отсеивается своей причиной.
func TestOprositSosedaOtseivaet(t *testing.T) {
	s := novyyStend(t)
	s.postavit("198.51.100.1", sosed{imena: []string{"chuzhoy.example.net"}, protokoly: []string{"h2"}})
	s.zapis("chuzhoy.example.net", "203.0.113.9") // CDN или чужой reality
	s.postavit("198.51.100.2", sosed{imena: []string{"bez-h2.example.net"}, protokoly: []string{"http/1.1"}})
	s.zapis("bez-h2.example.net", "198.51.100.2")
	s.postavit("198.51.100.3", sosed{imena: []string{"staryy.example.net"}, protokoly: []string{"h2"}, tls12: true})
	s.zapis("staryy.example.net", "198.51.100.3")
	s.postavit("198.51.100.4", sosed{imena: []string{"www.adobe.com"}, protokoly: []string{"h2"}})
	s.zapis("www.adobe.com", "198.51.100.4")
	s.postavit("198.51.100.5", sosed{imena: []string{"*.example.net"}, protokoly: []string{"h2"}})
	// Годный сосед: подстава сети работает, и отсев выше не случайный.
	s.postavit("198.51.100.9", sosed{imena: []string{"horoshiy.example.net"}, protokoly: []string{"h2"}})
	s.zapis("horoshiy.example.net", "198.51.100.9")
	sk := &skaner{o: s.opcii().sDefoltami(), tik: time.NewTicker(time.Millisecond)}
	defer sk.tik.Stop()
	ctx := context.Background()
	adr := netip.MustParseAddr

	if p := sk.oprositSoseda(ctx, adr("198.51.100.9")); p == nil || p.Imya != "horoshiy.example.net" {
		t.Fatalf("годный сосед: %+v", p)
	}
	for _, a := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3", "198.51.100.4", "198.51.100.5", "198.51.100.6"} {
		if p := sk.oprositSoseda(ctx, adr(a)); p != nil {
			t.Errorf("%s годен: %+v", a, p)
		}
	}
	// Имена из сертификата: TLS 1.2, известное имя, звёздочка и пустой
	// адрес не дают ни одного.
	if im := sk.imenaSoseda(ctx, adr("198.51.100.1")); !slices.Equal(im, []string{"chuzhoy.example.net"}) {
		t.Errorf("имена чужого: %q", im)
	}
	for _, a := range []string{"198.51.100.3", "198.51.100.4", "198.51.100.5", "198.51.100.6"} {
		if im := sk.imenaSoseda(ctx, adr(a)); len(im) != 0 {
			t.Errorf("%s назвал %q", a, im)
		}
	}
	// Имя есть, а отсеивает его своя причина.
	for imya, c := range map[string]struct {
		adres, prichina string
	}{
		"chuzhoy.example.net": {"198.51.100.1", "A-запись"},
		"bez-h2.example.net":  {"198.51.100.2", "h2"},
	} {
		if p := sk.proveritNaAdrese(ctx, imya, adr(c.adres)); p.Godna || !strings.Contains(p.Prichina, c.prichina) {
			t.Errorf("%s: %+v", imya, p)
		}
	}
}

// Первый адрес имени молчит, второй отвечает: имя годно.
func TestProveritVtoroyAdres(t *testing.T) {
	s := novyyStend(t)
	s.postavit("203.0.113.8", sosed{imena: []string{"dva.example.org"}, protokoly: []string{"h2"}})
	s.zapis("dva.example.org", "203.0.113.7", "203.0.113.8")
	if p := Proverit(context.Background(), "dva.example.org", s.opcii()); !p.Godna || p.Adres != netip.MustParseAddr("203.0.113.8") {
		t.Errorf("%+v", p)
	}
}

func TestPodobratZapasIPrezhnie(t *testing.T) {
	s := novyyStend(t)
	s.postavit("203.0.113.5", sosed{imena: []string{"zapas.example.org"}, protokoly: []string{"h2"}})
	s.zapis("zapas.example.org", "203.0.113.5")
	s.postavit("203.0.113.6", sosed{imena: []string{"dl.google.com"}, protokoly: []string{"h2"}})
	s.zapis("dl.google.com", "203.0.113.6")
	// Адрес IPv6: соседей по /24 нет, сразу запас.
	svoy := netip.MustParseAddr("2001:db8::1")

	o := s.opcii()
	o.Zapasnye, o.Prezhnie = []string{"net-takogo.example.org", "zapas.example.org"}, []string{"dl.google.com"}
	it, err := Podobrat(context.Background(), svoy, o)
	if err != nil || it.Maska != "zapas.example.org" || it.Istochnik != Zapas {
		t.Errorf("запас: %+v, %v", it, err)
	}

	o.Zapasnye = []string{"net-takogo.example.org"}
	it, err = Podobrat(context.Background(), svoy, o)
	if err != nil || it.Maska != "dl.google.com" || it.Istochnik != Brend {
		t.Errorf("прежние: %+v, %v", it, err)
	}

	o.Prezhnie = []string{}
	if _, err = Podobrat(context.Background(), svoy, o); !errors.Is(err, ErrNet) {
		t.Errorf("никого: %v", err)
	}
}

func TestPodobratOtmena(t *testing.T) {
	s := novyyStend(t)
	ctx, otmena := context.WithCancel(context.Background())
	otmena()
	if _, err := Podobrat(ctx, netip.MustParseAddr("198.51.100.20"), s.opcii()); !errors.Is(err, context.Canceled) {
		t.Errorf("отмена выдана за %v", err)
	}
}

func TestProverit(t *testing.T) {
	s := novyyStend(t)
	s.postavit("203.0.113.6", sosed{imena: []string{"www.adobe.com"}, protokoly: []string{"h2"}})
	s.zapis("www.adobe.com", "203.0.113.6")
	s.postavit("203.0.113.7", sosed{imena: []string{"svoy.example.org"}, protokoly: []string{"h2"}})
	s.zapis("svoy.example.org", "203.0.113.7")

	if p := Proverit(context.Background(), "WWW.Adobe.com.", s.opcii()); !p.Godna || !p.Brend || p.Imya != "www.adobe.com" {
		t.Errorf("известная: %+v", p)
	}
	if p := Proverit(context.Background(), "svoy.example.org", s.opcii()); !p.Godna || p.Brend || p.Adres != netip.MustParseAddr("203.0.113.7") {
		t.Errorf("своя: %+v", p)
	}
	for _, imya := range []string{"net-takogo.example.org", "под_чёркивание", ""} {
		if p := Proverit(context.Background(), imya, s.opcii()); p.Godna || p.Prichina == "" {
			t.Errorf("%q: %+v", imya, p)
		}
	}
}

func TestSetiIHosty(t *testing.T) {
	svoy := netip.MustParseAddr("198.51.0.20")
	s := seti(svoy)
	if len(s) != 2 || s[0] != netip.MustParsePrefix("198.51.0.0/24") || s[1] != netip.MustParsePrefix("198.51.1.0/24") {
		t.Errorf("сети %v", s)
	}
	h := hosty(s[0], svoy)
	if len(h) != 253 {
		t.Errorf("адресов %d", len(h))
	}
	for _, a := range h {
		if a == svoy || a.As4()[3] == 0 || a.As4()[3] == 255 {
			t.Errorf("лишний адрес %v", a)
		}
	}
}

// «Подобрать другую» не возвращает нынешнюю маску ни у соседей, ни из
// запаса.
func TestPodobratKrome(t *testing.T) {
	s := novyyStend(t)
	s.postavit("198.51.100.9", sosed{imena: []string{"horoshiy.example.net"}, protokoly: []string{"h2"}})
	s.zapis("horoshiy.example.net", "198.51.100.9")
	s.postavit("203.0.113.5", sosed{imena: []string{"zapas.example.org"}, protokoly: []string{"h2"}})
	s.zapis("zapas.example.org", "203.0.113.5")
	s.postavit("203.0.113.6", sosed{imena: []string{"dl.google.com"}, protokoly: []string{"h2"}})
	s.zapis("dl.google.com", "203.0.113.6")
	o := s.opcii()
	o.Krome = "Horoshiy.Example.net."
	sk := &skaner{o: o.sDefoltami(), tik: time.NewTicker(time.Millisecond)}
	defer sk.tik.Stop()
	if p := sk.oprositSoseda(context.Background(), netip.MustParseAddr("198.51.100.9")); p != nil {
		t.Errorf("сосед с нынешней маской годен: %+v", p)
	}

	o.Krome, o.Zapasnye, o.Prezhnie = "zapas.example.org", []string{"zapas.example.org"}, []string{"dl.google.com"}
	it, err := Podobrat(context.Background(), netip.MustParseAddr("2001:db8::1"), o)
	if err != nil || it.Maska != "dl.google.com" {
		t.Errorf("запас с нынешней маской: %+v, %v", it, err)
	}
}

// Причина для человека по-русски, исходная ошибка лежит отдельно.
func TestProveritPrichina(t *testing.T) {
	s := novyyStend(t)
	s.postavit("203.0.113.8", sosed{imena: []string{"drugoe.example.org"}, protokoly: []string{"h2"}})
	s.zapis("chuzhoy-sert.example.org", "203.0.113.8")
	p := Proverit(context.Background(), "chuzhoy-sert.example.org", s.opcii())
	if p.Godna || p.Prichina != "сайт не отвечает TLS 1.3 с X25519 или его сертификат недействителен" || p.Podrobno == "" {
		t.Errorf("%+v", p)
	}
	p = Proverit(context.Background(), "net-takogo.example.org", s.opcii())
	if p.Prichina != "сервер не находит адрес этого имени" || p.Podrobno == "" {
		t.Errorf("%+v", p)
	}
}

func TestEtoBrend(t *testing.T) {
	for imya, brend := range map[string]bool{
		"www.adobe.com": true, "dl.google.com": true, "ya.ru": true, "YA.RU.": true,
		"notadobe.com": false, "suckless.org": false, "google.com.example.net": false,
	} {
		if EtoBrend(imya) != brend {
			t.Errorf("%s: %v", imya, !brend)
		}
	}
}
