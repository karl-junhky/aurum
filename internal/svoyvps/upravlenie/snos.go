package upravlenie

import (
	"context"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var snesti = server.Snesti

// SnosItog это снятый сервер.
type SnosItog struct {
	Snos server.Snos
	Vhod Svedeniya
}

// NeChisto: снос прошёл, а осмотр после него ещё видит своё. Ключи группы
// тогда остаются в клиенте, снос можно повторить.
type NeChisto struct {
	Chto []string
}

func (n *NeChisto) Error() string {
	return "после сноса на сервере осталось: " + strings.Join(n.Chto, ", ")
}

// Snesti снимает с сервера всё своё (§3.9 плана) и проверяет осмотром, что
// чисто. Строка нашего ключа снимается последней, а соединение, вошедшее
// им, живо: осмотр идёт им же.
func Snesti(ctx context.Context, p Puti, z ssh.Zapros) (it SnosItog, err error) {
	defer zashchita("Snesti", &err)
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return SnosItog{}, err
	}
	defer s.Zakryt()
	it.Vhod = s.Svedeniya()
	if it.Snos, err = snesti(ctx, s); err != nil {
		return SnosItog{}, err
	}
	o, err := osmotret(ctx, s)
	if err != nil {
		return SnosItog{}, err
	}
	var chto []string
	if o.Otmetka != nil {
		chto = append(chto, "отметка установки")
	}
	if o.VpnUzel != "" {
		chto = append(chto, "программа vpn-uzel")
	}
	if len(chto) > 0 {
		return SnosItog{}, &NeChisto{Chto: chto}
	}
	return it, nil
}
