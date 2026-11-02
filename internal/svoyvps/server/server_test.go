package server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// podstavnaya отвечает на скрипты пакета заготовленными исходами и пишет,
// что у неё просили.
type podstavnaya struct {
	otvety  map[string]ssh.Itog
	vyzovy  []string
	fayly   map[string]string
	rezhimy map[string]fs.FileMode
	// vhod это что пришло в stdin последней команды.
	vhod    []byte
	oshibka error
	panika  bool
}

func novaya() *podstavnaya {
	return &podstavnaya{otvety: map[string]ssh.Itog{}, fayly: map[string]string{}, rezhimy: map[string]fs.FileMode{}}
}

func imyaSkripta(tekst string) string {
	switch tekst {
	case skriptOsmotra:
		return "osmotr"
	case skriptUstanovki:
		return "ustanovka"
	case skriptPortov:
		return "porty"
	case skriptSnosa:
		return "snos"
	case skriptOtpuska:
		return "otpusk"
	case skriptParolya:
		return "parol"
	}
	return "?"
}

func (p *podstavnaya) otvet(chto string) (ssh.Itog, error) {
	p.vyzovy = append(p.vyzovy, chto)
	if p.panika {
		panic("подстава")
	}
	if p.oshibka != nil {
		return ssh.Itog{Kod: -1}, p.oshibka
	}
	// Ответ по самому длинному ключу, с которого начинается просьба:
	// «ustanovka nachalo» раньше «ustanovka».
	luchshiy := ""
	for k := range p.otvety {
		if strings.HasPrefix(chto, k) && len(k) > len(luchshiy) {
			luchshiy = k
		}
	}
	if luchshiy != "" {
		return p.otvety[luchshiy], nil
	}
	return ssh.Itog{}, nil
}

func (p *podstavnaya) Vypolnit(_ context.Context, komanda string, vhod io.Reader) (ssh.Itog, error) {
	if vhod != nil {
		b, err := io.ReadAll(vhod)
		if err != nil {
			return ssh.Itog{}, err
		}
		p.vhod = b
	}
	for put := range p.otvety {
		if strings.HasPrefix(put, "/") && strings.Contains(komanda, put) {
			p.vyzovy = append(p.vyzovy, "cat "+put)
			return p.otvety[put], nil
		}
	}
	return p.otvet(komanda)
}

func (p *podstavnaya) Skript(_ context.Context, tekst string, argumenty ...string) (ssh.Itog, error) {
	return p.otvet(strings.Join(append([]string{imyaSkripta(tekst)}, argumenty...), " "))
}

func (p *podstavnaya) OtpravitFayl(_ context.Context, dannye io.ReadSeeker, kuda string, rezhim fs.FileMode) error {
	p.vyzovy = append(p.vyzovy, "fayl "+kuda)
	if _, err := dannye.Seek(0, io.SeekStart); err != nil {
		return err
	}
	b, err := io.ReadAll(dannye)
	if err != nil {
		return err
	}
	p.fayly[kuda], p.rezhimy[kuda] = string(b), rezhim
	return nil
}

