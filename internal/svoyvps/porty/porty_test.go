package porty

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
)

// osmotr: 8443/tcp держит nginx, 11444/udp неизвестно кто, 443/tcp своя
// программа (подбор заново на живом сервере), sshd на 22 и 2222.
func osmotr() *server.Osmotr {
	return &server.Osmotr{
		Slushayut: []server.Slushatel{
			{Proto: "tcp", Adres: "0.0.0.0", Port: 22, Process: "sshd"},
			{Proto: "tcp", Adres: "0.0.0.0", Port: 2222, Process: "sshd"},
			{Proto: "tcp", Adres: "[::]", Port: 8443, Process: "nginx"},
			{Proto: "udp", Adres: "*", Port: 11444},
			{Proto: "tcp", Adres: "*", Port: 443, Process: "vpn-uzel"},
		},
		Sshd: map[string]string{"port": "22 2222"},
	}
}

func TestKandidaty(t *testing.T) {
	nginx := Otsev{Proto: "tcp", Port: 8443, Prichina: PrichinaZanyat, Process: "nginx"}
	neizvestno := Otsev{Proto: "udp", Port: 11444, Prichina: PrichinaZanyat, Process: "?"}
	for _, c := range []struct {
		imya string
		pr   Prodlenie
		r    Ruchnye
		zhdu Spisok
	}{
		{"продление через 80", Cherez80, Ruchnye{}, Spisok{
			Reality: []int{443, 465, 993, 2053, 9443}, Hy2: []int{443, 8443, 5443, 2053},
			Otseyany: []Otsev{nginx, neizvestno}}},
		{"продлению нужен 443", Cherez443, Ruchnye{}, Spisok{
			Reality: []int{465, 993, 2053, 9443}, Hy2: []int{443, 8443, 5443, 2053},
			Otseyany: []Otsev{{Proto: "tcp", Port: 443, Prichina: PrichinaSert}, nginx, neizvestno}}},
		{"без сертификата", BezSerta, Ruchnye{Hy2: 5443}, Spisok{
			Reality: []int{443, 465, 993, 2053, 9443}, Otseyany: []Otsev{nginx}}},
		{"руками", Cherez443, Ruchnye{Reality: 8080, Hy2: 22}, Spisok{Reality: []int{8080}, Hy2: []int{22}}},
		{"руками свой порт", Cherez80, Ruchnye{Reality: 443}, Spisok{
			Reality: []int{443}, Hy2: []int{443, 8443, 5443, 2053}, Otseyany: []Otsev{neizvestno}}},
	} {
		t.Run(c.imya, func(t *testing.T) {
			s, err := Kandidaty(osmotr(), c.pr, c.r)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s, c.zhdu) {
				t.Fatalf("список\n%+v\nждала\n%+v", s, c.zhdu)
			}
		})
	}
}

// Негодный ручной порт это отказ с причиной, а не тихая замена подбором.
func TestKandidatyRuchnyeNegodny(t *testing.T) {
	for _, c := range []struct {
		pr   Prodlenie
		r    Ruchnye
		zhdu Otsev
	}{
		{Cherez80, Ruchnye{Reality: 22}, Otsev{Proto: "tcp", Port: 22, Prichina: PrichinaSsh}},
		{Cherez80, Ruchnye{Reality: 2222}, Otsev{Proto: "tcp", Port: 2222, Prichina: PrichinaSsh}},
		{BezSerta, Ruchnye{Reality: 80}, Otsev{Proto: "tcp", Port: 80, Prichina: PrichinaSert}},
		{Cherez443, Ruchnye{Reality: 443}, Otsev{Proto: "tcp", Port: 443, Prichina: PrichinaSert}},
		{Cherez80, Ruchnye{Reality: 8443}, Otsev{Proto: "tcp", Port: 8443, Prichina: PrichinaZanyat, Process: "nginx"}},
		{Cherez80, Ruchnye{Reality: 65536}, Otsev{Proto: "tcp", Port: 65536, Prichina: PrichinaVne}},
		{Cherez80, Ruchnye{Hy2: -1}, Otsev{Proto: "udp", Port: -1, Prichina: PrichinaVne}},
		{Cherez80, Ruchnye{Hy2: 11444}, Otsev{Proto: "udp", Port: 11444, Prichina: PrichinaZanyat, Process: "?"}},
	} {
		t.Run(c.zhdu.Error(), func(t *testing.T) {
			s, err := Kandidaty(osmotr(), c.pr, c.r)
			var ots Otsev
			if !errors.As(err, &ots) || ots != c.zhdu || s.Reality != nil || s.Hy2 != nil {
				t.Fatalf("список %+v, ошибка %v", s, err)
			}
		})
	}
}

