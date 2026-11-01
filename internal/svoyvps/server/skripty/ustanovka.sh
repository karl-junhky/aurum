# Ставит программу своего VPN. Идёт от root, повтор ничего не ломает:
# программа и юнит те же - служба не перезапускается.
#
#   ustanovka.sh nachalo <тело ключа>    отметка и ключ, каталог загрузки, sha256 установленного
#   ustanovka.sh <тело ключа>            установка
#   ustanovka.sh klyuch <тело ключа>     записать ключ ещё одного компьютера (захват)
#
# Новые программа и юнит лежат в /var/lib/affory/zagruzka со сверкой sha256.
# Файла там нет - установленный тот же, Go сверил его по sha256 из nachalo.
# Ни apt, ни скачивания: чего нет на машине, то отказ с названием. Порты в
# ufw открывает porty.sh после подбора, вход по паролю не трогается.
# Печатает строки «ключ<TAB>значение», как осмотр. Сам по себе не
# запускается: функции очереди (ochered, koren_iz, peresadit и другие) Go
# ставит перед ним из ochered.sh.
set -euo pipefail

v() { printf '%s\t%s\n' "$1" "$2"; }
otkaz() {
    echo "$*" >&2
    exit 1
}

otm=/etc/affory
z=/var/lib/affory/zagruzka
prog=/usr/local/sbin/vpn-uzel
yunit=/etc/systemd/system/vpn-uzel.service
fsys=/etc/sysctl.d/90-affory.conf
telo_re='^[A-Za-z0-9+/]+=*$'

[ "$(id -u)" -eq 0 ] || otkaz "запускать от root"

# Отметка: состав установки для осмотра, сноса и захвата с другого
# компьютера. Ключи управления лежат телами по строке в klyuchi, снос
# снимает их строки из authorized_keys root последними.
zapisat_otmetku() {
    local klyuchi
    klyuchi="$(awk -v re="$telo_re" '$0 ~ re { printf "%s\"%s\"", (n++ ? "," : ""), $0 }' "$otm/klyuchi")"
    printf '{"format":1,"sostoyanie":"%s","vpn_uzel":"%s","klyuchi":[%s]}\n' "$1" "${2:-}" "$klyuchi" > "$otm/sostav.json.novyy"
    mv -f -- "$otm/sostav.json.novyy" "$otm/sostav.json"
}

zapisat_klyuch() {
    [[ "$1" =~ $telo_re ]] || otkaz "нужно тело ключа"
    [ -f "$otm/klyuchi" ] || : > "$otm/klyuchi"
    grep -qxF -- "$1" "$otm/klyuchi" || printf '%s\n' "$1" >> "$otm/klyuchi"
}

# Прежние состояние и версия из отметки: «ustanovka», если отметки нет.
prezhnee_sostoyanie() { sed -n 's/.*"sostoyanie":"\([a-z]*\)".*/\1/p' "$otm/sostav.json" 2>/dev/null || true; }
prezhnyaya_versiya() { sed -n 's/.*"vpn_uzel":"\([^"]*\)".*/\1/p' "$otm/sostav.json" 2>/dev/null || true; }

umask 077
rezhim="${1:-}"
if [ "$rezhim" = klyuch ]; then
    [ -f "$otm/sostav.json" ] || otkaz "отметки установки нет"
    zapisat_klyuch "${2:-}"
    sostoyanie="$(prezhnee_sostoyanie)"
    zapisat_otmetku "${sostoyanie:-ustanovka}" "$(prezhnyaya_versiya)"
    v klyuch zapisan
    exit 0
fi
[ "$rezhim" != nachalo ] || shift
telo="${1:-}"
[[ "$telo" =~ $telo_re ]] || otkaz "нужно тело ключа"
if [ -e /usr/local/sbin/vpn-nablyudenie ] || grep -qs vpn-centr /root/.ssh/authorized_keys; then
    otkaz "сервер под центром, не трогаю"
fi

# Отметка раньше всего, ещё до загрузки файлов: ключ уже лежит в
# authorized_keys root, и оборванная на любом шаге установка тоже своя -
# снос её уберёт вместе с ключом.
mkdir -p -- "$otm"
zapisat_klyuch "$telo"
sostoyanie="$(prezhnee_sostoyanie)"
zapisat_otmetku "${sostoyanie:-ustanovka}" "$(prezhnyaya_versiya)"
if [ "$rezhim" = nachalo ]; then
    mkdir -p -- "$z"
    for f in "$prog" "$yunit"; do
        [ ! -f "$f" ] || v ustanovleno "$(sha256sum -- "$f" | awk '{ print $1 }') ${f##*/}"
    done
    exit 0
fi

[ "$(uname -m)" = x86_64 ] || otkaz "нужна машина x86_64, а тут $(uname -m)"
[ "$(cat /proc/1/comm 2>/dev/null)" = systemd ] || otkaz "первым процессом не systemd"
for u in systemctl sysctl ip cmp awk sed grep; do
    command -v "$u" >/dev/null || otkaz "нет утилиты $u"
done
[ -f "$z/vpn-uzel" ] || [ -x "$prog" ] || otkaz "программы нет ни в $z, ни на месте"
[ -f "$z/vpn-uzel.service" ] || [ -f "$yunit" ] || otkaz "юнита нет ни в $z, ни на месте"
zapisat_otmetku ustanovka "$(prezhnyaya_versiya)"

# Снимок сети до нас, один раз: снос вернёт по нему sysctl и очередь
# интерфейса. Повторная установка снимок не переписывает, иначе в нём
# оказались бы наши же значения.
klyuchi_sysctl="net.core.rmem_max net.core.wmem_max net.core.rmem_default net.core.default_qdisc net.ipv4.tcp_congestion_control"
# Вывод ip и tc сначала в переменную: под pipefail awk, вышедший на первой
# нужной строке, ронял бы их по SIGPIPE, а set -e - всю установку.
dev="$(ip -4 route show default 2>/dev/null || true)"
dev="$(awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }' <<<"$dev")"
if [ ! -f "$otm/prezhnee" ]; then
    {
        for k in $klyuchi_sysctl; do
            printf '%s=%s\n' "$k" "$(sysctl -n "$k" 2>/dev/null || true)"
        done
        if [ -n "$dev" ] && command -v tc >/dev/null; then
            koren="$(koren_iz "$(ochered)")"
            printf 'qdisc_dev=%s\nqdisc=%s\nqdisc_nomer=%s\n' "$dev" "${koren%% *}" "${koren#* }"
        fi
    } > "$otm/prezhnee.novyy"
    mv -f -- "$otm/prezhnee.novyy" "$otm/prezhnee"
fi
prezhnee() { awk -v k="$1" 'index($0, k "=") == 1 { print substr($0, length(k) + 2); exit }' "$otm/prezhnee"; }

# Программа и юнит: новое кладётся рядом и переносится на место одним mv.
izmeneno=""
if [ -f "$z/vpn-uzel" ] && ! cmp -s -- "$z/vpn-uzel" "$prog"; then
    cp -- "$z/vpn-uzel" "$prog.novyy"
    chown root:root -- "$prog.novyy"
    chmod 0755 -- "$prog.novyy"
    "$prog.novyy" versiya >/dev/null || {
        rm -f -- "$prog.novyy"
        otkaz "новая программа не запускается"
    }
    mv -f -- "$prog.novyy" "$prog"
    izmeneno="programma"
fi
if [ -f "$z/vpn-uzel.service" ] && ! cmp -s -- "$z/vpn-uzel.service" "$yunit"; then
    cp -- "$z/vpn-uzel.service" "$yunit.novyy"
    chmod 0644 -- "$yunit.novyy"
    mv -f -- "$yunit.novyy" "$yunit"
    izmeneno="$izmeneno yunit"
fi

