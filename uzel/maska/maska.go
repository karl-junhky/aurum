// Package maska подбирает и проверяет маску reality: настоящий сайт, под
// который сервер прячется от чужого приветствия.
//
// Опыт 05.11 на мобильном Билайне: приветствие с известным именем
// (www.adobe.com, ya.ru, dl.google.com) глушат на чужом адресе, а
// малоизвестное проходит. Поэтому маска берётся у соседей сервера по сети и
// своя у каждого сервера (МА1 плана 1.12.0). Сосед годен, если отвечает TLS
// 1.3, h2 и X25519, его сертификат действителен для имени из него, A-запись
// имени ведёт на этот же адрес (так отсеиваются чужие reality и CDN) и имени
// нет среди известных.
package maska

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Istochnik говорит, откуда маска.
type Istochnik string

const (
	// Sosedi: сайт из сети сервера, его /24 или соседних.
	Sosedi Istochnik = "sosedi"
	// Zapas: из общего запасного списка, у других серверов может быть та же.
	Zapas Istochnik = "zapas"
	// Brend: известное имя, на мобильном его глушат. Последняя надежда, когда
	// не ответил никто другой.
	Brend Istochnik = "brend"
)

// Zapasnye это малоизвестные настоящие сайты на случай, когда у соседей
// годного нет. С компьютера владельца 05.11 все три ответили TLS 1.3, h2 и
// X25519; на мобильном Билайне не проверены.
var Zapasnye = []string{"sourcehut.org", "www.haskell.org", "nim-lang.org"}

// Prezhnie это маски 1.11.0. Их глушат на мобильном, и берутся они, только
// если не ответил никто другой.
var Prezhnie = []string{"dl.google.com", "www.adobe.com"}

// Brendy это известные имена и их поддомены: такие маски глушат на
// мобильном. Сюда же попадают популярные маски reality из чужих инструкций.
var Brendy = []string{
	"google.com", "googleapis.com", "gstatic.com", "googlevideo.com", "youtube.com",
	"microsoft.com", "bing.com", "live.com", "office.com", "windows.com", "azure.com", "xbox.com",
	"apple.com", "icloud.com", "cdn-apple.com", "amazon.com", "amazonaws.com", "cloudfront.net",
	"cloudflare.com", "akamai.net", "akamaihd.net", "akamaized.net", "fastly.net",
	"adobe.com", "yahoo.com", "samsung.com", "nvidia.com", "intel.com", "amd.com", "oracle.com",
	"tesla.com", "discord.com", "twitch.tv", "speedtest.net", "mozilla.org", "github.com",
	"lovelive-anime.jp", "facebook.com", "instagram.com", "whatsapp.com", "telegram.org",
	"twitter.com", "x.com", "netflix.com", "spotify.com", "steampowered.com", "example.com",
	"yandex.ru", "yandex.net", "ya.ru", "vk.com", "mail.ru", "ok.ru", "sberbank.ru",
	"gosuslugi.ru", "ozon.ru", "wildberries.ru", "avito.ru",
}

// EtoBrend: imya из списка известных или их поддомен.
func EtoBrend(imya string) bool {
	imya = strings.ToLower(strings.TrimSuffix(imya, "."))
	return slices.ContainsFunc(Brendy, func(b string) bool {
		return imya == b || strings.HasSuffix(imya, "."+b)
	})
}

// reImya: имя, которое reality и ссылки клиента примут как есть.
var reImya = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// Resolver это то, чем узнают A-записи имени.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Opcii подбора; нули берут значения по умолчанию.
type Opcii struct {
	// Srok это весь перебор соседей, по умолчанию 2 минуты. Запасной список
	// идёт после него своим сроком.
	Srok time.Duration
	// Shag это промежуток между соединениями, по умолчанию 200 мс: не
	// быстрее пяти в секунду, чтобы хостер не счёл это сканированием.
	Shag time.Duration
	// Nuzhno это сколько годных собрать, прежде чем выбрать случайную, по
	// умолчанию 4.
	Nuzhno int
	// Dial, Resolver и Korni подменяются в тестах; nil - сеть и корни
	// системы.
	Dial     func(ctx context.Context, network, addr string) (net.Conn, error)
	Resolver Resolver
	Korni    *x509.CertPool
	// Zapasnye и Prezhnie: nil - списки пакета.
	Zapasnye, Prezhnie []string
	// Krome это имя, которое не годится: «Подобрать другую» не должна
	// вернуть ту же маску.
	Krome string
}

