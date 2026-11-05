// Пакет sayt это режим своего домена: сайт-заглушка и подписка на встроенном
// HTTPS.
//
// Reality ведёт сюда всех, кто не наш клиент («сам у себя»): цель reality это
// 127.0.0.1 на порту из настройки, а не чужой сайт. Снаружи сервер выглядит
// обычным сайтом на домене с настоящим сертификатом. Тот же HTTPS по
// секретному пути отдаёт подписку, которую собирает клиент. На 80/tcp сайт
// отвечает переадресацией на https, как обычный сайт, и проверкой Let's
// Encrypt для продления сертификата домена.
//
// Путь подписки НИКОГДА не попадает в журнал.
package sayt

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/brandmauer"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/zaglushka"
)

// Nastroyka это поле sayt пакета.
type Nastroyka struct {
	// Domen это имя сайта, на него выпущен сертификат (sert_ip_sam.domen).
	Domen string `json:"domen"`
	// Port это публичный порт https, то есть порт reality: туда ведёт
	// переадресация с 80.
	Port int `json:"port"`
	// Slushat это адрес HTTPS на петле, цель reality.
	Slushat string `json:"slushat"`
	// Zerno выбирает вид заглушки, 32 знака hex.
	Zerno    string   `json:"zerno"`
	Podpiska Podpiska `json:"podpiska"`
}

// Podpiska это ответ по секретному пути: заголовки и тело собирает клиент.
type Podpiska struct {
	Put       string            `json:"put"` // 32 знака hex
	Zagolovki map[string]string `json:"zagolovki,omitempty"`
	Telo      string            `json:"telo"`
}

var reHex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Proverit отвергает настройку, с которой сайт не поднимется или ответит
// чужим заголовком. Годность имени домена проверяет выпуск сертификата.
func (n Nastroyka) Proverit() error {
	switch {
	case n.Domen == "":
		return errors.New("sayt: нет домена")
	case n.Port < 1 || n.Port > 65535:
		return fmt.Errorf("sayt: порт %d", n.Port)
	case !reHex32.MatchString(n.Zerno):
		return errors.New("sayt: зерно не 32 знака hex")
	case !reHex32.MatchString(n.Podpiska.Put):
		return errors.New("sayt: путь подписки не 32 знака hex")
	}
	ap, err := netip.ParseAddrPort(n.Slushat)
	if err != nil || !ap.Addr().IsLoopback() || ap.Port() == 0 {
		return fmt.Errorf("sayt: адрес %q не на петле", n.Slushat)
	}
	for k, v := range n.Podpiska.Zagolovki {
		// Перевод строки в заголовке это чужой заголовок в ответе.
		if k == "" || strings.ContainsAny(k, " :\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("sayt: негодный заголовок подписки %q", k)
		}
	}
	return nil
}

// Vypusk это выпуск сертификата домена глазами сайта.
type Vypusk interface {
	Tekushchiy() *tls.Certificate
	OtvetitProverke(w http.ResponseWriter, r *http.Request) bool
	Derzhat80(da bool)
}

// Brandmauer открывает 80/tcp, пока сайт работает.
type Brandmauer interface {
	Otkryt80() error
	// Ubrat снимает правила сайта, в том числе от упавшей программы.
	Ubrat() error
}

// PometkaSayta стоит в комментарии правила, которое открывает 80/tcp.
const PometkaSayta = "affory-sayt"

type iptables struct{}

func (iptables) Otkryt80() error {
	return errors.Join(brandmauer.Otkryt(false, "tcp", 80, PometkaSayta), brandmauer.Otkryt(true, "tcp", 80, PometkaSayta))
}

func (iptables) Ubrat() error { return brandmauer.Ubrat(PometkaSayta) }

// Opcii это то, что тесты подменяют. Нули значат как на сервере.
type Opcii struct {
	Port80     string // адрес слушателя http, по умолчанию ":80"
	Brandmauer Brandmauer
	Chasy      func() time.Time
}

type tekushchee struct {
	n         Nastroyka
	zaglushka http.Handler
}

// Sostoyanie это сайт глазами клиента: поле sayt ответа sostoyanie.
type Sostoyanie struct {
	Domen string `json:"domen"`
	// Https: поднят HTTPS для reality, Http: поднят 80/tcp.
	Https bool `json:"https"`
	Http  bool `json:"http"`
	// Sert: сайту есть чем ответить на домене.
	Sert    bool   `json:"sert"`
	Oshibka string `json:"oshibka,omitempty"`
	// Zabrana: когда подписку последний раз забирали, unix-секунды.
	Zabrana int64 `json:"zabrana,omitempty"`
}

