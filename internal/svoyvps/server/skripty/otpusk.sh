# Снимает строку ключа управления этого компьютера из authorized_keys root
# (и из других файлов ключей root, если её туда скопировали), когда
# компьютер забывает свой сервер (З5). Больше на сервере не меняется
# ничего: VPN работает дальше, ключи на других устройствах тоже, отметка
# установки остаётся (лишнее тело в ней сносу не мешает).
#
#   otpusk.sh <тело ключа>
#
# Последний вход root не снимается: других ключей, которыми sshd пустит root
# снаружи, нет, а паролем root не пускают. Без нашего ключа root тогда не
# войти, разве что другим пользователем с sudo или консолью хостера. Это
# «posledniy_vhod da» и код 3; sshd -T не ответил и проверить входы не
# вышло - «posledniy_vhod neizvestno» и код 3. Печатает строки
# «ключ<TAB>значение»: «klyuch snyat» или «klyucha net».
set -euo pipefail

v() { printf '%s\t%s\n' "$1" "$2"; }
otkaz() {
    echo "$*" >&2
    exit 1
}

# Домашний каталог root с косой чертой в конце.
dom=/root/
ak=/root/.ssh/authorized_keys
telo_re='^[A-Za-z0-9+/]+=*$'
telo="${1:-}"

[ "$(id -u)" -eq 0 ] || otkaz "запускать от root"
# Пустое тело совпало бы с каждой строкой и стёрло бы файл целиком.
[[ "$telo" =~ $telo_re ]] || otkaz "нужно тело ключа"

# Входы root судит sshd -T с данными входа root снаружи: так применяются
# блоки Match. Адрес из документационного диапазона не попадёт ни под какой
# Match Address местных сетей, как и человек снаружи. Свой адрес и порт sshd
# берутся из SSH_CONNECTION этого входа, чтобы сработали Match LocalAddress
# и LocalPort. Сомнение везде решается отказом: лишний отказ лучше сервера
# без входа. Поэтому sshd -T не ответил - ключ не снимается.
spec=user=root,host=192.0.2.1,addr=192.0.2.1
read -r _ _ la lp _ <<< "${SSH_CONNECTION:-}" || true
if [[ "${la:-}" =~ ^[0-9A-Fa-f:.]+$ && "${lp:-}" =~ ^[0-9]+$ ]]; then
    spec="$spec,laddr=$la,lport=$lp"
fi
sshd="$(command -v sshd || echo /usr/sbin/sshd)"
nastroyki="$("$sshd" -T -C "$spec" 2>/dev/null)" || nastroyki=""
# Первая строка: пускает ли root ключом и паролем; дальше файлы ключей root
# из AuthorizedKeysFile. Вход одним способом нужен в AuthenticationMethods
# отдельным списком: «publickey,password» один ключ не пускает.
# keyboard-interactive пускает паролем только через PAM, а на старых OpenSSH
# ещё и если challengeresponse не no. Файл с непонятной подстановкой % не
# читается.
vhody="$(awk -v dom="$dom" '
    { v[$1] = $2 }
    $1 == "authenticationmethods" {
        delete m
        for (i = 2; i <= NF; i++) m[$i] = 1
    }
    $1 == "authorizedkeysfile" {
        nf = 0
        for (i = 2; i <= NF; i++) f[++nf] = $i
    }
    END {
        r = v["permitrootlogin"]
        lyuboy = ("any" in m)
        klyuch = (r == "yes" || r == "prohibit-password" || r == "without-password") &&
            v["pubkeyauthentication"] == "yes" && (lyuboy || ("publickey" in m))
        parol = r == "yes" &&
            ((v["passwordauthentication"] == "yes" && (lyuboy || ("password" in m))) ||
             (v["kbdinteractiveauthentication"] == "yes" && v["usepam"] == "yes" &&
              v["challengeresponseauthentication"] != "no" &&
              (lyuboy || ("keyboard-interactive" in m) || ("keyboard-interactive:pam" in m))))
        print (klyuch ? "da" : "net"), (parol ? "da" : "net")
        for (i = 1; i <= nf; i++) {
            p = f[i]
            if (p == "none") continue
            # Путь без корня sshd отсчитывает от домашнего каталога; %h в
            # начале даёт путь с корнем.
            if (substr(p, 1, 1) != "/" && substr(p, 1, 2) != "%h") p = dom p
            gsub(/%h/, dom, p)
            gsub(/%u/, "root", p)
            gsub(/%U/, "0", p)
            if (!index(p, "%")) print p
        }
    }' <<< "$nastroyki")"
{
    read -r klyuchom parolem
    mapfile -t fayly
} <<< "$vhody"
# sshd -T не ответил, и файлы ключей неизвестны: наша строка ищется ещё и во
# втором файле по умолчанию, чтобы не ответить «не было» мимо копии.
[ -n "$nastroyki" ] || fayly=("${ak}2")

# Файлы с нашей строкой: authorized_keys, куда её кладёт установка, и
# файлы ключей root, если человек скопировал её туда. У grep код 1 значит
# «не нашлось», ошибка чтения (2) это отказ.
nashi=()
for f in "$ak" "${fayly[@]}"; do
    [ -f "$f" ] || continue
    e=0
    grep -qF -- "$telo" "$f" || e=$?
    case $e in
    0) nashi+=("$f") ;;
    1) ;;
    *) exit "$e" ;;
    esac
done
if [ ${#nashi[@]} -eq 0 ]; then
    v klyucha net
    exit 0
fi
if [ -z "$nastroyki" ]; then
    v posledniy_vhod neizvestno
    exit 3
fi

# Другие ключи: строки с типом и телом ключа (пустая строка из одного CR и
# мусор не считаются) без принудительной команды и без срока: command=
# оболочки не даёт, а ключ с expiry-time= мог уже истечь.
drugie=0
if [ "$klyuchom" = da ]; then
    for f in "${fayly[@]}"; do
        [ -f "$f" ] || continue
        n="$(awk -v t="$telo" '
            /^[ \t]*#/ || index($0, t) { next }
            {
                s = " " $0
                if (match(s, /[ \t](ssh-(rsa|dss|ed25519)|ecdsa-sha2-nistp[0-9]+|sk-[a-z0-9-]+@openssh\.com)[ \t]+AAAA/) &&
                    substr(s, 1, RSTART) !~ /command=|expiry-time=/) n++
            }
            END { print n + 0 }' "$f")"
        drugie=$((drugie + n))
    done
fi
# Пароль у root ещё должен быть задан: на облачных образах он заблокирован.
# Второе поле passwd -S: P - задан, L и NP - нет.
if [ "$parolem" = da ] && [ "$(passwd -S root 2>/dev/null | awk '{ print $2 }')" != P ]; then
    parolem=net
fi
if [ "$drugie" -eq 0 ] && [ "$parolem" != da ]; then
    v posledniy_vhod da
    exit 3
fi

# Новый файл собирается рядом копией прежнего (cp -p оставляет права и
# владельца) и переносится на место: обрыв не оставит файл пустым. Один и
# тот же файл может стоять в списке дважды, повтор ничего не меняет.
t=""
trap 'rm -f -- "$t"' EXIT
for f in "${nashi[@]}"; do
    t="$f.affory-chast"
    cp -p -- "$f" "$t"
    grep -vF -- "$telo" "$f" > "$t" || [ $? -eq 1 ]
    mv -f -- "$t" "$f"
done
v klyuch snyat
