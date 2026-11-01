# Снос своего VPN по отметке, только своё: служба и программа, их каталоги,
# временные правила программы, sysctl и очередь интерфейса по снимку, наш
# файл sshd. Правила ufw с пометкой снимает porty.sh раньше этого скрипта.
# Каждый шаг переживает отсутствие своего, так что повтор после обрыва
# доводит снос до конца. Отметка и строки наших ключей в authorized_keys root
# уходят последними и только если всё остальное снялось: иначе повторить
# было бы нечем.
#
# Печатает строки «ключ<TAB>значение». Код 1: что-то не снялось, отметка на
# месте. Код 4: всё снято, но строки ключей в authorized_keys остались. Сам
# по себе не запускается: функции очереди Go ставит перед ним из ochered.sh.
set -uo pipefail

v() { printf '%s\t%s\n' "$1" "$2"; }
oshibki=0
ne_vyshlo() {
    v ne_vyshlo "$1"
    oshibki=$((oshibki + 1))
}

otm=/etc/affory
prog=/usr/local/sbin/vpn-uzel
yunit=/etc/systemd/system/vpn-uzel.service
fsys=/etc/sysctl.d/90-affory.conf
fsshd=/etc/ssh/sshd_config.d/0-affory.conf
ak=/root/.ssh/authorized_keys
telo_re='^[A-Za-z0-9+/]+=*$'

[ "$(id -u)" -eq 0 ] || {
    echo "запускать от root" >&2
    exit 1
}
# Отметка это сам каталог: установка, оборванная между ключом и sostav.json,
# тоже своя.
if [ ! -d "$otm" ]; then
    v otmetki net
    exit 0
fi
# Та же программа стоит у серверов центра: без этой проверки снос по
# случайной отметке погасил бы чужой VPN.
if [ -e /usr/local/sbin/vpn-nablyudenie ] || grep -qs vpn-centr "$ak"; then
    echo "сервер под центром, не трогаю" >&2
    exit 1
fi

# Служба и программа.
systemctl disable --now vpn-uzel.service >/dev/null 2>&1
if systemctl is-active --quiet vpn-uzel.service; then
    ne_vyshlo "служба vpn-uzel не остановилась"
else
    rm -f -- "$yunit" "$yunit.novyy" /etc/systemd/system/multi-user.target.wants/vpn-uzel.service ||
        ne_vyshlo "юнит vpn-uzel"
    systemctl daemon-reload || ne_vyshlo "daemon-reload"
    systemctl reset-failed vpn-uzel.service >/dev/null 2>&1
    rm -f -- "$prog" "$prog.novyy" || ne_vyshlo "программа vpn-uzel"
    rm -rf -- /var/lib/vpn-uzel /var/lib/affory || ne_vyshlo "каталоги программы"
fi

# Временные правила программы (проверка сертификата и подбор портов): она
# снимает их сама, но могла упасть посреди проверки.
for ipt in iptables ip6tables; do
    command -v "$ipt" >/dev/null || continue
    pravila="$("$ipt" -w 5 -S INPUT 2>/dev/null | grep -E -- '--comment "?affory-(acme|podbor)"?( |$)' | head -n 200)"
    while IFS= read -r p; do
        [ -n "$p" ] || continue
        read -ra chasti <<<"${p#-A }"
        "$ipt" -w 5 -D "${chasti[@]}" 2>/dev/null || ne_vyshlo "$ipt: временное правило"
    done <<<"$pravila"
done

# sysctl: живые значения по снимку. Значение, которое после нас поменял
# кто-то другой, не трогается. Наш файл уходит только после очереди и без
# ошибок: по нему повтор сноса знает, что возвращать.
prezhnee() { awk -v k="$1" 'index($0, k "=") == 1 { print substr($0, length(k) + 2); exit }' "$otm/prezhnee" 2>/dev/null; }
menyali_ochered=""
if [ -f "$fsys" ]; then
    nashi="$(awk -F' = ' '/^[a-z]/ { print $1 "=" $2 }' "$fsys")"
    case "$nashi" in *net.core.default_qdisc=*) menyali_ochered=1 ;; esac
    for para in $nashi; do
        k="${para%%=*}" nashe="${para#*=}"
        bylo="$(prezhnee "$k")"
        [ -n "$bylo" ] || continue
        [ "$(sysctl -n "$k" 2>/dev/null)" = "$nashe" ] || continue
        sysctl -q -w "$k=$bylo" >/dev/null 2>&1 || ne_vyshlo "sysctl $k не вернулся к $bylo"
    done
