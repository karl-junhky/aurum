package blok

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// boevye это значения пакета до подмены в TestMain: живой тест идёт на них.
var (
	boevye = struct {
		zarubezhnye, rossiyskie                    []string
		srokStuka, srokOtveta, pauzaStuka          time.Duration
		zhdatLovushku, srokStukaLovushki, zapasLov time.Duration
		srokPoiska, shagStuka                      time.Duration
	}{zarubezhnye, rossiyskie, srokStuka, srokOtveta, pauzaStuka, zhdatLovushku, srokStukaLovushki, zapasLovushki, srokPoiska, shagStuka}
	boevoyDoH = sprositDoH
)

func vernutBoevye() {
	zarubezhnye, rossiyskie = boevye.zarubezhnye, boevye.rossiyskie
	srokStuka, srokOtveta, pauzaStuka = boevye.srokStuka, boevye.srokOtveta, boevye.pauzaStuka
	zhdatLovushku, srokStukaLovushki, zapasLovushki = boevye.zhdatLovushku, boevye.srokStukaLovushki, boevye.zapasLov
	srokPoiska, sprositDoH, shagStuka = boevye.srokPoiska, boevoyDoH, boevye.shagStuka
}

// errDoH это публичный DNS в тестах: он закрыт, если тест не подставил свой.
var errDoH = errors.New("публичный DNS закрыт")

func TestMain(m *testing.M) {
	srokStuka = 500 * time.Millisecond
	srokOtveta = 500 * time.Millisecond
	srokPoiska = 500 * time.Millisecond
	sprositDoH = func(context.Context, func(context.Context, string, string) (net.Conn, error), string) ([]netip.Addr, error) {
		return nil, errDoH
	}
	pauzaStuka = 20 * time.Millisecond
	povtory = []time.Duration{150 * time.Millisecond, 400 * time.Millisecond}
	zhdatLovushku = 50 * time.Millisecond
	shagStuka = 200 * time.Millisecond
	srokStukaLovushki = 150 * time.Millisecond
	zapasLovushki = 2 * time.Second
	zarubezhnye = []string{"github.test:443", "google.test:443"}
	rossiyskie = []string{"ya.test:443", "vk.test:443"}
	os.Exit(m.Run())
}

const adresServera = "203.0.113.10"

// errTishina велит подставной сети молчать до срока соединения.
var errTishina = errors.New("тишина")

var (
	errOtkaz      = &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connectex", windows.WSAECONNREFUSED)}
	errSboy       = &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("нет буфера")}
	errNedostupen = &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connectex", windows.WSAEHOSTUNREACH)}
)

type popytka struct {
	network, adres string
	kogda          time.Time
}

// podstava это сеть человека: адрес назначения ведёт на локальный сокет или
// кончается заданной ошибкой; всё незнакомое молчит.
type podstava struct {
	mu      sync.Mutex
	kuda    map[string]string
	oshibki map[string]error
	popytki []popytka
}

func novayaSet() *podstava {
	return &podstava{kuda: map[string]string{}, oshibki: map[string]error{}}
}

func (p *podstava) dial(ctx context.Context, network, adres string) (net.Conn, error) {
	p.mu.Lock()
	p.popytki = append(p.popytki, popytka{network, adres, time.Now()})
	k, est := p.kuda[network+" "+adres]
	err := p.oshibki[network+" "+adres]
	p.mu.Unlock()
	switch {
	case est:
		var d net.Dialer
		return d.DialContext(ctx, network, k)
	case err != nil && err != errTishina:
		return nil, err
	}
	<-ctx.Done()
	return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
}

func (p *podstava) postavit(network, adres, kuda string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.kuda[network+" "+adres] = kuda
	delete(p.oshibki, network+" "+adres)
}

func (p *podstava) zapretit(network, adres string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.oshibki[network+" "+adres] = err
	delete(p.kuda, network+" "+adres)
}

// kServeru это попытки к адресу сервера по порядку.
func (p *podstava) kServeru() []popytka {
	p.mu.Lock()
	defer p.mu.Unlock()
	var k []popytka
	for _, x := range p.popytki {
		if strings.HasPrefix(x.adres, adresServera+":") {
			k = append(k, x)
		}
	}
	return k
}

