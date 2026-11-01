// Пакет lovushka считает, сколько попыток подключения дошло до сервера на
// порт: SYN для TCP, датаграммы для UDP. Детектор блокировок клиента
// стучится к серверу напрямую, а сервер по SSH отвечает, дошёл ли стук.
// Пакеты ловятся до брандмауэра, поэтому «дошёл, но ответа нет» (ufw
// сервера) отличается от «не дошёл» (режут по дороге). Без своей ловушки
// детектор зависел бы от tcpdump, которого нет в части образов.
package lovushka

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// PredelSekund это сколько секунд ловушка может ждать: пульт держит
// соединение минуту.
const PredelSekund = 30

// predelAdresov это сколько разных отправителей перечисляется; остальные
// складываются в «*», сканер с тысяч адресов не раздувает ответ.
const predelAdresov = 64

const (
	protoTCP = 6
	protoUDP = 17
)

// Itog это ответ ловушки.
type Itog struct {
	Proto  string           `json:"proto"`
	Port   int              `json:"port"`
	Sekund int              `json:"sekund"`
	Vsego  int64            `json:"vsego"`
	Adresa map[string]int64 `json:"adresa"` // отправитель -> сколько
}

func (i *Itog) dobavit(a netip.Addr) {
	i.Vsego++
	klyuch := a.Unmap().String()
	if _, est := i.Adresa[klyuch]; !est && len(i.Adresa) >= predelAdresov {
		klyuch = "*"
	}
	i.Adresa[klyuch]++
}

// Proverit проверяет просьбу до запуска: она приходит строкой по ssh.
func Proverit(proto string, port, sekund int) error {
	if proto != "tcp" && proto != "udp" {
		return fmt.Errorf("ловушка: протокол %q, ждали tcp или udp", proto)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("ловушка: порт %d вне 1-65535", port)
	}
	if sekund < 1 || sekund > PredelSekund {
		return fmt.Errorf("ловушка: %d с, можно от 1 до %d", sekund, PredelSekund)
	}
	return nil
}

// Lovit ждёт sekund секунд и считает пришедшие на порт попытки.
func Lovit(ctx context.Context, proto string, port, sekund int) (Itog, error) {
	if err := Proverit(proto, port, sekund); err != nil {
		return Itog{}, err
	}
	nomer := protoTCP
	if proto == "udp" {
		nomer = protoUDP
	}
	itog := Itog{Proto: proto, Port: port, Sekund: sekund, Adresa: map[string]int64{}}
	err := lovit(ctx, nomer, port, sekund, &itog)
	return itog, err
}

// razobrat узнаёт в пакете с сетевого заголовка попытку на порт: TCP с SYN
// без ACK или любую датаграмму UDP, не фрагмент. Отдаёт отправителя.
func razobrat(b []byte, proto, port int) (netip.Addr, bool) {
	if len(b) == 0 {
		return netip.Addr{}, false
	}
	var otkogo netip.Addr
	var dalshe []byte
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return netip.Addr{}, false
		}
		dlina := int(b[0]&0x0f) * 4
		if dlina < 20 || len(b) < dlina || int(b[9]) != proto || binary.BigEndian.Uint16(b[6:8])&0x1fff != 0 {
			return netip.Addr{}, false
		}
		otkogo, dalshe = netip.AddrFrom4([4]byte(b[12:16])), b[dlina:]
	case 6:
		// Заголовки расширений не разбираются: SYN и QUIC клиента их не несут.
		if len(b) < 40 || int(b[6]) != proto {
			return netip.Addr{}, false
		}
		otkogo, dalshe = netip.AddrFrom16([16]byte(b[8:24])), b[40:]
	default:
		return netip.Addr{}, false
	}
	if len(dalshe) < 4 || int(binary.BigEndian.Uint16(dalshe[2:4])) != port {
		return netip.Addr{}, false
	}
	if proto == protoTCP && (len(dalshe) < 14 || dalshe[13]&0x12 != 0x02) {
		return netip.Addr{}, false
	}
	return otkogo, true
}