func hesh(t *testing.T, s string) string {
	t.Helper()
	h, err := sha256Chitatelya(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func klyuchKompyutera(t *testing.T) (gossh.PublicKey, string) {
	t.Helper()
	pk, err := gossh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	if err != nil {
		t.Fatal(err)
	}
	return pk, teloKlyucha(pk)
}

func TestUstanovit(t *testing.T) {
	m := novaya()
	m.otvety["ustanovka"] = ssh.Itog{Vyvod: []byte("perezapushchen\tprogramma yunit\npreduprezhdenie\tbbr в ядре нет\nversiya\tvpn-uzel 0.6.0, sing-box 1.14.2\ngotovo\tda\n")}
	pk, telo := klyuchKompyutera(t)
	u, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk)
	if err != nil {
		t.Fatal(err)
	}
	if u.Versiya != "vpn-uzel 0.6.0, sing-box 1.14.2" || u.Perezapushchen != "programma yunit" ||
		!slices.Equal(u.Preduprezhdeniya, []string{"bbr в ядре нет"}) {
		t.Fatalf("итог %+v", u)
	}
	if m.fayly[katalogZagruzki+"/vpn-uzel"] != "ELF" || m.rezhimy[katalogZagruzki+"/vpn-uzel"] != 0o700 {
		t.Fatalf("программа не легла: %v %v", m.fayly, m.rezhimy)
	}
	if m.fayly[katalogZagruzki+"/vpn-uzel.service"] != string(yunit) || !strings.Contains(string(yunit), "ExecStart=/usr/local/sbin/vpn-uzel rabotat") {
		t.Fatal("юнит не тот")
	}
	// Ключ записывается в отметку раньше любой загрузки: обрыв на загрузке
	// иначе оставил бы ключ, который снос не знает.
	zhdu := []string{"ustanovka nachalo " + telo, "fayl " + katalogZagruzki + "/vpn-uzel",
		"fayl " + katalogZagruzki + "/vpn-uzel.service", "ustanovka " + telo}
	if !slices.Equal(m.vyzovy, zhdu) {
		t.Fatalf("порядок %q", m.vyzovy)
	}
}

// Установленный файл с тем же sha256 по сети не идёт, а его копия прошлой
// попытки в загрузке убирается, чтобы не встать поверх.
func TestUstanovitSverkaUstanovlennogo(t *testing.T) {
	pk, telo := klyuchKompyutera(t)
	m := novaya()
	m.otvety["ustanovka nachalo"] = ssh.Itog{Vyvod: []byte("ustanovleno\t" + hesh(t, "ELF") + " vpn-uzel\nustanovleno\t" +
		hesh(t, "staryy yunit") + " vpn-uzel.service\n")}
	m.otvety["ustanovka "+telo] = ssh.Itog{Vyvod: []byte("gotovo\tda\n")}
	if _, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk); err != nil {
		t.Fatal(err)
	}
	zhdu := []string{"ustanovka nachalo " + telo, "fayl " + katalogZagruzki + "/vpn-uzel.service",
		"rm -f -- " + katalogZagruzki + "/vpn-uzel", "ustanovka " + telo}
	if !slices.Equal(m.vyzovy, zhdu) {
		t.Fatalf("порядок %q", m.vyzovy)
	}

	m = novaya()
	m.otvety["ustanovka nachalo"] = ssh.Itog{Kod: 1, Oshibki: "сервер под центром, не трогаю"}
	var o *Oshibka
	if _, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk); !errors.As(err, &o) || len(m.vyzovy) != 1 {
		t.Fatalf("отказ начала: %v %q", err, m.vyzovy)
	}
}

func TestDozhdatsyaNastroyki(t *testing.T) {
	m := novaya()
	if err := DozhdatsyaNastroyki(context.Background(), m); err != nil || !strings.Contains(m.vyzovy[0], "cloud-init status --wait") {
		t.Fatalf("%v %q", err, m.vyzovy)
	}
	m.otvety["command -v cloud-init"] = ssh.Itog{Kod: 124}
	var o *Oshibka
	if err := DozhdatsyaNastroyki(context.Background(), m); !errors.As(err, &o) || o.Shag != "nastroyka" || o.Kod != 124 {
		t.Fatalf("%v", err)
	}
}