# Сеть ядра. Буфер сокета hy2 программа поднимает сама, но ответы сайтов
# по UDP приходят на исходящие сокеты с системным rmem_default: при 208 КБ
# сервер выбрасывал 0.7% входящего UDP. BBR даёт TCP-входу ровную скорость
# на длинных каналах. Пишется только то, чего не было до нас: поднятое
# раньше не опускается. Файл собирается по снимку, а не по живым значениям:
# повторная установка иначе получила бы пустой файл и после перезагрузки
# потеряла бы fq и bbr.
# Без конвейеров с grep -q: под pipefail ранний выход grep роняет find по
# SIGPIPE, и найденное читалось бы как ненайденное.
bbr_est() {
    grep -qw bbr /proc/sys/net/ipv4/tcp_available_congestion_control 2>/dev/null ||
        modinfo tcp_bbr >/dev/null 2>&1 ||
        [ -n "$(find "/lib/modules/$(uname -r)" -name 'tcp_bbr.ko*' 2>/dev/null)" ]
}
nastroyki=""
for para in net.core.rmem_max=16777216 net.core.wmem_max=16777216 net.core.rmem_default=16777216 \
    net.core.default_qdisc=fq net.ipv4.tcp_congestion_control=bbr; do
    k="${para%%=*}" nado="${para#*=}"
    bylo="$(prezhnee "$k")"
    if [[ "$nado" =~ ^[0-9]+$ ]] && [[ "$bylo" =~ ^[0-9]+$ ]] && [ "$bylo" -ge "$nado" ]; then continue; fi
    [ "$bylo" = "$nado" ] && continue
    if [ "$k" = net.ipv4.tcp_congestion_control ] && ! bbr_est; then
        v preduprezhdenie "bbr в ядре нет"
        continue
    fi
    nastroyki="$nastroyki$k = $nado"$'\n'
done
if [ -n "$nastroyki" ]; then
    printf '# affory: свой VPN. Прежние значения в %s, снос вернёт их.\n%s' "$otm/prezhnee" "$nastroyki" > "$fsys.novyy"
    chmod 0644 -- "$fsys.novyy"
    mv -f -- "$fsys.novyy" "$fsys"
    sysctl -q -p "$fsys" >/dev/null 2>&1 || v preduprezhdenie "sysctl применился не весь"
else
    rm -f -- "$fsys"
fi

# default_qdisc достаётся интерфейсу только при его подъёме, иначе до
# перезагрузки BBR без fq. Живому интерфейсу очередь пересаживается (см.
# ochered.sh), и ядро ставит свою по default_qdisc: fq 0: на одноочередной
# сетевой, mq с fq в каждой очереди на многоочередной. Корень человека не
# трогается.
if [ -n "$dev" ] && command -v tc >/dev/null && [ "$(sysctl -n net.core.default_qdisc 2>/dev/null)" = fq ]; then
    vse="$(ochered)"
    seychas="$(koren_iz "$vse")"
    ubrat_ustarevshuyu_otmetku "$seychas"
    if [ -z "$seychas" ]; then
        v preduprezhdenie "очередь $dev не прочитана, fq не ставлю"
    elif koren_ruchnoy "$seychas"; then
        [ "${seychas%% *}" = fq ] || v preduprezhdenie "очередь $dev поставлена руками (${seychas%% *}), fq не ставлю"
    elif awk '(($4 == "root" && $2 != "mq") || ($4 == "parent" && $5 ~ /^:/)) && $2 != "fq" { ne = 1 } END { exit !ne }' <<<"$vse"; then
        kod=0
        peresadit || kod=$?
        case "$kod" in
        1) v preduprezhdenie "отметка пересадки не записалась, fq на $dev не ставлю" ;;
        2) v preduprezhdenie "fq на $dev не встал" ;;
        3) v preduprezhdenie "fq на $dev не встал, корень остался pfifo" ;;
        esac
    fi
fi

[ -z "$izmeneno" ] || systemctl daemon-reload
systemctl enable vpn-uzel.service >/dev/null 2>&1 || otkaz "vpn-uzel не включается в автозапуск"
if [ -n "$izmeneno" ] || ! systemctl is-active --quiet vpn-uzel.service; then
    systemctl restart vpn-uzel.service || otkaz "vpn-uzel не запустился"
    izmeneno="${izmeneno# }"
    v perezapushchen "${izmeneno:-ne-rabotala}"
fi
for _ in $(seq 1 20); do
    [ -S /run/vpn-uzel/pult.sock ] && break
    sleep 0.5
done
if [ ! -S /run/vpn-uzel/pult.sock ]; then
    journalctl -u vpn-uzel.service -n 20 --no-pager >&2 2>/dev/null || true
    otkaz "vpn-uzel не открыл пульт"
fi

versiya="$("$prog" versiya | sed 's/[^A-Za-z0-9., -]//g')"
zapisat_otmetku gotovo "$versiya"
rm -rf -- /var/lib/affory
v versiya "$versiya"
v gotovo da
