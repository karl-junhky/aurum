package upravlenie

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
)

func smenaIz(p Podbor) SmenaMaski {
	return SmenaMaski{Puti: p.Puti, Vhod: p.Vhod, Imya: p.Imya, Opcii: p.Opcii}
}

// novayaSmena: поставленный сервер с маской www.example.com, подбор отдаёт
// соседа, проверка своей маски не ждётся.
func novayaSmena(t *testing.T) (*postavlennyy, SmenaMaski, *[]string) {
	t.Helper()
	m, p := novyyPostavlennyy(t, rabochiyServer)
	bS := proveritSvoyuMasku
	t.Cleanup(func() { proveritSvoyuMasku = bS })
	var krome []string
	podobratMasku = func(_ context.Context, x server.Mashina, adres netip.Addr, k string) (Maska, error) {
		if x != m.root || adres.String() != "203.0.113.10" {
			t.Fatal("маска подбирается не входом root или не для адреса сервера")
		}
		krome = append(krome, k)
		return Maska{Imya: "sosed.example.net", Istochnik: MaskaSosedi}, nil
	}
	proveritSvoyuMasku = func(context.Context, server.Mashina, string) (ProverkaMaski, error) {
		t.Fatal("своя маска не вводилась")
		return ProverkaMaski{}, nil
	}
	return m, smenaIz(p), &krome
}

// Новый пакет отличается от прежнего только маской reality.
func proveritPaketSmeny(t *testing.T, m *postavlennyy, maska string) {
	t.Helper()
	if len(m.pakety) != 1 {
		t.Fatalf("пакетов %d", len(m.pakety))
	}
	sek, vh, err := konfig.IzPaketa(m.pakety[0], "203.0.113.10", "Мой сервер")
	if err != nil {
		t.Fatal(err)
	}
	ozhid := m.sek
	ozhid.Maska = maska
	if sek != ozhid || !slices.Equal(vh.Reality, rabochiyServer.Reality) || !slices.Equal(vh.Hy2, rabochiyServer.Hy2) {
		t.Fatalf("пакет: секреты %+v, входы %+v", sek, vh)
	}
	if !strings.Contains(string(m.pakety[0]), `"staging":true`) {
		t.Fatal("пакет сменил каталог сертификата")
	}
}