const (
	srokZapasa        = 30 * time.Second
	srokSoedineniya   = 3 * time.Second
	srokRukopozhatiya = 4 * time.Second
	srokImeni         = 3 * time.Second
	rabochih          = 16
	imenNaAdres       = 3
	adresovNaImya     = 3
)

func (o Opcii) sDefoltami() Opcii {
	if o.Srok <= 0 {
		o.Srok = 2 * time.Minute
	}
	if o.Shag <= 0 {
		o.Shag = 200 * time.Millisecond
	}
	if o.Nuzhno <= 0 {
		o.Nuzhno = 4
	}
	if o.Dial == nil {
		d := &net.Dialer{}
		o.Dial = d.DialContext
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	if o.Zapasnye == nil {
		o.Zapasnye = Zapasnye
	}
	if o.Prezhnie == nil {
		o.Prezhnie = Prezhnie
	}
	return o
}

// Proverka это итог проверки одного имени.
type Proverka struct {
	Imya  string
	Adres netip.Addr
	Godna bool
	// Brend: имя из известных, на мобильном его глушат.
	Brend bool
	// Prichina говорит человеку, чем не годна; пусто у годной. Podrobno это
	// исходная ошибка для журнала.
	Prichina, Podrobno string
}

// otkaz записывает причину: у oshibka своя, остальное - прерванная проверка.
func (p *Proverka) otkaz(err error) {
	var o *oshibka
	if errors.As(err, &o) {
		p.Prichina = o.prichina
	} else {
		p.Prichina = "проверка прервана"
	}
	p.Podrobno = err.Error()
}

// oshibka это причина для человека и исходная ошибка под ней.
type oshibka struct {
	prichina string
	err      error
}

func (o *oshibka) Error() string {
	if o.err == nil {
		return o.prichina
	}
	return o.prichina + ": " + o.err.Error()
}

func (o *oshibka) Unwrap() error { return o.err }

// Itog это подобранная маска.
type Itog struct {
	Maska     string
	Istochnik Istochnik
	// Prosmotreno это сколько соседей спрошено.
	Prosmotreno int
}

// ErrNet: не ответил никто, ни соседи, ни запасные, ни прежние маски.
var ErrNet = errors.New("ни один сайт не годится в маску: сервер не достаёт до сайтов с TLS 1.3 и h2")

// Podobrat ищет маску у соседей адреса svoy, потом в запасном списке,
// потом среди прежних масок.
func Podobrat(ctx context.Context, svoy netip.Addr, o Opcii) (Itog, error) {
	o = o.sDefoltami()
	s := &skaner{o: o, tik: time.NewTicker(o.Shag)}
	defer s.tik.Stop()

	var it Itog
	if svoy.Is4() {
		sctx, otmena := context.WithTimeout(ctx, o.Srok)
		var godnye []Proverka
		for _, set := range seti(svoy) {
			godnye = append(godnye, s.oprositSet(sctx, set, svoy, o.Nuzhno-len(godnye))...)
			if len(godnye) >= o.Nuzhno || sctx.Err() != nil {
				break
			}
		}
		otmena()
		it.Prosmotreno = int(s.prosmotreno.Load())
		if len(godnye) > 0 {
			it.Maska, it.Istochnik = godnye[rand.IntN(len(godnye))].Imya, Sosedi
			return it, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return it, err
	}
	// У каждого списка свой срок: молчащий запас не съедает проверку
	// прежних масок, с которыми сервер вставал в 1.11.0.
	for _, sp := range []struct {
		imena []string
		ist   Istochnik
	}{{o.Zapasnye, Zapas}, {o.Prezhnie, Brend}} {
		zctx, otmena := context.WithTimeout(ctx, srokZapasa)
		for _, i := range rand.Perm(len(sp.imena)) {
			if s.krome(sp.imena[i]) {
				continue
			}
			if p := s.proverit(zctx, sp.imena[i]); p.Godna {
				otmena()
				it.Maska, it.Istochnik = p.Imya, sp.ist
				return it, nil
			}
		}
		otmena()
	}
	if err := ctx.Err(); err != nil {
		return it, err
	}
	return it, ErrNet
}

// Proverit проверяет имя теми же условиями, что соседей: A-запись, TLS 1.3,
// h2, X25519 и сертификат. Известное имя проверяется так же, но помечается.
func Proverit(ctx context.Context, imya string, o Opcii) Proverka {
	o = o.sDefoltami()
	s := &skaner{o: o, tik: time.NewTicker(o.Shag)}
	defer s.tik.Stop()
	return s.proverit(ctx, imya)
}

type skaner struct {
	o           Opcii
	tik         *time.Ticker
	prosmotreno atomic.Int64
}

// krome: имя совпадает с o.Krome без учёта регистра и точки в конце.
func (s *skaner) krome(imya string) bool {
	norm := func(x string) string { return strings.ToLower(strings.TrimSuffix(x, ".")) }
	return s.o.Krome != "" && norm(imya) == norm(s.o.Krome)
}

// zhdat пропускает соединения не чаще o.Shag на все потоки разом.
func (s *skaner) zhdat(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.tik.C:
		return nil
	}
}

// seti это /24 адреса и две соседние, в этом порядке.
func seti(a netip.Addr) []netip.Prefix {
	b := a.As4()
	svoya := netip.PrefixFrom(netip.AddrFrom4([4]byte{b[0], b[1], b[2], 0}), 24)
	s := []netip.Prefix{svoya}
	for _, d := range []int{-1, 1} {
		c := int(b[2]) + d
		if c < 0 || c > 255 {
			continue
		}
		s = append(s, netip.PrefixFrom(netip.AddrFrom4([4]byte{b[0], b[1], byte(c), 0}), 24))
	}
	return s
}

// hosty это адреса сети без нулевого, широковещательного и svoy, вперемешку.
func hosty(set netip.Prefix, svoy netip.Addr) []netip.Addr {
	b := set.Addr().As4()
	var h []netip.Addr
	for i := 1; i < 255; i++ {
		a := netip.AddrFrom4([4]byte{b[0], b[1], b[2], byte(i)})
		if a != svoy {
			h = append(h, a)
		}
	}
	rand.Shuffle(len(h), func(i, j int) { h[i], h[j] = h[j], h[i] })
	return h
}

// oprositSet спрашивает соседей в сети set, пока не наберёт nuzhno годных
// или не кончится срок ctx.
func (s *skaner) oprositSet(ctx context.Context, set netip.Prefix, svoy netip.Addr, nuzhno int) []Proverka {
	ctx, otmena := context.WithCancel(ctx)
	defer otmena()
	ochered := make(chan netip.Addr)
	itogi := make(chan *Proverka)
	var wg sync.WaitGroup
	for range rabochih {
		wg.Go(func() {
			for a := range ochered {
				itogi <- s.oprositSoseda(ctx, a)
			}
		})
	}
	go func() {
		defer close(ochered)
		for _, a := range hosty(set, svoy) {
			select {
			case ochered <- a:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(itogi)
	}()
	var godnye []Proverka
	for p := range itogi {
		if p != nil && len(godnye) < nuzhno {
			godnye = append(godnye, *p)
			if len(godnye) == nuzhno {
				otmena()
			}
		}
	}
	return godnye
}

// oprositSoseda берёт имена из сертификата, который сосед отдаёт без имени
// в приветствии, и проверяет их на его адресе. nil - годного нет.
func (s *skaner) oprositSoseda(ctx context.Context, a netip.Addr) *Proverka {
	if s.zhdat(ctx) != nil {
		return nil
	}
	s.prosmotreno.Add(1)
	imena := s.imenaSoseda(ctx, a)
	for _, imya := range imena {
		if p := s.proveritNaAdrese(ctx, imya, a); p.Godna {
			return &p
		}
	}
	return nil
}

// imenaSoseda только узнаёт, какие имена сосед называет в сертификате:
// сертификату здесь не верят и ничего не передают, годность решает
// проверенное рукопожатие в proveritTLS.
func (s *skaner) imenaSoseda(ctx context.Context, a netip.Addr) []string {
	c, err := s.rukopozhatie(ctx, a, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519}})
	if err != nil {
		return nil
	}
	defer c.Close()
	serty := c.ConnectionState().PeerCertificates
	if len(serty) == 0 {
		return nil
	}
	// Только SAN: CommonName проверка сертификата в Go не принимает.
	var imena []string
	for _, imya := range serty[0].DNSNames {
		imya = strings.ToLower(imya)
		if reImya.MatchString(imya) && !EtoBrend(imya) && !s.krome(imya) && !slices.Contains(imena, imya) {
			imena = append(imena, imya)
		}
		if len(imena) == imenNaAdres {
			break
		}
	}
	return imena
}

// proverit ищет адреса имени и проверяет имя на них по очереди, не больше
// adresovNaImya: годен хоть один - годно имя.
func (s *skaner) proverit(ctx context.Context, imya string) Proverka {
	imya = strings.ToLower(strings.TrimSuffix(imya, "."))
	p := Proverka{Imya: imya, Brend: EtoBrend(imya)}
	if !reImya.MatchString(imya) {
		p.Prichina = "это не имя сайта"
		return p
	}
	adresa, err := s.adresaImeni(ctx, imya)
	if err != nil {
		p.otkaz(err)
		return p
	}
	var q Proverka
	for _, a := range adresa[:min(len(adresa), adresovNaImya)] {
		if q = s.proveritTLS(ctx, p, a); q.Godna || ctx.Err() != nil {
			break
		}
	}
	return q
}

func (s *skaner) adresaImeni(ctx context.Context, imya string) ([]netip.Addr, error) {
	rctx, otmena := context.WithTimeout(ctx, srokImeni)
	defer otmena()
	adresa, err := s.o.Resolver.LookupNetIP(rctx, "ip4", imya)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &oshibka{"сервер не находит адрес этого имени", err}
	}
	if len(adresa) == 0 {
		return nil, &oshibka{"у имени нет адреса IPv4", nil}
	}
	for i := range adresa {
		adresa[i] = adresa[i].Unmap()
	}
	return adresa, nil
}