func TestUstanovitOtkaz(t *testing.T) {
	pk, telo := klyuchKompyutera(t)
	m := novaya()
	m.otvety["ustanovka "+telo] = ssh.Itog{Kod: 1, Oshibki: "journal...\nvpn-uzel не открыл пульт\x1b[0m\n"}
	_, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk)
	var o *Oshibka
	if !errors.As(err, &o) || o.Shag != "ustanovka" || o.Kod != 1 || o.Tekst != "vpn-uzel не открыл пульт [0m" {
		t.Fatalf("отказ %#v", err)
	}

	// Код 0 без строки gotovo: скрипт оборвался, а не дошёл.
	m = novaya()
	m.otvety["ustanovka"] = ssh.Itog{Vyvod: []byte("perezapushchen\tprogramma\n")}
	if _, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk); !errors.As(err, &o) {
		t.Fatalf("неполный вывод принят: %v", err)
	}

	m = novaya()
	m.oshibka = &ssh.Otkaz{Prichina: ssh.PrichinaObryv}
	if _, err := Ustanovit(context.Background(), m, strings.NewReader("ELF"), pk); ssh.PrichinaIz(err) != ssh.PrichinaObryv {
		t.Fatalf("отказ SSH потерян: %v", err)
	}
}

func TestOtkrytPorty(t *testing.T) {
	m := novaya()
	m.otvety["porty"] = ssh.Itog{Vyvod: []byte("ufw\tactive\notkryt\t443/tcp\nuzhe_otkryt\t8443/udp\nzakryt\t2053/tcp\nzakryt\t9443/tcp\n")}
	p, err := OtkrytPorty(context.Background(), m, []int{443, 443}, []int{8443, 443, 8443})
	if err != nil {
		t.Fatal(err)
	}
	if m.vyzovy[0] != "porty 443 8443,443" || !slices.Equal(p.Otkryty, []string{"443/tcp"}) ||
		!slices.Equal(p.UzheOtkryty, []string{"8443/udp"}) || p.Zakryto != 2 || p.UFW != "active" {
		t.Fatalf("%q %+v", m.vyzovy, p)
	}

	m.otvety["porty"] = ssh.Itog{Vyvod: []byte("ufw\tnet\n")}
	if p, err := OtkrytPorty(context.Background(), m, nil, nil); err != nil || p.UFW != "net" {
		t.Fatalf("%+v %v", p, err)
	}

	m = novaya()
	for _, plohoy := range []int{0, 65536, -1} {
		if _, err := OtkrytPorty(context.Background(), m, []int{plohoy}, nil); err == nil {
			t.Errorf("порт %d принят", plohoy)
		}
	}
	if len(m.vyzovy) != 0 {
		t.Fatalf("с негодным портом скрипт звали: %v", m.vyzovy)
	}

	m.otvety["porty"] = ssh.Itog{Kod: 1, Oshibki: "в ufw своё правило на 443/tcp: DENY\n"}
	var o *Oshibka
	if _, err := OtkrytPorty(context.Background(), m, []int{443}, nil); !errors.As(err, &o) || o.Shag != "porty" {
		t.Fatalf("%v", err)
	}
}

func TestSnesti(t *testing.T) {
	m := novaya()
	m.otvety["snos"] = ssh.Itog{Vyvod: []byte("parol\tvernulsya\notmetka\tsnyata\nklyuchi\tsnyaty\ngotovo\tda\n")}
	s, err := Snesti(context.Background(), m)
	if err != nil || !s.ParolVernulsya || s.BezOtmetki {
		t.Fatalf("%+v %v", s, err)
	}
	// Правила ufw снимаются раньше: снос после них их уже не увидел бы
	// своими, отметки не будет.
	if !slices.Equal(m.vyzovy, []string{"porty  ", "snos"}) {
		t.Fatalf("порядок %q", m.vyzovy)
	}

	m.otvety["snos"] = ssh.Itog{Vyvod: []byte("otmetki\tnet\n")}
	if s, err := Snesti(context.Background(), m); err != nil || !s.BezOtmetki {
		t.Fatalf("%+v %v", s, err)
	}

	m.otvety["snos"] = ssh.Itog{Kod: 1, Vyvod: []byte("ne_vyshlo\tслужба vpn-uzel не остановилась\nne_vyshlo\tsysctl net.core.rmem_max\nostalos\t2\n")}
	var o *Oshibka
	if _, err := Snesti(context.Background(), m); !errors.As(err, &o) ||
		!slices.Equal(o.NeVyshlo, []string{"служба vpn-uzel не остановилась", "sysctl net.core.rmem_max"}) || o.KlyuchOstalsya {
		t.Fatalf("%#v", err)
	}

	m.otvety["snos"] = ssh.Itog{Kod: 4, Vyvod: []byte("otmetka\tsnyata\nne_vyshlo\tстроки ключей\n")}
	if _, err := Snesti(context.Background(), m); !errors.As(err, &o) || !o.KlyuchOstalsya {
		t.Fatalf("%#v", err)
	}

	// Отказ правки ufw останавливает снос до скрипта сноса.
	m = novaya()
	m.otvety["porty"] = ssh.Itog{Kod: 1, Oshibki: "ufw сломан"}
	if _, err := Snesti(context.Background(), m); err == nil || slices.Contains(m.vyzovy, "snos") {
		t.Fatalf("%v %q", err, m.vyzovy)
	}
}

