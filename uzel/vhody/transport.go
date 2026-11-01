package vhody

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"

	M "github.com/sagernet/sing/common/metadata"
)

// Входы с транспортом (gRPC и прочие поверх HTTP) отдают в обработчик не
// сокет, а поток внутри HTTP/2. Закрытие потока вежливое: всё, что HTTP/2 и
// ядро уже поставили в очередь, клиент дочитывает. В тесте это 6,6 секунды
// после отзыва. Поэтому отзыв должен рвать сам TCP под потоком.
//
// Адрес клиента, который транспорт передаёт в обработчик, для этого не годится:
// sing берёт его из X-Forwarded-For, если клиент прислал такой заголовок, и
// отозванный мог бы подставить чужой адрес. Надёжный путь другой: HTTP-сервер
// Go кладёт в контекст каждого запроса LocalAddr() соединения
// (http.LocalAddrContextKey). Слушатель отдаёт соединения, у которых этот адрес
// несёт ссылку на настоящий сокет. TLS и reality отдают LocalAddr нижнего
// соединения как есть, так что ссылка доходит до запроса через них.

// Слушатель заодно отмечает приход каждого сокета: TLS транспорта идёт
// внутри него, и отказ рукопожатия сюда не доходит. Приход без последующего
// входа это всё, что видно о неудачной попытке на таком входе.
type slushatelSoSsylkoy struct {
	net.Listener
	uchet *Uchet
	vhod  string
}

func (l slushatelSoSsylkoy) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.uchet != nil {
		l.uchet.Otmetit(l.vhod, adresIzNet(c.RemoteAddr()), "", VidPriem)
	}
	if tcp, ok := c.(*net.TCPConn); ok {
		return &soedinenieSoSsylkoy{TCPConn: tcp}, nil
	}
	return c, nil
}

type soedinenieSoSsylkoy struct{ *net.TCPConn }

func (c *soedinenieSoSsylkoy) LocalAddr() net.Addr {
	return adresSoSsylkoy{Addr: c.TCPConn.LocalAddr(), tcp: c.TCPConn}
}

// Upstream отдаёт настоящий сокет тем, кто разворачивает обёртки по цепочке
// sing (common.Cast).
func (c *soedinenieSoSsylkoy) Upstream() any { return c.TCPConn }

// adresSoSsylkoy пишется и сравнивается как обычный адрес, ссылка на сокет
// снаружи не видна.
type adresSoSsylkoy struct {
	net.Addr
	tcp *net.TCPConn
}

func tcpIzKonteksta(ctx context.Context) *net.TCPConn {
	if a, ok := ctx.Value(http.LocalAddrContextKey).(adresSoSsylkoy); ok {
		return a.tcp
	}
	return nil
}

// sSoketom это поток транспорта вместе с сокетом под ним: zakrytSoSbrosom
// находит сокет через Upstream и рвёт его первым.
type sSoketom struct {
	io.Closer
	tcp *net.TCPConn
}

func (s sSoketom) Upstream() any { return s.tcp }

// adresKlienta это адрес клиента без порта для отметок входа. У входа с
// транспортом он берётся с сокета под потоком по той же причине, что и выше:
// адрес из metadata клиент может подменить заголовком X-Forwarded-For.
func adresKlienta(ctx context.Context, istochnik M.Socksaddr) string {
	if tcp := tcpIzKonteksta(ctx); tcp != nil {
		return adresIzNet(tcp.RemoteAddr())
	}
	return istochnik.Unwrap().AddrString()
}

func adresIzNet(a net.Addr) string {
	return M.SocksaddrFromNet(a).Unwrap().AddrString()
}

// vidTLS раскладывает отказ TLS-рукопожатия по видам отметок. Срок и обрыв
// разведены намеренно: провайдер, который глушит соединение после
// приветствия, даёт срок, а который шлёт сброс, даёт обрыв.
func vidTLS(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded),
		errors.As(err, &ne) && ne.Timeout():
		return VidTLSSrok
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed),
		errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return VidTLSObryv
	}
	return VidTLS
}

// zapisDlyaTransporta выбирает, что закрывать при отзыве для соединения из
// транспорта: поток и сокет под ним, если сокет известен.
func zapisDlyaTransporta(ctx context.Context, conn net.Conn) io.Closer {
	if tcp := tcpIzKonteksta(ctx); tcp != nil {
		return sSoketom{Closer: conn, tcp: tcp}
	}
	return conn
}