// sertifikat выпускает самоподписанный сертификат на имена и кладёт его в
// корни.
func sertifikat(t *testing.T, imena ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: imena[0]}, DNSNames: imena,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, pool
}

// sluga слушает 127.0.0.1 и отдаёт каждое соединение obsluzhit.
func sluga(t *testing.T, obsluzhit func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				obsluzhit(c)
			}()
		}
	}()
	return l.Addr().String()
}

func tlsSluga(t *testing.T, sert tls.Certificate) string {
	return sluga(t, func(c net.Conn) {
		s := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{sert}})
		if s.Handshake() == nil {
			io.Copy(io.Discard, s)
		}
	})
}

func pisat(tekst string) func(net.Conn) {
	return func(c net.Conn) {
		io.WriteString(c, tekst)
		io.Copy(io.Discard, c)
	}
}

func molchat(c net.Conn) { io.Copy(io.Discard, c) }

// sbrosit читает приветствие и рвёт соединение.
func sbrosit(c net.Conn) {
	b := make([]byte, 512)
	c.Read(b)
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetLinger(0)
	}
}

// stend это живая своя сеть: зарубежные и российские цели отвечают, имена
// ищутся подставным поиском.
type stend struct {
	set   *podstava
	sert  tls.Certificate
	celi  map[string]string // имя -> адрес цели в подставной сети
	opcii Opcii
}

func novyyStend(t *testing.T) *stend {
	t.Helper()
	sert, pool := sertifikat(t, "github.test", "google.test", "ya.test", "vk.test", "maska.test")
	bylo, byloIskat := korni, iskat
	t.Cleanup(func() { korni, iskat = bylo, byloIskat })
	korni = pool
	s := &stend{set: novayaSet(), sert: sert, celi: map[string]string{}}
	adres := tlsSluga(t, sert)
	for i, imya := range []string{"github.test", "google.test", "ya.test", "vk.test"} {
		ip := fmt.Sprintf("198.51.100.%d", i+1)
		s.celi[imya] = ip + ":443"
		s.set.postavit("tcp", ip+":443", adres)
	}
	iskat = func(_ context.Context, network, imya string) ([]netip.Addr, error) {
		if network != "ip4" {
			t.Errorf("поиск %s", network)
		}
		a, est := s.celi[imya]
		if !est {
			return nil, &net.DNSError{Err: "no such host", Name: imya, IsNotFound: true}
		}
		return []netip.Addr{netip.MustParseAddrPort(a).Addr()}, nil
	}
	s.opcii = Opcii{Adres: netip.MustParseAddr(adresServera), PortSsh: 22, Pryamo: s.set.dial}
	return s
}

func (s *stend) port(n int) string { return fmt.Sprintf("%s:%d", adresServera, n) }

