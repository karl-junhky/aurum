package upravlenie

import (
	"context"
	"errors"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var snyatKlyuch = server.SnyatKlyuch

// OtpuskItog это итог «забыть и снять ключ» (З5). Snyat false: строки ключа
// на сервере уже не было.
type OtpuskItog struct {
	Snyat bool
	Vhod  Svedeniya
}

// Otpustit входит ключом управления этого компьютера и снимает его строку из
// authorized_keys root. Соединение, вошедшее ключом, живо и после этого.
// Снятие строки ничего на сервере не перезапускает, поэтому вход идёт и
// туннелем через этот же сервер (BezPerezapuska): иначе сервер, до которого
// прямой SSH режут, нельзя было бы отпустить.
func Otpustit(ctx context.Context, p Puti, z ssh.Zapros) (it OtpuskItog, err error) {
	defer zashchita("Otpustit", &err)
	if z.Vhod.Podpis == nil {
		return OtpuskItog{}, errors.New("отпуск: не задан ключ управления")
	}
	z.BezPerezapuska = true
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return OtpuskItog{}, err
	}
	defer s.Zakryt()
	it.Vhod = s.Svedeniya()
	if it.Snyat, err = snyatKlyuch(ctx, s, z.Vhod.Podpis.PublicKey()); err != nil {
		return OtpuskItog{}, err
	}
	return it, nil
}