func TestSmenitMaskuPodbor(t *testing.T) {
	m, p, krome := novayaSmena(t)
	// Запись с той же маской, что на сервере: подбор идёт.
	p.Bylo = "WWW.example.com."
	var shagi []Shag
	p.Hod = func(h Hod) { shagi = append(shagi, h.Shag) }
	itog, err := SmenitMasku(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Shag{ShagVhod, ShagOsmotr, ShagMaska, ShagPaket}; !slices.Equal(shagi, want) {
		t.Fatalf("шаги %v", shagi)
	}
	// Подбор не возвращает маску, что уже стоит.
	if !slices.Equal(*krome, []string{"www.example.com"}) {
		t.Fatalf("кроме %q", *krome)
	}
	proveritPaketSmeny(t, m, "sosed.example.net")
	if itog.MaskaIstochnik != MaskaSosedi || itog.Sekrety.Maska != "sosed.example.net" || itog.Adres != "203.0.113.10" ||
		itog.Versiya != "0.6.0" || itog.Sert == nil || len(itog.Klyuchi) != 2 || m.root.zakryt != 1 {
		t.Fatalf("итог %+v, вход закрыт %d раз", itog, m.root.zakryt)
	}
	if itog.Klyuchi[0].Server.Sni != "sosed.example.net" {
		t.Fatalf("ключ reality с именем %q", itog.Klyuchi[0].Server.Sni)
	}
}

func TestSmenitMaskuSvoya(t *testing.T) {
	for _, c := range []struct {
		imya      string
		otvet     ProverkaMaski
		soglasen  bool
		istochnik string
		test      func(error) bool
	}{
		{"годна", ProverkaMaski{Imya: "svoya.example.org", Godna: true}, false, MaskaSvoya, nil},
		{"известная с согласием", ProverkaMaski{Imya: "www.adobe.com", Godna: true, Brend: true}, true, MaskaBrend, nil},
		{"известная без согласия", ProverkaMaski{Imya: "www.adobe.com", Godna: true, Brend: true}, false, "",
			func(err error) bool {
				var o *MaskaIzvestnaya
				return errors.As(err, &o) && o.Imya == "www.adobe.com"
			}},
		{"негодна", ProverkaMaski{Imya: "suckless.org", Prichina: "сайт не отвечает h2", Podrobno: "no application protocol"}, true, "",
			func(err error) bool {
				var o *MaskaNegodna
				return errors.As(err, &o) && o.Prichina == "сайт не отвечает h2" && strings.Contains(err.Error(), "no application protocol")
			}},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m, p, krome := novayaSmena(t)
			p.Svoya, p.BrendSoglasen = "  "+c.otvet.Imya, c.soglasen
			proveritSvoyuMasku = func(_ context.Context, x server.Mashina, imya string) (ProverkaMaski, error) {
				if x != m.root || imya != c.otvet.Imya {
					t.Fatalf("проверка не входом root или имени %q", imya)
				}
				return c.otvet, nil
			}
			itog, err := SmenitMasku(context.Background(), p)
			if len(*krome) != 0 {
				t.Fatal("своя маска ушла в подбор")
			}
			if c.test != nil {
				if !c.test(err) || len(m.pakety) != 0 {
					t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			proveritPaketSmeny(t, m, c.otvet.Imya)
			if itog.MaskaIstochnik != c.istochnik {
				t.Fatalf("источник %q", itog.MaskaIstochnik)
			}
		})
	}
}

// Своя маска уже стоит на сервере (повтор после обрыва посреди смены): пакет
// не трогается, ключи отдаются с ней, даже если сайт сейчас не отвечает или
// он из известных.
func TestSmenitMaskuTaZhe(t *testing.T) {
	for imya, otvet := range map[string]ProverkaMaski{
		"годна":     {Imya: "www.example.com", Godna: true},
		"не годна":  {Imya: "www.example.com", Prichina: "сайт не отвечает h2"},
		"известная": {Imya: "www.example.com", Godna: true, Brend: true},
	} {
		t.Run(imya, func(t *testing.T) {
			m, p, _ := novayaSmena(t)
			p.Svoya = "WWW.Example.com."
			proveritSvoyuMasku = func(context.Context, server.Mashina, string) (ProverkaMaski, error) { return otvet, nil }
			var shagi []Shag
			p.Hod = func(h Hod) { shagi = append(shagi, h.Shag) }
			itog, err := SmenitMasku(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.pakety) != 0 || slices.Contains(shagi, ShagPaket) {
				t.Fatalf("пакетов %d, шаги %v", len(m.pakety), shagi)
			}
			if itog.Sekrety != m.sek || len(itog.Klyuchi) != 2 || itog.Klyuchi[0].Server.Sni != "www.example.com" {
				t.Fatalf("итог %+v", itog)
			}
		})
	}
}

// Подбор встал на сервере, а ответ оборвался: в записи прежняя маска, на
// сервере новая. Повтор не подбирает третью, а отдаёт ключи с маской сервера.
func TestSmenitMaskuPodborUzheVstal(t *testing.T) {
	m, p, krome := novayaSmena(t)
	p.Bylo = "staraya.example.org"
	var shagi []Shag
	p.Hod = func(h Hod) { shagi = append(shagi, h.Shag) }
	itog, err := SmenitMasku(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(*krome) != 0 || len(m.pakety) != 0 || slices.Contains(shagi, ShagPaket) {
		t.Fatalf("подборов %d, пакетов %d, шаги %v", len(*krome), len(m.pakety), shagi)
	}
	if itog.Sekrety != m.sek || itog.MaskaIstochnik != "" || len(itog.Klyuchi) != 2 || itog.Klyuchi[0].Server.Sni != "www.example.com" {
		t.Fatalf("итог %+v", itog)
	}
}

func TestSmenitMaskuOtkazy(t *testing.T) {
	sboy := errors.New("оборвалось")
	for _, c := range []struct {
		imya   string
		izmeni func(*postavlennyy, *SmenaMaski)
		test   func(error) bool
	}{
		{"нет установки", func(m *postavlennyy, _ *SmenaMaski) { m.osm.Otmetka = nil },
			func(err error) bool { return errors.Is(err, server.ErrNetUstanovki) }},
		{"под центром", func(m *postavlennyy, _ *SmenaMaski) { m.osm.Centr = []string{"klyuch"} },
			func(err error) bool {
				var o *Ostanovka
				return errors.As(err, &o) && o.Zamechaniya[0].Kod == server.KodPodCentrom
			}},
		{"не дошла", func(m *postavlennyy, _ *SmenaMaski) { m.paket = nil },
			func(err error) bool { return errors.Is(err, ErrNeDoshla) }},
		{"идёт проба", func(m *postavlennyy, _ *SmenaMaski) {
			b, err := konfig.Paket(m.sek, rabochiyServer, konfig.Opcii{Proba: 300})
			if err != nil {
				t.Fatal(err)
			}
			m.paket = b
		}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "пробный пакет") }},
		{"не нашлось", func(*postavlennyy, *SmenaMaski) {
			podobratMasku = func(context.Context, server.Mashina, netip.Addr, string) (Maska, error) {
				return Maska{}, ErrNetMaski
			}
		}, func(err error) bool { return errors.Is(err, ErrNetDrugoyMaski) && errors.Is(err, ErrNetMaski) }},
		{"ответила только известная", func(*postavlennyy, *SmenaMaski) {
			podobratMasku = func(context.Context, server.Mashina, netip.Addr, string) (Maska, error) {
				return Maska{Imya: "www.adobe.com", Istochnik: MaskaBrend}, nil
			}
		}, func(err error) bool {
			return errors.Is(err, ErrNetDrugoyMaski) && strings.Contains(err.Error(), "www.adobe.com")
		}},
		{"старая программа", func(_ *postavlennyy, p *SmenaMaski) {
			p.Svoya = "svoya.example.org"
			proveritSvoyuMasku = func(context.Context, server.Mashina, string) (ProverkaMaski, error) {
				return ProverkaMaski{}, ErrStarayaProgramma
			}
		}, func(err error) bool { return errors.Is(err, ErrStarayaProgramma) }},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m, p, _ := novayaSmena(t)
			c.izmeni(m, &p)
			if _, err := SmenitMasku(context.Background(), p); !c.test(err) {
				t.Fatalf("отказ: %v", err)
			}
			if len(m.pakety) != 0 || m.root.zakryt != 1 {
				t.Fatalf("пакетов %d, вход закрыт %d раз", len(m.pakety), m.root.zakryt)
			}
		})
	}
	t.Run("пакет не принят", func(t *testing.T) {
		_, p, _ := novayaSmena(t)
		primenit = func(context.Context, server.Mashina, []byte) (server.Kvitanciya, error) {
			return server.Kvitanciya{}, sboy
		}
		if _, err := SmenitMasku(context.Background(), p); !errors.Is(err, sboy) {
			t.Fatalf("отказ: %v", err)
		}
	})
	for imya, pravo := range map[string]func(*SmenaMaski){
		"имя вместо IP":        func(p *SmenaMaski) { p.Vhod.Adres = "vps.example.net" },
		"без ключа управления": func(p *SmenaMaski) { p.Vhod.Vhod.Podpis = nil },
		"без имени":            func(p *SmenaMaski) { p.Imya = "" },
	} {
		t.Run(imya, func(t *testing.T) {
			m, p, _ := novayaSmena(t)
			pravo(&p)
			if _, err := SmenitMasku(context.Background(), p); err == nil || len(m.vhod.zaprosy) != 0 {
				t.Fatalf("ошибка %v, входов %d", err, len(m.vhod.zaprosy))
			}
		})
	}
}
