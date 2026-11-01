# Осмотр машины для своего VPN: только чтение, на машине ничего не меняется
# и не ставится. Печатает строки «ключ<TAB>значение», значение в одну строку;
# разбирает osmotr.go. Ключи утилит общие для GNU и uutils (26.04).
set -u

v() { printf '%s\t%s\n' "$1" "$(printf '%s' "$2" | tr '\t\r\n' '   ')"; }
est() { command -v "$1" >/dev/null 2>&1; }

# Система. os-release читается в подоболочке: его переменные не должны
# перекрыть переменные скрипта.
if [ -r /etc/os-release ]; then
    (
        . /etc/os-release
        v os "${ID:-}"
        v os_versiya "${VERSION_ID:-}"
        v os_imya "${PRETTY_NAME:-}"
    )
fi
v arch "$(uname -m)"
v pid1 "$(cat /proc/1/comm 2>/dev/null)"
# Без виртуализации systemd-detect-virt печатает none и выходит с 1.
v virt "$(systemd-detect-virt 2>/dev/null || true)"
v pamyat_mb "$(awk '/^MemAvailable:/ { print int($2 / 1024) }' /proc/meminfo 2>/dev/null)"
v disk_mb "$(df -Pm /var/lib 2>/dev/null | awk 'NR == 2 { print $4 }')"
v chasy "$(date +%s)"
if est timedatectl; then v ntp "$(timedatectl show -p NTPSynchronized --value 2>/dev/null)"; fi
if est cloud-init; then v cloud_init "$(cloud-init status 2>/dev/null | sed -n 's/^status: //p')"; fi

# Утилиты, без которых установка и снос не обходятся, и полезные.
for u in bash systemctl sysctl ip ss awk sed grep cmp sha256sum timeout tc openssl; do
    est "$u" || v net_utility "$u"
done

# Адреса и маршрут по умолчанию. Адрес точка-точка ip пишет без маски
# («inet 203.0.113.10 peer 203.0.113.1/32»): свой адрес это один узел.
ip -4 -o addr show scope global 2>/dev/null | while read -r _ dev _ adres slovo _; do
    [ "$slovo" != peer ] || adres="${adres%/*}/32"
    v ipv4 "$dev $adres"
done
ip -6 -o addr show scope global 2>/dev/null | while read -r _ dev _ adres slovo _; do
    [ "$slovo" != peer ] || adres="${adres%/*}/128"
    v ipv6 "$dev $adres"
done
dev4="$(ip -4 route show default 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
v marshrut4 "$dev4"
v marshrut6 "$(ip -6 route show default 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
# Очередь интерфейса из ip link: tc на 20.04 не понимает «root» в показе.
if [ -n "$dev4" ]; then
    v qdisc "$(ip -o link show dev "$dev4" 2>/dev/null | sed -n 's/.* qdisc \([^ ]*\) .*/\1/p')"
fi

# Сеть ядра.
for k in net.core.rmem_max net.core.wmem_max net.core.rmem_default net.core.default_qdisc net.ipv4.tcp_congestion_control; do
    v sysctl "$k=$(sysctl -n "$k" 2>/dev/null)"
done
if grep -qw bbr /proc/sys/net/ipv4/tcp_available_congestion_control 2>/dev/null; then
    v bbr zagruzhen
elif modinfo tcp_bbr >/dev/null 2>&1 || find "/lib/modules/$(uname -r)" -name 'tcp_bbr.ko*' 2>/dev/null | grep -q .; then
    v bbr modul
else
    v bbr net
fi

# Кто слушает: протокол, адрес:порт, процесс. Упавший ss назван отдельно:
# пустой список значил бы «все порты свободны».
if est ss; then
    for proto in tcp udp; do
        if spisok="$(ss -Hl"${proto:0:1}"np 2>/dev/null)"; then
            [ -z "$spisok" ] || printf '%s\n' "$spisok" | while read -r _ _ _ lok _ proc; do v slushaet "$proto $lok ${proc:-}"; done
        else
            v sboy "ss $proto"
        fi
    done
fi

# Брандмауэр: ufw, политика входа в nft и в iptables.
if est ufw; then
    v ufw "$(ufw status 2>/dev/null | sed -n 's/^Status: //p')"
else
    v ufw net
fi
if est nft; then
    nft list chains 2>/dev/null | awk '
        /^table / { t = $2 " " $3 }
        /chain / { c = $2 }
        /hook input/ {
            p = "accept"
            if (match($0, /policy [a-z]+/)) p = substr($0, RSTART + 7, RLENGTH - 7)
            print t " " c " " p
        }' | while IFS= read -r s; do v nft_input "$s"; done
fi
if est iptables; then
    v iptables_input "$(iptables -S INPUT 2>/dev/null | sed -n 's/^-P INPUT //p')"
fi

# sshd.
for f in /etc/ssh/sshd_config.d/*.conf; do
    [ -e "$f" ] && v sshd_fayl "${f##*/}"
done
sshd="$(command -v sshd || echo /usr/sbin/sshd)"
"$sshd" -T 2>/dev/null | awk '$1 == "port" || $1 == "permitrootlogin" || $1 == "passwordauthentication" || $1 == "kbdinteractiveauthentication" { print $1, $2 }' |
    while read -r k z; do v "sshd_$k" "$z"; done

# Своё и чужое. Чужие службы ищутся по известным именам: на образах хостеров
# в /etc/systemd/system лежат свои файлы (snap.lxd на 20.04), и это не
# чужая установка.
# Отметка это сам каталог, как у сноса: без sostav.json она «испорчена».
[ -d /etc/affory ] && v otmetka "$(head -c 4096 /etc/affory/sostav.json 2>/dev/null)"
[ -e /usr/local/sbin/vpn-uzel ] && v vpn_uzel "$(timeout 5 /usr/local/sbin/vpn-uzel versiya 2>/dev/null || echo '?')"
grep -qs 'vpn-centr' /root/.ssh/authorized_keys && v centr klyuch
[ -e /usr/local/sbin/vpn-nablyudenie ] && v centr nablyudenie
for p in docker nginx apache2 httpd caddy haproxy xray v2ray sing-box hysteria x-ui marzban hiddify outline-ss-server ss-server ssserver openvpn trojan trojan-go naive mtg; do
    est "$p" && v chuzhoe "programma $p"
done
for d in /opt/amnezia /opt/marzban /opt/hiddify-manager /opt/hiddify-config /opt/outline /usr/local/x-ui /etc/x-ui /etc/xray /usr/local/etc/xray /etc/sing-box /etc/hysteria /etc/v2ray /etc/openvpn/server /etc/wireguard; do
    [ -n "$(ls -A "$d" 2>/dev/null)" ] && v chuzhoe "katalog $d"
done
{
    systemctl list-units --type=service --state=active --no-legend --plain 2>/dev/null
    systemctl list-unit-files --type=service --state=enabled --no-legend 2>/dev/null
} | awk '{ print $1 }' | grep -E '^(xray|v2ray|sing-box|hysteria|x-ui|3x-ui|marzban|hiddify|outline|shadowsocks|ss-server|trojan|naive|amnezia|wg-quick@|openvpn|nginx|caddy|apache2|httpd|haproxy|docker)' |
    sort -u | while read -r u; do v chuzhoe "yunit $u"; done
# Последней строкой: без неё вывод оборван, и пустые поля значили бы «нет».
v konec da
exit 0
