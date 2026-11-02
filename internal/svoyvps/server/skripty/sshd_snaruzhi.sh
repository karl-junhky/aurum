# Общий кусок снятия ключа и переключения пароля: настройки sshd глазами
# входа пользователя $1 (по умолчанию root) снаружи. Так применяются блоки
# Match. Адрес из документационного диапазона не попадёт ни под какой Match
# Address местных сетей, как и человек снаружи. Свой адрес и порт sshd
# берутся из SSH_CONNECTION этого входа, чтобы сработали Match LocalAddress
# и LocalPort. sshd -T не ответил - вывода нет.

sshd="$(command -v sshd || echo /usr/sbin/sshd)"

sshd_snaruzhi() {
    local spec="user=${1:-root},host=192.0.2.1,addr=192.0.2.1" la lp
    read -r _ _ la lp _ <<< "${SSH_CONNECTION:-}" || true
    if [[ "${la:-}" =~ ^[0-9A-Fa-f:.]+$ && "${lp:-}" =~ ^[0-9]+$ ]]; then
        spec="$spec,laddr=$la,lport=$lp"
    fi
    "$sshd" -T -C "$spec" 2>/dev/null
}
