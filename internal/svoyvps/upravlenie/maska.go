package upravlenie

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// Maski это имена, под чьи сайты прячется reality (§3.3 плана). Обе с
// тестовых машин отвечают TLS 1.3 и h2 (проба П5); список пополняется
// проверкой с телефона (И3).
var Maski = []string{"dl.google.com", "www.adobe.com"}

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
