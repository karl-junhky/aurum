// Пакет podpiska отдаёт людям подписку с самого VPN-сервера.
//
// Подписку собирает центр: для каждого токена готовые заголовки и тело, как их
// отдавала служба подписок на главной машине (internal/veb). Сервер ничего не
// решает и в центр не ходит, он только отвечает на /sub/<токен> тем, что ему
// прислали. Поэтому подписка работает, даже когда центр лежит, и адрес у людей
// не меняется: имя подписки смотрит на все живые серверы сразу.
//
// Токен НИКОГДА не попадает в журнал, как и у прежней службы.
package podpiska

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Otvet это то, что сервер отдаёт по одному токену. Отозванному тело пустое,
// а причина едет в заголовке announce: так клиент покажет её человеку.
type Otvet struct {
	Imya      string            `json:"imya"`
	Zagolovki map[string]string `json:"zagolovki"`
	Telo      string            `json:"telo"`
}

// Nabor это подписка целиком.
type Nabor struct {
	// Slushat это адрес, например ":8443": тот же порт, что у прежней службы,
	// иначе у людей поменялся бы адрес подписки.
	Slushat string           `json:"slushat"`
	Lyudi   map[string]Otvet `json:"lyudi"`
}

// Sert это сертификат имени подписки (PEM).
type Sert struct {
	Cert   string `json:"cert"`
	Klyuch string `json:"klyuch"`
}

// Сколько запросов подписки в минуту пускается с одного адреса. Как у прежней
// службы: офис выходит под одним адресом, перебор отсекает длина токена.
const potolok = 120

var (
	vidTokena = regexp.MustCompile(`^[0-9a-f]{32}$`)
	vidImeni  = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)
)

// Proverit отвечает, годятся ли набор и сертификат, ничего не трогая, и
// разбирает сертификат. Пустой набор годен: он значит «подписку не раздавать».
// Сертификат без набора тоже годен: он может быть нужен только ядру.
func Proverit(n *Nabor, s *Sert) (*tls.Certificate, error) {
	var sert *tls.Certificate
	if s != nil {
		para, err := tls.X509KeyPair([]byte(s.Cert), []byte(s.Klyuch))
		if err != nil {
			return nil, fmt.Errorf("сертификат негоден: %w", err)
		}
		sert = &para
	}
	if n == nil {
		return sert, nil
	}
	if sert == nil {
		return nil, errors.New("подписка без сертификата")
	}
	if _, port, err := net.SplitHostPort(n.Slushat); err != nil || port == "" {
		return nil, fmt.Errorf("адрес подписки %q негоден", n.Slushat)
	}
	for token, o := range n.Lyudi {
		if !vidTokena.MatchString(token) {
			return nil, errors.New("в подписке негодный токен")
		}
		if !vidImeni.MatchString(o.Imya) {
			return nil, fmt.Errorf("в подписке негодное имя %q", o.Imya)
		}
		for k, v := range o.Zagolovki {
			// Перевод строки в заголовке это чужой заголовок в ответе.
			if k == "" || strings.ContainsAny(k+v, "\r\n") {
				return nil, fmt.Errorf("у %s негодный заголовок", o.Imya)
			}
		}
	}
	return sert, nil
}

type tekushchee struct {
	nabor *Nabor
	sert  *tls.Certificate
}

// Server отдаёт подписку. Набор и сертификат меняются на лету без закрытия
// соединений, слушатель перезапускается только при смене адреса.
type Server struct {
	zhurnal *slog.Logger
	chasy   func() time.Time
	limit   *limiter

	// tek читают обработчики и рукопожатия, поэтому он без замка: замок
	// слушателя держится и на время закрытия сервера, а закрытие ждёт
	// обработчики.
	tek atomic.Pointer[tekushchee]

	mu      sync.Mutex
	adres   string
	srv     *http.Server
	oshibka string // почему не слушаем, если не слушаем

	zmu     sync.Mutex
	zaprosy map[string]int64
}

func Novyy(zhurnal *slog.Logger) *Server {
	return &Server{
		zhurnal: zhurnal,
		chasy:   time.Now,
		limit:   novyyLimiter(potolok, time.Minute),
		zaprosy: map[string]int64{},
	}
}

// Primenit ставит набор, уже проверенный Proverit. nil гасит раздачу.
//
// Занятый порт не отказ пакета: ядро важнее подписки, поэтому набор принят,
// слушатель повторяет Podderzhivat, а причина видна в Oshibka.
func (s *Server) Primenit(n *Nabor, sert *tls.Certificate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n == nil {
		s.tek.Store(nil)
		s.ostanovit()
		s.oshibka = ""
		return
	}
	s.tek.Store(&tekushchee{nabor: n, sert: sert})
	if s.srv != nil && s.adres == n.Slushat {
		return
	}
	s.ostanovit()
	s.slushat(n.Slushat)
}

