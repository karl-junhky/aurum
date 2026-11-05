package upravlenie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
)

const domenTesta = "kot.example.net"

func rezhimIz(p Podbor, domen string) SmenaRezhima {
	return SmenaRezhima{Puti: p.Puti, Vhod: p.Vhod, Imya: p.Imya, Opcii: p.Opcii, Domen: domen,
		Routing: "happ://routing/onadd/vkl"}
}

// vRezhimeDomena переводит подставной сервер в режим домена: пакет с сайтом
// и выпуск на домен.
func vRezhimeDomena(t *testing.T, m *postavlennyy) *konfig.Sayt {
	t.Helper()
	sayt, err := konfig.NovyySayt(domenTesta, "happ://routing/onadd/vkl")
	if err != nil {
		t.Fatal(err)
	}
	m.sek.Maska = domenTesta
	vh := rabochiyServer
	vh.Sayt = sayt
	if m.paket, err = konfig.Paket(m.sek, vh, konfig.Opcii{Staging: true}); err != nil {
		t.Fatal(err)
	}
	m.stoit = m.paket
	m.serty = []*server.SertUzla{{IP: "203.0.113.10", Domen: domenTesta, Est: true, Sposob: "http-01", Staging: true}}
	return sayt
}

type razobrannyyPaket struct {
	Yadro struct {
		Inbounds []struct {
			Tls struct {
				ServerName string `json:"server_name"`
				Reality    struct {
					Handshake struct {
						Server string `json:"server"`
					} `json:"handshake"`
				} `json:"reality"`
			} `json:"tls"`
		} `json:"inbounds"`
	} `json:"yadro"`
	SertIPSam *struct {
		Domen   string `json:"domen"`
		Staging bool   `json:"staging"`
	} `json:"sert_ip_sam"`
	Sayt *struct {
		Domen    string `json:"domen"`
		Podpiska struct {
			Put       string            `json:"put"`
			Zagolovki map[string]string `json:"zagolovki"`
		} `json:"podpiska"`
	} `json:"sayt"`
}

