package obhoddns

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// otvetDNS собирает ответ сервиса: rcode, затем записи по порядку.
func otvetDNS(t *testing.T, rcode dnsmessage.RCode, zapisi ...func(*dnsmessage.Builder) error) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, RecursionAvailable: true, RCode: rcode})
	if err := b.StartQuestions(); err != nil {
		t.Fatal(err)
	}
	if err := b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName("vpn.example.net."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatal(err)
	}
	if err := b.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	for _, z := range zapisi {
		if err := z(&b); err != nil {
			t.Fatal(err)
		}
	}
	m, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func zapisA(imya string, a [4]byte) func(*dnsmessage.Builder) error {
	return func(b *dnsmessage.Builder) error {
		return b.AResource(dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(imya), Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.AResource{A: a})
	}
}

func zapisCNAME(imya, cel string) func(*dnsmessage.Builder) error {
	return func(b *dnsmessage.Builder) error {
		return b.CNAMEResource(dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(imya), Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.CNAMEResource{CNAME: dnsmessage.MustNewName(cel)})
	}
}

func TestRazobratBeryotAdresaZaCNAME(t *testing.T) {
	telo := otvetDNS(t, dnsmessage.RCodeSuccess,
		zapisCNAME("vpn.example.net.", "edge.example.net."),
		zapisA("edge.example.net.", [4]byte{198, 51, 100, 7}),
		zapisA("edge.example.net.", [4]byte{198, 51, 100, 8}))
	a, err := razobrat(telo, "vpn.example.net")
	if err != nil {
		t.Fatal(err)
	}
	if want := []netip.Addr{netip.MustParseAddr("198.51.100.7"), netip.MustParseAddr("198.51.100.8")}; !slices.Equal(a, want) {
		t.Fatalf("адреса %v, ждали %v", a, want)
	}
}

func TestRazobratNazyvaetOtsutstvieImeni(t *testing.T) {
	if _, err := razobrat(otvetDNS(t, dnsmessage.RCodeNameError), "vpn.example.net"); !errors.Is(err, ErrNetImeni) {
		t.Fatalf("NXDOMAIN дал %v, ждали ErrNetImeni", err)
	}
	if _, err := razobrat(otvetDNS(t, dnsmessage.RCodeServerFailure), "vpn.example.net"); err == nil || errors.Is(err, ErrNetImeni) {
		t.Fatalf("SERVFAIL дал %v, это отказ сервиса, а не отсутствие имени", err)
	}
	if _, err := razobrat(otvetDNS(t, dnsmessage.RCodeSuccess, zapisCNAME("vpn.example.net.", "edge.example.net.")), "vpn.example.net"); err == nil {
		t.Fatal("ответ без единой записи A принят как адрес")
	}
	if _, err := razobrat([]byte("<html>"), "vpn.example.net"); err == nil {
		t.Fatal("страница вместо сообщения DNS принята как ответ")
	}
}

// servis поднимает сервис DNS через HTTPS. Каждый запрос проверяется так, как
// его проверил бы настоящий: POST, тип тела, вопрос про нужное имя и тип A.
func servis(t *testing.T, kod int, otvet []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var zaprosov atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zaprosov.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/dns-message" {
			http.Error(w, "не тот запрос", http.StatusBadRequest)
			return
		}
		telo, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var p dnsmessage.Parser
		if _, err := p.Start(telo); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q, err := p.Question()
		if err != nil || q.Name.String() != "vpn.example.net." || q.Type != dnsmessage.TypeA {
			http.Error(w, "не тот вопрос", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		w.WriteHeader(kod)
		_, _ = w.Write(otvet)
	}))
	t.Cleanup(s.Close)
	return s, &zaprosov
}

// Закрытый у провайдера сервис не мешает ответу соседа.
func TestPervyyGodnyyOtvetPobezhdaet(t *testing.T) {
	plohoy, _ := servis(t, http.StatusBadGateway, nil)
	horoshiy, zaprosov := servis(t, http.StatusOK, otvetDNS(t, dnsmessage.RCodeSuccess, zapisA("vpn.example.net.", [4]byte{198, 51, 100, 7})))
	// Один клиент на оба: сертификаты httptest выписаны одним корнем.
	a, err := sprositVse(context.Background(), horoshiy.Client(), []string{plohoy.URL + "/dns-query", horoshiy.URL + "/dns-query"}, "vpn.example.net")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, []netip.Addr{netip.MustParseAddr("198.51.100.7")}) || zaprosov.Load() != 1 {
		t.Fatalf("адреса %v, запросов к годному %d", a, zaprosov.Load())
	}
}

