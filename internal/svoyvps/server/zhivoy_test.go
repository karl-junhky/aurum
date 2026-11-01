//go:build zhivoy

package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// Живая проверка установки и сноса на настоящей машине, только по метке:
//
//	AFFORY_SSH_ADRES=<адрес> AFFORY_SSH_KLYUCH=<файл ключа root> AFFORY_SSH_PUT=pryamo|tunnel \
//	AFFORY_UZEL=<собранный vpn-uzel> [AFFORY_UFW=1] \
//	go test -tags zhivoy -run TestZhivoy -v ./internal/svoyvps/server/
//
// AFFORY_UFW=1 на машине с выключенным ufw включает его на время проверки
// портов и выключает обратно. Адреса в репозиторий не попадают: он
// публичный. Машина в конце сверяется со снимком до установки.

// devMarshruta кладёт в dev интерфейс маршрута по умолчанию.
const devMarshruta = `dev=$(ip -4 route show default | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')`

// ocheredFq печатает «fq» и номер корня интерфейса маршрута по умолчанию,
// если fq стоит везде: на многоочередной сетевой корень это mq, а fq стоит в
// каждой очереди под ним.
const ocheredFq = devMarshruta + ` &&
tc qdisc show dev "$dev" | awk '$4 == "root" { n = $3 }
    ($4 == "root" || ($4 == "parent" && $5 ~ /^:/)) && $2 != "mq" && $2 != "fq" { ne = 1 }
    END { print (ne || n == "") ? "ne-fq" : "fq", n }'`

// ubratProbnyyKlyuch убирает строку пробного ключа управления. Снос знает
// только ключи из отметки: упади проверка до установки, строка осталась бы.
// Файл заменяется, только если grep отработал: иначе на место
// authorized_keys встал бы пустой.
const ubratProbnyyKlyuch = `f=/root/.ssh/authorized_keys
if grep -q ' affory-proba-servera$' "$f"; then
    t="$f.affory-chast"
    cp -p -- "$f" "$t" && { grep -v ' affory-proba-servera$' "$f" > "$t" || [ $? -eq 1 ]; } && mv -f -- "$t" "$f" || {
        rm -f -- "$t"
        exit 1
    }
fi
echo da`