func TestKandidatyNetSvobodnyh(t *testing.T) {
	o := osmotr()
	for _, p := range kandidatyUDP {
		o.Slushayut = append(o.Slushayut, server.Slushatel{Proto: "udp", Adres: "*", Port: p, Process: "xray"})
	}
	if _, err := Kandidaty(o, Cherez80, Ruchnye{}); !errors.Is(err, ErrNetKandidatov) || !strings.Contains(err.Error(), "udp") {
		t.Fatal(err)
	}
	// Без hy2 занятый UDP не мешает.
	if _, err := Kandidaty(o, BezSerta, Ruchnye{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Kandidaty(nil, Cherez80, Ruchnye{}); err == nil {
		t.Fatal("без осмотра подбор пошёл")
	}
	// Забытый способ продления не выключает hy2 молча.
	for _, pr := range []Prodlenie{0, Cherez443 + 1} {
		if _, err := Kandidaty(osmotr(), pr, Ruchnye{}); err == nil {
			t.Fatalf("способ продления %d принят", pr)
		}
	}
}

func sekrety(t *testing.T) konfig.Sekrety {
	t.Helper()
	s, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func vseKandidaty() konfig.Vhody {
	return konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: kandidatyTCP, Hy2: kandidatyUDP}
}

// Срок пробы покрывает самую долгую проверку всех кандидатов, и программа
// сервера его примет.
func TestSrokProby(t *testing.T) {
	v := vseKandidaty()
	srok := SrokProby(v)
	if float64(srok) < (proverka.Predel(len(v.Reality)+len(v.Hy2), 2) + zapasDostavki).Seconds() {
		t.Fatalf("срок %d с", srok)
	}
	if _, err := konfig.Paket(sekrety(t), v, konfig.Opcii{Proba: srok}); err != nil {
		t.Fatal(err)
	}
}

// podstavit ставит вместо проверки ключей готовые итоги по портам ключей:
// «reality-tcp 443» -> вид. Ключа без итога подстава не проверяет, как
// проверка после первого везущего.
func podstavit(t *testing.T, vidy map[string]proverka.Vid, oshibka error) *[]protokol.Server {
	t.Helper()
	var klyuchi []protokol.Server
	bylo := proverit
	t.Cleanup(func() { proverit = bylo })
	proverit = func(_ context.Context, k []protokol.Server, o proverka.Opcii, _ proverka.Zapusk) ([]proverka.Itog, error) {
		if !o.PervyyNaVid || o.Adapter != "Ethernet" || o.Hod == nil {
			t.Errorf("опции проверки %+v", o)
		}
		klyuchi = k
		if oshibka != nil {
			return nil, oshibka
		}
		var itogi []proverka.Itog
		for _, x := range k {
			if vid, est := vidy[fmt.Sprintf("%s %d", x.Transport, x.Port)]; est {
				itogi = append(itogi, proverka.Itog{Server: x, Vid: vid, Otkaz: "отказ " + string(vid)})
			}
		}
		return itogi, nil
	}
	return &klyuchi
}

var opcii = proverka.Opcii{Adapter: "Ethernet", Hod: func(proverka.Itog) {}}

func TestPodobrat(t *testing.T) {
	for _, c := range []struct {
		imya         string
		vidy         map[string]proverka.Vid
		reality, hy2 int
	}{
		{"первые прошедшие", map[string]proverka.Vid{
			"reality-tcp 443": proverka.NetOtveta, "reality-tcp 8443": proverka.Zamerz, "reality-tcp 465": proverka.Vezyot,
			"hy2 443": proverka.Vezyot}, 465, 443},
		{"ни один не прошёл", map[string]proverka.Vid{
			"reality-tcp 443": proverka.NetOtveta, "reality-tcp 8443": proverka.Zamerz, "hy2 443": proverka.NetOtveta}, 0, 0},
		{"ответ не тот, но дальше везёт", map[string]proverka.Vid{
			"reality-tcp 443": proverka.Cel, "reality-tcp 8443": proverka.Vezyot, "hy2 443": proverka.ChuzhoyVyhod,
			"hy2 8443": proverka.Vezyot}, 8443, 8443},
	} {
		t.Run(c.imya, func(t *testing.T) {
			klyuchi := podstavit(t, c.vidy, nil)
			v := vseKandidaty()
			vybor, err := Podobrat(context.Background(), sekrety(t), v, opcii, nil)
			if err != nil {
				t.Fatal(err)
			}
			if vybor.Reality != c.reality || vybor.Hy2 != c.hy2 || len(vybor.Proverki) != len(c.vidy) {
				t.Fatalf("выбор %+v", vybor)
			}
			// Кандидаты идут в порядке списков, сначала reality.
			var porty []int
			for _, k := range *klyuchi {
				porty = append(porty, k.Port)
			}
			if !reflect.DeepEqual(porty, append(append([]int{}, v.Reality...), v.Hy2...)) {
				t.Fatalf("порядок %v", porty)
			}
		})
	}
}

// Вид, где кандидат получил не тот ответ, а везущего нет, подбор не решает:
// «ни один не прошёл» было бы неправдой.
func TestPodobratNeReshil(t *testing.T) {
	for vid, otvet := range map[string]proverka.Vid{"reality-tcp 443": proverka.Cel, "hy2 8443": proverka.ChuzhoyVyhod} {
		t.Run(string(otvet), func(t *testing.T) {
			podstavit(t, map[string]proverka.Vid{
				"reality-tcp 443": proverka.NetOtveta, "hy2 443": proverka.NetOtveta, vid: otvet}, nil)
			_, err := Podobrat(context.Background(), sekrety(t), vseKandidaty(), opcii, nil)
			if !errors.Is(err, ErrNeReshil) || !strings.Contains(err.Error(), "отказ "+string(otvet)) {
				t.Fatal(err)
			}
		})
	}
}

func TestPodobratOtkazy(t *testing.T) {
	sboy := errors.New("ядро проверки не поднялось")
	podstavit(t, nil, sboy)
	if _, err := Podobrat(context.Background(), sekrety(t), vseKandidaty(), opcii, nil); !errors.Is(err, sboy) {
		t.Fatal(err)
	}
	klyuchi := podstavit(t, nil, nil)
	v := vseKandidaty()
	v.Adres = "10.0.0.5"
	if _, err := Podobrat(context.Background(), sekrety(t), v, opcii, nil); err == nil || *klyuchi != nil {
		t.Fatalf("ошибка %v, проверено %d", err, len(*klyuchi))
	}
}