func TestOtsutstvieImeniVazhneeSboevSoseda(t *testing.T) {
	plohoy, _ := servis(t, http.StatusBadGateway, nil)
	net1, _ := servis(t, http.StatusOK, otvetDNS(t, dnsmessage.RCodeNameError))
	_, err := sprositVse(context.Background(), net1.Client(), []string{plohoy.URL, net1.URL}, "vpn.example.net")
	if !errors.Is(err, ErrNetImeni) {
		t.Fatalf("ошибка %v, ждали «такого имени нет»", err)
	}
	_, err = sprositVse(context.Background(), plohoy.Client(), []string{plohoy.URL}, "vpn.example.net")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("ошибка %v, ждали код ответа сервиса", err)
	}
}

// Свой путь получает все соединения обхода, ко всем адресам сервисов.
func TestSprositCherezIdyotSvoimPutyom(t *testing.T) {
	var (
		mu     sync.Mutex
		kuda   []string
		zakryt = errors.New("путь закрыт")
	)
	dial := func(_ context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		kuda = append(kuda, network+" "+addr)
		return nil, zakryt
	}
	if _, err := SprositCherez(context.Background(), dial, "vpn.example.net"); !errors.Is(err, zakryt) {
		t.Fatalf("ошибка %v, ждали отказ своего пути", err)
	}
	var zhdali []string
	for _, a := range Adresa {
		zhdali = append(zhdali, "tcp "+strings.TrimSuffix(strings.TrimPrefix(a, "https://"), "/dns-query")+":443")
	}
	slices.Sort(kuda)
	slices.Sort(zhdali)
	if !slices.Equal(kuda, zhdali) {
		t.Fatalf("соединения %q, ждали %q", kuda, zhdali)
	}
}

// zakrytyyDNS это резолвер, у которого DNS не отвечает ничем: так выглядит
// провайдер, не отдающий имя. Ошибка поиска имени у net приходит *DNSError.
func zakrytyyDNS() *net.Dialer {
	return &net.Dialer{Resolver: &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("DNS провайдера закрыт")
	}}}
}

func TestNabratIdyotPoAdresuObhoda(t *testing.T) {
	slushatel, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer slushatel.Close()
	_, port, _ := net.SplitHostPort(slushatel.Addr().String())
	var sprosheno string
	obhod := func(_ context.Context, host string) ([]netip.Addr, error) {
		sprosheno = host
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	conn, err := nabrat(context.Background(), zakrytyyDNS(), obhod, "tcp", net.JoinHostPort("vpn.example.net", port))
	if err != nil {
		t.Fatalf("соединение по адресу обхода не состоялось: %v", err)
	}
	conn.Close()
	if sprosheno != "vpn.example.net" {
		t.Fatalf("обход спросил %q", sprosheno)
	}
}

// Обход не помог: наружу уходит исходная ошибка поиска имени, по ней
// классификатор сбоев называет шаг DNS.
func TestNabratBezObhodaOtdayotOshibkuDNS(t *testing.T) {
	obhod := func(context.Context, string) ([]netip.Addr, error) { return nil, ErrNetImeni }
	_, err := nabrat(context.Background(), zakrytyyDNS(), obhod, "tcp", "vpn.example.net:443")
	var oshibkaDNS *net.DNSError
	if !errors.As(err, &oshibkaDNS) {
		t.Fatalf("ошибка %v потеряла *net.DNSError", err)
	}
}

// Отказ, не связанный с именем, обход не трогает: адрес и так известен.
func TestNabratNeZovyotObhodBezOshibkiDNS(t *testing.T) {
	slushatel, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	adres := slushatel.Addr().String()
	slushatel.Close()
	zvali := false
	obhod := func(context.Context, string) ([]netip.Addr, error) { zvali = true; return nil, nil }
	if _, err := nabrat(context.Background(), &net.Dialer{}, obhod, "tcp", adres); err == nil {
		t.Fatal("соединение с закрытым портом прошло")
	}
	if zvali {
		t.Fatal("обход позван при отказе соединения, а не поиска имени")
	}
}
