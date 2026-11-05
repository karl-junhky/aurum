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
// (МА1), кроме $2. Подбор идёт до трёх минут.
const skriptPodbora = `exec /usr/local/sbin/vpn-uzel maska podobrat -adres "$1" -krome "$2"`

// skriptProverkiMaski просит программу сервера проверить имя теми же
// условиями, что соседей (МА2, «Ввести свою»).
const skriptProverkiMaski = `exec /usr/local/sbin/vpn-uzel maska proverit "$1"`

// otvetStaroyProgrammy это начало отказа vpn-uzel на незнакомую команду.
const otvetStaroyProgrammy = "неизвестная команда"

// ErrStarayaProgramma: программа сервера старше 0.7.0 и маску не подбирает и
// не проверяет. Установка тогда берёт маску из Maski, а смена маски просит
// обновить программу.
var ErrStarayaProgramma = errors.New("программа сервера не умеет подбирать маску: сначала обнови её")

// otvetMaski разбирает ответ команды vpn-uzel maska: код 1 подбора это
// ErrNetMaski, «неизвестная команда» - ErrStarayaProgramma.
func otvetMaski(it ssh.Itog) (map[string]string, error) {
	switch {
	case it.Kod == 0:
	case it.Kod == 1:
		return nil, fmt.Errorf("%w: %s", ErrNetMaski, ssh.PoslednyayaStroka(it.Oshibki))
	case it.Kod == 2 && strings.HasPrefix(ssh.PoslednyayaStroka(it.Oshibki), otvetStaroyProgrammy):
		return nil, ErrStarayaProgramma
	default:
		return nil, &server.Oshibka{Shag: "maska", Kod: it.Kod, Tekst: ssh.PoslednyayaStroka(it.Oshibki)}
	}
	v := map[string]string{}
	for s := range strings.SplitSeq(string(it.Vyvod), "\n") {
		if k, z, ok := strings.Cut(strings.TrimRight(s, "\r"), "\t"); ok {
			v[k] = z
		}
	}
	return v, nil
}

// podobratMasku: маску подбирает программа сервера, кроме krome (пусто -
// любую).
var podobratMasku = func(ctx context.Context, m server.Mashina, adres netip.Addr, krome string) (Maska, error) {
	it, err := m.Skript(ctx, skriptPodbora, adres.String(), krome)
	if err != nil {
		return Maska{}, err
	}
	v, err := otvetMaski(it)
	if err != nil {
		return Maska{}, err
	}
	mk := Maska{Imya: v["maska"], Istochnik: v["istochnik"]}
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

// ProverkaMaski это ответ программы сервера о маске, которую ввёл человек.
type ProverkaMaski struct {
	Imya  string
	Godna bool
	// Brend: имя из известных, на мобильном его глушат.
	Brend bool
	// Prichina - чем не годна, для человека; Podrobno - исходная ошибка.
	Prichina, Podrobno string
}

var proveritSvoyuMasku = func(ctx context.Context, m server.Mashina, imya string) (ProverkaMaski, error) {
	it, err := m.Skript(ctx, skriptProverkiMaski, imya)
	if err != nil {
		return ProverkaMaski{}, err
	}
	v, err := otvetMaski(it)
	if err != nil {
		return ProverkaMaski{}, err
	}
	p := ProverkaMaski{Imya: v["imya"], Godna: v["godna"] == "da", Brend: v["brend"] == "da", Prichina: v["prichina"], Podrobno: v["podrobno"]}
	if p.Godna {
		if err := konfig.ProveritMasku(p.Imya); err != nil {
			return ProverkaMaski{}, &server.Oshibka{Shag: "maska", Tekst: "программа сервера назвала негодную маску: " + err.Error()}
		}
	}
	return p, nil
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
