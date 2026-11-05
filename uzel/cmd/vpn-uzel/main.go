// vpn-uzel это программа VPN-сервера: sing-box библиотекой, состав людей на
// лету, счёт трафика, раздача подписки, свой сертификат на IP без центра,
// пульт для центра и для клиента своего VPN.
//
//	vpn-uzel rabotat            служба (systemd)
//	vpn-uzel pult [команда]     принудительная команда ssh-ключа центра
//	vpn-uzel proverit <пакет>   проверить пакет, ничего не запуская и не записывая
//	vpn-uzel maska podobrat -adres <адрес сервера> [-krome <маска>]
//	                            подобрать маску reality у соседей сервера
//	vpn-uzel maska proverit <имя>
//	                            проверить маску теми же условиями
//	vpn-uzel versiya
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	C "github.com/sagernet/sing-box/constant"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/lovushka"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/maska"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/podpiska"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/pult"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/sertip"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/trafik"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/yadro"
)

var Versiya = "0.6.0"

const (
	katalogPoUmolchaniyu = "/var/lib/vpn-uzel"
	pultPoUmolchaniyu    = "/run/vpn-uzel/pult.sock"
	kazhdye              = time.Minute
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "команды: rabotat, pult, proverit, maska, versiya")
		os.Exit(2)
	}
	komanda, argumenty := os.Args[1], os.Args[2:]
	switch komanda {
	case "rabotat":
		os.Exit(rabotat(argumenty))
	case "pult":
		os.Exit(pultKomanda(argumenty))
	case "proverit":
		os.Exit(proverit(argumenty))
	case "maska":
		os.Exit(maskaKomanda(argumenty))
	case "versiya":
		fmt.Printf("vpn-uzel %s, sing-box %s\n", Versiya, C.Version)
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n", komanda)
		os.Exit(2)
	}
}

func rabotat(argumenty []string) int {
	fl := flag.NewFlagSet("rabotat", flag.ExitOnError)
	katalog := fl.String("katalog", katalogPoUmolchaniyu, "каталог состояния и журнала трафика")
	putPulta := fl.String("pult", pultPoUmolchaniyu, "сокет пульта")
	if err := fl.Parse(argumenty); err != nil {
		return 2
	}
	zhurnal := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := os.MkdirAll(*katalog, 0o700); err != nil {
		zhurnal.Error("каталог состояния не создаётся", "oshibka", err)
		return 1
	}
	ctx, otmena := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer otmena()

	schyotchik := trafik.NovyySchyotchik()
	u := &uzel{yadro: yadro.Novoe(*katalog, schyotchik, zhurnal), podpiska: podpiska.Novyy(zhurnal)}
	u.storozh = maska.NovyyStorozh(u.yadro.Maska)
	u.yadro.PodklyuchitPodpisku(u.podpiska)
	u.vypusk = u.yadro.PodklyuchitVypusk(sertip.Opcii{})
	zh, preduprezhdeniya, err := trafik.Otkryt(*katalog)
	if err != nil {
		// Без журнала цифры трафика копятся в памяти до следующего
		// перезапуска, но VPN из-за этого стоять не должен.
		zhurnal.Error("журнал трафика не открылся, трафик не пишется", "oshibka", err)
		u.zhOshibka = err
	}
	u.zh = zh
	for _, p := range preduprezhdeniya {
		zhurnal.Warn(p)
	}

	if err := u.yadro.Zapustit(); err != nil {
		zhurnal.Error("ядро не поднялось при старте, повтор раз в полминуты", "oshibka", err)
	}
	// Выпуск после ядра: его слушатель проверки на 443/tcp не должен успеть
	// раньше входов.
	go u.vypusk.Rabotat(ctx)
	go u.yadro.Podderzhivat(ctx)
	go u.podpiska.Podderzhivat(ctx, yadro.PovtorPodnyatiya)
	go u.storozh.Rabotat(ctx)
	go func() {
		if err := pult.Slushat(ctx, *putPulta, u, zhurnal); err != nil {
			zhurnal.Error("пульт не работает", "oshibka", err)
		}
	}()
	zhurnal.Info("vpn-uzel работает", "versiya", Versiya, "sing_box", C.Version)

	t := time.NewTicker(kazhdye)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			u.zapisatTrafik(schyotchik, zhurnal)
		case <-ctx.Done():
			// Ядро гасится раньше последней записи: закрываясь, соединения
			// досчитывают свои байты.
			u.podpiska.Ostanovit()
			u.yadro.Ostanovit()
			u.zapisatTrafik(schyotchik, zhurnal)
			zhurnal.Info("vpn-uzel остановлен")
			return 0
		}
	}
}

