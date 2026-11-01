package server

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	putProgrammy = "/usr/local/sbin/vpn-uzel"
	// PredelLovushki это сколько секунд ловушка программы сервера может
	// ждать (PredelSekund в uzel/lovushka).
	PredelLovushki = 30
)

// Lovit спрашивает программу сервера, сколько попыток подключения пришло на
// порт за sekund секунд: SYN для tcp, датаграмм для udp. Пакеты ловятся до
// брандмауэра сервера. Ответ приходит по истечении срока, поэтому ctx обязан
// жить дольше sekund. Ловушка считает всех отправителей, адреса здесь не
// отдаются: они чужие и в журнал не идут.
func Lovit(ctx context.Context, m Mashina, proto string, port, sekund int) (vsego int64, err error) {
	defer zashchita("Lovit", &err)
	switch {
	case proto != "tcp" && proto != "udp":
		return 0, fmt.Errorf("ловушка: протокол %q", proto)
	case port < 1 || port > 65535:
		return 0, fmt.Errorf("ловушка: порт %d вне 1-65535", port)
	case sekund < 1 || sekund > PredelLovushki:
		return 0, fmt.Errorf("ловушка: %d с, можно от 1 до %d", sekund, PredelLovushki)
	}
	it, err := m.Vypolnit(ctx, fmt.Sprintf("%s pult lovushka %s %d %d", putProgrammy, proto, port, sekund), nil)
	if err != nil {
		return 0, err
	}
	var o struct {
		Vsego   *int64 `json:"vsego"`
		Oshibka string `json:"oshibka"`
	}
	if jerr := json.Unmarshal(it.Vyvod, &o); jerr != nil {
		if it.Kod != 0 {
			// Программы нет или пульт не поднят: ответа JSON не будет.
			return 0, oshibkaSkripta("lovushka", it)
		}
		return 0, &Oshibka{Shag: "lovushka", Tekst: "ответ ловушки не разобран: " + chisto(jerr.Error())}
	}
	if o.Oshibka != "" {
		return 0, &Oshibka{Shag: "lovushka", Kod: it.Kod, Tekst: chisto(o.Oshibka)}
	}
	if it.Kod != 0 {
		return 0, oshibkaSkripta("lovushka", it)
	}
	if o.Vsego == nil {
		return 0, &Oshibka{Shag: "lovushka", Tekst: "в ответе ловушки нет счёта"}
	}
	if *o.Vsego < 0 {
		return 0, &Oshibka{Shag: "lovushka", Tekst: fmt.Sprintf("ловушка насчитала %d", *o.Vsego)}
	}
	return *o.Vsego, nil
}