func razobratPaket(t *testing.T, b []byte) razobrannyyPaket {
	t.Helper()
	var p razobrannyyPaket
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSmenitRezhimNaDomen(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	gotov := &server.SertUzla{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true}
	m.serty = []*server.SertUzla{gotov, gotov, {IP: "203.0.113.10", Domen: domenTesta, Idyot: true, Staging: true},
		{IP: "203.0.113.10", Domen: domenTesta, Est: true, Sposob: "http-01", Staging: true}}
	var shagi []Shag
	pr := rezhimIz(p, domenTesta)
	pr.Hod = func(h Hod) { shagi = append(shagi, h.Shag) }
	// Домен переезжает с другого своего сервера: путь подписки прежний.
	const put = "0123456789abcdef0123456789abcdef"
	pr.Put = put
	itog, err := SmenitRezhim(context.Background(), pr)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Shag{ShagVhod, ShagOsmotr, ShagSertifikat, ShagPaket}; !slices.Equal(shagi, want) {
		t.Fatalf("шаги %v", shagi)
	}
	if len(m.pakety) != 2 {
		t.Fatalf("пакетов %d, ждали два: выпуск на домен и сайт", len(m.pakety))
	}
	pered, posle := razobratPaket(t, m.pakety[0]), razobratPaket(t, m.pakety[1])
	if pered.SertIPSam == nil || pered.SertIPSam.Domen != domenTesta || !pered.SertIPSam.Staging || pered.Sayt != nil ||
		pered.Yadro.Inbounds[0].Tls.Reality.Handshake.Server != "www.example.com" {
		t.Fatalf("первый пакет не прежние входы с выпуском на домен: %s", m.pakety[0])
	}
	if posle.Sayt == nil || posle.Sayt.Domen != domenTesta || posle.Sayt.Podpiska.Zagolovki["routing"] != "happ://routing/onadd/vkl" ||
		posle.Yadro.Inbounds[0].Tls.Reality.Handshake.Server != "127.0.0.1" || posle.Yadro.Inbounds[0].Tls.ServerName != domenTesta ||
		posle.Sayt.Podpiska.Put != put {
		t.Fatalf("второй пакет не режим домена: %s", m.pakety[1])
	}
	if itog.Sekrety.Maska != domenTesta || itog.Vhody.Sayt == nil || itog.Vhody.Sayt.Put != put || itog.MaskaIstochnik != MaskaDomen ||
		itog.Klyuchi[0].Server.Sni != domenTesta || itog.Klyuchi[1].Server.Sni != domenTesta || itog.Sert == nil ||
		itog.Sert.Domen != domenTesta || m.root.zakryt != 1 {
		t.Fatalf("итог %+v", itog)
	}
	if itog.Sekrety.Uuid != m.sek.Uuid || !slices.Equal(itog.Vhody.Reality, rabochiyServer.Reality) {
		t.Fatal("переход сменил ключи или порты")
	}
}

// Сертификат на домен не вышел: сервер возвращается к прежнему пакету, а
// человек видит, почему.
func TestSmenitRezhimSertNeVyshel(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	bylo := m.paket
	gotov := &server.SertUzla{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true, Popytka: 100}
	m.serty = []*server.SertUzla{gotov, gotov, {IP: "203.0.113.10", Domen: domenTesta, Idyot: true},
		{IP: "203.0.113.10", Domen: domenTesta, Popytka: 200, Otkaz: "DNS problem: NXDOMAIN"}}
	_, err := SmenitRezhim(context.Background(), rezhimIz(p, domenTesta))
	var ne *SertDomenaNeVyshel
	if !errors.As(err, &ne) || ne.Domen != domenTesta || ne.Otkaz != "DNS problem: NXDOMAIN" {
		t.Fatalf("отказ %v", err)
	}
	if len(m.pakety) != 2 || !bytes.Equal(m.pakety[1], bylo) {
		t.Fatalf("пакетов %d, второй должен быть прежним", len(m.pakety))
	}
}

// Попытки выпуска этого часа кончились, и следующая позже конца ожидания:
// отказ сразу и с пределом, без трёх минут ожидания и совета проверить
// запись домена. Сервер возвращается к прежнему пакету.
func TestSmenitRezhimPredel(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	bylo := m.paket
	gotov := &server.SertUzla{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true, Popytka: 100}
	m.serty = []*server.SertUzla{gotov, gotov,
		{IP: "203.0.113.10", Domen: domenTesta, Popytka: 100, Predel: 40 * 60}}
	shagSerta = time.Hour
	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	nado := time.Now().Unix() + 40*60
	_, err := SmenitRezhim(ctx, rezhimIz(p, domenTesta))
	var ne *SertDomenaNeVyshel
	if !errors.As(err, &ne) || ne.Predel < nado || ne.Predel > nado+5 {
		t.Fatalf("отказ %v", err)
	}
	if len(m.pakety) != 2 || !bytes.Equal(m.pakety[1], bylo) {
		t.Fatalf("пакетов %d, второй должен быть прежним", len(m.pakety))
	}
}

// Сертификат на домен вышел, а дальше отказ: второй пакет не принят или
// сайт не поднялся. Сервер возвращается к прежнему пакету, иначе hy2 ждал бы
// имени IP на сертификате домена.
func TestSmenitRezhimOtkatPosleSerta(t *testing.T) {
	for _, c := range []struct {
		imya     string
		nastroit func(m *postavlennyy)
		proverit func(t *testing.T, err error)
	}{
		{"пакет не принят", func(m *postavlennyy) { m.zanyat[2] = &server.Zanyat{Port: 50443, Proto: "tcp"} },
			func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("отказ пакета потерян")
				}
			}},
		{"сайт не встал", func(m *postavlennyy) {
			m.saytOshibka = "сайт не слушает 127.0.0.1:10443: address already in use"
		},
			func(t *testing.T, err error) {
				var ne *SaytNeVstal
				if !errors.As(err, &ne) || ne.Prichina != "сайт не слушает 127.0.0.1:10443: address already in use" {
					t.Fatalf("отказ %v", err)
				}
			}},
		// Прежний пакет тоже не принят: человек должен узнать, что сервер
		// ждёт не тех ключей, что в записи.
		{"откат не прошёл", func(m *postavlennyy) {
			m.zanyat[2] = &server.Zanyat{Port: 50443, Proto: "tcp"}
			m.zanyat[3] = &server.Zanyat{Port: 50443, Proto: "tcp"}
		},
			func(t *testing.T, err error) {
				var otkat *OtkatNeVyshel
				var osh *server.Oshibka
				if !errors.As(err, &otkat) || otkat.Otkat == nil || !errors.As(err, &osh) {
					t.Fatalf("отказ %v", err)
				}
			}},
	} {
		t.Run(c.imya, func(t *testing.T) {
			m, p := novyyPostavlennyy(t, rabochiyServer)
			m.versiya = "0.7.0"
			bylo := m.paket
			gotov := &server.SertUzla{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true}
			m.serty = []*server.SertUzla{gotov, gotov, {IP: "203.0.113.10", Domen: domenTesta, Est: true, Sposob: "http-01", Staging: true}}
			c.nastroit(m)
			_, err := SmenitRezhim(context.Background(), rezhimIz(p, domenTesta))
			c.proverit(t, err)
			if len(m.pakety) != 3 || !bytes.Equal(m.pakety[2], bylo) {
				t.Fatalf("пакетов %d, последний должен быть прежним", len(m.pakety))
			}
		})
	}
}

