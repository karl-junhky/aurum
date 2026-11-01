package sertip

import (
	"net"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/brandmauer"
)

// Pometka стоит в комментарии каждого правила проверки: по ней правило
// снимается, и по ней его узнаёт снос установки.
const Pometka = "affory-acme"

// Iptables открывает порт проверки правилом в начале INPUT
// (пакет brandmauer).
type Iptables struct{}

func (b Iptables) Otkryt(ip net.IP, port int) (func() error, error) {
	if err := brandmauer.Otkryt(ip.To4() == nil, "tcp", port, Pometka); err != nil {
		return nil, err
	}
	return b.Ubrat, nil
}

func (b Iptables) Ubrat() error { return brandmauer.Ubrat(Pometka) }
