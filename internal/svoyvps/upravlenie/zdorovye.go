package upravlenie

import (
	"context"
	"errors"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// ZdorovyeServera это ответ «Проверить сервер» (З1): как вошли, что видит
// осмотр и что говорит о себе программа сервера. Программа, которая не
// ответила, и маска, которую не проверить, это итог проверки, а не её
// отказ: в них и беда, о которой человеку надо знать.
type ZdorovyeServera struct {
	Vhod   Svedeniya
	Osmotr *server.Osmotr
	// Uzel пуст, если программа не ответила: тогда UzelOtkaz говорит почему.
	Uzel      server.SostoyanieUzla
	UzelOtkaz error
	// Maska: маска отвечает с машины TLS 1.3 и h2. MaskaProverena false -
	// проверить нечем (нет openssl) или проверка упала, тогда MaskaOtkaz.
	Maska, MaskaProverena bool
	MaskaOtkaz            error
}

// ErrChuzhayaUstanovka: на сервере установка без ключа управления этого
// компьютера.
var ErrChuzhayaUstanovka = errors.New("в отметке установки нет ключа этого компьютера")

// ProveritServer входит и смотрит, как живёт свой сервер. На сервере не
// меняется ничего, поэтому и туннель через этот же сервер годится.
func ProveritServer(ctx context.Context, p Puti, z ssh.Zapros, maska string) (zd ZdorovyeServera, err error) {
	defer zashchita("ProveritServer", &err)
	z.BezPerezapuska = true
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return ZdorovyeServera{}, err
	}
	defer s.Zakryt()
	zd.Vhod = s.Svedeniya()
	if zd.Osmotr, err = osmotret(ctx, s); err != nil {
		return ZdorovyeServera{}, err
	}
	switch o := zd.Osmotr.Otmetka; {
	case o == nil:
		return ZdorovyeServera{}, server.ErrNetUstanovki
	case o.Sostoyanie != "gotovo":
		return ZdorovyeServera{}, ErrNeDoshla
	// Ключом этого компьютера вошли, а в отметке его нет: сервер переставлял
	// другой, и ключи VPN в наборе к нему уже не подходят.
	case z.Vhod.Podpis == nil || !nashaOtmetka(o, z.Vhod.Podpis.PublicKey()):
		return ZdorovyeServera{}, ErrChuzhayaUstanovka
	}
	if zd.Uzel, err = sostoyanie(ctx, s); err != nil {
		zd.Uzel, zd.UzelOtkaz = server.SostoyanieUzla{}, err
	}
	if maska != "" {
		if zd.Maska, zd.MaskaProverena, err = proveritMasku(ctx, s, maska); err != nil {
			zd.Maska, zd.MaskaProverena, zd.MaskaOtkaz = false, false, err
		}
	}
	// Отменённая проверка не итог: оборванные шаги выше записались бы
	// отказами программы и маски.
	if err := ctx.Err(); err != nil {
		return ZdorovyeServera{}, err
	}
	return zd, nil
}
