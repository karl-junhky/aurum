package lovushka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// lovit слушает все интерфейсы сокетом AF_PACKET: он видит пакет раньше
// netfilter, то есть и тот, что ufw потом отбросит. Фильтр в ядре
// пропускает в сокет только попытки на порт, остальной трафик сервера мимо.
func lovit(ctx context.Context, proto, port, sekund int, itog *Itog) error {
	syroe, err := sobratFiltr(proto, port)
	if err != nil {
		return err
	}
	filtr := make([]unix.SockFilter, len(syroe))
	for n, r := range syroe {
		filtr[n] = unix.SockFilter{Code: r.Op, Jt: r.Jt, Jf: r.Jf, K: r.K}
	}
	// Сокет без протокола ничего не получает, пока не привязан: фильтр
	// встаёт раньше первого пакета.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("ловушка: сокет AF_PACKET: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER,
		&unix.SockFprog{Len: uint16(len(filtr)), Filter: &filtr[0]}); err != nil {
		return fmt.Errorf("ловушка: фильтр: %w", err)
	}
	tv := unix.NsecToTimeval((200 * time.Millisecond).Nanoseconds())
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		return fmt.Errorf("ловушка: срок приёма: %w", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL)}); err != nil {
		return fmt.Errorf("ловушка: привязка: %w", err)
	}
	buf := make([]byte, snaplen)
	konec := time.Now().Add(time.Duration(sekund) * time.Second)
	for time.Now().Before(konec) && ctx.Err() == nil {
		n, otkuda, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("ловушка: приём: %w", err)
		}
		if ll, est := otkuda.(*unix.SockaddrLinklayer); !est || ll.Pkttype == unix.PACKET_OUTGOING {
			continue
		}
		// Фильтр уже отобрал, разбор страхует от ошибки в нём.
		if a, est := razobrat(buf[:n], proto, port); est {
			itog.dobavit(a)
		}
	}
	return nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }
