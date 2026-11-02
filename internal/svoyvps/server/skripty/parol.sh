# Вход по паролю на своём сервере (З6). Выключает его наш файл sshd: имя
# стоит раньше хостерских 00-*.conf (21.10 хостерский 00-billpanel.conf
# держал пароль включённым), а sshd берёт первое значение. Удаление файла
# возвращает как было.
#
#   parol.sh vyklyuchit   файл, таймер отката на 5 минут, sshd перечитывает
#                         настройку, сверка sshd -T снаружи
#   parol.sh podtverdit   вход ключом новым соединением прошёл: таймер снят
#   parol.sh vklyuchit    файл уходит, sshd перечитывает настройку, таймер снят
#
# Печатает строки «ключ<TAB>значение». Отказ - код 1 и причина в stderr.
# Снаружи пароль держит чужая настройка - код 2, строка «parol chuzhoy» и
# строки «vklyuchaet <файл>», наш файл тогда снят. Сам по себе не запускается: функцию sshd_snaruzhi Go
# ставит перед ним из sshd_snaruzhi.sh.
set -euo pipefail

v() { printf '%s\t%s\n' "$1" "$2"; }
otkaz() {
    echo "$*" >&2
    exit 1
}

fsshd=/etc/ssh/sshd_config.d/0-affory.conf
otkat=affory-parol-otkat

[ "$(id -u)" -eq 0 ] || otkaz "запускать от root"

perechitat() {
    systemctl try-reload-or-restart ssh.service 2>/dev/null ||
        systemctl try-reload-or-restart sshd.service 2>/dev/null
}

# Таймера отката может не быть: тогда stop и reset-failed отвечают ошибкой,
# и это не отказ. Судит итог: таймер не должен работать.
snyat_otkat() {
    systemctl stop "$otkat.timer" "$otkat.service" >/dev/null 2>&1 || true
    systemctl reset-failed "$otkat.timer" "$otkat.service" >/dev/null 2>&1 || true
    ! systemctl is-active --quiet "$otkat.timer"
}

parol_iz() {
    awk '
        $1 == "passwordauthentication" { p = $2 }
        $1 == "kbdinteractiveauthentication" { k = $2 }
        END {
            if (p == "") exit 1
            print (p == "no" && k == "no") ? "net" : "da"
        }'
}

# Кого sshd может пустить паролем: root и люди с оболочкой входа (uid от
# 1000) из /etc/passwd, не больше 50. Блок Match User включает пароль
# одному из них, и сверка одним root его не увидит. Заблокированным паролем
# (passwd -S даёт L) не войти, такой человек не в счёт.
polzovateli() {
    echo root
    awk -F: '$3 >= 1000 && $3 < 65534 && $7 !~ /(nologin|false)$/ { print $1 }' /etc/passwd 2>/dev/null |
        head -n 50 | while IFS= read -r u; do
        [ "$(passwd -S "$u" 2>/dev/null | awk '{ print $2 }')" = L ] || printf '%s\n' "$u"
    done || true
}

# da: снаружи паролем пускают хоть кого-то из polzovateli (с $1 = root -
# root), net: никого, пусто: sshd -T не ответил. Имя, которое не передать в
# данных входа sshd -T (запятая, «=», пробел), проверить нельзя, и оно
# считается за da. Порядок файлов в sshd_config.d у sshd всегда как в языке
# C: сам он язык не переключает (02.11 проверено на OpenSSH 9.6 и 10.2), и
# sshd -T видит тот же порядок.
parol_snaruzhi() {
    local u p itog=net
    while IFS= read -r u; do
        if [[ ! "$u" =~ ^[^,=[:space:]]+$ ]]; then
            itog=da
            continue
        fi
        p="$(sshd_snaruzhi "$u" | parol_iz)" || {
            echo ""
            return
        }
        [ "$p" = net ] || itog=da
    done < <(if [ "${1:-}" = root ]; then echo root; else polzovateli; fi)
    echo "$itog"
}

# Файлы настроек, где пароль или keyboard-interactive стоит в значении $1.
# Между словом и значением sshd принимает и пробелы, и знак «=».
fayly_s() {
    grep -lisE "^[[:space:]]*(PasswordAuthentication|KbdInteractiveAuthentication|ChallengeResponseAuthentication)[[:space:]=]+$1([[:space:]]|\$)" \
        /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null | head -n 10 || true
}

