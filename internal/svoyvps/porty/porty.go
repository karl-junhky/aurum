// Package porty подбирает порты своего VPN из сети человека (§3.3 плана).
//
// Выводы про порты снимались на одном компьютере у одного провайдера, у
// другого человека бывает наоборот, поэтому порты не зашиваются. Кандидаты
// идут от самых обычных, отсеиваются по осмотру сервера, встают на сервер
// пробным пакетом с теми секретами, что уйдут в ключи, и проверяются
// трафиком с компьютера человека. Берётся первый прошедший по порядку: Avast,
// чужой VPN и заморозка провайдера отсекают свои порты сами, потому что
// проба идёт тем же путём, что настоящий ключ.
//
// Подбор мерит сеть компьютера: телефон на мобильном может вести себя иначе.
package porty

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
)

// Кандидаты от самых обычных. Список правится выпуском клиента.
var (
	kandidatyTCP = []int{443, 8443, 465, 993, 2053, 9443}
	kandidatyUDP = []int{443, 8443, 11444, 5443, 2053}
)

const (
	transportReality = "reality-tcp"
	transportHy2     = "hy2"
	// portSsh отсеивается всегда, даже если sshd сейчас слушает другой: на
	// нём sshd встанет после любой правки конфига к заводскому.
	portSsh = 22
	// portHTTP нужен выпуску сертификата проверкой HTTP-01.
	portHTTP = 80
	// portTLS нужен выпуску проверкой TLS-ALPN-01.
	portTLS = 443
)

// Prodlenie это чем программа сервера продлевает сертификат hy2. Нулевое
// значение негодно: забытое поле не должно молча выключать hy2.
type Prodlenie int

const (
	// BezSerta: сертификата нет (закрыты 80 и 443/tcp), hy2 не ставится.
	BezSerta Prodlenie = iota + 1
	// Cherez80: сертификат выпущен проверкой HTTP-01, 443/tcp может взять
	// reality.
	Cherez80
	// Cherez443: выпущен проверкой TLS-ALPN-01 или путь неизвестен (стоит
	// копия прошлой установки). Займи reality 443/tcp, продлению не останется
	// ни одного пути.
	Cherez443
)

// Prichina это почему порт не годится.
type Prichina string

const (
	PrichinaVne    Prichina = "vne"    // вне 1-65535
	PrichinaSsh    Prichina = "ssh"    // на нём вход SSH
	PrichinaSert   Prichina = "sert"   // нужен выпуску или продлению сертификата
	PrichinaZanyat Prichina = "zanyat" // слушает чужая программа, она в Process
)

// Otsev это порт, который не идёт в подбор.
type Otsev struct {
	Proto    string // tcp или udp
	Port     int
	Prichina Prichina
	// Process называет занявшую порт программу; «?», если ss её не назвал.
	Process string
}

func (o Otsev) Error() string {
	s := fmt.Sprintf("порт %d/%s не годится: %s", o.Port, o.Proto, o.Prichina)
	if o.Process != "" {
		s += " " + o.Process
	}
	return s
}

// Ruchnye это порты, заданные руками в «Дополнительно»; 0 - подбирать.
type Ruchnye struct {
	Reality int // TCP
	Hy2     int // UDP
}

// Spisok это что пойдёт в пробный пакет.
type Spisok struct {
	// Reality и Hy2 это кандидаты по порядку проверки. Hy2 пустой при
	// BezSerta.
	Reality, Hy2 []int
	// Otseyany это кандидаты списка, выпавшие по осмотру, для хода подбора.
	Otseyany []Otsev
}

// ErrNetKandidatov: все кандидаты вида отсеяны осмотром, порт задаётся
// руками.
var ErrNetKandidatov = errors.New("свободного кандидата нет")

// Kandidaty отбирает кандидатов по осмотру сервера o (§3.3, шаг 2). Порт,
// заданный руками, идёт один вместо списка, и негодный это ошибка Otsev, а не
// тихая замена подбором. При BezSerta порт hy2 не нужен, заданный руками
// тоже: о том, что hy2 не встанет, человеку говорит выпуск сертификата.
func Kandidaty(o *server.Osmotr, pr Prodlenie, r Ruchnye) (_ Spisok, err error) {
	defer zashchita("Kandidaty", &err)
	if o == nil {
		return Spisok{}, errors.New("подбор портов: нет осмотра сервера")
	}
	if pr < BezSerta || pr > Cherez443 {
		return Spisok{}, fmt.Errorf("подбор портов: не назван способ продления сертификата (%d)", pr)
	}
	var s Spisok
	vid := func(proto string, ruchnoy int, kandidaty []int) ([]int, error) {
		if ruchnoy != 0 {
			if ots, ok := otseyat(o, pr, proto, ruchnoy); !ok {
				return nil, ots
			}
			return []int{ruchnoy}, nil
		}
		var godnye []int
		for _, port := range kandidaty {
			if ots, ok := otseyat(o, pr, proto, port); !ok {
				s.Otseyany = append(s.Otseyany, ots)
				continue
			}
			godnye = append(godnye, port)
		}
		if len(godnye) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrNetKandidatov, proto)
		}
		return godnye, nil
	}
	if s.Reality, err = vid("tcp", r.Reality, kandidatyTCP); err != nil {
		return Spisok{}, err
	}
	if pr != BezSerta {
		if s.Hy2, err = vid("udp", r.Hy2, kandidatyUDP); err != nil {
			return Spisok{}, err
		}
	}
	return s, nil
}