// Sayt держит оба слушателя. Настройка меняется на лету без закрытия
// соединений, слушатель HTTPS перезапускается только при смене адреса.
type Sayt struct {
	zhurnal *slog.Logger
	vypusk  Vypusk
	o       Opcii

	tek atomic.Pointer[tekushchee]
	// posledniy это последний сертификат на домен, что отдал выпуск: пока
	// выпуск переходит на другое имя, сайт отвечает им, а не обрывом, иначе
	// reality не дождётся своей цели и не пустит и наших.
	posledniy atomic.Pointer[tls.Certificate]
	zabrana   atomic.Int64

	mu         sync.Mutex
	https      *http.Server
	adresHttps string
	http80     *http.Server
	oshibkaS   string // HTTPS не слушает
	oshibka80  string // 80 не слушает
}

func Novyy(zhurnal *slog.Logger, vypusk Vypusk, o Opcii) *Sayt {
	if o.Port80 == "" {
		o.Port80 = ":80"
	}
	if o.Brandmauer == nil {
		o.Brandmauer = iptables{}
	}
	if o.Chasy == nil {
		o.Chasy = time.Now
	}
	return &Sayt{zhurnal: zhurnal, vypusk: vypusk, o: o}
}

// Primenit ставит настройку, уже проверенную Proverit. nil гасит сайт и
// закрывает 80/tcp.
//
// Занятый порт не отказ пакета: ядро важнее сайта, поэтому настройка
// принята, слушатель повторяет Podderzhivat, а причина видна в состоянии.
func (s *Sayt) Primenit(n *Nastroyka) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n == nil {
		s.tek.Store(nil)
		s.ostanovit()
		return
	}
	t := &tekushchee{n: *n}
	if staroe := s.tek.Load(); staroe != nil && staroe.n.Zerno == n.Zerno {
		t.zaglushka = staroe.zaglushka
	} else {
		t.zaglushka = zaglushka.Novyy([]byte(n.Zerno))
	}
	s.tek.Store(t)
	if s.https == nil || s.adresHttps != n.Slushat {
		s.ostanovitHttps()
		s.slushatHttps(n.Slushat)
	}
	if s.http80 == nil {
		s.slushat80()
	}
}

// Podderzhivat повторяет запуск слушателей, пока порты не освободятся.
func (s *Sayt) Podderzhivat(ctx context.Context, kazhdye time.Duration) {
	t := time.NewTicker(kazhdye)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		if tek := s.tek.Load(); tek != nil {
			if s.https == nil {
				s.slushatHttps(tek.n.Slushat)
			}
			if s.http80 == nil {
				s.slushat80()
			}
		}
		s.mu.Unlock()
	}
}

// Ostanovit гасит сайт при выходе программы. Правило 80/tcp остаётся: после
// перезапуска сайт откроет его заново, а снимать и ставить его незачем.
func (s *Sayt) Ostanovit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ostanovitHttps()
	s.ostanovit80()
}

// Sostoyanie отдаёт nil, если сайта нет.
func (s *Sayt) Sostoyanie() *Sostoyanie {
	t := s.tek.Load()
	if t == nil {
		return nil
	}
	s.mu.Lock()
	itog := &Sostoyanie{Domen: t.n.Domen, Https: s.https != nil, Http: s.http80 != nil,
		Oshibka: strings.TrimSpace(s.oshibkaS + " " + s.oshibka80)}
	s.mu.Unlock()
	_, err := s.sertifikat(t.n.Domen)
	itog.Sert = err == nil
	itog.Zabrana = s.zabrana.Load()
	return itog
}

func (s *Sayt) ostanovit() {
	s.ostanovitHttps()
	s.ostanovit80()
	s.oshibkaS, s.oshibka80 = "", ""
	if err := s.o.Brandmauer.Ubrat(); err != nil {
		s.zhurnal.Error("правило 80/tcp сайта не снялось", "oshibka", err)
	}
}

func (s *Sayt) ostanovitHttps() {
	if s.https == nil {
		return
	}
	zakryt(s.https, s.zhurnal, "https")
	s.https, s.adresHttps = nil, ""
}

func (s *Sayt) ostanovit80() {
	if s.http80 == nil {
		return
	}
	// Сначала выпуск перестаёт класть проверки в закрытый сайт.
	s.vypusk.Derzhat80(false)
	zakryt(s.http80, s.zhurnal, "http")
	s.http80 = nil
}