func TestProchitatUstanovku(t *testing.T) {
	m := novaya()
	m.otvety[putOtmetki] = ssh.Itog{Kod: 3}
	if _, _, err := ProchitatUstanovku(context.Background(), m); !errors.Is(err, ErrNetUstanovki) {
		t.Fatalf("%v", err)
	}

	m.otvety[putOtmetki] = ssh.Itog{Vyvod: []byte(`{"format":1,"sostoyanie":"gotovo","vpn_uzel":"vpn-uzel 0.6.0","klyuchi":["AAAA","BBBB"]}`)}
	m.otvety[putPaketa] = ssh.Itog{Kod: 3}
	s, paket, err := ProchitatUstanovku(context.Background(), m)
	if err != nil || paket != nil || s.Sostoyanie != "gotovo" || !slices.Equal(s.Klyuchi, []string{"AAAA", "BBBB"}) {
		t.Fatalf("%+v %q %v", s, paket, err)
	}

	m.otvety[putPaketa] = ssh.Itog{Vyvod: []byte(`{"versiya":3}`)}
	if _, paket, err := ProchitatUstanovku(context.Background(), m); err != nil || string(paket) != `{"versiya":3}` {
		t.Fatalf("%q %v", paket, err)
	}

	m.otvety[putOtmetki] = ssh.Itog{Vyvod: []byte(`{"format":`)}
	if _, _, err := ProchitatUstanovku(context.Background(), m); err == nil {
		t.Fatal("испорченная отметка принята")
	}

	m.otvety[putOtmetki] = ssh.Itog{Kod: 1, Oshibki: "cat: Permission denied"}
	var o *Oshibka
	if _, _, err := ProchitatUstanovku(context.Background(), m); !errors.As(err, &o) || o.Shag != "chtenie" {
		t.Fatalf("%v", err)
	}
}

func TestOsmotretOtkazy(t *testing.T) {
	m := novaya()
	m.otvety["osmotr"] = ssh.Itog{Vyvod: []byte("os\tubuntu\n")}
	var o *Oshibka
	if _, err := Osmotret(context.Background(), m); !errors.As(err, &o) {
		t.Fatalf("оборванный осмотр: %v", err)
	}
	m.otvety["osmotr"] = ssh.Itog{Vyvod: []byte(chistyy)}
	if os, err := Osmotret(context.Background(), m); err != nil || os.OS != "ubuntu" {
		t.Fatalf("%v", err)
	}
}

