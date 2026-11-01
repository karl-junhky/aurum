package upravlenie

import (
	"context"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var osmotret = server.Osmotret

// Otchet это шаг «Проверка» мастера: как вошли и что увидел осмотр. На
// сервере при этом не меняется ничего.
type Otchet struct {
	Vhod        Svedeniya
	Osmotr      *server.Osmotr
	Zamechaniya []server.Zamechanie
}

// Osmotret входит и осматривает машину (§3.8, шаги 1-3).
func Osmotret(ctx context.Context, p Puti, z ssh.Zapros) (o Otchet, err error) {
	defer zashchita("Osmotret", &err)
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return Otchet{}, err
	}
	defer s.Zakryt()
	o.Vhod = s.Svedeniya()
	if o.Osmotr, err = osmotret(ctx, s); err != nil {
		return Otchet{}, err
	}
	o.Zamechaniya = o.Osmotr.Zamechaniya(seychas())
	return o, nil
}