// Podderzhivat повторяет запуск слушателя, пока порт не освободится.
func (s *Server) Podderzhivat(ctx context.Context, kazhdye time.Duration) {
	t := time.NewTicker(kazhdye)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		if t := s.tek.Load(); t != nil && s.srv == nil {
			s.slushat(t.nabor.Slushat)
		}
		s.mu.Unlock()
	}
}

// Oshibka отдаёт, почему подписка не отдаётся, или пустую строку.
func (s *Server) Oshibka() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.oshibka
}

// Sostoyanie это подписка глазами центра.
type Sostoyanie struct {
	Slushaet string `json:"slushaet,omitempty"`
	Lyudey   int    `json:"lyudey"`
	SertDo   int64  `json:"sert_do,omitempty"`
	Oshibka  string `json:"oshibka,omitempty"`
}

// Sostoyanie отдаёт nil, если подписку раздавать не велено.
func (s *Server) Sostoyanie() *Sostoyanie {
	t := s.tek.Load()
	if t == nil {
		return nil
	}
	s.mu.Lock()
	itog := &Sostoyanie{Slushaet: s.adres, Lyudey: len(t.nabor.Lyudi), Oshibka: s.oshibka}
	s.mu.Unlock()
	if t.sert != nil && t.sert.Leaf != nil {
		itog.SertDo = t.sert.Leaf.NotAfter.Unix()
	}
	return itog
}

// Zaprosy отдаёт, когда каждый человек последний раз забирал подписку
// (unix-секунды по имени). Живёт в памяти: после перезапуска пусто, и это
// не беда, центр хранит своё.
func (s *Server) Zaprosy() map[string]int64 {
	s.zmu.Lock()
	defer s.zmu.Unlock()
	itog := make(map[string]int64, len(s.zaprosy))
	for k, v := range s.zaprosy {
		itog[k] = v
	}
	return itog
}

// Ostanovit гасит раздачу при выходе программы.
func (s *Server) Ostanovit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ostanovit()
}

func (s *Server) ostanovit() {
	if s.srv == nil {
		return
	}
	ctx, otmena := context.WithTimeout(context.Background(), 3*time.Second)
	defer otmena()
	if err := s.srv.Shutdown(ctx); err != nil {
		s.zhurnal.Warn("раздача подписки закрылась не чисто", "oshibka", err)
	}
	s.srv, s.adres = nil, ""
}

func (s *Server) slushat(adres string) {
	ln, err := net.Listen("tcp", adres)
	if err != nil {
		prichina := "подписка не слушает " + adres + ": " + err.Error()
		if s.oshibka != prichina {
			s.zhurnal.Error("подписка не слушает", "adres", adres, "oshibka", err)
		}
		s.oshibka = prichina
		return
	}
	srv := &http.Server{
		Handler: s,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				t := s.tek.Load()
				if t == nil {
					return nil, errors.New("подписка выключена")
				}
				return t.sert, nil
			},
		},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		// Сканеры порта дают по ошибке рукопожатия в минуту, журналу это шум.
		ErrorLog: log.New(io.Discard, "", 0),
	}
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.zhurnal.Error("раздача подписки упала", "oshibka", err)
		}
	}()
	s.srv, s.adres, s.oshibka = srv, adres, ""
	s.zhurnal.Info("подписка слушает", "adres", adres)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	seychas := s.chasy()
	if !s.limit.Mozhno(adresKlienta(r), seychas) {
		w.Header().Set("retry-after", "60")
		http.Error(w, "слишком часто", http.StatusTooManyRequests)
		return
	}
	token, est := strings.CutPrefix(r.URL.Path, "/sub/")
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !est || !vidTokena.MatchString(token) {
		// Не объяснять формат: перебирающему всё выглядит одинаково.
		http.NotFound(w, r)
		return
	}
	var o Otvet
	najden := false
	if t := s.tek.Load(); t != nil {
		o, najden = t.nabor.Lyudi[token]
	}
	if !najden {
		http.NotFound(w, r)
		return
	}
	for k, v := range o.Zagolovki {
		w.Header().Set(k, v)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, o.Telo) // клиент ушёл, отвечать некому
	}
	if o.Telo != "" {
		s.zmu.Lock()
		s.zaprosy[o.Imya] = seychas.Unix()
		s.zmu.Unlock()
	}
}

// adresKlienta достаёт адрес обратившегося. X-Forwarded-For не верим: сервер
// смотрит в интернет сам, а заголовок подделывается одной строкой.
func adresKlienta(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