// proveritNaAdrese: A-запись имени ведёт на a, и a отвечает на имя TLS 1.3,
// h2 и X25519 с действительным сертификатом.
func (s *skaner) proveritNaAdrese(ctx context.Context, imya string, a netip.Addr) Proverka {
	p := Proverka{Imya: imya, Adres: a, Brend: EtoBrend(imya)}
	adresa, err := s.adresaImeni(ctx, imya)
	if err != nil {
		p.otkaz(err)
		return p
	}
	if !slices.Contains(adresa, a) {
		p.Prichina = "A-запись имени ведёт не на адрес сайта"
		return p
	}
	return s.proveritTLS(ctx, p, a)
}

// proveritTLS: a отвечает на имя p.Imya TLS 1.3, h2 и X25519 с
// действительным сертификатом.
func (s *skaner) proveritTLS(ctx context.Context, p Proverka, a netip.Addr) Proverka {
	imya := p.Imya
	p.Adres = a
	if err := s.zhdat(ctx); err != nil {
		p.otkaz(err)
		return p
	}
	// Предлагаются оба протокола, как у браузера: сайт без h2 иначе рвёт
	// рукопожатие, и причина вышла бы невнятной.
	c, err := s.rukopozhatie(ctx, a, &tls.Config{ServerName: imya, RootCAs: s.o.Korni, MinVersion: tls.VersionTLS13,
		NextProtos: []string{"h2", "http/1.1"}, CurvePreferences: []tls.CurveID{tls.X25519}})
	if err != nil {
		if ctx.Err() == nil {
			err = &oshibka{"сайт не отвечает TLS 1.3 с X25519 или его сертификат недействителен", err}
		}
		p.otkaz(err)
		return p
	}
	defer c.Close()
	if c.ConnectionState().NegotiatedProtocol != "h2" {
		p.Prichina = "сайт не отвечает h2"
		return p
	}
	p.Godna = true
	return p
}

func (s *skaner) rukopozhatie(ctx context.Context, a netip.Addr, conf *tls.Config) (*tls.Conn, error) {
	dctx, otmena := context.WithTimeout(ctx, srokSoedineniya)
	defer otmena()
	tcp, err := s.o.Dial(dctx, "tcp", netip.AddrPortFrom(a, 443).String())
	if err != nil {
		return nil, err
	}
	hctx, otmena2 := context.WithTimeout(ctx, srokRukopozhatiya)
	defer otmena2()
	c := tls.Client(tcp, conf)
	if err := c.HandshakeContext(hctx); err != nil {
		tcp.Close()
		return nil, err
	}
	return c, nil
}
