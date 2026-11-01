package ssh

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// metkaProby открывает ответ пробы: всё, что до неё, напечатала оболочка
// входа.
const metkaProby = "affory-proba-vhoda"

// komandaProbyVhoda одной сессией отдаёт uid, адрес, с которого сервер видит
// подключение, и адреса самого сервера. Идёт через sh: оболочка входа может
// быть любой.
var komandaProbyVhoda = "sh -c " + kavychki(`u=$(id -u) || exit 1
printf '%s\n%s\n%s\n' `+metkaProby+` "$u" "${SSH_CLIENT%% *}"
hostname -I 2>/dev/null
exit 0`)

// uznatPrava решает, как дальше получать root: как есть или через sudo.
// Решение по коду выхода, а не по тексту: sudo 1.8-1.9 и sudo-rs (26.04)
// пишут разное, а коды у них одни (пробы 01.11 на всех четырёх LTS).
func (s *Soedinenie) uznatPrava(ctx context.Context, parol string) error {
	it, err := s.syroe(ctx, komandaProbyVhoda, nil)
	if err != nil {
		return err
	}
	vyvod := string(it.Vyvod)
	i := strings.Index(vyvod, metkaProby+"\n")
	if it.Kod != 0 || i < 0 {
		// Просроченный пароль OpenSSH с PAM пускает, а потом вместо команды
		// пишет «Password change required but no TTY available». Ищется,
		// только когда команда не дошла до метки: иначе это шум оболочки.
		if trebuetSmenyParolya(vyvod + " " + it.Oshibki) {
			return otkaz(PrichinaParolProsrochen, "сервер просит сменить пароль", nil)
		}
		return otkaz(PrichinaSboy, fmt.Sprintf("проба входа: код %d, %q", it.Kod, Obrezat(vyvod, 80)), nil)
	}
	// Пустые строки перед ответом терпимы: дальше zapustit срезает их с
	// вывода каждой команды.
	if shum := strings.TrimSpace(vyvod[:i]); shum != "" {
		return otkaz(PrichinaShumVhoda, Obrezat(shum, 80), nil)
	}
	s.pustoyShum = []byte(vyvod[:i])
	stroki := strings.Split(vyvod[i+len(metkaProby)+1:], "\n")
	uid := strings.TrimSpace(stroki[0])
	if uid == "" || strings.Trim(uid, "0123456789") != "" {
		return otkaz(PrichinaSboy, fmt.Sprintf("id -u: %q", Obrezat(uid, 40)), nil)
	}
	// Адрес подключения нужен не всегда: без него откажут только проверка
	// туннеля и укладка ключа не root.
	if len(stroki) > 1 {
		if a, err := netip.ParseAddr(strings.TrimSpace(stroki[1])); err == nil {
			s.otkuda = a.Unmap()
		}
	}
	s.svoi = []string{s.adres}
	if len(stroki) > 2 {
		s.svoi = append(s.svoi, strings.Fields(stroki[2])...)
	}
	if uid == "0" {
		s.prava, s.Root = pravaRoot, true
		return nil
	}

	it, err = s.syroe(ctx, "sudo -n true", nil)
	if err != nil {
		return err
	}
	switch {
	case it.Kod == 0:
		s.prava = pravaSudo
		return nil
	case it.Kod == 127:
		return otkaz(PrichinaNetSudo, "", nil)
	case parol == "":
		return otkaz(PrichinaSudoPrositParol, "", nil)
	}
	// Одна попытка: пароль тот, с которым только что вошли, значит отказ
	// это «не в sudoers», и вторая ничего не изменит.
	it, err = s.syroe(ctx, "sudo -k -S -p '' true", strings.NewReader(parol+"\n"))
	if err != nil {
		return err
	}
	switch it.Kod {
	case 0:
		s.prava, s.parolSudo = pravaSudoSParolem, parol
		return nil
	case 127:
		return otkaz(PrichinaNetSudo, "", nil)
	}
	return otkaz(PrichinaSudoNePustil, PoslednyayaStroka(it.Oshibki), nil)
}