func zakryt(srv *http.Server, zhurnal *slog.Logger, chto string) {
	ctx, otmena := context.WithTimeout(context.Background(), 3*time.Second)
	defer otmena()
	if err := srv.Shutdown(ctx); err != nil {
		zhurnal.Warn("сайт закрылся не чисто", "slushatel", chto, "oshibka", err)
	}
}

func novyyServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		// Сканеры дают по ошибке рукопожатия в минуту, журналу это шум.
		ErrorLog: log.New(io.Discard, "", 0),
	}
}

func (s *Sayt) slushatHttps(adres string) {
	ln, err := net.Listen("tcp", adres)
	if err != nil {
		prichina := "сайт не слушает " + adres + ": " + err.Error()
		if s.oshibkaS != prichina {
			s.zhurnal.Error("сайт не слушает", "adres", adres, "oshibka", err)
		}
		s.oshibkaS = prichina
		return
	}
	srv := novyyServer(http.HandlerFunc(s.otvetitHttps))
	srv.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Reality перенимает рукопожатие цели, а его клиенты ждут X25519:
		// гибридный постквантовый обмен цель выбирать не должна.
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			t := s.tek.Load()
			if t == nil {
				return nil, errors.New("сайт выключен")
			}
			return s.sertifikat(t.n.Domen)
		},
	}
	go func() {
		// ServeTLS сам добавляет h2: обычный сайт его умеет.
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.zhurnal.Error("сайт упал", "slushatel", "https", "oshibka", err)
		}
	}()
	s.https, s.adresHttps, s.oshibkaS = srv, adres, ""
	s.zhurnal.Info("сайт слушает", "adres", adres)
}

func (s *Sayt) slushat80() {
	ln, err := net.Listen("tcp", s.o.Port80)
	if err != nil {
		prichina := "80/tcp не слушается: " + err.Error()
		if s.oshibka80 != prichina {
			s.zhurnal.Error("сайт не слушает 80/tcp", "oshibka", err)
		}
		s.oshibka80 = prichina
		return
	}
	// Правило ставится заново: от прошлого запуска могло остаться своё же.
	if err := s.o.Brandmauer.Ubrat(); err != nil {
		s.zhurnal.Warn("старое правило 80/tcp сайта не снялось", "oshibka", err)
	}
	if err := s.o.Brandmauer.Otkryt80(); err != nil {
		// Брандмауэра может и не быть; переадресация не главное, главное
		// проверка Let's Encrypt, и её отказ будет виден в выпуске.
		s.zhurnal.Warn("80/tcp сайта не открыт в брандмауэре", "oshibka", err)
	}
	srv := novyyServer(http.HandlerFunc(s.otvetit80))
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.zhurnal.Error("сайт упал", "slushatel", "http", "oshibka", err)
		}
	}()
	s.http80, s.oshibka80 = srv, ""
	s.vypusk.Derzhat80(true)
}

// sertifikat это сертификат на домен: от выпуска, а пока выпуск переходит
// на другое имя, последний годный.
func (s *Sayt) sertifikat(domen string) (*tls.Certificate, error) {
	if c := s.vypusk.Tekushchiy(); c != nil && c.Leaf != nil && c.Leaf.VerifyHostname(domen) == nil {
		s.posledniy.Store(c)
		return c, nil
	}
	if c := s.posledniy.Load(); c != nil && c.Leaf != nil && c.Leaf.VerifyHostname(domen) == nil {
		return c, nil
	}
	return nil, errors.New("сертификата на домен ещё нет")
}

func (s *Sayt) otvetitHttps(w http.ResponseWriter, r *http.Request) {
	t := s.tek.Load()
	if t == nil {
		http.NotFound(w, r)
		return
	}
	p := t.n.Podpiska
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte("/"+p.Put)) == 1 {
		for k, v := range p.Zagolovki {
			w.Header().Set(k, v)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, p.Telo) // клиент ушёл, отвечать некому
			s.zabrana.Store(s.o.Chasy().Unix())
		}
		return
	}
	t.zaglushka.ServeHTTP(w, r)
}

// otvetit80 отвечает проверкой Let's Encrypt, если её ждёт выпуск, а на всё
// остальное переадресует на https своего домена. Имя берётся из настройки, а
// не из запроса: чужое имя в Host не делает из сайта переадресацию куда
// угодно.
func (s *Sayt) otvetit80(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") && s.vypusk.OtvetitProverke(w, r) {
		return
	}
	t := s.tek.Load()
	if t == nil {
		http.NotFound(w, r)
		return
	}
	host := t.n.Domen
	if t.n.Port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(t.n.Port))
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusMovedPermanently)
}
