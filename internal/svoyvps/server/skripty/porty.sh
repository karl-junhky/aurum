# Приводит наши правила ufw к портам окончательного пакета: недостающие
# открывает, лишние наши закрывает. Наши правила несут пометку affory-vhod,
# снимаются только они.
#
#   porty.sh <tcp через запятую> <udp через запятую>     пустые: закрыть все наши
#
# Открываются порты только при включённом ufw: без него машина и так
# открыта, а любое правило в выключенном ufw переписывает его файлы
# user.rules из заводского вида в свой (24.04, 01.11), и снос не вернул бы
# их байт в байт. Наши правила снимаются в обоих состояниях: ufw могли
# выключить после установки. Список правил берётся из `ufw show added`, он
# отвечает в обоих состояниях, а `ufw status` у выключенного пуст. Удаление
# по описанию снимает правило вместе с пометкой и сразу в IPv4 и IPv6.
#
# Чужое правило на тот же порт не трогается: `ufw allow` поверх него
# переписал бы ему пометку (проверено 01.11), и снос потом снял бы правило
# человека. Открыто чужим - порт уже открыт; запрещено чужим - отказ.
set -euo pipefail

pometka=affory-vhod
tcp="${1:-}" udp="${2:-}"
spisok_re='^([0-9]+(,[0-9]+)*)?$'
[[ "$tcp" =~ $spisok_re ]] && [[ "$udp" =~ $spisok_re ]] || {
    echo "порты: только числа через запятую" >&2
    exit 1
}
if ! command -v ufw >/dev/null; then
    printf 'ufw\tnet\n'
    exit 0
fi
# Вывод в переменную, а не в grep -q: под pipefail ранний выход grep
# ронял бы ufw по SIGPIPE, и включённый читался бы выключенным. Упасть
# status может только у включённого (выключенный iptables не спрашивает),
# поэтому сбой это отказ, а не «выключен». Предупреждения ufw идут в тот же
# вывод, поэтому строка состояния ищется среди всех.
vklyuchen=""
if ! sostoyanie="$(ufw status 2>&1)"; then
    echo "ufw status не отработал: ${sostoyanie##*$'\n'}" >&2
    exit 1
fi
if [[ $'\n'"$sostoyanie"$'\n' == *$'\n'"Status: active"$'\n'* ]]; then
    vklyuchen=1
    printf 'ufw\tactive\n'
else
    printf 'ufw\tinactive\n'
fi

nado=" "
for p in ${tcp//,/ }; do nado="$nado$p/tcp "; done
for p in ${udp//,/ }; do nado="$nado$p/udp "; done

# Простые правила «ufw <действие> [in] <порты>[/<протокол>] [comment '...']»
# по строке: действие, порты, пометка. Порты это число, список «80,443» или
# диапазон «2000:3000»; список и диапазон ufw принимает только с
# протоколом. Правила с адресом, интерфейсом или на выход порт для всех не
# открывают и не закрывают. Профили приложений («OpenSSH», «Nginx Full») не
# разбираются: их порты это SSH и веб-серверы, а веб-сервер осмотр и так
# считает чужим и останавливает установку.
pravila() {
    ufw show added | awk -v q="'" '
        $1 != "ufw" { next }
        {
            pm = ""
            s = $0
            if (match(s, " comment " q ".*" q "$")) {
                pm = substr(s, RSTART + 10, RLENGTH - 11)
                s = substr(s, 1, RSTART - 1)
            }
            n = split(s, f, " ")
            j = 3
            if (f[j] == "in") j++
            if (j != n || f[2] !~ /^(allow|deny|reject|limit)$/ || f[j] !~ /^[0-9]+([:,][0-9]+)*(\/(tcp|udp))?$/) next
            print f[2] "\t" f[j] "\t" pm
        }'
}

# kasaetsya <порты правила> <порт/протокол>: правило «443», «443/tcp»,
# «80,443/tcp» или «2000:3000/tcp» касается нужного порта. Правило без
# протокола («ufw allow 443») касается обоих.
kasaetsya() {
    local porty="${1%/*}" proto="" n="${2%/*}" chast
    [[ "$1" != */* ]] || proto="${1#*/}"
    [ -z "$proto" ] || [ "$proto" = "${2#*/}" ] || return 1
    for chast in ${porty//,/ }; do
        if [[ "$chast" == *:* ]]; then
            if [ "$n" -ge "${chast%%:*}" ] && [ "$n" -le "${chast##*:}" ]; then return 0; fi
        elif [ "$chast" = "$n" ]; then
            return 0
        fi
    done
    return 1
}

# Сначала решение по каждому порту, правка потом: запрет человека на одном
# порту не должен оставить ufw наполовину переделанным.
tekushchie="$(pravila)"
uzhe="" dobavit="" est=""
for kuda in $nado; do
    [ -n "$vklyuchen" ] || break
    otkryt_chuzhim=""
    nashe=""
    while IFS=$'\t' read -r d port pm; do
        [ -n "$d" ] || continue
        kasaetsya "$port" "$kuda" || continue
        if [ "$pm" = "$pometka" ]; then
            nashe=1
        elif [ "$d" = allow ]; then
            otkryt_chuzhim=1
        else
            echo "в ufw своё правило на $kuda: $d" >&2
            exit 1
        fi
    done <<<"$tekushchie"
    if [ -n "$nashe" ]; then
        est="$est $kuda"
    elif [ -n "$otkryt_chuzhim" ]; then
        uzhe="$uzhe $kuda"
    else
        dobavit="$dobavit $kuda"
    fi
done
for kuda in $uzhe; do printf 'uzhe_otkryt\t%s\n' "$kuda"; done
for kuda in $dobavit; do ufw allow "$kuda" comment "$pometka" >/dev/null; done
for kuda in $est $dobavit; do printf 'otkryt\t%s\n' "$kuda"; done

# Наши правила на порты, которых больше нет в пакете.
while IFS=$'\t' read -r d port pm; do
    [ "$pm" = "$pometka" ] || continue
    case "$nado" in *" $port "*) continue ;; esac
    ufw delete "$d" "$port" >/dev/null
    printf 'zakryt\t%s\n' "$port"
done <<<"$(pravila)"