// proveritTunnel смотрит, откуда сервер видит подключение. Если туннель
// выходит в интернет с этого же сервера, подключение придёт с его
// собственного адреса: перезапуск программы на нём оборвал бы туннель вместе
// с SSH посреди операции.
func (s *Soedinenie) proveritTunnel() error {
	if !s.otkuda.IsValid() {
		return otkaz(PrichinaSboy, "сервер не назвал адрес подключения", nil)
	}
	if s.otkuda.IsLoopback() {
		return otkaz(PrichinaTunnelCherezNego, s.otkuda.String(), nil)
	}
	for _, a := range s.svoi {
		if p, err := netip.ParseAddr(a); err == nil && p.Unmap() == s.otkuda {
			return otkaz(PrichinaTunnelCherezNego, s.otkuda.String(), nil)
		}
	}
	return nil
}

// skriptKlyuchaRoot дописывает строку ключа в authorized_keys root, если
// такого тела там ещё нет, и тогда печатает «dobavlen». Перевод строки в
// конце прежнего файла проверяется: иначе наша строка приклеилась бы к
// последней чужой. Права прежних файла и каталога не трогаются: sshd годится
// и 644; созданные заново получают 700 и 600 от umask. Второй аргумент,
// другой файл, только для живой проверки.
const skriptKlyuchaRoot = `set -e
k=$1
f=${2:-/root/.ssh/authorized_keys}
telo=$(printf '%s\n' "$k" | awk '{print $2}')
[ -n "$telo" ] || { echo 'пустое тело ключа' >&2; exit 2; }
d=$(dirname -- "$f")
umask 077
mkdir -p -- "$d"
[ -f "$f" ] || : > "$f"
e=0; grep -qF -- "$telo" "$f" || e=$?
case $e in 0) exit 0 ;; 1) ;; *) exit "$e" ;; esac
if [ -s "$f" ] && [ "$(tail -c 1 -- "$f" | wc -l)" -eq 0 ]; then printf '\n' >> "$f"; fi
printf '%s\n' "$k" >> "$f"
echo dobavlen`

// skriptUbratKlyuchRoot убирает строки с телом нашего ключа. Пустое тело
// совпало бы с каждой строкой и стёрло бы файл целиком, поэтому отказ. Новый
// файл собирается рядом копией прежнего (cp -p оставляет права и владельца)
// и переносится на место: обрыв не оставит authorized_keys пустым. У grep
// код 1 значит «не нашлось», а не ошибку; ошибка чтения (2) это отказ.
const skriptUbratKlyuchRoot = `set -e
k=$1
f=${2:-/root/.ssh/authorized_keys}
telo=$(printf '%s\n' "$k" | awk '{print $2}')
[ -n "$telo" ] || { echo 'пустое тело ключа' >&2; exit 2; }
[ -f "$f" ] || exit 0
e=0; grep -qF -- "$telo" "$f" || e=$?
case $e in 0) ;; 1) exit 0 ;; *) exit "$e" ;; esac
t=$f.affory-chast
trap 'rm -f -- "$t"' EXIT
cp -p -- "$f" "$t"
grep -vF -- "$telo" "$f" > "$t" || [ $? -eq 1 ]
mv -f -- "$t" "$f"`

// skriptVhodaRoot отдаёт, как sshd пустит root с нашего адреса. Под sudo
// окружение сбрасывается, поэтому адрес приходит аргументом.
const skriptVhodaRoot = `a=$1
p=$(command -v sshd || echo /usr/sbin/sshd)
"$p" -T -C "user=root,host=$a,addr=$a" 2>/dev/null | awk '$1=="permitrootlogin"{print $2}'
true`

// srokOtkata: уборка своей строки идёт и после отмены операции.
var srokOtkata = 30 * time.Second

