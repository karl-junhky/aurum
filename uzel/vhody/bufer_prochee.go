//go:build !linux

package vhody

import (
	"fmt"
	"net"
)

// nastroitBufer вне Linux нужен только для тестов: ставит, сколько даст ОС.
func nastroitBufer(pc net.PacketConn) error {
	uc, ok := pc.(*net.UDPConn)
	if !ok {
		return fmt.Errorf("сокет hy2 не UDP (%T), буфер не трогаю", pc)
	}
	if err := uc.SetReadBuffer(razmerBufera); err != nil {
		return err
	}
	return uc.SetWriteBuffer(razmerBufera)
}