func TestPanikaNeRonyaet(t *testing.T) {
	pk, _ := klyuchKompyutera(t)
	m := novaya()
	m.panika = true
	ctx := context.Background()
	proverit := func(chto string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "паника") {
			t.Errorf("%s: %v", chto, err)
		}
	}
	proverit("DozhdatsyaNastroyki", DozhdatsyaNastroyki(ctx, m))
	_, err := Osmotret(ctx, m)
	proverit("Osmotret", err)
	_, err = Ustanovit(ctx, m, strings.NewReader("ELF"), pk)
	proverit("Ustanovit", err)
	proverit("ZapisatKlyuch", ZapisatKlyuch(ctx, m, pk))
	_, err = OtkrytPorty(ctx, m, []int{443}, nil)
	proverit("OtkrytPorty", err)
	_, err = Snesti(ctx, m)
	proverit("Snesti", err)
	_, _, err = ProchitatUstanovku(ctx, m)
	proverit("ProchitatUstanovku", err)
	_, err = SnyatKlyuch(ctx, m, pk)
	proverit("SnyatKlyuch", err)
	proverit("VyklyuchitParol", VyklyuchitParol(ctx, m))
	proverit("PodtverditParol", PodtverditParol(ctx, m))
	_, err = VklyuchitParol(ctx, m)
	proverit("VklyuchitParol", err)
}