func TestZhivoy(t *testing.T) {
	adres, putKlyucha, putUzla := os.Getenv("AFFORY_SSH_ADRES"), os.Getenv("AFFORY_SSH_KLYUCH"), os.Getenv("AFFORY_UZEL")
	if adres == "" || putKlyucha == "" || putUzla == "" {
		t.Skip("нет AFFORY_SSH_ADRES, AFFORY_SSH_KLYUCH и AFFORY_UZEL")
	}
	klyuch, err := os.ReadFile(putKlyucha)
	if err != nil {
		t.Fatal(err)
	}
	var put ssh.Put
	if os.Getenv("AFFORY_SSH_PUT") == "tunnel" {
		put = ssh.PutCherezTunnel("127.0.0.1:10809")
	} else {
		a, err := ssh.FizicheskiyAdapter()
		if err != nil {
			t.Fatal(err)
		}
		put = ssh.PutPryamo(a.Indeks, a.Imya)
	}
	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Minute)
	defer otmena()
	s, err := ssh.Podklyuchit(ctx, ssh.Zapros{Adres: adres, Vhod: ssh.Vhod{Polzovatel: "root", Klyuch: klyuch}, Put: put})
	if err != nil {
		t.Fatalf("вход: %v", err)
	}
	defer s.Zakryt()

	if err := DozhdatsyaNastroyki(ctx, s); err != nil {
		t.Fatalf("cloud-init: %v", err)
	}
	do := snimok(t, ctx, s)
	o, err := Osmotret(ctx, s)
	if err != nil {
		t.Fatalf("осмотр: %v", err)
	}
	zamechaniya := o.Zamechaniya(time.Now())
	t.Logf("осмотр до: %s %s, %v", o.OSImya, o.Virt, kody(zamechaniya))
	for _, z := range zamechaniya {
		if z.Stop || z.Kod == KodUzheStoit {
			t.Fatalf("машина не годится для проверки: %v", kody(zamechaniya))
		}
	}

	// Ключ управления как у клиента: свой ключ кладётся в authorized_keys
	// root, вход им новым соединением.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	podpis, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.PolozhitKlyuchRoot(ctx, podpis, "affory-proba-servera", true)
	if err != nil {
		t.Fatalf("ключ root: %v", err)
	}
	defer r.Zakryt()
	defer proverit(t, context.Background(), s, ubratProbnyyKlyuch, "da\n")
	snesti := func() {
		if _, err := Snesti(context.Background(), r); err != nil {
			t.Errorf("снос: %v", err)
		}
	}
	snesen := false
	defer func() {
		if !snesen {
			snesti()
		}
	}()

	f, err := os.Open(putUzla)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	nachalo := time.Now()
	u, err := Ustanovit(ctx, r, f, podpis.PublicKey())
	if err != nil {
		t.Fatalf("установка: %v", err)
	}
	t.Logf("установка за %v: %+v", time.Since(nachalo).Round(time.Millisecond), u)
	if !strings.Contains(u.Perezapushchen, "programma") || !strings.HasPrefix(u.Versiya, "vpn-uzel ") {
		t.Fatalf("первая установка: %+v", u)
	}
	proverit(t, ctx, r, `systemctl is-active vpn-uzel.service && systemctl is-enabled vpn-uzel.service &&
test -S /run/vpn-uzel/pult.sock && test ! -e /var/lib/affory &&
sysctl -n net.core.rmem_default net.core.default_qdisc net.ipv4.tcp_congestion_control && `+ocheredFq+` &&
cat /etc/affory/sostav.json`,
		"active\nenabled\n16777216\nfq\nbbr\nfq 0:\n", `"sostoyanie":"gotovo"`, teloKlyucha(podpis.PublicKey()))

	nachalo = time.Now()
	u, err = Ustanovit(ctx, r, f, podpis.PublicKey())
	if err != nil || u.Perezapushchen != "" {
		t.Fatalf("повторная установка перезапустила службу: %+v %v", u, err)
	}
	t.Logf("повтор за %v", time.Since(nachalo).Round(time.Millisecond))
	// Остановленная служба поднимается, и итог это говорит.
	proverit(t, ctx, r, "systemctl stop vpn-uzel.service && echo da", "da\n")
	if u, err = Ustanovit(ctx, r, f, podpis.PublicKey()); err != nil || u.Perezapushchen != "ne-rabotala" {
		t.Fatalf("установка поверх остановленной: %+v %v", u, err)
	}

	o, err = Osmotret(ctx, r)
	if err != nil {
		t.Fatalf("осмотр после установки: %v", err)
	}
	if got := kody(o.Zamechaniya(time.Now())); !slices.Contains(got, string(KodUzheStoit)) || slices.ContainsFunc(got, func(k string) bool { return strings.HasSuffix(k, "!") }) {
		t.Fatalf("осмотр после установки: %v", got)
	}
	if o.VpnUzel == "" || o.Zanyat("tcp", 443) != "" {
		t.Fatalf("осмотр не видит свою программу: %q %q", o.VpnUzel, o.Zanyat("tcp", 443))
	}

	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	vtoroy, _ := gossh.NewSignerFromKey(priv2)
	if err := ZapisatKlyuch(ctx, r, vtoroy.PublicKey()); err != nil {
		t.Fatalf("ключ второго компьютера: %v", err)
	}
	sostav, paket, err := ProchitatUstanovku(ctx, r)
	if err != nil || sostav.Sostoyanie != "gotovo" || len(sostav.Klyuchi) != 2 || paket != nil {
		t.Fatalf("чтение установки: %+v %d байт %v", sostav, len(paket), err)
	}

	if os.Getenv("AFFORY_UFW") == "1" {
		proverkaUFW(t, ctx, r)
	}

	// Наш файл sshd, как его оставит выключение пароля (З6), только без
	// действующих строк: снос обязан его снять и перечитать sshd.
	proverit(t, ctx, r, "printf '# affory: proba\\n' > /etc/ssh/sshd_config.d/0-affory.conf && echo da", "da\n")
	s2, err := Snesti(ctx, r)
	snesen = true
	if err != nil || s2.BezOtmetki || !s2.ParolVernulsya {
		t.Fatalf("снос: %+v %v", s2, err)
	}
	if s2, err := Snesti(ctx, s); err != nil || !s2.BezOtmetki {
		t.Fatalf("повторный снос: %+v %v", s2, err)
	}
	o, err = Osmotret(ctx, s)
	if err != nil {
		t.Fatalf("осмотр после сноса: %v", err)
	}
	if o.Otmetka != nil || o.VpnUzel != "" || len(o.Chuzhoe) > 0 {
		t.Fatalf("после сноса не чисто: %+v", o)
	}
	if posle := snimok(t, ctx, s); posle != do {
		t.Fatalf("машина не вернулась.\nтолько до:\n%s\nтолько после:\n%s", raznica(do, posle), raznica(posle, do))
	}
}

