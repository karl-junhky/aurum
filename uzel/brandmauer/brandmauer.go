// Пакет brandmauer ставит и снимает временные разрешения входящих в начале
// цепочки INPUT тем iptables, что стоит в системе (nft или legacy, тот же,
// что у ufw). Своя таблица nft не годится: accept в своей таблице не
// отменяет drop в таблице ufw.
//
// Каждое правило несёт пометку в комментарии: по ней снимаются все правила
// одного назначения разом, в том числе оставшиеся от упавшей программы.
// Нет iptables - нет и брандмауэра, который надо открывать (26.04 из
// минимального образа).
package brandmauer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// predelPravil это сколько правил с одной пометкой снимается за раз: больше
// не бывает, а бесконечный цикл на чужом выводе хуже недоснятого правила.
const predelPravil = 200

// Otkryt пускает входящие на порт: proto это tcp или udp, v6 выбирает
// ip6tables.
func Otkryt(v6 bool, proto string, port int, pometka string) error {
	if proto != "tcp" && proto != "udp" {
		return fmt.Errorf("брандмауэр: протокол %q, ждали tcp или udp", proto)
	}
	komanda := "iptables"
	if v6 {
		if !estIPv6() {
			return nil
		}
		komanda = "ip6tables"
	}
	put, err := exec.LookPath(komanda)
	if err != nil {
		return nil
	}
	_, err = zapustit(put, "-w", "5", "-I", "INPUT", "1", "-p", proto, "--dport", strconv.Itoa(port),
		"-m", "comment", "--comment", pometka, "-j", "ACCEPT")
	return err
}

// Ubrat снимает из INPUT все правила с пометкой, в iptables и ip6tables.
func Ubrat(pometka string) error {
	var oshibki []error
	for _, komanda := range []string{"iptables", "ip6tables"} {
		if komanda == "ip6tables" && !estIPv6() {
			continue
		}
		put, err := exec.LookPath(komanda)
		if err != nil {
			continue
		}
		vyvod, err := zapustit(put, "-w", "5", "-S", "INPUT")
		if err != nil {
			oshibki = append(oshibki, err)
			continue
		}
		pravila := sPometkoy(vyvod, pometka)
		if len(pravila) > predelPravil {
			oshibki = append(oshibki, fmt.Errorf("%s: правил %s %d, снимаю первые %d", komanda, pometka, len(pravila), predelPravil))
			pravila = pravila[:predelPravil]
		}
		for _, p := range pravila {
			if _, err := zapustit(put, append([]string{"-w", "5", "-D"}, p...)...); err != nil {
				oshibki = append(oshibki, err)
			}
		}
	}
	return errors.Join(oshibki...)
}

// estIPv6: IPv6 выключен в ядре (ipv6.disable=1) - нет и его брандмауэра,
// а ip6tables на такой машине отвечает ошибкой на любую команду.
func estIPv6() bool {
	_, err := os.Stat("/proc/net/if_inet6")
	return err == nil
}

// sPometkoy находит в выводе `iptables -S INPUT` правила с пометкой и отдаёт
// их без ключа -A: «INPUT -p tcp ... -j ACCEPT», готовыми для -D. Пометка
// без пробелов, поэтому iptables печатает её без кавычек.
func sPometkoy(vyvod, pometka string) [][]string {
	var itog [][]string
	for _, stroka := range strings.Split(vyvod, "\n") {
		polya := strings.Fields(stroka)
		if len(polya) < 2 || polya[0] != "-A" {
			continue
		}
		for i := 1; i+1 < len(polya); i++ {
			if polya[i] == "--comment" && polya[i+1] == pometka {
				itog = append(itog, polya[1:])
				break
			}
		}
	}
	return itog
}

// zapustit отдаёт ошибку с выводом команды, ExitError остаётся внутри.
func zapustit(put string, argumenty ...string) (string, error) {
	ctx, otmena := context.WithTimeout(context.Background(), 15*time.Second)
	defer otmena()
	vyvod, err := exec.CommandContext(ctx, put, argumenty...).CombinedOutput()
	if err != nil {
		return string(vyvod), fmt.Errorf("%s %s: %w: %s", put, strings.Join(argumenty, " "), err, strings.TrimSpace(string(vyvod)))
	}
	return string(vyvod), nil
}
