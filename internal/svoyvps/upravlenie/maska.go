package upravlenie

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// Maski это маски 1.11.0. С 1.12.0 маску подбирает программа сервера у
// соседей (podobratMasku), а список остаётся на случай программы без
// подбора: на мобильном эти имена глушат (опыт 05.11).
var Maski = []string{"dl.google.com", "www.adobe.com"}

// Откуда маска: так её называет vpn-uzel maska podobrat (uzel/maska).
const (
	MaskaSosedi = "sosedi"
	MaskaZapas  = "zapas"
	MaskaBrend  = "brend"
)

// Maska это маска reality и откуда она взялась: MaskaSosedi, MaskaZapas или
// MaskaBrend; пусто - из списка, заданного установке.
type Maska struct {
	Imya      string
	Istochnik string
}

// skriptPodbora просит программу сервера подобрать маску у соседей по сети
// (МА1). Подбор идёт до трёх минут.
const skriptPodbora = `exec /usr/local/sbin/vpn-uzel maska podobrat -adres "$1"`

// otvetStaroyProgrammy это начало отказа vpn-uzel на незнакомую команду.
const otvetStaroyProgrammy = "неизвестная команда"

// podobratMasku: маску подбирает программа сервера. Программа без подбора
// (старше 0.7.0) отвечает на команду «неизвестная команда», и маска
// берётся из Maski, как в 1.11.0. Другой отказ с тем же кодом 2 - отказ.
var podobratMasku = func(ctx context.Context, m server.Mashina, adres netip.Addr) (Maska, error) {
	it, err := m.Skript(ctx, skriptPodbora, adres.String())
	if err != nil {
		return Maska{}, err
	}
	switch {
	case it.Kod == 0:
	case it.Kod == 1:
		return Maska{}, fmt.Errorf("%w: %s", ErrNetMaski, ssh.PoslednyayaStroka(it.Oshibki))
	case it.Kod == 2 && strings.HasPrefix(ssh.PoslednyayaStroka(it.Oshibki), otvetStaroyProgrammy):
		imya, err := vybratMasku(ctx, m, Maski)
		return Maska{Imya: imya, Istochnik: MaskaBrend}, err
	default:
		return Maska{}, &server.Oshibka{Shag: "maska", Kod: it.Kod, Tekst: ssh.PoslednyayaStroka(it.Oshibki)}
	}
	var mk Maska
	for s := range strings.SplitSeq(string(it.Vyvod), "\n") {
		k, v, _ := strings.Cut(strings.TrimRight(s, "\r"), "\t")
		switch k {
		case "maska":
			mk.Imya = v
		case "istochnik":
			mk.Istochnik = v
		}
	}
	if err := konfig.ProveritMasku(mk.Imya); err != nil {
		return Maska{}, &server.Oshibka{Shag: "maska", Tekst: "программа сервера назвала негодную маску: " + err.Error()}
	}
	switch mk.Istochnik {
	case MaskaSosedi, MaskaZapas, MaskaBrend:
	default:
		mk.Istochnik = MaskaBrend
	}
	return mk, nil
}

// skriptMaski спрашивает маску с самой машины: reality отдаёт чужому
// приветствию её сертификат, и маска, которую сервер не достаёт или которая
// не умеет TLS 1.3 и h2, выдала бы вход. Нет openssl - проверить нечем.
const skriptMaski = `command -v openssl >/dev/null 2>&1 || { echo net-openssl; exit 0; }
t=""
command -v timeout >/dev/null 2>&1 && t="timeout 15"
out=$($t openssl s_client -connect "$1:443" -servername "$1" -tls1_3 -alpn h2 </dev/null 2>&1)
if printf '%s\n' "$out" | grep -q 'ALPN protocol: h2' && printf '%s\n' "$out" | grep -q 'TLSv1.3'; then
    echo godna
else
    echo negodna
fi`

// ErrNetMaski: ни одна маска списка не ответила с машины TLS 1.3 и h2.
var ErrNetMaski = errors.New("ни одна маска не отвечает с сервера")

var proveritMasku = func(ctx context.Context, m server.Mashina, maska string) (godna, proveryaemo bool, err error) {
	it, err := m.Skript(ctx, skriptMaski, maska)
	if err != nil {
		return false, false, err
	}
	if it.Kod != 0 {
		return false, false, &server.Oshibka{Shag: "maska", Kod: it.Kod, Tekst: ssh.PoslednyayaStroka(it.Oshibki)}
	}
	switch strings.TrimSpace(string(it.Vyvod)) {
	case "godna":
		return true, true, nil
	case "net-openssl":
		return false, false, nil
	}
	return false, true, nil
}

// vybratMasku берёт маски в случайном порядке и отдаёт первую, что отвечает
// с машины. Без openssl проверить нечем, и берётся первая из перемешанных.
func vybratMasku(ctx context.Context, m server.Mashina, spisok []string) (string, error) {
	if len(spisok) == 0 {
		return "", errors.New("маска: список пуст")
	}
	poryadok := make([]string, len(spisok))
	for i, j := range rand.Perm(len(spisok)) {
		poryadok[i] = spisok[j]
	}
	for _, maska := range poryadok {
		godna, proveryaemo, err := proveritMasku(ctx, m, maska)
		switch {
		case err != nil:
			return "", err
		case !proveryaemo:
			return poryadok[0], nil
		case godna:
			return maska, nil
		}
	}
	return "", ErrNetMaski
}
