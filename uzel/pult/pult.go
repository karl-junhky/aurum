// Пакет pult это управление программой сервера через локальный сокет.
//
// Снаружи сюда ходит центр, и только так: ssh с ключом, у которого в
// authorized_keys стоит принудительная команда `vpn-uzel pult`. Та читает,
// что центр попросил (SSH_ORIGINAL_COMMAND), и передаёт в сокет работающей
// программы. Клиент своего VPN входит root и зовёт `vpn-uzel pult` сам.
// Своих открытых портов у программы нет.
package pult

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/lovushka"
)

// PredelPaketa это сколько байт пакета принимается. Конфиг на сотню людей
// весит десятки килобайт.
const PredelPaketa = 8 << 20

// PredelZapisey это сколько записей трафика отдаётся за раз.
const PredelZapisey = 1000

// Obrabotchik это то, что умеет работающая программа.
type Obrabotchik interface {
	Sostoyanie() any
	Primenit(paket []byte) any
	Zhurnal(posle uint64, predel int) (any, error)
	Podtverdit(n uint64) error
	Zaprosy() any
	Sert() any
	Lovushka(proto string, port, sekund int) any
}

// Komanda это разобранная просьба центра.
type Komanda struct {
	Imya   string
	Chislo uint64
	// Ловушка: lovushka <tcp|udp> <порт> <секунд>.
	Proto  string
	Port   int
	Sekund int
}

// Razobrat разбирает строку команды. Разрешено ровно семь видов, всё
// остальное отказ: строка приходит по ssh, и лишнего она уметь не должна.
func Razobrat(stroka string) (Komanda, error) {
	polya := strings.Fields(stroka)
	if len(polya) == 0 {
		return Komanda{}, errors.New("пустая команда")
	}
	k := Komanda{Imya: polya[0]}
	switch k.Imya {
	case "sostoyanie", "primenit", "zaprosy", "sert":
		if len(polya) != 1 {
			return Komanda{}, fmt.Errorf("у %s нет аргументов", k.Imya)
		}
	case "zhurnal", "podtverdit":
		if len(polya) != 2 {
			return Komanda{}, fmt.Errorf("%s ждёт номер записи", k.Imya)
		}
		n, err := strconv.ParseUint(polya[1], 10, 64)
		if err != nil {
			return Komanda{}, fmt.Errorf("%s: номер записи не число: %q", k.Imya, polya[1])
		}
		k.Chislo = n
	case "lovushka":
		if len(polya) != 4 {
			return Komanda{}, errors.New("lovushka ждёт протокол, порт и секунды")
		}
		port, errP := strconv.Atoi(polya[2])
		sekund, errS := strconv.Atoi(polya[3])
		if errP != nil || errS != nil {
			return Komanda{}, fmt.Errorf("lovushka: порт и секунды не числа: %q %q", polya[2], polya[3])
		}
		if err := lovushka.Proverit(polya[1], port, sekund); err != nil {
			return Komanda{}, err
		}
		k.Proto, k.Port, k.Sekund = polya[1], port, sekund
	default:
		return Komanda{}, fmt.Errorf("неизвестная команда %q", k.Imya)
	}
	return k, nil
}

type oshibka struct {
	Oshibka string `json:"oshibka"`
}

// Slushat принимает команды на сокете put, пока не отменён ctx.
func Slushat(ctx context.Context, put string, o Obrabotchik, zhurnal *slog.Logger) error {
	// Старый сокет остаётся от прошлого запуска, если программа упала.
	if err := os.Remove(put); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("старый сокет пульта не убирается: %w", err)
	}
	ln, err := net.Listen("unix", put)
	if err != nil {
		return fmt.Errorf("сокет пульта: %w", err)
	}
	if err := os.Chmod(put, 0o600); err != nil {
		ln.Close()
		return fmt.Errorf("права на сокет пульта: %w", err)
	}
	context.AfterFunc(ctx, func() { ln.Close() })
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("приём на сокете пульта: %w", err)
		}
		go obsluzhit(c, o, zhurnal)
	}
}