// TestZhivoyPerezagruzka: установка, перезагрузка сервера, снос. После
// перезагрузки очередь интерфейса ставит само ядро с нулевым номером: fq,
// а на многоочередной сетевой mq с fq под ним. Снос обязан вернуть исходную. Те же переменные, что у TestZhivoy;
// машина перезагружается, поэтому только с AFFORY_PEREZAGRUZKA=1.
func TestZhivoyPerezagruzka(t *testing.T) {
	adres, putKlyucha, putUzla := os.Getenv("AFFORY_SSH_ADRES"), os.Getenv("AFFORY_SSH_KLYUCH"), os.Getenv("AFFORY_UZEL")
	if adres == "" || putKlyucha == "" || putUzla == "" || os.Getenv("AFFORY_PEREZAGRUZKA") != "1" {
		t.Skip("нет AFFORY_SSH_ADRES, AFFORY_SSH_KLYUCH, AFFORY_UZEL и AFFORY_PEREZAGRUZKA=1")
	}
	klyuch, err := os.ReadFile(putKlyucha)
	if err != nil {
		t.Fatal(err)
	}
	put := ssh.PutCherezTunnel("127.0.0.1:10809")
	if os.Getenv("AFFORY_SSH_PUT") != "tunnel" {
		a, err := ssh.FizicheskiyAdapter()
		if err != nil {
			t.Fatal(err)
		}
		put = ssh.PutPryamo(a.Indeks, a.Imya)
	}
	ctx, otmena := context.WithTimeout(context.Background(), 15*time.Minute)
	defer otmena()
	voyti := func() *ssh.Soedinenie {
		t.Helper()
		for {
			s, err := ssh.Podklyuchit(ctx, ssh.Zapros{Adres: adres, Vhod: ssh.Vhod{Polzovatel: "root", Klyuch: klyuch}, Put: put})
			if err == nil {
				return s
			}
			if ctx.Err() != nil {
				t.Fatalf("вход: %v", err)
			}
			t.Logf("вход: %v, жду", err)
			select {
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
			}
		}
	}
	s := voyti()
	// Уборка идёт через то соединение, что живо к концу: после перезагрузки
	// это уже новое. Закрытие отдельно: Fatalf в уборке до него не дошёл бы.
	defer func() { s.Zakryt() }()
	defer func() { proverit(t, context.Background(), s, ubratProbnyyKlyuch, "da\n") }()
	do := snimok(t, ctx, s)
	f, err := os.Open(putUzla)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Ключ управления свой, как у клиента: ключ проверки в отметку не
	// попадает, и снос его строку не тронет.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	podpis, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.PolozhitKlyuchRoot(ctx, podpis, "affory-proba-servera", true)
	if err != nil {
		t.Fatalf("ключ root: %v", err)
	}
	// Упавшая проверка не оставляет машину с установкой: снос по отметке,
	// без неё он ничего не делает.
	snesen := false
	defer func() {
		if snesen {
			return
		}
		if _, err := Snesti(context.Background(), s); err != nil {
			t.Errorf("снос: %v", err)
		}
	}()
	_, err = Ustanovit(ctx, r, f, podpis.PublicKey())
	r.Zakryt()
	if err != nil {
		t.Fatalf("установка: %v", err)
	}
	proverit(t, ctx, s, "systemd-run --on-active=3 --quiet /usr/bin/systemctl reboot && echo da", "da\n")
	s.Zakryt()
	time.Sleep(20 * time.Second)
	s = voyti()
	proverit(t, ctx, s, ocheredFq+`; systemctl is-active vpn-uzel.service`, "fq 0:\nactive\n")
	_, err = Snesti(ctx, s)
	snesen = true
	if err != nil {
		t.Fatalf("снос после перезагрузки: %v", err)
	}
	if posle := snimok(t, ctx, s); posle != do {
		t.Fatalf("машина не вернулась.\nтолько до:\n%s\nтолько после:\n%s", raznica(do, posle), raznica(posle, do))
	}
}