# Отодвинутый файл остался от оборванного включения или сноса: они его
# возвращают на месте, и здесь так же.
[ ! -f "$fsshd.snos" ] || [ -f "$fsshd" ] || mv -f -- "$fsshd.snos" "$fsshd"

vyklyuchit() {
    [ -d "${fsshd%/*}" ] || otkaz "нет каталога ${fsshd%/*}: настройка sshd этого сервера наш файл не прочтёт"
    "$sshd" -t 2>/dev/null || otkaz "sshd -t отвергает настройку сервера ещё до нашего файла"
    snyat_otkat || otkaz "таймер отката прошлого раза не снялся"
    t="$fsshd.affory-chast"
    trap 'rm -f -- "$t"' EXIT
    printf '%s\n' "# Affory: вход по паролю выключен из программы Affory, удаление файла его вернёт" \
        "PasswordAuthentication no" "KbdInteractiveAuthentication no" > "$t"
    chmod 644 -- "$t"
    mv -f -- "$t" "$fsshd"
    if ! "$sshd" -t 2>/dev/null; then
        rm -f -- "$fsshd"
        otkaz "sshd -t отвергает настройку с нашим файлом, файл снят"
    fi
    # Таймер отката заводится раньше, чем sshd перечитает настройку: оборвись
    # связь сразу после, через 5 минут файл уйдёт сам.
    if ! systemd-run --quiet --unit="$otkat" --on-active=300 --timer-property=AccuracySec=1s \
        /bin/sh -c "rm -f $fsshd; systemctl try-reload-or-restart ssh.service || systemctl try-reload-or-restart sshd.service"; then
        rm -f -- "$fsshd"
        otkaz "таймер отката не завёлся, файл снят"
    fi
    # Не перечитал - таймер остаётся: он перечитает настройку ещё раз.
    if ! perechitat; then
        rm -f -- "$fsshd"
        otkaz "sshd не перечитал настройку, файл снят"
    fi
    p="$(parol_snaruzhi)"
    if [ "$p" = net ]; then
        v parol vyklyuchen
        return
    fi
    # Пароль снаружи не выключился: файл уходит, а таймер снимается, только
    # если sshd перечитал настройку без файла.
    rm -f -- "$fsshd"
    if perechitat; then
        snyat_otkat || true
    fi
    [ -n "$p" ] || otkaz "sshd -T не ответил, файл снят"
    v parol chuzhoy
    fayly_s yes | while IFS= read -r f; do v vklyuchaet "$f"; done
    exit 2
}

podtverdit() {
    # Сначала таймер: сработай он между проверкой файла и снятием, файл ушёл
    # бы после ответа «снят».
    snyat_otkat || otkaz "таймер отката не снялся, через 5 минут он вернёт пароль"
    [ -f "$fsshd" ] || otkaz "таймер отката успел вернуть пароль"
    v otkat snyat
}

vklyuchit() {
    # Файл сначала отодвигается и уходит, только когда sshd принял настройку
    # без него и перечитал её, как у сноса.
    if [ -f "$fsshd" ]; then
        mv -f -- "$fsshd" "$fsshd.snos"
        if ! "$sshd" -t 2>/dev/null; then
            mv -f -- "$fsshd.snos" "$fsshd"
            otkaz "sshd -t отвергает настройку и без нашего файла"
        fi
        if ! perechitat; then
            mv -f -- "$fsshd.snos" "$fsshd"
            otkaz "sshd не перечитал настройку"
        fi
        rm -f -- "$fsshd.snos"
    fi
    # Оставшийся таймер только снял бы уже снятый файл. Включение отвечает
    # про root: им человек входит с паролем из письма хостера.
    snyat_otkat || true
    case "$(parol_snaruzhi root)" in
    da) v parol vklyuchen ;;
    net)
        v parol vyklyuchen_chuzhim
        fayly_s no | while IFS= read -r f; do v vyklyuchaet "$f"; done
        ;;
    *) v parol neizvestno ;;
    esac
}

case "${1:-}" in
vyklyuchit) vyklyuchit ;;
podtverdit) podtverdit ;;
vklyuchit) vklyuchit ;;
*) otkaz "неизвестное действие: ${1:-}" ;;
esac