type uzel struct {
	yadro     *yadro.Yadro
	podpiska  *podpiska.Server
	vypusk    *sertip.Vypusk
	storozh   *maska.Storozh
	zh        *trafik.Zhurnal
	zhOshibka error
}

func (u *uzel) zapisatTrafik(s *trafik.Schyotchik, zhurnal *slog.Logger) {
	if u.zh == nil {
		return
	}
	snyato := s.Snyat()
	if snyato.Pust() {
		return
	}
	if err := u.zh.Dopisat(time.Now().Unix(), snyato); err != nil {
		s.Vernut(snyato)
		zhurnal.Error("трафик не записался, попробую через минуту", "oshibka", err)
	}
}

type sostoyanie struct {
	Versiya string `json:"versiya"`
	yadro.Zdorove
	Trafik        *trafik.Sostoyanie   `json:"trafik,omitempty"`
	TrafikOshibka string               `json:"trafik_oshibka,omitempty"`
	Podpiska      *podpiska.Sostoyanie `json:"podpiska,omitempty"`
	SertIPSam     *sertip.Sostoyanie   `json:"sert_ip_sam,omitempty"`
	// Maska это сторож маски reality (МА3); нет reality - поля нет.
	Maska *maska.SostoyanieStorozha `json:"maska,omitempty"`
}

func (u *uzel) Sostoyanie() any {
	s := sostoyanie{Versiya: Versiya, Zdorove: u.yadro.Zdorove(), Podpiska: u.podpiska.Sostoyanie(),
		SertIPSam: u.vypusk.Sostoyanie(), Maska: u.storozh.Sostoyanie()}
	if u.zh != nil {
		t := u.zh.Sostoyanie()
		s.Trafik = &t
	} else if u.zhOshibka != nil {
		s.TrafikOshibka = u.zhOshibka.Error()
	}
	return s
}

func (u *uzel) Primenit(paket []byte) any { return u.yadro.Primenit(paket) }

func (u *uzel) Zaprosy() any { return u.podpiska.Zaprosy() }

// Sert будит выпуск своего сертификата вне круга и отдаёт его состояние.
// Лимит попыток в силе: клиент узнаёт итог из следующих sostoyanie.
func (u *uzel) Sert() any {
	u.vypusk.Seychas()
	if s := u.vypusk.Sostoyanie(); s != nil {
		return s
	}
	return map[string]string{"oshibka": "самостоятельный выпуск выключен: пакет не третьей версии"}
}

// Lovushka ждёт попыток подключения на порт: детектор блокировок клиента
// стучится напрямую и узнаёт, дошёл ли стук.
func (u *uzel) Lovushka(proto string, port, sekund int) any {
	itog, err := lovushka.Lovit(context.Background(), proto, port, sekund)
	if err != nil {
		return map[string]string{"oshibka": err.Error()}
	}
	return itog
}

func (u *uzel) Zhurnal(posle uint64, predel int) (any, error) {
	if u.zh == nil {
		return nil, fmt.Errorf("журнал трафика не открылся: %v", u.zhOshibka)
	}
	return u.zh.Posle(posle, predel)
}

func (u *uzel) Podtverdit(n uint64) error {
	if u.zh == nil {
		return fmt.Errorf("журнал трафика не открылся: %v", u.zhOshibka)
	}
	return u.zh.Podtverdit(n)
}

func pultKomanda(argumenty []string) int {
	fl := flag.NewFlagSet("pult", flag.ExitOnError)
	putPulta := fl.String("pult", pultPoUmolchaniyu, "сокет пульта")
	if err := fl.Parse(argumenty); err != nil {
		return 2
	}
	// По ssh просьба центра приходит в SSH_ORIGINAL_COMMAND, а аргументы
	// задаёт сама принудительная команда. Руками на сервере можно и так:
	// vpn-uzel pult sostoyanie.
	stroka := os.Getenv("SSH_ORIGINAL_COMMAND")
	if stroka == "" {
		stroka = strings.Join(fl.Args(), " ")
	}
	return pult.Sprosit(*putPulta, stroka, os.Stdin, os.Stdout)
}

func proverit(argumenty []string) int {
	fl := flag.NewFlagSet("proverit", flag.ExitOnError)
	// Каталог состояния нужен только затем, чтобы узнать в конфиге ядра пути
	// к сертификатам: проверка подменяет их на временный каталог.
	katalog := fl.String("katalog", katalogPoUmolchaniyu, "каталог состояния, на который ссылается пакет")
	if err := fl.Parse(argumenty); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "vpn-uzel proverit [-katalog каталог] <файл пакета>")
		return 2
	}
	paket, err := os.ReadFile(fl.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	zhurnal := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := yadro.Novoe(*katalog, trafik.NovyySchyotchik(), zhurnal).Proverit(paket); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("пакет принят ядром, хэш", yadro.Hesh(paket))
	return 0
}
