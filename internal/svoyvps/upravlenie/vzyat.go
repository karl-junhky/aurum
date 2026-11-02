package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var (
	zapisatKlyuch      = server.ZapisatKlyuch
	prochitatUstanovku = server.ProchitatUstanovku
)

// ErrNeDoshla: отметка своя, а окончательного пакета на сервере нет,
// установка оборвалась раньше ключей. Ключ управления уже лёг, и установка
// заново продолжит её.
var ErrNeDoshla = errors.New("установка на сервере не дошла до ключей")

// Vzyatie это всё, что нужно захвату (§3.9 плана): сервер под Affory с
// другого компьютера или запись о нём пришла импортом без ключа управления.
type Vzyatie struct {
	Puti Puti
	// Vhod: вход человека (пароль или свой ключ) и Izvesten, ключ хоста с
	// осмотра или из записи.
	Vhod        ssh.Zapros
	Imya        string
	Upravlenie  gossh.Signer
	Kommentariy string
}

// Vzyat кладёт ключ управления этого компьютера в authorized_keys root и в
// отметку и собирает ключи из пакета, принятого программой сервера: без
// перевыпуска, сервер остаётся источником правды. Ключи VPN на других
// компьютерах продолжают работать.
func Vzyat(ctx context.Context, v Vzyatie) (itog Itog, err error) {
	defer zashchita("Vzyat", &err)
	a, err := netip.ParseAddr(v.Vhod.Adres)
	switch {
	case err != nil:
		return Itog{}, fmt.Errorf("захват: адрес %q не IP", ssh.Obrezat(v.Vhod.Adres, 80))
	case v.Upravlenie == nil || v.Imya == "":
		return Itog{}, errors.New("захват: не задан ключ управления или имя")
	}
	adres := a.Unmap()

	s, err := Voyti(ctx, v.Puti, v.Vhod)
	if err != nil {
		return Itog{}, err
	}
	defer func() { s.Zakryt() }()
	itog.Vhod = s.Svedeniya()
	o, err := osmotret(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	for _, z := range o.Zamechaniya(seychas()) {
		if z.Kod == server.KodPodCentrom {
			return Itog{}, &Ostanovka{Zamechaniya: []server.Zamechanie{z}}
		}
	}
	if o.Otmetka == nil {
		return Itog{}, server.ErrNetUstanovki
	}
	itog.ParolVyklyuchen = slices.Contains(o.SshdFayly, server.FaylParolya)

	// «Взять под управление» это то же согласие менять машину, что галочка
	// «Сервер пустой» при установке. Ключ кладётся и тогда, когда он уже есть
	// в отметке: строку в authorized_keys могли снять руками.
	r, err := s.KlyuchRoot(ctx, v.Upravlenie, v.Kommentariy, true)
	if err != nil {
		return Itog{}, err
	}
	s.Zakryt()
	s = r
	if err := zapisatKlyuch(ctx, s, v.Upravlenie.PublicKey()); err != nil {
		return Itog{}, err
	}
	sostav, paket, err := prochitatUstanovku(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	if len(paket) == 0 {
		return Itog{}, ErrNeDoshla
	}
	sek, vh, err := konfig.IzPaketa(paket, adres.String(), v.Imya)
	if err != nil {
		return Itog{}, err
	}
	klyuchi, err := konfig.Klyuchi(sek, vh)
	if err != nil {
		return Itog{}, err
	}
	st, err := sostoyanie(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	itog.Sert, itog.Prodlenie = prodlenieStoyashchego(vh, st)
	itog.Adres, itog.Sekrety, itog.Vhody, itog.Klyuchi = adres.String(), sek, vh, klyuchi
	itog.Versiya = sostav.VpnUzel
	return itog, nil
}

// prodlenieStoyashchego это чем продлевается сертификат поставленного
// сервера и что о выпуске говорит программа. Hy2 стоит - сертификат был.
// Путь его выпуска неизвестен, если программа о нём молчит: тогда
// считается, что продлению нужен 443.
func prodlenieStoyashchego(vh konfig.Vhody, st server.SostoyanieUzla) (*server.SertUzla, porty.Prodlenie) {
	switch {
	case len(vh.Hy2) == 0:
		return nil, porty.BezSerta
	case st.Sert != nil:
		return st.Sert, prodlenieIz(st.Sert.Sposob)
	}
	return nil, porty.Cherez443
}
