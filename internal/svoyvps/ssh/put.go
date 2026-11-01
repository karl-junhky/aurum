package ssh

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"syscall"

	"golang.org/x/net/proxy"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// IP_UNICAST_IF и IPV6_UNICAST_IF из ws2ipdef.h: в x/sys/windows их нет.
const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

// PutPryamo ведёт соединение через физический адаптер indeks мимо туннеля.
//
// Правило process_path у входа TUN соединение из ядра не выводит: TUN
// собран с auto_route и strict_route, и соединение службы открывает само
// ядро, а перезапуск ядра на том же сервере рвал бы SSH посреди операции.
// IP_UNICAST_IF велит Windows отправить сокет через этот адаптер, что бы ни
// говорила таблица маршрутов.
func PutPryamo(indeks uint32, imya string) Put {
	d := &net.Dialer{Control: func(network, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			h := windows.Handle(fd)
			switch network {
			case "tcp6", "udp6":
				serr = windows.SetsockoptInt(h, windows.IPPROTO_IPV6, ipv6UnicastIf, int(indeks))
			default:
				// Для IPv4 индекс ждут в сетевом порядке байт. Windows
				// живёт только на little-endian, поэтому переворот.
				serr = windows.SetsockoptInt(h, windows.IPPROTO_IP, ipUnicastIf, int(int32(bits.ReverseBytes32(indeks))))
			}
		})
		if err != nil {
			return err
		}
		if serr != nil {
			return fmt.Errorf("привязка к адаптеру %d: %w", indeks, serr)
		}
		return nil
	}}
	return Put{Imya: "напрямую через " + imya, Dial: d.DialContext}
}

// FizicheskiyAdapter отдаёт адаптер, который несёт трафик наружу, не считая
// туннелей: своего TUN, чужих VPN и встроенного VPN Windows. Правило «похож
// на туннель» то же, что у поиска помех.
func FizicheskiyAdapter() (a set.Adapter, err error) {
	defer zashchita("FizicheskiyAdapter", &err)
	spisok, err := set.Kandidaty()
	if err != nil {
		return set.Adapter{}, err
	}
	for _, a := range spisok {
		if !pomehi.PohozhNaTunnel(a) {
			return a, nil
		}
	}
	return set.Adapter{}, errors.New("нет адаптера с выходом в интернет, кроме туннелей")
}

// PutCherezTunnel ведёт соединение по SOCKS5 через локальный вход ядра
// (mixed на 127.0.0.1:10809). Так сервер видно, когда напрямую его адрес
// закрыт по дороге. Судит только ответ самого сервера: ядро принимает
// соединение у себя даже на закрытом порту.
//
// Что на vhod слушает именно наше ядро, слой не проверяет: чужая программа
// на 10809 приняла бы соединение, и до сервера дошло бы через неё. Служба
// выбирает этот путь, только когда ядро подняла она сама (Е2).
func PutCherezTunnel(vhod string) Put {
	return Put{Imya: "через туннель", Tunnel: true, Dial: func(ctx context.Context, network, adres string) (net.Conn, error) {
		d, err := proxy.SOCKS5("tcp", vhod, nil, &net.Dialer{})
		if err != nil {
			return nil, err
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS5 без срока")
		}
		return cd.DialContext(ctx, network, adres)
	}}
}

// netVhodaTunnelya: не открылось соединение до самого входа SOCKS5, то есть
// ядра на 10809 нет. Отказ уже от ядра (сервер не ответил) сюда не попадает.
func netVhodaTunnelya(err error) bool {
	var op *net.OpError
	if !errors.As(err, &op) || op.Op != "socks connect" {
		return false
	}
	var vnutri *net.OpError
	return errors.As(op.Err, &vnutri) && vnutri.Op == "dial"
}

// otkazPorta: порт ответил отказом, на нём никто не слушает.
func otkazPorta(err error) bool {
	return !netVhodaTunnelya(err) && (errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, syscall.ECONNREFUSED))
}