fi

# Очередь интерфейса, когда до нас она была ядерной: пересадка (см.
# ochered.sh) по уже возвращённому default_qdisc возвращает и корень, и
# дочерние очереди mq, которые стали fq. Корень человека - поставленный до
# установки или после неё - установка не трогала, и снос тоже.
dev="$(prezhnee qdisc_dev)" bylo="$(prezhnee qdisc)" nomer="$(prezhnee qdisc_nomer)"
if [ -n "$dev" ] && [ -n "$bylo" ] && [ "$nomer" = "0:" ] && command -v tc >/dev/null; then
    koren="$(koren_iz "$(ochered)")"
    ubrat_ustarevshuyu_otmetku "$koren"
    if [ -n "$koren" ] && koren_ruchnoy "$koren"; then
        :
    elif [ -n "$menyali_ochered" ] || [ "$koren" != "$bylo $nomer" ]; then
        kod=0
        peresadit || kod=$?
        case "$kod" in
        1) ne_vyshlo "отметка пересадки не записалась, очередь $dev не вернулась к $bylo" ;;
        2) ne_vyshlo "очередь $dev не вернулась к $bylo" ;;
        3) ne_vyshlo "очередь $dev не вернулась к $bylo, корень остался pfifo" ;;
        esac
    fi
fi
if [ -f "$fsys" ] && [ "$oshibki" -eq 0 ]; then
    rm -f -- "$fsys" || ne_vyshlo "файл sysctl"
fi

# Наш файл sshd (вход по паролю выключали мы): без него пароль вернётся.
# Файл сначала отодвигается и уходит, только когда sshd принял настройку без
# него и перечитал её: иначе повтор сноса не нашёл бы файла и не перечитал
# бы sshd. Отодвинутый остался от оборванного сноса - он возвращается.
[ ! -f "$fsshd.snos" ] || [ -f "$fsshd" ] || mv -f -- "$fsshd.snos" "$fsshd"
if [ -f "$fsshd" ]; then
    sshd="$(command -v sshd || echo /usr/sbin/sshd)"
    if ! mv -f -- "$fsshd" "$fsshd.snos"; then
        ne_vyshlo "файл sshd"
    elif ! "$sshd" -t 2>/dev/null; then
        mv -f -- "$fsshd.snos" "$fsshd"
        ne_vyshlo "sshd -t отвергает настройку и без нашего файла"
    elif ! systemctl try-reload-or-restart ssh.service 2>/dev/null && ! systemctl try-reload-or-restart sshd.service 2>/dev/null; then
        mv -f -- "$fsshd.snos" "$fsshd"
        ne_vyshlo "sshd не перечитал настройку"
    else
        rm -f -- "$fsshd.snos"
        v parol vernulsya
    fi
fi

if [ "$oshibki" -gt 0 ]; then
    v ostalos "$oshibki"
    exit 1
fi

klyuchi="$(awk -v re="$telo_re" '$0 ~ re' "$otm/klyuchi" 2>/dev/null)"
rm -rf -- "$otm" || {
    v ne_vyshlo "отметка $otm"
    exit 1
}
v otmetka snyata

# Строки наших ключей последними: до этого места снос можно повторить.
if [ -n "$klyuchi" ] && [ -f "$ak" ]; then
    t="$ak.affory-chast"
    trap 'rm -f -- "$t"' EXIT
    if cp -p -- "$ak" "$t" && { grep -vF -f <(printf '%s\n' "$klyuchi") -- "$ak" > "$t" || [ $? -eq 1 ]; } && mv -f -- "$t" "$ak"; then
        v klyuchi snyaty
    else
        v ne_vyshlo "строки ключей в $ak"
        exit 4
    fi
fi
v gotovo da