func TestVynesti(t *testing.T) {
	ssh := Stuk{Port: 22, Ishod: Otkryt, Otvet: OtvetEst}
	tls := func(o Otvet) Stuk { return Stuk{Port: 443, Ishod: Otkryt, Otvet: o} }
	molchit := Stuk{Port: 22, Ishod: Tishina}
	sbros := Stuk{Port: 443, Ishod: Otkaz}
	for _, c := range []struct {
		imya   string
		set    Set
		stuki  []Stuk
		tunnel Tunnel
		doshlo int64
		vid    Vid
	}{
		{"сеть мертва", SetMertva, nil, TunnelOtvetil, -1, NeSostoyalas},
		{"белые списки", SetBelyeSpiski, nil, TunnelOtvetil, -1, BelyeSpiski},
		{"все пробы своей стороной", SetZhivaya, []Stuk{{Ishod: Sboy}, {Ishod: Sboy}}, TunnelOtvetil, -1, NeSostoyalas},
		{"проб нет", SetZhivaya, nil, TunnelOtvetil, -1, NeSostoyalas},
		{"SSH отвечает", SetZhivaya, []Stuk{ssh, sbros}, TunnelNeProveren, -1, Dostupen},
		{"TLS глушат", SetZhivaya, []Stuk{ssh, tls(OtvetObryv)}, TunnelOtvetil, -1, Glushat},
		{"TLS молчит", SetZhivaya, []Stuk{molchit, tls(OtvetTishina)}, TunnelOtvetil, -1, Glushat},
		{"чужой ответ", SetZhivaya, []Stuk{molchit, tls(OtvetChuzhoy)}, TunnelOtvetil, -1, Podmena},
		{"TCP без вопроса", SetZhivaya, []Stuk{molchit, {Port: 80, Ishod: Otkryt}}, TunnelOtvetil, -1, Dostupen},
		{"SSH молчит после соединения", SetZhivaya, []Stuk{{Port: 22, Ishod: Otkryt}}, TunnelOtvetil, -1, Dostupen},
		{"стук дошёл", SetZhivaya, []Stuk{molchit, sbros}, TunnelOtvetil, 2, OtvetNeVozvrashchaetsya},
		{"не доходит", SetZhivaya, []Stuk{molchit, sbros}, TunnelOtvetil, 0, NeDohodit},
		{"не доходит без ловушки", SetZhivaya, []Stuk{molchit}, TunnelOtvetil, -1, NeDohodit},
		{"молчит везде", SetZhivaya, []Stuk{molchit}, TunnelMolchit, -1, ServerMolchit},
		{"туннеля нет", SetZhivaya, []Stuk{molchit, {Ishod: Sboy}}, TunnelNeProveren, -1, MolchitNapryamuyu},
		// Ловушку спрашивают через туннель: её ответ доказывает, что сервер жив.
		{"ловушка ответила без входа", SetZhivaya, []Stuk{molchit}, TunnelNeProveren, 0, NeDohodit},
		{"ловушка ответила при молчащем входе", SetZhivaya, []Stuk{molchit}, TunnelMolchit, 0, NeDohodit},
		{"путь отверг адрес", SetZhivaya, []Stuk{{Port: 22, Ishod: Nedostupen}}, TunnelOtvetil, -1, NeDohodit},
	} {
		t.Run(c.imya, func(t *testing.T) {
			if v := vynesti(c.set, c.stuki, c.tunnel, c.doshlo); v != c.vid {
				t.Fatalf("вердикт %s, ждала %s", v, c.vid)
			}
		})
	}
}

func TestStuk(t *testing.T) {
	st := novyyStend(t)
	chuzhoySert, _ := sertifikat(t, "maska.test")
	trevoga := sluga(t, func(c net.Conn) {
		b := make([]byte, 512)
		c.Read(b)
		// Запись TLS: тревога, фатальная, handshake_failure.
		c.Write([]byte{21, 3, 3, 0, 2, 2, 40})
	})
	for _, c := range []struct {
		imya  string
		kuda  string
		err   error
		p     PortTCP
		ssh   bool
		ishod Ishod
		otvet Otvet
	}{
		{"SSH", sluga(t, pisat("SSH-2.0-OpenSSH_9.6\r\n")), nil, PortTCP{Port: 22}, true, Otkryt, OtvetEst},
		{"не SSH", sluga(t, pisat("HTTP/1.1 400 Bad Request\r\n")), nil, PortTCP{Port: 22}, true, Otkryt, OtvetChuzhoy},
		{"SSH молчит", sluga(t, molchat), nil, PortTCP{Port: 22}, true, Otkryt, ""},
		{"TLS", tlsSluga(t, st.sert), nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetEst},
		{"тревога TLS", trevoga, nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetEst},
		{"чужой сертификат", tlsSluga(t, chuzhoySert), nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetChuzhoy},
		{"не TLS", sluga(t, pisat("HTTP/1.1 400 Bad Request\r\n\r\n")), nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetChuzhoy},
		{"обрыв после приветствия", sluga(t, sbrosit), nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetObryv},
		{"тишина после приветствия", sluga(t, molchat), nil, PortTCP{Port: 443, TLS: "maska.test"}, false, Otkryt, OtvetTishina},
		{"только TCP", sluga(t, molchat), nil, PortTCP{Port: 80}, false, Otkryt, ""},
		{"сброс", "", errOtkaz, PortTCP{Port: 443}, false, Otkaz, ""},
		{"тишина", "", errTishina, PortTCP{Port: 443}, false, Tishina, ""},
		{"недоступен", "", errNedostupen, PortTCP{Port: 443}, false, Nedostupen, ""},
		{"своя сторона", "", errSboy, PortTCP{Port: 443}, false, Sboy, ""},
	} {
		t.Run(c.imya, func(t *testing.T) {
			if c.kuda != "" {
				st.set.postavit("tcp", st.port(c.p.Port), c.kuda)
			} else {
				st.set.zapretit("tcp", st.port(c.p.Port), c.err)
			}
			s := stuk(context.Background(), st.opcii, c.p, c.ssh)
			if s.Port != c.p.Port || s.Ishod != c.ishod || s.Otvet != c.otvet {
				t.Fatalf("стук %+v", s)
			}
		})
	}
}

