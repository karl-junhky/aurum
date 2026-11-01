//go:build linux

package vhody

import (
	"fmt"
	"net"
	"syscall"
)

// nastroitBufer ставит сокету hy2 буферы в razmerBufera в обход потолка
// net.core.rmem_max (нужен root или CAP_NET_ADMIN, служба под root).
//
// quic-go пытается поднять буфер сам, но при маскировке salamander получает
// не сокет, а обёртку без SetReadBuffer, и остаётся на системных 208 КБ. На
// 153 это давало до 20% отброшенных пакетов приёма, лечилось sysctl
// rmem_default. Здесь то же самое делает программа для сокета hy2. Исходящим
// UDP-сокетам прямого выхода (ответы сайтов людям) rmem_default всё равно
// нужен, его ставит ustanovka-uzla.sh.
func nastroitBufer(pc net.PacketConn) error {
	uc, ok := pc.(*net.UDPConn)
	if !ok {
		return fmt.Errorf("сокет hy2 не UDP (%T), буфер не трогаю", pc)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var oshibka error
	err = raw.Control(func(fd uintptr) {
		oshibka = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUFFORCE, razmerBufera)
		if oshibka == nil {
			oshibka = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUFFORCE, razmerBufera)
		}
	})
	if err != nil {
		return err
	}
	if oshibka != nil {
		// Без прав на обход потолка ставим, сколько дадут.
		return fmt.Errorf("буфер в обход потолка не встал (%w), ставлю в пределах rmem_max: %v %v",
			oshibka, uc.SetReadBuffer(razmerBufera), uc.SetWriteBuffer(razmerBufera))
	}
	return nil
}