// Пакет на домене негоден (reality на порту сайта): сервер не трогается.
func TestSmenitRezhimNegodnyyPaket(t *testing.T) {
	vh := rabochiyServer
	vh.Reality = []int{konfig.PortSayta}
	m, p := novyyPostavlennyy(t, vh)
	m.versiya = "0.7.0"
	if _, err := SmenitRezhim(context.Background(), rezhimIz(p, domenTesta)); err == nil || len(m.pakety) != 0 {
		t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
	}
}

func TestSmenitRezhimOtkazy(t *testing.T) {
	t.Run("старая программа", func(t *testing.T) {
		m, p := novyyPostavlennyy(t, rabochiyServer)
		if _, err := SmenitRezhim(context.Background(), rezhimIz(p, domenTesta)); !errors.Is(err, ErrStarayaProgrammaDomena) || len(m.pakety) != 0 {
			t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
		}
	})
	t.Run("другой домен", func(t *testing.T) {
		m, p := novyyPostavlennyy(t, rabochiyServer)
		m.versiya = "0.7.0"
		vRezhimeDomena(t, m)
		_, err := SmenitRezhim(context.Background(), rezhimIz(p, "drugoy.example.net"))
		var d *ErrDrugoyDomen
		if !errors.As(err, &d) || d.Domen != domenTesta || len(m.pakety) != 0 {
			t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
		}
	})
	t.Run("негодный домен", func(t *testing.T) {
		m, p := novyyPostavlennyy(t, rabochiyServer)
		m.versiya = "0.7.0"
		if _, err := SmenitRezhim(context.Background(), rezhimIz(p, "Кот.рф")); err == nil || len(m.pakety) != 0 {
			t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
		}
	})
	t.Run("смена маски в режиме домена", func(t *testing.T) {
		m, p := novyyPostavlennyy(t, rabochiyServer)
		vRezhimeDomena(t, m)
		if _, err := SmenitMasku(context.Background(), smenaIz(p)); !errors.Is(err, ErrRezhimDomena) || len(m.pakety) != 0 {
			t.Fatalf("отказ %v, пакетов %d", err, len(m.pakety))
		}
	})
}