func TestSet(t *testing.T) {
	st := novyyStend(t)
	if s := proveritSet(context.Background(), st.opcii); s != SetZhivaya {
		t.Fatalf("сеть %s", s)
	}
	// Зарубежные молчат, а одна отвечает подменённым сертификатом: это не
	// ответ.
	chuzhoy, _ := sertifikat(t, "github.test")
	st.set.postavit("tcp", st.celi["github.test"], tlsSluga(t, chuzhoy))
	st.set.zapretit("tcp", st.celi["google.test"], errTishina)
	if s := proveritSet(context.Background(), st.opcii); s != SetBelyeSpiski {
		t.Fatalf("сеть %s", s)
	}
	st.set.zapretit("tcp", st.celi["ya.test"], errOtkaz)
	st.set.zapretit("tcp", st.celi["vk.test"], errTishina)
	if s := proveritSet(context.Background(), st.opcii); s != SetMertva {
		t.Fatalf("сеть %s", s)
	}
}

// Система не нашла имя (туннель мёртв): его даёт публичный DNS, и спрашивают
// его тем же прямым путём, мимо туннеля.
func TestSetCherezDoH(t *testing.T) {
	st := novyyStend(t)
	// Способы зовутся по очереди в одной горутине.
	var poryadok []string
	iskat = func(_ context.Context, _, imya string) ([]netip.Addr, error) {
		poryadok = append(poryadok, "система "+imya)
		return nil, &net.DNSError{Err: "i/o timeout", Name: imya, IsTimeout: true}
	}
	bylo := sprositDoH
	t.Cleanup(func() { sprositDoH = bylo })
	st.set.postavit("tcp", "192.0.2.53:443", sluga(t, molchat))
	sprositDoH = func(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error), imya string) ([]netip.Addr, error) {
		poryadok = append(poryadok, "doh "+imya)
		c, err := dial(ctx, "tcp", "192.0.2.53:443")
		if err != nil {
			return nil, err
		}
		c.Close()
		return []netip.Addr{netip.MustParseAddrPort(st.celi[imya]).Addr()}, nil
	}
	if s := proveritSet(context.Background(), st.opcii); s != SetZhivaya {
		t.Fatalf("сеть %s", s)
	}
	if want := []string{"система github.test", "doh github.test"}; !slices.Equal(poryadok, want) {
		t.Fatalf("порядок %q, ждала %q", poryadok, want)
	}
	// Пустой ответ это не адрес: следующий способ.
	poryadok = nil
	iskat = func(_ context.Context, _, imya string) ([]netip.Addr, error) {
		poryadok = append(poryadok, "система "+imya)
		return nil, nil
	}
	if s := proveritSet(context.Background(), st.opcii); s != SetZhivaya || len(poryadok) != 2 {
		t.Fatalf("сеть %s, порядок %q", s, poryadok)
	}
}

// Ни система, ни публичный DNS имени не дали: сеть не судится.
func TestSetBezImyon(t *testing.T) {
	st := novyyStend(t)
	iskat = func(_ context.Context, _, imya string) ([]netip.Addr, error) {
		return nil, &net.DNSError{Err: "i/o timeout", Name: imya, IsTimeout: true}
	}
	if s := proveritSet(context.Background(), st.opcii); s != SetMertva {
		t.Fatalf("сеть %s", s)
	}
	if _, err := najti(context.Background(), st.opcii, "github.test"); !errors.Is(err, errDoH) {
		t.Fatalf("ошибка %v без отказа публичного DNS", err)
	}
}

