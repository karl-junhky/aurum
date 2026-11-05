# Обновляет программу своего VPN на поставленном сервере (З2). Идёт от root.
#
#   obnovlenie.sh nachalo      каталог загрузки и то же, что sostoyanie
#   obnovlenie.sh sostoyanie   sha256 и версия программы на месте, подтверждена ли
#   obnovlenie.sh zamenit     новая программа из загрузки: проверка на живом
#                              пакете, копия прежней, таймер отката, подмена,
#                              перезапуск службы руками systemd
#   obnovlenie.sh podtverdit   клиент проверил новую: таймер снят, копия прежней
#                              убрана, версия в отметке
#   obnovlenie.sh otkatit      проверка клиента не прошла: прежняя на место,
#                              перезапуск, таймер снят
#
# Перезапуск ставится в очередь systemd и идёт без нас: VPN человека может
# идти через этот же сервер, и вместе с программой рвётся и этот SSH. Клиент
# входит заново и проверяет. Не дождался таймер подтверждения за 10 минут -
# возвращает прежнюю сам. Таймер, подтверждение и откат берут один замок:
# кто первым, тот и решил. Таймер systemd-run пропадает с перезагрузкой
# сервера, поэтому неподтверждённую программу выдаёт копия прежней: она
# лежит, пока новую не подтвердили или не откатили.
#
# Юнит службы не меняется: обновляется только программа.
# Печатает строки «ключ<TAB>значение». Отказ - код 1 и причина в stderr.
set -euo pipefail
# Отметка и копии только для root, как у установки.
umask 077

v() { printf '%s\t%s\n' "$1" "$2"; }
otkaz() {
    echo "$*" >&2
    exit 1
}

otm=/etc/affory
z=/var/lib/affory/zagruzka
prog=/usr/local/sbin/vpn-uzel
prezh=/usr/local/sbin/vpn-uzel.prezhnyaya
katalog=/var/lib/vpn-uzel
paket=/var/lib/vpn-uzel/sostoyanie.json
otkat=affory-obnovlenie-otkat
zamok=/run/affory-obnovlenie.lock
srok_otkata=600

[ "$(id -u)" -eq 0 ] || otkaz "запускать от root"
if [ -e /usr/local/sbin/vpn-nablyudenie ] || grep -qs vpn-centr /root/.ssh/authorized_keys; then
    otkaz "сервер под центром, не трогаю"
fi
[ -f "$otm/sostav.json" ] || otkaz "отметки установки нет"
grep -q '"sostoyanie":"gotovo"' "$otm/sostav.json" || otkaz "установка не закончена"
[ -x "$prog" ] || otkaz "программы нет на месте"

# Обрыв SSH посреди дела не должен оборвать и его: таймер без подмены или
# подмена без таймера хуже любого исхода.
trap '' HUP

# Программа, упавшая на versiya, даёт пустоту, а не конец скрипта по set -e:
# о пустоте говорит понятный отказ.
versiya_iz() { { "$1" versiya 2>/dev/null || true; } | sed 's/[^A-Za-z0-9., -]//g'; }

# Таймер идёт: ждёт срока или его служба уже возвращает прежнюю. Сработавший
# так и числится active (elapsed), поэтому судит подсостояние.
otkat_idyot() {
    case "$(systemctl show -p SubState "$otkat.timer" 2>/dev/null)" in
    SubState=waiting | SubState=running) return 0 ;;
    esac
    return 1
}

# Новая не подтверждена: таймер отката идёт или пропал с перезагрузкой, а
# копия прежней ещё лежит.
nepodtverzhdena() { otkat_idyot || [ -f "$prezh" ]; }

# Таймера может не быть: тогда stop и reset-failed отвечают ошибкой, и это
# не отказ. Судит итог: таймер не должен работать. Его служба снимается
# тоже: сработавшая ждёт замка, который держим мы, и решать ей уже нечего,
# а живая не даст завести таймер с тем же именем.
snyat_otkat() {
    systemctl stop "$otkat.timer" "$otkat.service" >/dev/null 2>&1 || true
    systemctl reset-failed "$otkat.timer" "$otkat.service" >/dev/null 2>&1 || true
    ! otkat_idyot
}

vzyat_zamok() {
    exec 9>"$zamok"
    flock -w 60 9 || otkaz "замок обновления занят дольше минуты"
}

# Версия в отметке: её читают осмотр и захват с другого компьютера.
zapisat_versiyu() {
    sed "s/\"vpn_uzel\":\"[^\"]*\"/\"vpn_uzel\":\"$1\"/" "$otm/sostav.json" > "$otm/sostav.json.novyy"
    mv -f -- "$otm/sostav.json.novyy" "$otm/sostav.json"
}

sostoyanie() {
    v ustanovleno "$(sha256sum -- "$prog" | awk '{ print $1 }')"
    v versiya "$(versiya_iz "$prog")"
    if nepodtverzhdena; then v otkat idyot; fi
}

nachalo() {
    mkdir -p -- "$z"
    sostoyanie
}