// Повтор после обрыва: сервер уже на этом домене, пакет не трогается.
func TestSmenitRezhimPovtor(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	sayt := vRezhimeDomena(t, m)
	itog, err := SmenitRezhim(context.Background(), rezhimIz(p, domenTesta))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.pakety) != 0 || itog.Vhody.Sayt == nil || *itog.Vhody.Sayt != *sayt || len(itog.Preduprezhdeniya) != 0 {
		t.Fatalf("пакетов %d, сайт %+v, предупреждения %+v", len(m.pakety), itog.Vhody.Sayt, itog.Preduprezhdeniya)
	}

	// Пакет на домене стоит, а сайт молчит (откат после него не прошёл):
	// ключи домена отдаются, но с предупреждением, а не как успех.
	m.saytOshibka = "address already in use"
	itog, err = SmenitRezhim(context.Background(), rezhimIz(p, domenTesta))
	if err != nil || len(m.pakety) != 0 || itog.Vhody.Sayt == nil || len(itog.Preduprezhdeniya) != 1 ||
		itog.Preduprezhdeniya[0].Vid != SaytNeOtvetil || itog.Preduprezhdeniya[0].Otkaz != "address already in use" {
		t.Fatalf("итог %+v, ошибка %v", itog.Preduprezhdeniya, err)
	}
}