func TestParol(t *testing.T) {
	ctx := context.Background()
	m := novaya()
	m.otvety["parol vyklyuchit"] = ssh.Itog{Vyvod: []byte("parol\tvyklyuchen\n")}
	m.otvety["parol podtverdit"] = ssh.Itog{Vyvod: []byte("otkat\tsnyat\n")}
	if err := VyklyuchitParol(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := PodtverditParol(ctx, m); err != nil || !slices.Equal(m.vyzovy, []string{"parol vyklyuchit", "parol podtverdit"}) {
		t.Fatalf("%v %q", err, m.vyzovy)
	}
	// Чужая настройка держит пароль: файлы называются.
	m.otvety["parol vyklyuchit"] = ssh.Itog{Kod: 2, Vyvod: []byte("parol\tchuzhoy\nvklyuchaet\t/etc/ssh/sshd_config.d/00-billpanel.conf\nvklyuchaet\t\n")}
	var ch *ParolChuzhoy
	if err := VyklyuchitParol(ctx, m); !errors.As(err, &ch) || !slices.Equal(ch.Fayly, []string{"/etc/ssh/sshd_config.d/00-billpanel.conf"}) {
		t.Fatalf("чужая настройка: %v", err)
	}
	// Код 2 без своей строки мог дать и упавший шаг: это сбой.
	m.otvety["parol vyklyuchit"] = ssh.Itog{Kod: 2, Vyvod: []byte("vklyuchaet\t/etc/ssh/sshd_config\n")}
	if err := VyklyuchitParol(ctx, m); errors.As(err, &ch) || err == nil {
		t.Fatalf("код 2 без строки: %v", err)
	}
	m.otvety["parol vklyuchit"] = ssh.Itog{Vyvod: []byte("parol\tvyklyuchen_chuzhim\nvyklyuchaet\t/etc/ssh/sshd_config.d/60-cloudimg-settings.conf\n")}
	if v, err := VklyuchitParol(ctx, m); err != nil || v.Parol != "vyklyuchen_chuzhim" || !slices.Equal(v.Fayly, []string{"/etc/ssh/sshd_config.d/60-cloudimg-settings.conf"}) {
		t.Fatalf("включение: %+v %v", v, err)
	}
	// Отказ скрипта и успех без итоговой строки - сбой шага parol.
	var o *Oshibka
	for imya, it := range map[string]ssh.Itog{
		"parol vyklyuchit": {Kod: 1, Oshibki: "sshd не перечитал настройку, файл снят"},
		"parol podtverdit": {},
		"parol vklyuchit":  {Vyvod: []byte("parol\tmozhet\n")},
	} {
		m.otvety[imya] = it
		var err error
		switch imya {
		case "parol vyklyuchit":
			err = VyklyuchitParol(ctx, m)
		case "parol podtverdit":
			err = PodtverditParol(ctx, m)
		default:
			_, err = VklyuchitParol(ctx, m)
		}
		if !errors.As(err, &o) || o.Shag != "parol" {
			t.Fatalf("%s: %v", imya, err)
		}
	}
	m.otvety["parol vyklyuchit"] = ssh.Itog{Kod: 1, Oshibki: "sshd не перечитал настройку, файл снят"}
	if err := VyklyuchitParol(ctx, m); !errors.As(err, &o) || o.Tekst != "sshd не перечитал настройку, файл снят" {
		t.Fatalf("текст отказа: %v", err)
	}
	m.otvety["parol podtverdit"] = ssh.Itog{Vyvod: []byte("otkat\tmozhet\n")}
	if err := PodtverditParol(ctx, m); !errors.As(err, &o) {
		t.Fatalf("подтверждение без «снят»: %v", err)
	}
}

func TestSnyatKlyuch(t *testing.T) {
	pk, telo := klyuchKompyutera(t)
	ctx := context.Background()
	m := novaya()
	m.otvety["otpusk"] = ssh.Itog{Vyvod: []byte("klyuch\tsnyat\n")}
	if snyat, err := SnyatKlyuch(ctx, m, pk); err != nil || !snyat || m.vyzovy[0] != "otpusk "+telo {
		t.Fatalf("%v %v %q", snyat, err, m.vyzovy)
	}
	m.otvety["otpusk"] = ssh.Itog{Vyvod: []byte("klyucha\tnet\n")}
	if snyat, err := SnyatKlyuch(ctx, m, pk); err != nil || snyat {
		t.Fatalf("строки не было: %v %v", snyat, err)
	}
	m.otvety["otpusk"] = ssh.Itog{Kod: 3, Vyvod: []byte("posledniy_vhod\tda\n")}
	if _, err := SnyatKlyuch(ctx, m, pk); !errors.Is(err, ErrPosledniyVhod) || errors.Is(err, ErrVhodyNeProvereny) {
		t.Fatalf("последний вход: %v", err)
	}
	m.otvety["otpusk"] = ssh.Itog{Kod: 3, Vyvod: []byte("posledniy_vhod\tneizvestno\n")}
	if _, err := SnyatKlyuch(ctx, m, pk); !errors.Is(err, ErrVhodyNeProvereny) || !errors.Is(err, ErrPosledniyVhod) {
		t.Fatalf("входы не проверены: %v", err)
	}
	// Код 3 без строки о последнем входе или с незнакомым значением и успех
	// без итоговой строки - сбой, а не «последний вход» и не «не было».
	var o *Oshibka
	for _, it := range []ssh.Itog{{Kod: 3}, {Kod: 3, Vyvod: []byte("posledniy_vhod\tmozhet\n")}, {}, {Kod: 1, Oshibki: "cp: No space left on device"}} {
		m.otvety["otpusk"] = it
		if _, err := SnyatKlyuch(ctx, m, pk); !errors.As(err, &o) || o.Shag != "otpusk" {
			t.Fatalf("%+v: %v", it, err)
		}
	}
	m = novaya()
	m.oshibka = errors.New("обрыв")
	if _, err := SnyatKlyuch(ctx, m, pk); err == nil {
		t.Fatal("обрыв потерян")
	}
}

func TestZapisatKlyuch(t *testing.T) {
	pk, telo := klyuchKompyutera(t)
	m := novaya()
	if err := ZapisatKlyuch(context.Background(), m, pk); err != nil || m.vyzovy[0] != "ustanovka klyuch "+telo {
		t.Fatalf("%v %q", err, m.vyzovy)
	}
	m.otvety["ustanovka"] = ssh.Itog{Kod: 1, Oshibki: "отметки установки нет"}
	if err := ZapisatKlyuch(context.Background(), m, pk); err == nil {
		t.Fatal("отказ потерян")
	}
}