// lovushka это ловушка программы сервера: считает попытки к порту, пока
// слушает, по подставной сети.
func lovushka(set *podstava, okno time.Duration, oshibka error) func(context.Context, string, int, int) (int64, error) {
	return func(ctx context.Context, proto string, port, _ int) (int64, error) {
		nachalo := time.Now()
		if err := podozhdat(ctx, okno); err != nil {
			return 0, err
		}
		if oshibka != nil {
			return 0, oshibka
		}
		var n int64
		for _, x := range set.stuki(proto, port) {
			if x.kogda.After(nachalo) {
				n++
			}
		}
		return n, nil
	}
}

// stuki это попытки к порту сервера по порядку.
func (p *podstava) stuki(proto string, port int) []popytka {
	var s []popytka
	for _, x := range p.kServeru() {
		if x.network == proto && x.adres == fmt.Sprintf("%s:%d", adresServera, port) {
			s = append(s, x)
		}
	}
	return s
}

func TestProveritDostupen(t *testing.T) {
	st := novyyStend(t)
	st.set.postavit("tcp", st.port(22), sluga(t, pisat("SSH-2.0-OpenSSH_9.6\r\n")))
	st.set.postavit("tcp", st.port(443), tlsSluga(t, st.sert))
	st.set.zapretit("tcp", st.port(80), errOtkaz)
	o := st.opcii
	o.TCP = []PortTCP{{Port: 80}, {Port: 443, TLS: "maska.test"}}
	var krugi []Krug
	o.Hod = func(k Krug) { krugi = append(krugi, k) }
	o.Lovushka = func(context.Context, string, int, int) (int64, error) {
		t.Error("ловушка, хотя сервер отвечает")
		return 0, nil
	}
	itog, err := Proverit(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if itog.Vid != Dostupen || itog.Krugov != 1 || len(itog.Stuki) != 3 || itog.Doshlo != -1 || len(krugi) != 1 || !krugi[0].Posledniy {
		t.Fatalf("итог %+v, круги %+v", itog, krugi)
	}
	// К своему серверу по одному и не чаще паузы.
	k := st.set.kServeru()
	for i := 1; i < len(k); i++ {
		if d := k[i].kogda.Sub(k[i-1].kogda); d < pauzaStuka {
			t.Errorf("между %s и %s всего %s", k[i-1].adres, k[i].adres, d)
		}
	}
}

// Отрицательный вердикт только после повторов; ловушка говорит, дошёл ли стук.
func TestProveritPovtory(t *testing.T) {
	const okno = time.Second
	for _, c := range []struct {
		imya   string
		lov    func(*podstava) func(context.Context, string, int, int) (int64, error)
		tunnel Tunnel
		vid    Vid
		tcp    int64
		udp    int64
	}{
		// Ловушка считает настоящие стуки подставной сети: все по расписанию.
		{"стук доходит", func(s *podstava) func(context.Context, string, int, int) (int64, error) {
			return lovushka(s, okno, nil)
		}, TunnelOtvetil, OtvetNeVozvrashchaetsya, int64(stukovLovushki), int64(stukovLovushki)},
		{"режут по дороге", func(*podstava) func(context.Context, string, int, int) (int64, error) {
			return func(ctx context.Context, _ string, _, _ int) (int64, error) { return 0, podozhdat(ctx, okno) }
		}, TunnelOtvetil, NeDohodit, 0, 0},
		{"ловушка отказала", func(s *podstava) func(context.Context, string, int, int) (int64, error) {
			return lovushka(s, okno, errors.New("программа сервера не отвечает"))
		}, TunnelMolchit, ServerMolchit, -1, -1},
	} {
		t.Run(c.imya, func(t *testing.T) {
			st := novyyStend(t)
			o := st.opcii
			o.TCP = []PortTCP{{Port: 443, TLS: "maska.test"}}
			o.UDP = []int{8443}
			o.Tunnel = c.tunnel
			o.Lovushka = c.lov(st.set)
			var krugi []Krug
			o.Hod = func(k Krug) { krugi = append(krugi, k) }
			nachalo := time.Now()
			itog, err := Proverit(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			if itog.Vid != c.vid || itog.Krugov != 3 || itog.Doshlo != c.tcp || itog.DoshloUDP != c.udp {
				t.Fatalf("итог %+v", itog)
			}
			// Ни стук ловушки, ни её порт не трогают порт hy2, и к серверу
			// не чаще паузы.
			k := st.set.kServeru()
			for i, x := range k {
				if x.network == "udp" && x.adres == st.port(8443) {
					t.Fatalf("стук UDP в порт hy2: %+v", x)
				}
				if i > 0 && x.kogda.Sub(k[i-1].kogda) < pauzaStuka {
					t.Errorf("между %s и %s всего %s", k[i-1].adres, x.adres, x.kogda.Sub(k[i-1].kogda))
				}
			}
			if (c.tcp < 0) != (itog.OtkazLovushki != "") {
				t.Fatalf("отказ ловушки %q", itog.OtkazLovushki)
			}
			if len(krugi) != 3 || krugi[0].Posledniy || krugi[0].Sleduyushchiy <= 0 || krugi[1].Posledniy ||
				!krugi[2].Posledniy || krugi[2].Sleduyushchiy != 0 {
				t.Fatalf("круги %+v", krugi)
			}
			// Время оставшихся кругов: после первого оба повтора, после второго
			// последний, тот же, что назван после первого.
			if len(krugi[0].Povtory) != 2 || len(krugi[1].Povtory) != 1 || len(krugi[2].Povtory) != 0 ||
				!krugi[1].Povtory[0].Equal(krugi[0].Povtory[1]) || !krugi[0].Povtory[0].Before(krugi[0].Povtory[1]) {
				t.Fatalf("время повторов %+v", krugi)
			}
			if time.Since(nachalo) < povtory[1] {
				t.Fatalf("три круга за %s", time.Since(nachalo))
			}
		})
	}
}

// Ловушка встала после второго стука: стук по расписанию идёт через всё
// окно, и остальные до неё доходят.
func TestLovitPozdnyayaLovushka(t *testing.T) {
	st := novyyStend(t)
	o := st.opcii
	o.Lovushka = func(ctx context.Context, proto string, port, _ int) (int64, error) {
		for len(st.set.stuki(proto, port)) < 2 {
			if err := podozhdat(ctx, 5*time.Millisecond); err != nil {
				return 0, err
			}
		}
		if err := podozhdat(ctx, time.Second); err != nil {
			return 0, err
		}
		return int64(len(st.set.stuki(proto, port)) - 2), nil
	}
	n, otkaz := lovit(context.Background(), o, "tcp", 27123)
	if otkaz != "" || n != int64(stukovLovushki-2) {
		t.Fatalf("дошло %d, отказ %q", n, otkaz)
	}
}

// Ловушка отказала сразу: стук прекращается, к серверу не идёт ничего.
func TestLovitOtkazLovushkiBezStuka(t *testing.T) {
	// Первый стук дальше, чем нужно горутине ловушки на отказ.
	bylo := zhdatLovushku
	zhdatLovushku = 2 * time.Second
	t.Cleanup(func() { zhdatLovushku = bylo })
	st := novyyStend(t)
	o := st.opcii
	o.Lovushka = func(context.Context, string, int, int) (int64, error) {
		return 0, errors.New("программы сервера нет")
	}
	n, otkaz := lovit(context.Background(), o, "tcp", 27123)
	if n != -1 || !strings.Contains(otkaz, "программы сервера нет") || len(st.set.kServeru()) != 0 {
		t.Fatalf("дошло %d, отказ %q, к серверу %d", n, otkaz, len(st.set.kServeru()))
	}
}

// Стук не ушёл с компьютера или ловушка ответила раньше него: её ноль
// ничего не значит.
func TestLovitNolBezStuka(t *testing.T) {
	for _, c := range []struct {
		imya  string
		err   error
		okno  time.Duration
		tekst string
	}{
		{"стук не ушёл", errSboy, time.Second, "не ушёл с компьютера"},
		{"ответ раньше стука", errTishina, 0, "раньше стука"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			st := novyyStend(t)
			st.set.zapretit("tcp", st.port(27123), c.err)
			o := st.opcii
			o.Lovushka = func(ctx context.Context, _ string, _, _ int) (int64, error) { return 0, podozhdat(ctx, c.okno) }
			if n, otkaz := lovit(context.Background(), o, "tcp", 27123); n != -1 || !strings.Contains(otkaz, c.tekst) {
				t.Fatalf("дошло %d, отказ %q", n, otkaz)
			}
		})
	}
	// Тишина и сброс приходят с той стороны: стук ушёл, ноль это ответ.
	st := novyyStend(t)
	st.set.zapretit("tcp", st.port(27123), errOtkaz)
	o := st.opcii
	o.Lovushka = func(ctx context.Context, _ string, _, _ int) (int64, error) { return 0, podozhdat(ctx, time.Second) }
	if n, otkaz := lovit(context.Background(), o, "tcp", 27123); n != 0 || otkaz != "" {
		t.Fatalf("дошло %d, отказ %q", n, otkaz)
	}
}

func TestSluchaynyyPort(t *testing.T) {
	for range 1000 {
		if p := sluchaynyyPort([]int{22, 443}); p < 20000 || p > 32767 {
			t.Fatalf("порт %d в эфемерном диапазоне Linux или ниже", p)
		}
	}
}

// Ответил на повторе: закрывали ненадолго.
func TestProveritVremenno(t *testing.T) {
	st := novyyStend(t)
	o := st.opcii
	o.Tunnel = TunnelOtvetil
	ssh := sluga(t, pisat("SSH-2.0-OpenSSH_9.6\r\n"))
	o.Hod = func(k Krug) {
		if k.Nomer == 1 {
			st.set.postavit("tcp", st.port(22), ssh)
		}
	}
	itog, err := Proverit(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if itog.Vid != Dostupen || itog.Krugov != 2 {
		t.Fatalf("итог %+v", itog)
	}
}

// Своя сеть мертва или в белых списках: к серверу не стучат и не повторяют.
func TestProveritSetNeTa(t *testing.T) {
	for _, c := range []struct {
		imya     string
		rossiya  bool
		vid      Vid
		zhduSeti Set
	}{{"мертва", false, NeSostoyalas, SetMertva}, {"белые списки", true, BelyeSpiski, SetBelyeSpiski}} {
		t.Run(c.imya, func(t *testing.T) {
			st := novyyStend(t)
			for imya, a := range st.celi {
				if !c.rossiya || imya == "github.test" || imya == "google.test" {
					st.set.zapretit("tcp", a, errTishina)
				}
			}
			itog, err := Proverit(context.Background(), st.opcii)
			if err != nil {
				t.Fatal(err)
			}
			if itog.Vid != c.vid || itog.Set != c.zhduSeti || itog.Krugov != 1 || len(st.set.kServeru()) != 0 {
				t.Fatalf("итог %+v, к серверу %d", itog, len(st.set.kServeru()))
			}
		})
	}
}

func TestProveritOtmena(t *testing.T) {
	st := novyyStend(t)
	ctx, otm := context.WithCancel(context.Background())
	o := st.opcii
	o.Hod = func(Krug) { otm() }
	if _, err := Proverit(ctx, o); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestProveritOtkazy(t *testing.T) {
	st := novyyStend(t)
	for _, c := range []struct {
		imya string
		o    func(Opcii) Opcii
	}{
		{"без адреса", func(o Opcii) Opcii { o.Adres = netip.Addr{}; return o }},
		{"без порта SSH", func(o Opcii) Opcii { o.PortSsh = 0; return o }},
		{"без пути", func(o Opcii) Opcii { o.Pryamo = nil; return o }},
		{"порт TCP", func(o Opcii) Opcii { o.TCP = []PortTCP{{Port: 70000}}; return o }},
		{"порт UDP", func(o Opcii) Opcii { o.UDP = []int{0}; return o }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			if _, err := Proverit(context.Background(), c.o(st.opcii)); err == nil {
				t.Fatal("опции приняты")
			}
		})
	}
	if len(st.set.popytki) != 0 {
		t.Fatalf("попытки %d", len(st.set.popytki))
	}
}