func obsluzhit(c net.Conn, o Obrabotchik, zhurnal *slog.Logger) {
	defer c.Close()
	// Применение с перезапуском ядра занимает секунды, минуты хватает с
	// запасом, а зависший клиент не держит обработчик вечно.
	if err := c.SetDeadline(time.Now().Add(time.Minute)); err != nil {
		zhurnal.Warn("срок на сокете пульта не встал", "oshibka", err)
	}
	r := bufio.NewReader(io.LimitReader(c, PredelPaketa+4096))
	stroka, err := r.ReadString('\n')
	if err != nil {
		otvetit(c, oshibka{"команда не дочиталась: " + err.Error()}, zhurnal)
		return
	}
	k, err := Razobrat(stroka)
	if err != nil {
		otvetit(c, oshibka{err.Error()}, zhurnal)
		return
	}
	switch k.Imya {
	case "sostoyanie":
		otvetit(c, o.Sostoyanie(), zhurnal)
	case "zaprosy":
		otvetit(c, o.Zaprosy(), zhurnal)
	case "sert":
		otvetit(c, o.Sert(), zhurnal)
	case "lovushka":
		otvetit(c, o.Lovushka(k.Proto, k.Port, k.Sekund), zhurnal)
	case "primenit":
		paket, err := io.ReadAll(io.LimitReader(r, PredelPaketa+1))
		if err != nil {
			otvetit(c, oshibka{"пакет не дочитался: " + err.Error()}, zhurnal)
			return
		}
		if len(paket) > PredelPaketa {
			otvetit(c, oshibka{fmt.Sprintf("пакет больше %d байт", PredelPaketa)}, zhurnal)
			return
		}
		otvet := o.Primenit(paket)
		otvetit(c, otvet, zhurnal)
	case "zhurnal":
		zapisi, err := o.Zhurnal(k.Chislo, PredelZapisey)
		if err != nil {
			otvetit(c, oshibka{err.Error()}, zhurnal)
			return
		}
		otvetit(c, zapisi, zhurnal)
	case "podtverdit":
		if err := o.Podtverdit(k.Chislo); err != nil {
			otvetit(c, oshibka{err.Error()}, zhurnal)
			return
		}
		otvetit(c, map[string]uint64{"podtverzhdeno": k.Chislo}, zhurnal)
	}
}

func otvetit(c net.Conn, otvet any, zhurnal *slog.Logger) {
	if err := json.NewEncoder(c).Encode(otvet); err != nil {
		zhurnal.Warn("ответ пульта не ушёл", "oshibka", err)
	}
}

// Sprosit это сторона принудительной команды: передаёт просьбу в сокет и
// печатает ответ. Код выхода 1, если ответ это ошибка.
func Sprosit(put, stroka string, vhod io.Reader, vyhod io.Writer) int {
	k, err := Razobrat(stroka)
	if err != nil {
		fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", err.Error())
		return 1
	}
	c, err := net.DialTimeout("unix", put, 5*time.Second)
	if err != nil {
		fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", "программа сервера не отвечает: "+err.Error())
		return 1
	}
	defer c.Close()
	if _, err := io.WriteString(c, stroka+"\n"); err != nil {
		fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", "команда не ушла: "+err.Error())
		return 1
	}
	if k.Imya == "primenit" {
		if _, err := io.Copy(c, io.LimitReader(vhod, PredelPaketa+1)); err != nil {
			fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", "пакет не ушёл: "+err.Error())
			return 1
		}
	}
	if uc, ok := c.(*net.UnixConn); ok {
		if err := uc.CloseWrite(); err != nil {
			fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", "конец пакета не ушёл: "+err.Error())
			return 1
		}
	}
	otvet, err := io.ReadAll(io.LimitReader(c, 64<<20))
	if err != nil {
		fmt.Fprintf(vyhod, "{\"oshibka\":%q}\n", "ответ не дочитался: "+err.Error())
		return 1
	}
	if _, err := vyhod.Write(otvet); err != nil {
		return 1
	}
	var o oshibka
	if json.Unmarshal(otvet, &o) == nil && o.Oshibka != "" {
		return 1
	}
	return 0
}