func TestSmenitRezhimNaProstye(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	vRezhimeDomena(t, m)
	naDomene := m.serty[0]
	m.serty = []*server.SertUzla{naDomene, naDomene, {IP: "203.0.113.10", Idyot: true, Staging: true},
		{IP: "203.0.113.10", Est: true, Sposob: "http-01", Staging: true}}
	podobratMasku = func(_ context.Context, x server.Mashina, _ netip.Addr, krome string) (Maska, error) {
		if x != m.root || krome != domenTesta {
			t.Fatalf("подбор маски кроме %q", krome)
		}
		return Maska{Imya: "sosed.example.net", Istochnik: MaskaSosedi}, nil
	}
	itog, err := SmenitRezhim(context.Background(), rezhimIz(p, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.pakety) != 1 {
		t.Fatalf("пакетов %d", len(m.pakety))
	}
	pk := razobratPaket(t, m.pakety[0])
	if pk.Sayt != nil || pk.SertIPSam == nil || pk.SertIPSam.Domen != "" ||
		pk.Yadro.Inbounds[0].Tls.Reality.Handshake.Server != "sosed.example.net" {
		t.Fatalf("пакет простых ключей: %s", m.pakety[0])
	}
	if itog.Vhody.Sayt != nil || itog.Sekrety.Maska != "sosed.example.net" || itog.MaskaIstochnik != MaskaSosedi ||
		itog.Klyuchi[1].Server.Sni != "203.0.113.10" || len(itog.Preduprezhdeniya) != 0 {
		t.Fatalf("итог %+v", itog)
	}
}

// Возврат к простым ключам упёрся в предел попыток: hy2 остаётся в пакете с
// выпуском, и предупреждение говорит, что сертификат сервер получит сам.
func TestSmenitRezhimNaProstyePredel(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	vRezhimeDomena(t, m)
	naDomene := m.serty[0]
	m.serty = []*server.SertUzla{naDomene, naDomene, {IP: "203.0.113.10", Staging: true, Popytka: 100, Predel: 40 * 60}}
	podobratMasku = func(context.Context, server.Mashina, netip.Addr, string) (Maska, error) {
		return Maska{Imya: "sosed.example.net", Istochnik: MaskaSosedi}, nil
	}
	shagSerta = time.Hour
	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	itog, err := SmenitRezhim(ctx, rezhimIz(p, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != NetSerta || itog.Preduprezhdeniya[0].Predel == 0 ||
		!itog.Preduprezhdeniya[0].Hy2Zhdyot || len(itog.Vhody.Hy2) == 0 || razobratPaket(t, m.pakety[0]).SertIPSam == nil {
		t.Fatalf("итог %+v", itog)
	}
}

func TestObnovitPodpisku(t *testing.T) {
	m, p := novyyPostavlennyy(t, rabochiyServer)
	m.versiya = "0.7.0"
	sayt := vRezhimeDomena(t, m)
	vykl := "happ://routing/onadd/vykl"
	itog, err := ObnovitPodpisku(context.Background(), SmenaPodpiski{Puti: p.Puti, Vhod: p.Vhod, Imya: "Дача",
		Routing: &vykl, NovyyPut: true, Opcii: p.Opcii})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.pakety) != 1 {
		t.Fatalf("пакетов %d", len(m.pakety))
	}
	pk := razobratPaket(t, m.pakety[0])
	if pk.Sayt == nil || pk.Sayt.Podpiska.Put == sayt.Put || pk.Sayt.Podpiska.Zagolovki["routing"] != vykl {
		t.Fatalf("подписка: %s", m.pakety[0])
	}
	stal := razobratPaket(t, m.paket)
	if pk.Yadro.Inbounds[0].Tls != stal.Yadro.Inbounds[0].Tls {
		t.Fatal("смена подписки тронула входы")
	}
	if itog.Vhody.Sayt.Put != pk.Sayt.Podpiska.Put || itog.Klyuchi[0].Server.Imya != "Дача, reality" || len(itog.Preduprezhdeniya) != 0 {
		t.Fatalf("итог %+v", itog.Vhody.Sayt)
	}

	// Пакет принят, а сайт не ответил: новая подписка уже на сервере, и
	// запись обязана её получить, поэтому это предупреждение, а не отказ.
	m3, p3 := novyyPostavlennyy(t, rabochiyServer)
	m3.versiya = "0.7.0"
	bylo := vRezhimeDomena(t, m3)
	m3.saytOshibka = "address already in use"
	itog, err = ObnovitPodpisku(context.Background(), SmenaPodpiski{Puti: p3.Puti, Vhod: p3.Vhod, Imya: "Дача", NovyyPut: true, Opcii: p3.Opcii})
	if err != nil || itog.Vhody.Sayt == nil || itog.Vhody.Sayt.Put == bylo.Put || len(itog.Preduprezhdeniya) != 1 {
		t.Fatalf("итог %+v, ошибка %v", itog.Preduprezhdeniya, err)
	}
	if pr := itog.Preduprezhdeniya[0]; pr.Vid != SaytNeOtvetil || pr.Otkaz != "address already in use" || pr.Domen != domenTesta {
		t.Fatalf("предупреждение %+v", pr)
	}

	// Спросить программу сервера о сайте не вышло: о сайте ничего не
	// известно, и это не «сайт молчит».
	m4, p4 := novyyPostavlennyy(t, rabochiyServer)
	m4.versiya = "0.7.0"
	vRezhimeDomena(t, m4)
	prezhnee, sprosov := sostoyanie, 0
	sostoyanie = func(ctx context.Context, x server.Mashina) (server.SostoyanieUzla, error) {
		if sprosov++; sprosov > 1 {
			return server.SostoyanieUzla{}, errors.New("обрыв")
		}
		return prezhnee(ctx, x)
	}
	itog, err = ObnovitPodpisku(context.Background(), SmenaPodpiski{Puti: p4.Puti, Vhod: p4.Vhod, Imya: "Дача", NovyyPut: true, Opcii: p4.Opcii})
	if err != nil || len(m4.pakety) != 1 || len(itog.Preduprezhdeniya) != 1 || itog.Preduprezhdeniya[0].Vid != SaytNeProveren ||
		itog.Preduprezhdeniya[0].Otkaz != "обрыв" {
		t.Fatalf("итог %+v, ошибка %v", itog.Preduprezhdeniya, err)
	}

	m2, p2 := novyyPostavlennyy(t, rabochiyServer)
	if _, err := ObnovitPodpisku(context.Background(), SmenaPodpiski{Puti: p2.Puti, Vhod: p2.Vhod, Imya: "x", NovyyPut: true}); !errors.Is(err, ErrProstoyRezhim) || len(m2.pakety) != 0 {
		t.Fatalf("подписка без домена: %v", err)
	}
}

func TestVersiyaNeNizhe(t *testing.T) {
	for v, zhdali := range map[string]bool{"0.7.0": true, "0.7.1": true, "0.10.0": true, "1.0.0": true, "v0.7.0": true,
		"0.7.0-rc1": true, "0.6.9": false, "0.6.0": false, "": false, "0.7": false, "abc": false} {
		if got := versiyaNeNizhe(v, versiyaSDomenom); got != zhdali {
			t.Errorf("%q: %v", v, got)
		}
	}
}
