package upravlenie

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

func TestProveritServer(t *testing.T) {
	sboyUzla := errors.New("программа не ответила")
	sboyMaski := errors.New("openssl упал")
	sboyOsmotra := errors.New("осмотр упал")
	for _, c := range []struct {
		imya       string
		otmetka    string // nasha, net, ne-doshla, chuzhaya
		errOsmotra error
		errUzla    error
		maska      string
		errMaski   error
		otmena     bool
		zhdyom     error
	}{
		{imya: "чисто", maska: "dl.google.com"},
		{imya: "без маски", maska: ""},
		{imya: "нет отметки", otmetka: "net", maska: "dl.google.com", zhdyom: server.ErrNetUstanovki},
		{imya: "установка не дошла", otmetka: "ne-doshla", maska: "dl.google.com", zhdyom: ErrNeDoshla},
		{imya: "отметка чужая", otmetka: "chuzhaya", maska: "dl.google.com", zhdyom: ErrChuzhayaUstanovka},
		{imya: "осмотр упал", errOsmotra: sboyOsmotra, zhdyom: sboyOsmotra},
		{imya: "программа не ответила", errUzla: sboyUzla, maska: "dl.google.com"},
		{imya: "маска упала", maska: "dl.google.com", errMaski: sboyMaski},
		{imya: "отмена посреди проверки", maska: "dl.google.com", otmena: true, zhdyom: context.Canceled},
	} {
		t.Run(c.imya, func(t *testing.T) {
			root := &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "напрямую"}}
			v := &vhody{pryamo: root}
			v.podmenit(t)
			ctx, otmenit := context.WithCancel(context.Background())
			defer otmenit()
			nash := klyuchUpravleniya(t)
			podmenitOsmotr(t, func(_ context.Context, m server.Mashina) (*server.Osmotr, error) {
				if m != root {
					t.Fatal("осмотр не тем входом")
				}
				o := chistyyOsmotr()
				klyuchi := []string{base64.StdEncoding.EncodeToString(klyuchUpravleniya(t).PublicKey().Marshal()),
					base64.StdEncoding.EncodeToString(nash.PublicKey().Marshal())}
				switch c.otmetka {
				case "":
					o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: klyuchi}
				case "ne-doshla":
					o.Otmetka = &server.Sostav{Sostoyanie: "ustanovka", Klyuchi: klyuchi}
				case "chuzhaya":
					o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: klyuchi[:1]}
				}
				return o, c.errOsmotra
			})
			bS, bM := sostoyanie, proveritMasku
			t.Cleanup(func() { sostoyanie, proveritMasku = bS, bM })
			sprosheno := 0
			sostoyanie = func(_ context.Context, m server.Mashina) (server.SostoyanieUzla, error) {
				if m != root {
					t.Fatal("состояние не тем входом")
				}
				sprosheno++
				if c.otmena {
					otmenit()
				}
				if c.errUzla != nil {
					return server.SostoyanieUzla{Versiya: "мусор"}, c.errUzla
				}
				return server.SostoyanieUzla{Versiya: "0.6.0", YadroZhivo: true}, nil
			}
			var maski []string
			proveritMasku = func(_ context.Context, m server.Mashina, maska string) (bool, bool, error) {
				if m != root {
					t.Fatal("маска не тем входом")
				}
				maski = append(maski, maska)
				if c.errMaski != nil {
					return true, true, c.errMaski
				}
				return true, true, nil
			}

			zd, err := ProveritServer(ctx, puti(false), ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: nash}}, c.maska)
			if root.zakryt != 1 {
				t.Fatalf("вход закрыт %d раз", root.zakryt)
			}
			if len(v.zaprosy) != 1 || !v.zaprosy[0].BezPerezapuska {
				t.Fatalf("проверка входила не как смотрящая: %+v", v.zaprosy)
			}
			if c.zhdyom != nil {
				if !errors.Is(err, c.zhdyom) || zd.Osmotr != nil {
					t.Fatalf("итог %+v, ошибка %v", zd, err)
				}
				// Без своей готовой отметки программу и маску не спрашивают.
				if c.otmetka != "" && (sprosheno != 0 || len(maski) != 0) {
					t.Fatal("сервер без своей отметки проверялся дальше")
				}
				return
			}
			if err != nil || zd.Vhod.Put != "напрямую" || zd.Osmotr == nil || zd.Osmotr.Otmetka == nil {
				t.Fatalf("итог %+v, ошибка %v", zd, err)
			}
			if c.errUzla != nil {
				if !errors.Is(zd.UzelOtkaz, sboyUzla) || zd.Uzel.Versiya != "" {
					t.Fatalf("отказ программы %v, состояние %+v", zd.UzelOtkaz, zd.Uzel)
				}
			} else if zd.UzelOtkaz != nil || zd.Uzel.Versiya != "0.6.0" || !zd.Uzel.YadroZhivo {
				t.Fatalf("программа %+v, отказ %v", zd.Uzel, zd.UzelOtkaz)
			}
			switch {
			case c.maska == "":
				if len(maski) != 0 || zd.MaskaProverena || zd.MaskaOtkaz != nil {
					t.Fatalf("маску без имени проверяли: %v", maski)
				}
			case c.errMaski != nil:
				if !errors.Is(zd.MaskaOtkaz, sboyMaski) || zd.Maska || zd.MaskaProverena {
					t.Fatalf("маска %v %v, отказ %v", zd.Maska, zd.MaskaProverena, zd.MaskaOtkaz)
				}
			default:
				if len(maski) != 1 || maski[0] != c.maska || !zd.Maska || !zd.MaskaProverena || zd.MaskaOtkaz != nil {
					t.Fatalf("маска %v: %v %v %v", maski, zd.Maska, zd.MaskaProverena, zd.MaskaOtkaz)
				}
			}
		})
	}

	// Вход не открылся: проверки нет, ошибка входа наружу.
	v := &vhody{errPryamo: otkaz(ssh.PrichinaParolNePrinyat)}
	v.podmenit(t)
	if _, err := ProveritServer(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"}, ""); ssh.PrichinaIz(err) != ssh.PrichinaParolNePrinyat {
		t.Fatal(err)
	}
}