zamenit() {
    # Загрузка нужна только до копии: при любом исходе её место на диске
    # освобождается. Файл .novyy до замка может быть чужим и убирается
    # только под ним.
    trap 'rm -rf -- /var/lib/affory' EXIT
    for u in flock systemd-run systemctl sha256sum; do
        command -v "$u" >/dev/null || otkaz "нет утилиты $u"
    done
    [ -f "$z/vpn-uzel" ] || otkaz "новой программы нет в $z"
    vzyat_zamok
    novyy="$prog.novyy"
    trap 'rm -f -- "$novyy"; rm -rf -- /var/lib/affory' EXIT
    cp -- "$z/vpn-uzel" "$novyy"
    chown root:root -- "$novyy"
    chmod 0755 -- "$novyy"
    versiya="$(versiya_iz "$novyy")"
    [ -n "$versiya" ] || otkaz "новая программа не запускается"
    # Новая обязана принять пакет, на котором сервер работает сейчас: иначе
    # после подмены ядро не поднимется вовсе.
    if [ -f "$paket" ] && ! vyvod="$("$novyy" proverit -katalog "$katalog" "$paket" 2>&1)"; then
        otkaz "новая программа не принимает пакет сервера: $(printf '%s\n' "$vyvod" | tail -n 1)"
    fi
    # Прошлое обновление не подтверждено: на месте непроверенная программа, а
    # проверенная лежит копией. Копия тогда остаётся той же.
    kopiya_est=""
    if [ -f "$prezh" ]; then kopiya_est=1; fi
    snyat_otkat || otkaz "таймер отката прошлого обновления не снялся"
    if [ -z "$kopiya_est" ]; then
        cp -p -- "$prog" "$prezh.chast"
        mv -f -- "$prezh.chast" "$prezh"
    fi
    # Таймер раньше подмены: оборвись всё сразу после, прежняя вернётся сама.
    if ! systemd-run --quiet --unit="$otkat" --on-active="$srok_otkata" --timer-property=AccuracySec=1s \
        /bin/sh -c "exec 9>$zamok; flock 9; [ -f $prezh ] || exit 0; mv -f $prezh $prog; systemctl restart vpn-uzel.service"; then
        # Своя копия равна программе на месте: без неё та не считается
        # неподтверждённой.
        [ -n "$kopiya_est" ] || rm -f -- "$prezh"
        otkaz "таймер отката не завёлся, программа не тронута"
    fi
    mv -f -- "$novyy" "$prog"
    if ! systemctl --no-block restart vpn-uzel.service; then
        # Служба не перезапускалась и работает тем, что стояло до замены.
        if [ -n "$kopiya_est" ]; then
            # Это непроверенная программа прошлого раза. На диск идёт
            # проверенная из копии, а копия и таймер остаются: таймер
            # перезапустит службу ею. Не легла на диск - положит он же.
            if ! { cp -p -- "$prezh" "$prog.chast" && mv -f -- "$prog.chast" "$prog"; }; then
                rm -f -- "$prog.chast"
            fi
            otkaz "перезапуск vpn-uzel не встал в очередь, прежнюю программу вернёт таймер"
        fi
        # Копия равна работающей: на диск её же. Таймер не снимется - копии
        # уже нет, и он ничего не тронет.
        mv -f -- "$prezh" "$prog" || otkaz "перезапуск vpn-uzel не встал в очередь, прежнюю программу вернёт таймер"
        snyat_otkat || true
        otkaz "перезапуск vpn-uzel не встал в очередь, прежняя программа на месте"
    fi
    v versiya "$versiya"
    v otkat "$srok_otkata"
    v gotovo da
}

podtverdit() {
    vzyat_zamok
    # Копии уже не было - таймер успел вернуть прежнюю. Версия это покажет:
    # в отметку идёт та, что на месте. Не назвала версию - подтверждать
    # нечего, и таймер остаётся.
    versiya="$(versiya_iz "$prog")"
    [ -n "$versiya" ] || otkaz "программа на месте не назвала версию, подтверждения нет"
    snyat_otkat || otkaz "таймер отката не снялся"
    rm -f -- "$prezh"
    zapisat_versiyu "$versiya"
    rm -rf -- /var/lib/affory
    v versiya "$versiya"
    v gotovo da
}

otkatit() {
    vzyat_zamok
    snyat_otkat || otkaz "таймер отката не снялся"
    if [ -f "$prezh" ]; then
        mv -f -- "$prezh" "$prog"
        systemctl --no-block restart vpn-uzel.service || otkaz "перезапуск vpn-uzel не встал в очередь"
    fi
    rm -rf -- /var/lib/affory
    v versiya "$(versiya_iz "$prog")"
    v gotovo da
}

case "${1:-}" in
nachalo) nachalo ;;
sostoyanie) sostoyanie ;;
zamenit) zamenit ;;
podtverdit) podtverdit ;;
otkatit) otkatit ;;
*) otkaz "obnovlenie.sh nachalo|sostoyanie|zamenit|podtverdit|otkatit" ;;
esac