// raznica это строки a, которых нет в b.
func raznica(a, b string) string {
	est := map[string]bool{}
	for s := range strings.Lines(b) {
		est[s] = true
	}
	var r strings.Builder
	for s := range strings.Lines(a) {
		if !est[s] {
			r.WriteString(s)
		}
	}
	return r.String()
}

// proverkaUFW гоняет порты при выключенном и включённом ufw (с правилом на
// SSH) и возвращает ufw выключенным с одним нашим правилом: его уберёт снос
// из выключенного.
func proverkaUFW(t *testing.T, ctx context.Context, r *ssh.Soedinenie) {
	// Порт, на который пришло само это соединение: у sshd их бывает
	// несколько, и включённый ufw без нужного оборвал бы проверку.
	it, err := r.Vypolnit(ctx, `printf '%s' "${SSH_CONNECTION##* }"`, nil)
	portSSH := string(it.Vyvod)
	if _, perr := strconv.Atoi(portSSH); err != nil || perr != nil {
		t.Fatalf("порт SSH соединения %q %v: включать ufw не на что", portSSH, err)
	}
	it, err = r.Vypolnit(ctx, "ufw status | head -n 1; md5sum /etc/ufw/user.rules", nil)
	if err != nil || !strings.HasPrefix(string(it.Vyvod), "Status: inactive\n") {
		t.Fatalf("ufw не выключен, проверку портов не делаю: %q %v", it.Vyvod, err)
	}
	// Выключенный ufw не трогается вовсе, даже его файлы.
	p, err := OtkrytPorty(ctx, r, []int{443, 8443}, []int{443})
	if err != nil || p.UFW != "inactive" || len(p.Otkryty) != 0 || p.Zakryto != 0 {
		t.Fatalf("открыть при выключенном: %+v %v", p, err)
	}
	proverit(t, ctx, r, "ufw status | head -n 1; md5sum /etc/ufw/user.rules", string(it.Vyvod))

	proverit(t, ctx, r, "ufw allow "+portSSH+"/tcp comment affory-proba-ssh >/dev/null && ufw --force enable >/dev/null && echo da", "da\n")
	defer func() {
		proverit(t, context.Background(), r, `ufw --force disable >/dev/null
ufw delete allow `+portSSH+`/tcp >/dev/null; ufw delete allow 2053/tcp >/dev/null 2>&1; ufw delete deny 9443/tcp >/dev/null 2>&1
ufw status | head -n 1; ufw show added | grep -c "comment 'affory-vhod'"`, "Status: inactive\n1\n")
	}()
	if p, err = OtkrytPorty(ctx, r, []int{443, 8443}, []int{443}); err != nil || p.UFW != "active" || len(p.Otkryty) != 3 || p.Zakryto != 0 {
		t.Fatalf("открыть при включённом: %+v %v", p, err)
	}
	if p, err = OtkrytPorty(ctx, r, []int{443, 8443}, []int{443}); err != nil || len(p.Otkryty) != 3 || p.Zakryto != 0 {
		t.Fatalf("повтор: %+v %v", p, err)
	}
	proverit(t, ctx, r, `ufw status | grep -c '# affory-vhod'`, "6\n")

	// Чужое правило на порт: порт открыт, правило человека не трогается.
	proverit(t, ctx, r, `ufw allow 2053/tcp >/dev/null && ufw deny 9443/tcp >/dev/null && echo da`, "da\n")
	p, err = OtkrytPorty(ctx, r, []int{2053}, []int{443})
	if err != nil || !slices.Equal(p.UzheOtkryty, []string{"2053/tcp"}) || !slices.Equal(p.Otkryty, []string{"443/udp"}) || p.Zakryto != 2 {
		t.Fatalf("смена портов: %+v %v", p, err)
	}
	proverit(t, ctx, r, `ufw show added | grep -c '^ufw allow 2053/tcp$'; ufw status | grep -c '# affory-vhod'`, "1\n2\n")
	if _, err := OtkrytPorty(ctx, r, []int{9443}, nil); err == nil {
		t.Fatal("порт с запретом человека открыт")
	}
}

