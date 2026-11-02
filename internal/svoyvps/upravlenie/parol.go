package upravlenie

import (
	"context"
	"errors"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var (
	vyklyuchitParol = server.VyklyuchitParol
	podtverditParol = server.PodtverditParol
	vklyuchitParol  = server.VklyuchitParol
)

// srokVozvrataParolya: пароль после неудачного входа ключом возвращается и
// тогда, когда дело уже отменено.
var srokVozvrataParolya = 30 * time.Second

// pauzaPerevhoda: перечитывая настройку, sshd перезапускает сам себя, и без
// сокета systemd (20.04, 22.04) порт в это мгновение отвечает сбросом.
var pauzaPerevhoda = time.Second

// perevoyti входит ключом новым соединением; сброс на порту повторяется
// ещё дважды.
func perevoyti(ctx context.Context, s Svyaz, podpis gossh.Signer) (Svyaz, error) {
	for i := 0; ; i++ {
		r, err := s.Perevoyti(ctx, podpis)
		if err == nil || i == 2 || ssh.PrichinaIz(err) != ssh.PrichinaPortZakryt {
			return r, err
		}
		t := time.NewTimer(pauzaPerevhoda)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, err
		case <-t.C:
		}
	}
}

// ParolItog это итог переключения входа по паролю (З6).
type ParolItog struct {
	Vhod Svedeniya
	// Posle: вход по паролю после включения (VklyuchitParol).
	Posle server.Vklyuchenie
}

// KlyuchNePustil: пароль выключился, а новый вход ключом управления той же
// дорогой не прошёл, и пароль включён обратно. Vozvrat - почему вернуть его
// сразу не вышло: тогда его вернёт таймер на сервере через 5 минут.
type KlyuchNePustil struct {
	Vhod    error
	Vozvrat error
}

func (k *KlyuchNePustil) Error() string {
	s := "после выключения пароля вход ключом не прошёл: " + k.Vhod.Error()
	if k.Vozvrat != nil {
		s += "; пароль сразу не вернулся: " + k.Vozvrat.Error()
	}
	return s
}

// VyklyuchitParol выключает вход по паролю на своём сервере: наш файл sshd с
// таймером отката, новый вход ключом той же дорогой и только потом снятие
// таймера. Вход ключом не прошёл - пароль возвращается сразу, а не через 5
// минут.
func VyklyuchitParol(ctx context.Context, p Puti, z ssh.Zapros) (it ParolItog, err error) {
	defer zashchita("VyklyuchitParol", &err)
	if z.Vhod.Podpis == nil {
		return ParolItog{}, errors.New("пароль: не задан ключ управления")
	}
	// sshd перечитывает настройку, не трогая ни открытые соединения, ни VPN:
	// туннель через этот же сервер делу не помеха.
	z.BezPerezapuska = true
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return ParolItog{}, err
	}
	defer s.Zakryt()
	it.Vhod = s.Svedeniya()
	if err := vyklyuchitParol(ctx, s); err != nil {
		return it, err
	}
	r, err := perevoyti(ctx, s, z.Vhod.Podpis)
	if err != nil {
		vctx, otmena := context.WithTimeout(context.WithoutCancel(ctx), srokVozvrataParolya)
		defer otmena()
		_, verr := vklyuchitParol(vctx, s)
		return it, &KlyuchNePustil{Vhod: err, Vozvrat: verr}
	}
	r.Zakryt()
	return it, podtverditParol(ctx, s)
}

// VklyuchitParol снимает наш файл sshd: вход по паролю становится таким,
// каким его держат остальные настройки сервера.
func VklyuchitParol(ctx context.Context, p Puti, z ssh.Zapros) (it ParolItog, err error) {
	defer zashchita("VklyuchitParol", &err)
	if z.Vhod.Podpis == nil {
		return ParolItog{}, errors.New("пароль: не задан ключ управления")
	}
	z.BezPerezapuska = true
	s, err := Voyti(ctx, p, z)
	if err != nil {
		return ParolItog{}, err
	}
	defer s.Zakryt()
	it.Vhod = s.Svedeniya()
	if it.Posle, err = vklyuchitParol(ctx, s); err != nil {
		return it, err
	}
	return it, nil
}