// PolozhitKlyuchRoot кладёт открытую часть ключа управления podpis в
// authorized_keys root и входит им тем же путём новым соединением. Отдаёт
// это соединение root; старое закрывает вызывающий.
//
// serverPustoy это галочка «Сервер пустой»: до неё на машине не меняется
// ничего, наш ключ тоже. Вошли не root - сначала sshd -T говорит, пустит ли
// он root ключом: при PermitRootLogin no ключ класть незачем. Остальное
// (AllowUsers, свой AuthorizedKeysFile, AuthenticationMethods) видно только
// по входу.
//
// Root нашим ключом не пустили - строка с его телом бесполезна, кто бы её ни
// положил (и прошлая попытка, оборванная после укладки), и убирается. Вход
// сорвался по дороге - убирается только положенное сейчас: прежняя строка
// могла работать.
func (s *Soedinenie) PolozhitKlyuchRoot(ctx context.Context, podpis gossh.Signer, kommentariy string, serverPustoy bool) (r *Soedinenie, err error) {
	defer zashchita("PolozhitKlyuchRoot", &err)
	if !serverPustoy {
		return nil, otkaz(PrichinaSboy, "ключ управления кладётся только после галочки «Сервер пустой»", nil)
	}
	if podpis == nil {
		return nil, otkaz(PrichinaSboy, "ключ управления не задан", nil)
	}
	stroka, err := strokaKlyucha(podpis.PublicKey(), kommentariy)
	if err != nil {
		return nil, err
	}
	if !s.Root {
		if !s.otkuda.IsValid() {
			return nil, otkaz(PrichinaSboy, "сервер не назвал адрес подключения", nil)
		}
		it, err := s.Skript(ctx, skriptVhodaRoot, s.otkuda.String())
		if err != nil {
			return nil, err
		}
		// Пустой ответ значит sshd -T не сработал: тогда судит вход ниже.
		switch v := strings.TrimSpace(string(it.Vyvod)); v {
		case "no", "forced-commands-only":
			return nil, otkaz(PrichinaRootZapreshchyon, "PermitRootLogin "+v, nil)
		}
	}
	it, err := s.Skript(ctx, skriptKlyuchaRoot, stroka)
	if err != nil {
		return nil, err
	}
	if it.Kod != 0 {
		return nil, otkaz(PrichinaSboy, fmt.Sprintf("ключ root не лёг: код %d: %s", it.Kod, PoslednyayaStroka(it.Oshibki)), nil)
	}
	dobavlen := strings.TrimSpace(string(it.Vyvod)) == "dobavlen"

	izvesten := s.KlyuchHosta
	r, err = Podklyuchit(ctx, Zapros{
		Adres:    s.adres,
		Port:     s.port,
		Vhod:     Vhod{Polzovatel: "root", Podpis: podpis},
		Izvesten: &izvesten,
		Put:      s.put,
	})
	if err == nil && !r.Root {
		r.Zakryt()
		r, err = nil, otkaz(PrichinaSboy, "вход ключом root дал не root", nil)
	}
	if err == nil {
		return r, nil
	}
	nePustil := PrichinaIz(err) == PrichinaKlyuchNePrinyat
	if nePustil {
		err = otkaz(PrichinaRootZapreshchyon, "sshd не пустил root нашим ключом", err)
	}
	if nePustil || dobavlen {
		octx, otmena := context.WithTimeout(context.WithoutCancel(ctx), srokOtkata)
		defer otmena()
		if oerr := s.ubratKlyuchRoot(octx, stroka); oerr != nil {
			var o *Otkaz
			if !errors.As(err, &o) {
				o = otkaz(PrichinaSboy, "", err)
				err = o
			}
			o.KlyuchOstalsya = true
			if o.Podrobno != "" {
				o.Podrobno += "; "
			}
			o.Podrobno += "строка ключа в authorized_keys root не убрана: " + Obrezat(oerr.Error(), 200)
		}
	}
	return nil, err
}

func (s *Soedinenie) ubratKlyuchRoot(ctx context.Context, stroka string) error {
	it, err := s.Skript(ctx, skriptUbratKlyuchRoot, stroka)
	if err != nil {
		return err
	}
	if it.Kod != 0 {
		return otkaz(PrichinaSboy, fmt.Sprintf("код %d: %s", it.Kod, PoslednyayaStroka(it.Oshibki)), nil)
	}
	return nil
}

// strokaKlyucha собирает строку authorized_keys: тип, тело и комментарий
// только из простых символов (имя компьютера и дата).
func strokaKlyucha(pk gossh.PublicKey, kommentariy string) (string, error) {
	kommentariy = strings.TrimSpace(kommentariy)
	for _, r := range kommentariy {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(" ._@:-", r)) {
			return "", otkaz(PrichinaSboy, "в комментарии ключа негодный символ", nil)
		}
	}
	stroka := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pk)))
	if kommentariy != "" {
		stroka += " " + kommentariy
	}
	return stroka, nil
}