func proverit(t *testing.T, ctx context.Context, m Mashina, komanda string, zhdu ...string) {
	t.Helper()
	it, err := m.Vypolnit(ctx, komanda, nil)
	if err != nil || it.Kod != 0 {
		t.Fatalf("%q: %v код %d %q %q", komanda, err, it.Kod, it.Vyvod, it.Oshibki)
	}
	if !strings.HasPrefix(string(it.Vyvod), zhdu[0]) {
		t.Fatalf("%q: вывод %q, ждали начало %q", komanda, it.Vyvod, zhdu[0])
	}
	for _, z := range zhdu[1:] {
		if !strings.Contains(string(it.Vyvod), z) {
			t.Fatalf("%q: в выводе нет %q:\n%s", komanda, z, it.Vyvod)
		}
	}
}

// snimok это то, что установка меняет, а снос обязан вернуть. Из iptables
// только политика и наши пометки: включение ufw самой проверкой оставляет
// до перезагрузки переходы в его пустые цепочки. С проверкой ufw файлы
// user.rules сверяются по списку правил, а не байтами: ufw переписывает их
// в свой вид при первой правке, и правит их тут сама проверка.
func snimok(t *testing.T, ctx context.Context, m Mashina) string {
	t.Helper()
	pravilaUFW := "md5sum /etc/ufw/user.rules /etc/ufw/user6.rules"
	if os.Getenv("AFFORY_UFW") == "1" {
		pravilaUFW = "ufw show added"
	}
	it, err := m.Vypolnit(ctx, `sysctl net.core.rmem_max net.core.wmem_max net.core.rmem_default net.core.default_qdisc net.ipv4.tcp_congestion_control
`+devMarshruta+`
tc qdisc show dev "$dev" | awk '{ print "qdisc", $2, $3, $4, $5 }'
ls /etc/sysctl.d /etc/ssh/sshd_config.d
md5sum /root/.ssh/authorized_keys; stat -c '%a %U' /root/.ssh/authorized_keys
ls -d /etc/affory /var/lib/affory /var/lib/vpn-uzel /usr/local/sbin/vpn-uzel /etc/systemd/system/vpn-uzel.service 2>&1
systemctl is-enabled vpn-uzel.service 2>&1
if command -v ufw >/dev/null; then ufw status numbered; `+pravilaUFW+`; md5sum /etc/ufw/ufw.conf; fi
if command -v iptables >/dev/null; then iptables -S INPUT | grep -E '^-P|affory'; fi
sshd -T 2>/dev/null | grep -E '^(passwordauthentication|permitrootlogin) '
exit 0`, nil)
	if err != nil || it.Kod != 0 {
		t.Fatalf("снимок: %v %q", err, it.Oshibki)
	}
	return string(it.Vyvod)
}
