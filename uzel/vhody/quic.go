package vhody

import (
	"context"
	"crypto/rand"
	"net"
	"sync"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/common/tls"
	qtls "github.com/sagernet/sing-quic"
	M "github.com/sagernet/sing/common/metadata"
)

// slezhkaQUIC помнит живые QUIC-соединения входа hy2.
//
// У hy2 пароль проверяется один раз на соединение, а потоки внутри приходят в
// обработчик уже с готовым именем и без самого соединения, только с адресом
// клиента. Поэтому соединения запоминаются при приёме, по адресу, и отзыв
// закрывает соединение целиком: клиент получает закрытие сразу, а не висит до
// тайм-аута.
type slezhkaQUIC struct {
	uchet    *Uchet
	vhod     string
	mu       sync.Mutex
	poAdresu map[string]*soedinenieQUIC
}

type soedinenieQUIC struct {
	conn  *quic.Conn
	zapis *Zapis
	adres string
}

func novayaSlezhkaQUIC(uchet *Uchet, vhod string) *slezhkaQUIC {
	return &slezhkaQUIC{uchet: uchet, vhod: vhod, poAdresu: make(map[string]*soedinenieQUIC)}
}

// adresKlyuch повторяет то, как служба hy2 подаёт адрес клиента в обработчик:
// M.SocksaddrFromNet(RemoteAddr()).Unwrap().
func adresKlyuch(a net.Addr) string {
	return M.SocksaddrFromNet(a).Unwrap().String()
}

func (s *slezhkaQUIC) dobavit(c *quic.Conn) {
	so := &soedinenieQUIC{
		conn:  c,
		zapis: s.uchet.Novaya(zakrytieQUIC{c}, s.vhod, adresIzNet(c.RemoteAddr())),
		adres: adresKlyuch(c.RemoteAddr()),
	}
	s.mu.Lock()
	// С одного адреса и порта второе соединение при живом первом бывает,
	// только если клиент переподключился раньше, чем старое истекло. Потоки
	// тогда идут на новое, оно и запоминается.
	s.poAdresu[so.adres] = so
	s.mu.Unlock()
	context.AfterFunc(c.Context(), func() {
		s.mu.Lock()
		if s.poAdresu[so.adres] == so {
			delete(s.poAdresu, so.adres)
		}
		s.mu.Unlock()
		s.uchet.Otpustit(so.zapis)
	})
}

func (s *slezhkaQUIC) zapis(istochnik M.Socksaddr) *Zapis {
	s.mu.Lock()
	defer s.mu.Unlock()
	if so := s.poAdresu[istochnik.String()]; so != nil {
		return so.zapis
	}
	return nil
}

// zakrytVse шлёт закрытие всем живым соединениям и ждёт, пока пакеты уйдут.
//
// Без этого остановка входа роняет соединения молча: quic-go при закрытии
// транспорта уничтожает их, не сообщая клиенту, и клиент hy2 ждёт 30 с
// тишины, прежде чем переподключиться. Подсмотрено у s-ui (quicgrace).
func (s *slezhkaQUIC) zakrytVse() {
	s.mu.Lock()
	vse := make([]*quic.Conn, 0, len(s.poAdresu))
	for _, so := range s.poAdresu {
		vse = append(vse, so.conn)
	}
	s.mu.Unlock()
	gotovo := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for _, c := range vse {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = c.CloseWithError(0, "") // уже закрытое соединение ответит ошибкой, она не нужна
			}()
		}
		wg.Wait()
		close(gotovo)
	}()
	select {
	case <-gotovo:
	case <-time.After(3 * time.Second):
		// Закрытие ждёт только своего цикла отправки, на сеть не смотрит;
		// три секунды это страховка от зависшего соединения, а не ожидание.
	}
}

type zakrytieQUIC struct{ c *quic.Conn }

func (z zakrytieQUIC) Close() error { return z.c.CloseWithError(0, "") }

// tlsSoSlezhkoy это TLS-настройка входа hy2, которая сама поднимает
// QUIC-слушатель.
//
// sing-quic перед подъёмом слушателя спрашивает, не умеет ли TLS-настройка
// делать это сама (qtls.ServerConfig), и если умеет, отдаёт подъём ей. Мы
// повторяем ровно путь sing-quic по умолчанию (ListenWithOptions) и только
// добавляем слежку за принятыми соединениями.
type tlsSoSlezhkoy struct {
	tls.ServerConfig
	slezhka *slezhkaQUIC
	// obfs повторяет ListenOptions, которые sing-quic передал бы пути по
	// умолчанию: при маскировке пакеты согласования версий и сброса не шлются.
	obfs    bool
	oshibka error
}

var _ qtls.ServerConfig = (*tlsSoSlezhkoy)(nil)

func (t *tlsSoSlezhkoy) ConfigureHTTP3() {
	// Путь по умолчанию: http3.ConfigureTLSConfig ради побочного действия
	// (инициализация ключей сессий), результат sing-quic тоже выбрасывает.
	std, err := t.STDConfig()
	if err != nil {
		t.oshibka = err
		return
	}
	http3.ConfigureTLSConfig(std)
}

func (t *tlsSoSlezhkoy) Listen(conn net.PacketConn, quicConfig *quic.Config) (qtls.Listener, error) {
	if t.oshibka != nil {
		return nil, t.oshibka
	}
	std, err := t.STDConfig()
	if err != nil {
		return nil, err
	}
	transport := &quic.Transport{Conn: conn, DisableVersionNegotiationPackets: t.obfs}
	transport.SetSingleUse(true)
	if !t.obfs {
		transport.StatelessResetKey = new(quic.StatelessResetKey)
		if _, err := rand.Read(transport.StatelessResetKey[:]); err != nil {
			return nil, err
		}
	}
	ln, err := transport.Listen(std, quicConfig)
	if err != nil {
		return nil, qtls.WrapError(err)
	}
	return &slushatelQUIC{Listener: ln, slezhka: t.slezhka}, nil
}

func (t *tlsSoSlezhkoy) ListenEarly(conn net.PacketConn, quicConfig *quic.Config) (qtls.EarlyListener, error) {
	// hy2 ранний приём не зовёт. Если начнёт, ошибка на старте лучше, чем
	// соединения мимо слежки и мимо отзыва.
	return nil, errRanniyPriyom
}

type oshibkaVhoda string

func (o oshibkaVhoda) Error() string { return string(o) }

const errRanniyPriyom = oshibkaVhoda("ранний приём QUIC не поддержан слежкой за соединениями")

type slushatelQUIC struct {
	*quic.Listener
	slezhka *slezhkaQUIC
}

func (l *slushatelQUIC) Accept(ctx context.Context) (*quic.Conn, error) {
	c, err := l.Listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	l.slezhka.dobavit(c)
	return c, nil
}

func (l *slushatelQUIC) Close() error {
	l.slezhka.zakrytVse()
	return l.Listener.Close()
}
