package main

import (
	"context"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/maska"
)

const ispolzovanieMaski = "vpn-uzel maska podobrat -adres <адрес сервера> | vpn-uzel maska proverit <имя>"

// maskaKomanda подбирает или проверяет маску reality для клиента «Своего
// VPN». Печатает строки «ключ<TAB>значение»; подбор не нашёл ничего - код 1
// и причина в stderr. Обрыв SSH останавливает подбор.
func maskaKomanda(argumenty []string) int {
	if len(argumenty) == 0 {
		fmt.Fprintln(os.Stderr, ispolzovanieMaski)
		return 2
	}
	ctx, otmena := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer otmena()
	switch argumenty[0] {
	case "podobrat":
		fl := flag.NewFlagSet("maska podobrat", flag.ContinueOnError)
		adres := fl.String("adres", "", "адрес сервера")
		if err := fl.Parse(argumenty[1:]); err != nil {
			return 2
		}
		a, err := netip.ParseAddr(*adres)
		if err != nil {
			fmt.Fprintln(os.Stderr, "адрес сервера:", err)
			return 2
		}
		it, err := maska.Podobrat(ctx, a.Unmap(), maska.Opcii{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		stroka("maska", it.Maska)
		stroka("istochnik", string(it.Istochnik))
		stroka("prosmotreno", strconv.Itoa(it.Prosmotreno))
		return 0
	case "proverit":
		if len(argumenty) != 2 {
			fmt.Fprintln(os.Stderr, ispolzovanieMaski)
			return 2
		}
		p := maska.Proverit(ctx, argumenty[1], maska.Opcii{})
		stroka("imya", p.Imya)
		stroka("godna", daNet(p.Godna))
		stroka("brend", daNet(p.Brend))
		if p.Prichina != "" {
			stroka("prichina", p.Prichina)
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, ispolzovanieMaski)
	return 2
}

// stroka печатает «ключ<TAB>значение»; табуляции и переводы строк значения
// становятся пробелами, иначе клиент разобрал бы их как новые ключи.
func stroka(klyuch, znachenie string) {
	znachenie = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, znachenie)
	fmt.Printf("%s\t%s\n", klyuch, znachenie)
}

func daNet(b bool) string {
	if b {
		return "da"
	}
	return "net"
}