// otseyat решает об одном порте: ok - годен.
func otseyat(o *server.Osmotr, pr Prodlenie, proto string, port int) (Otsev, bool) {
	ots := Otsev{Proto: proto, Port: port}
	switch {
	case port < 1 || port > 65535:
		ots.Prichina = PrichinaVne
	case proto == "tcp" && (port == portSsh || slices.Contains(portySsh(o), port)):
		ots.Prichina = PrichinaSsh
	// 80 держится и без сертификата: reality на нём закрыл бы выпуск и
	// после того, как человек откроет 80 у хостера.
	case proto == "tcp" && (port == portHTTP || port == portTLS && pr == Cherez443):
		ots.Prichina = PrichinaSert
	default:
		if p := o.Zanyat(proto, port); p != "" {
			ots.Prichina, ots.Process = PrichinaZanyat, p
		}
	}
	return ots, ots.Prichina == ""
}

// portySsh это порты sshd из sshd -T; несколько идут через пробел.
func portySsh(o *server.Osmotr) []int {
	var p []int
	for _, z := range strings.Fields(o.Sshd["port"]) {
		if n, err := strconv.Atoi(z); err == nil {
			p = append(p, n)
		}
	}
	return p
}

// zapasDostavki это срок пробы сверх самой проверки: пробный пакет
// применяется на сервере до начала проверки, окончательный едет после.
const zapasDostavki = 3 * time.Minute

// SrokProby это срок пробного пакета на кандидатов v в секундах: программа
// сервера держит пробу столько и возвращает принятое, если окончательный
// пакет не пришёл (клиент закрыли посреди подбора).
func SrokProby(v konfig.Vhody) int {
	vidov := 0
	for _, porty := range [][]int{v.Reality, v.Hy2} {
		if len(porty) > 0 {
			vidov++
		}
	}
	d := proverka.Predel(len(v.Reality)+len(v.Hy2), vidov) + zapasDostavki
	return int(math.Ceil(d.Seconds()))
}

// Vybor это итог подбора: порт каждого вида и проверки в порядке хода.
// Порт 0 значит одно из двух, и различает их только список кандидатов:
// кандидаты вида были, и ни один не прошёл (слово за детектором
// блокировки), или кандидатов вида не было вовсе (hy2 при BezSerta).
type Vybor struct {
	Reality, Hy2 int
	Proverki     []proverka.Itog
}

// ErrNeReshil: кандидат вида получил ответ, но не тот (цель ответила не так
// или выход не с адреса сервера), а везущего нет. О портах это ничего не
// говорит, и «не прошёл ни один» было бы неправдой.
var ErrNeReshil = errors.New("подбор портов не решил")

// proverit это проверка ключей; тесты подбора ставят свою.
var proverit = proverka.Proverit

// Podobrat проверяет кандидатов v из сети человека: пробный пакет с ними уже
// применён на сервере, s те же секреты. По каждому виду берётся первый
// кандидат, что везёт, дальше вид не проверяется. Ход подбора идёт в o.Hod.
func Podobrat(ctx context.Context, s konfig.Sekrety, v konfig.Vhody, o proverka.Opcii, zapusk proverka.Zapusk) (_ Vybor, err error) {
	defer zashchita("Podobrat", &err)
	k, err := konfig.Klyuchi(s, v)
	if err != nil {
		return Vybor{}, err
	}
	klyuchi := make([]protokol.Server, len(k))
	for i, x := range k {
		klyuchi[i] = x.Server
	}
	o.PervyyNaVid = true
	itogi, err := proverit(ctx, klyuchi, o, zapusk)
	if err != nil {
		return Vybor{}, err
	}
	vybor := Vybor{Proverki: itogi}
	for _, vid := range []struct {
		transport string
		port      *int
	}{{transportReality, &vybor.Reality}, {transportHy2, &vybor.Hy2}} {
		var sOtvetom *proverka.Itog
		for i, it := range itogi {
			if it.Server.Transport != vid.transport {
				continue
			}
			if it.Vid == proverka.Vezyot {
				*vid.port = it.Server.Port
				break
			}
			if sOtvetom == nil && (it.Vid == proverka.Cel || it.Vid == proverka.ChuzhoyVyhod) {
				sOtvetom = &itogi[i]
			}
		}
		if *vid.port == 0 && sOtvetom != nil {
			return Vybor{}, fmt.Errorf("%w: %s: %s", ErrNeReshil, sOtvetom.Server.Imya, sOtvetom.Otkaz)
		}
	}
	return vybor, nil
}

// zashchita ставится первой строкой каждого входа: пакет живёт в службе.
func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/porty."+gde, r)
		*err = fmt.Errorf("паника в porty.%s: %v", gde, r)
	}
}
