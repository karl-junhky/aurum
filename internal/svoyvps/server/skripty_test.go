package server

import (
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// bashSkript гоняет скрипт настоящим bash (на Windows это Git Bash), как
// его гоняет SSH-слой: текст в stdin, аргументы после «--». Нет bash -
// проверка пропускается.
func bashSkript(t *testing.T, tekst string, env []string, argumenty ...string) (vyvod string, kod int) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("нет bash")
	}
	cmd := exec.Command(bash, append([]string{"-s", "--"}, argumenty...)...)
	cmd.Stdin = strings.NewReader(tekst)
	cmd.Env = append(os.Environ(), env...)
	var oshibki strings.Builder
	cmd.Stderr = &oshibki
	out, err := cmd.Output()
	if err != nil {
		var vyhod *exec.ExitError
		if !errors.As(err, &vyhod) {
			t.Fatalf("bash: %v", err)
		}
		kod = vyhod.ExitCode()
	}
	// Виден, только если проверка упала.
	if oshibki.Len() > 0 {
		t.Logf("stderr скрипта:\n%s", oshibki.String())
	}
	return string(out), kod
}

// razbit делит вывод на строки, табуляция становится пробелом.
func razbit(s string) []string {
	var r []string
	for _, x := range strings.Split(strings.TrimSpace(strings.ReplaceAll(s, "\r", "")), "\n") {
		if x != "" {
			r = append(r, strings.ReplaceAll(x, "\t", " "))
		}
	}
	return r
}

func chitat(t *testing.T, put string) []string {
	t.Helper()
	b, err := os.ReadFile(put)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return razbit(string(b))
}

// zapisat кладёт строки файлом, создавая каталоги.
func zapisat(t *testing.T, put string, stroki ...string) {
	t.Helper()
	tekst := ""
	if len(stroki) > 0 {
		tekst = strings.Join(stroki, "\n") + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(put), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte(tekst), 0o600); err != nil {
		t.Fatal(err)
	}
}

func est(put string) bool {
	_, err := os.Stat(put)
	return err == nil
}

// podstavaSistemy заменяет утилиты, которые трогают систему: правки пишутся
// в $LOG, sysctl живёт в файле $SYSCTL, показ tc берётся из $TC, показ
// iptables и ip6tables - из $IPT. Служба vpn-uzel работает, если задана
// AKTIVNA. Сбои задаются переменными SSHD_T_SBOY, SSHD_RELOAD_SBOY,
// SYSCTL_SBOY (имя ключа, запись которого падает) и TC_SBOY (replace или
// del).
const podstavaSistemy = `id() { echo 0; }
uname() { if [ "$1" = -m ]; then echo x86_64; else echo 6.8.0-test; fi; }
chown() { :; }
systemctl() {
    echo "systemctl $*" >> "$LOG"
    case "$1" in
    is-active) [ -n "${AKTIVNA:-}" ] ;;
    try-reload-or-restart) [ -z "${SSHD_RELOAD_SBOY:-}" ] ;;
    esac
}
sysctl() {
    case "$1 $2" in
    "-n "*)
        awk -v k="$2" 'index($0, k "=") == 1 { print substr($0, length(k) + 2) }' "$SYSCTL"
        return ;;
    "-q -p")
        awk -F' = ' '/^[a-z]/ { print $1 "=" $2 }' "$3" | while IFS= read -r para; do sysctl -q -w "$para"; done
        return ;;
    esac
    [ "${3%%=*}" != "${SYSCTL_SBOY:-}" ] || return 1
    echo "sysctl $3" >> "$LOG"
    { grep -v "^${3%%=*}=" "$SYSCTL" || true; echo "$3"; } > "$SYSCTL.t"
    mv "$SYSCTL.t" "$SYSCTL"
}
tc() {
    if [ "$2" = show ]; then cat "$TC"; return; fi
    echo "tc $*" >> "$LOG"
    [ "$2" != "${TC_SBOY:-}" ]
}
ip() { echo "default via 203.0.113.1 dev eth0 proto static"; }
iptables() { ipt iptables "$@"; }
ip6tables() { ipt ip6tables "$@"; }
ipt() {
    local imya="$1"
    shift
    if [ "$3" = -S ]; then cat "$IPT"; else echo "$imya $*" >> "$LOG"; fi
}
sshd() { echo "sshd $*" >> "$LOG"; [ -z "${SSHD_T_SBOY:-}" ]; }
`

// koren это машина в каталоге: пути скриптов переносятся в put.
type koren struct{ put, log, sysctl, tc, ipt string }

func (k koren) f(put string) string { return k.put + put }

func pustoyKoren(t *testing.T) koren {
	t.Helper()
	d := filepath.ToSlash(t.TempDir())
	k := koren{put: d + "/k", log: d + "/log", sysctl: d + "/sysctl", tc: d + "/tc", ipt: d + "/ipt"}
	zapisat(t, k.ipt, "-P INPUT ACCEPT")
	return k
}

// prognat гоняет скрипт на машине k с подставными утилитами.
func (k koren) prognat(t *testing.T, tekst string, env []string, argumenty ...string) ([]string, int) {
	t.Helper()
	r := strings.NewReplacer("/etc/", k.put+"/etc/", "/root/", k.put+"/root/", "/usr/local/", k.put+"/usr/local/",
		"/var/lib/", k.put+"/var/lib/", "/proc/", k.put+"/proc/", "/run/", k.put+"/run/")
	out, kod := bashSkript(t, podstavaSistemy+r.Replace(tekst),
		append([]string{"LOG=" + k.log, "SYSCTL=" + k.sysctl, "TC=" + k.tc, "IPT=" + k.ipt}, env...), argumenty...)
	return razbit(out), kod
}

// novyyKoren это машина после установки: отметка с двумя ключами, снимок
// с очередью mq ядра, наш sysctl с default_qdisc, наш файл sshd, строки
// ключей рядом со строкой человека.
func novyyKoren(t *testing.T) koren {
	t.Helper()
	k := pustoyKoren(t)
	// Временные правила программы, оставшиеся от упавшей проверки, рядом с
	// правилом человека.
	zapisat(t, k.ipt, "-P INPUT ACCEPT", "-A INPUT -p tcp -m tcp --dport 22 -j ACCEPT",
		"-A INPUT -p tcp -m tcp --dport 80 -m comment --comment affory-acme -j ACCEPT",
		"-A INPUT -p udp -m udp --dport 9443 -m comment --comment affory-podbor -j ACCEPT")
	zapisat(t, k.f("/etc/affory/klyuchi"), "AAAAnash1", "AAAAnash2")
	zapisat(t, k.f("/etc/affory/sostav.json"), `{"format":1,"sostoyanie":"gotovo"}`)
	zapisat(t, k.f("/etc/affory/prezhnee"), "net.core.rmem_default=212992", "net.core.default_qdisc=fq_codel",
		"qdisc_dev=eth0", "qdisc=mq", "qdisc_nomer=0:")
	zapisat(t, k.f("/etc/sysctl.d/90-affory.conf"), "# affory", "net.core.rmem_default = 16777216", "net.core.default_qdisc = fq")
	zapisat(t, k.f("/etc/ssh/sshd_config.d/0-affory.conf"), "PasswordAuthentication no")
	zapisat(t, k.f("/root/.ssh/authorized_keys"), "ssh-ed25519 AAAAchelovek me@pc", "ssh-ed25519 AAAAnash1 affory-PC", "ssh-ed25519 AAAAnash2 affory-PC2")
	zapisat(t, k.f("/usr/local/sbin/vpn-uzel"), "ELF")
	zapisat(t, k.f("/etc/systemd/system/vpn-uzel.service"), "[Unit]")
	zapisat(t, k.f("/var/lib/vpn-uzel/sostoyanie.json"), "{}")
	zapisat(t, k.sysctl, "net.core.rmem_default=16777216", "net.core.default_qdisc=fq")
	// После перезагрузки: корень mq ядра, дети fq по нашему default_qdisc.
	zapisat(t, k.tc, "qdisc mq 0: root", "qdisc fq 0: parent :1 limit 10000p")
	return k
}

func prognatSnos(t *testing.T, k koren, env ...string) ([]string, int) {
	t.Helper()
	return k.prognat(t, skriptSnosa, env)
}

// peresadka это пересадка очереди ядром: корень на pfifo и его снятие.
var peresadka = []string{"tc qdisc replace dev eth0 root pfifo", "tc qdisc del dev eth0 root"}

// vyzovyTC это правки очереди из журнала подставных утилит.
func (k koren) vyzovyTC(t *testing.T) []string {
	t.Helper()
	var tc []string
	for _, z := range chitat(t, k.log) {
		if strings.HasPrefix(z, "tc ") {
			tc = append(tc, z)
		}
	}
	return tc
}

// sneseno проверяет машину после полного сноса.
func sneseno(t *testing.T, k koren) {
	t.Helper()
	for _, put := range []string{"/etc/affory", "/etc/sysctl.d/90-affory.conf", "/etc/ssh/sshd_config.d/0-affory.conf",
		"/etc/ssh/sshd_config.d/0-affory.conf.snos", "/usr/local/sbin/vpn-uzel", "/etc/systemd/system/vpn-uzel.service", "/var/lib/vpn-uzel"} {
		if est(k.f(put)) {
			t.Errorf("осталось %s", put)
		}
	}
	if ak := chitat(t, k.f("/root/.ssh/authorized_keys")); !slices.Equal(ak, []string{"ssh-ed25519 AAAAchelovek me@pc"}) {
		t.Errorf("authorized_keys %q", ak)
	}
	sysctl := chitat(t, k.sysctl)
	slices.Sort(sysctl)
	if !slices.Equal(sysctl, []string{"net.core.default_qdisc=fq_codel", "net.core.rmem_default=212992"}) {
		t.Errorf("sysctl %q", sysctl)
	}
}

func TestSnosSkript(t *testing.T) {
	gotovo := []string{"parol vernulsya", "otmetka snyata", "klyuchi snyaty", "gotovo da"}

	t.Run("полный", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k)
		if kod != 0 || !slices.Equal(vyvod, gotovo) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		sneseno(t, k)
		// Корень совпал со снимком, но default_qdisc меняли мы: дети mq
		// возвращаются только через pfifo и снятие корня.
		log := chitat(t, k.log)
		for _, z := range []string{"tc qdisc replace dev eth0 root pfifo", "tc qdisc del dev eth0 root", "sshd -t", "systemctl try-reload-or-restart ssh.service",
			"iptables -w 5 -D INPUT -p tcp -m tcp --dport 80 -m comment --comment affory-acme -j ACCEPT",
			"iptables -w 5 -D INPUT -p udp -m udp --dport 9443 -m comment --comment affory-podbor -j ACCEPT"} {
			if !slices.Contains(log, z) {
				t.Errorf("нет вызова %q: %q", z, log)
			}
		}
		if slices.ContainsFunc(log, func(s string) bool { return strings.Contains(s, "--dport 22") }) {
			t.Errorf("снято правило человека: %q", log)
		}
	})

	t.Run("sshd -t отвергает, повтор", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k, "SSHD_T_SBOY=1")
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo sshd -t отвергает настройку и без нашего файла") {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		// Файл sshd и отметка на месте, строки ключей тоже: повтору есть что
		// снимать. sysctl и очередь к этому шагу уже вернулись, отметки
		// пересадки нет.
		if !est(k.f("/etc/ssh/sshd_config.d/0-affory.conf")) || est(k.f("/etc/ssh/sshd_config.d/0-affory.conf.snos")) ||
			!est(k.f("/etc/affory/klyuchi")) || len(chitat(t, k.f("/root/.ssh/authorized_keys"))) != 3 ||
			!slices.Equal(k.vyzovyTC(t), peresadka) || est(k.f("/etc/affory/peresadka")) {
			t.Fatal("после отказа sshd -t машина не та")
		}
		if vyvod, kod = prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo) {
			t.Fatalf("повтор: код %d %q", kod, vyvod)
		}
		sneseno(t, k)
	})

	t.Run("sshd не перечитал", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k, "SSHD_RELOAD_SBOY=1")
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo sshd не перечитал настройку") || !est(k.f("/etc/ssh/sshd_config.d/0-affory.conf")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})

	t.Run("sysctl не вернулся, повтор", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k, "SYSCTL_SBOY=net.core.rmem_default")
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo sysctl net.core.rmem_default не вернулся к 212992") ||
			!slices.Contains(vyvod, "parol vernulsya") || !est(k.f("/etc/sysctl.d/90-affory.conf")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		// Пароль вернулся ещё в первом проходе, повтор доснимает остальное.
		if vyvod, kod = prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo[1:]) {
			t.Fatalf("повтор: код %d %q", kod, vyvod)
		}
		sneseno(t, k)
	})

	t.Run("отодвинутый файл sshd", func(t *testing.T) {
		k := novyyKoren(t)
		if err := os.Rename(k.f("/etc/ssh/sshd_config.d/0-affory.conf"), k.f("/etc/ssh/sshd_config.d/0-affory.conf.snos")); err != nil {
			t.Fatal(err)
		}
		if vyvod, kod := prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		sneseno(t, k)
	})

	// Очередь не меняли: default_qdisc в нашем файле нет, корень как в
	// снимке. Корень с номером ставил человек, до установки или после: его
	// не трогают вовсе.
	for _, s := range []struct {
		imya     string
		prezhnee []string
		tc       string
	}{
		{"очередь не меняли", []string{"qdisc_dev=eth0", "qdisc=mq", "qdisc_nomer=0:"}, "qdisc mq 0: root"},
		{"корень руками до установки", []string{"qdisc_dev=eth0", "qdisc=cake", "qdisc_nomer=8001:"}, "qdisc cake 8001: root refcnt 2 bandwidth 100Mbit"},
		{"корень руками после установки", []string{"qdisc_dev=eth0", "qdisc=fq_codel", "qdisc_nomer=0:"}, "qdisc cake 8001: root refcnt 2 bandwidth 100Mbit"},
		{"pfifo руками после установки", []string{"qdisc_dev=eth0", "qdisc=fq_codel", "qdisc_nomer=0:"}, "qdisc pfifo 8001: root refcnt 2 limit 500p"},
	} {
		t.Run(s.imya, func(t *testing.T) {
			k := novyyKoren(t)
			zapisat(t, k.f("/etc/affory/prezhnee"), append([]string{"net.core.rmem_default=212992"}, s.prezhnee...)...)
			zapisat(t, k.f("/etc/sysctl.d/90-affory.conf"), "# affory", "net.core.rmem_default = 16777216")
			zapisat(t, k.tc, s.tc)
			if vyvod, kod := prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo) {
				t.Fatalf("код %d %q", kod, vyvod)
			}
			if tc := k.vyzovyTC(t); len(tc) > 0 {
				t.Fatalf("очередь трогали: %q", tc)
			}
		})
	}

	// pfifo с номером и отметкой оставляет наша же пересадка, у которой не
	// прошло снятие корня: её снос доводит.
	t.Run("pfifo оборванной пересадки", func(t *testing.T) {
		k := novyyKoren(t)
		zapisat(t, k.f("/etc/affory/prezhnee"), "net.core.rmem_default=212992", "qdisc_dev=eth0", "qdisc=fq_codel", "qdisc_nomer=0:")
		zapisat(t, k.f("/etc/sysctl.d/90-affory.conf"), "# affory", "net.core.rmem_default = 16777216")
		zapisat(t, k.f("/etc/affory/peresadka"))
		zapisat(t, k.tc, "qdisc pfifo 8001: root refcnt 2 limit 1000p")
		if vyvod, kod := prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		if tc := k.vyzovyTC(t); !slices.Equal(tc, peresadka) {
			t.Fatalf("вызовы tc %q", tc)
		}
	})

	// Снятие корня не прошло: отметка и всё остальное для повтора на месте,
	// повтор доводит пересадку. Не прошла замена: отметки нет.
	t.Run("пересадка оборвалась, повтор", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k, "TC_SBOY=del")
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo очередь eth0 не вернулась к mq, корень остался pfifo") ||
			!est(k.f("/etc/affory/peresadka")) || !est(k.f("/etc/sysctl.d/90-affory.conf")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		zapisat(t, k.tc, "qdisc pfifo 8001: root refcnt 2 limit 1000p")
		if vyvod, kod = prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, gotovo[1:]) {
			t.Fatalf("повтор: код %d %q", kod, vyvod)
		}
		sneseno(t, k)
		if tc := k.vyzovyTC(t); !slices.Equal(tc, append(slices.Clone(peresadka), peresadka...)) {
			t.Fatalf("вызовы tc %q", tc)
		}
	})
	// Отметка, пережившая свою пересадку (корень уже не её pfifo), уходит,
	// даже если снос оборвался дальше: pfifo, поставленный потом человеком,
	// повтор не тронет.
	t.Run("устаревшая отметка", func(t *testing.T) {
		k := novyyKoren(t)
		zapisat(t, k.f("/etc/affory/prezhnee"), "net.core.rmem_default=212992", "qdisc_dev=eth0", "qdisc=fq_codel", "qdisc_nomer=0:")
		zapisat(t, k.f("/etc/sysctl.d/90-affory.conf"), "# affory", "net.core.rmem_default = 16777216")
		zapisat(t, k.f("/etc/affory/peresadka"))
		zapisat(t, k.tc, "qdisc cake 8001: root refcnt 2 bandwidth 100Mbit")
		if vyvod, kod := prognatSnos(t, k, "SSHD_T_SBOY=1"); kod != 1 || est(k.f("/etc/affory/peresadka")) || len(k.vyzovyTC(t)) > 0 {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})
	t.Run("отметка не записалась", func(t *testing.T) {
		k := novyyKoren(t)
		if err := os.MkdirAll(k.f("/etc/affory/peresadka"), 0o700); err != nil {
			t.Fatal(err)
		}
		zapisat(t, k.tc, "qdisc pfifo 8001: root refcnt 2 limit 1000p")
		vyvod, kod := prognatSnos(t, k)
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo отметка пересадки не записалась, очередь eth0 не вернулась к mq") || len(k.vyzovyTC(t)) > 0 {
			t.Fatalf("код %d %q, вызовы tc %q", kod, vyvod, k.vyzovyTC(t))
		}
	})
	t.Run("замена корня не прошла", func(t *testing.T) {
		k := novyyKoren(t)
		vyvod, kod := prognatSnos(t, k, "TC_SBOY=replace")
		if kod != 1 || !slices.Contains(vyvod, "ne_vyshlo очередь eth0 не вернулась к mq") || est(k.f("/etc/affory/peresadka")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})

	t.Run("без отметки", func(t *testing.T) {
		k := novyyKoren(t)
		if err := os.RemoveAll(k.f("/etc/affory")); err != nil {
			t.Fatal(err)
		}
		if vyvod, kod := prognatSnos(t, k); kod != 0 || !slices.Equal(vyvod, []string{"otmetki net"}) ||
			!est(k.f("/usr/local/sbin/vpn-uzel")) || len(chitat(t, k.f("/root/.ssh/authorized_keys"))) != 3 {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})

	t.Run("под центром", func(t *testing.T) {
		k := novyyKoren(t)
		zapisat(t, k.f("/root/.ssh/authorized_keys"), "ssh-ed25519 AAAAcentr vpn-centr")
		if vyvod, kod := prognatSnos(t, k); kod != 1 || len(vyvod) != 0 || !est(k.f("/usr/local/sbin/vpn-uzel")) || !est(k.f("/etc/affory")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})
}

// mashinaDlyaUstanovki это чистая машина для ustanovka.sh с очередью tc:
// systemd, bbr в ядре, sysctl по умолчанию, каталоги, которые есть всегда.
func mashinaDlyaUstanovki(t *testing.T, tc ...string) koren {
	t.Helper()
	k := pustoyKoren(t)
	zapisat(t, k.tc, tc...)
	zapisat(t, k.f("/proc/1/comm"), "systemd")
	zapisat(t, k.f("/proc/sys/net/ipv4/tcp_available_congestion_control"), "reno cubic bbr")
	zapisat(t, k.f("/run/vpn-uzel/pult.sock"))
	// Каталоги, которые на машине есть всегда.
	for _, d := range []string{"/usr/local/sbin", "/etc/systemd/system", "/etc/sysctl.d"} {
		if err := os.MkdirAll(k.f(d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	zapisat(t, k.sysctl, "net.core.rmem_max=212992", "net.core.wmem_max=212992", "net.core.rmem_default=212992",
		"net.core.default_qdisc=fq_codel", "net.ipv4.tcp_congestion_control=cubic")
	return k
}

// prognatUstanovku кладёт программу и юнит в каталог загрузки, как первая
// Ustanovit, и гоняет установку. Программа это скрипт, отвечающий на
// versiya.
func prognatUstanovku(t *testing.T, k koren, env ...string) ([]string, int) {
	t.Helper()
	zapisat(t, k.f("/var/lib/affory/zagruzka/vpn-uzel"), "#!/bin/sh", "echo 'vpn-uzel 0.6.0'")
	zapisat(t, k.f("/var/lib/affory/zagruzka/vpn-uzel.service"), string(yunit))
	return ustanovka(t, k, env...)
}

// ustanovka гоняет установку с тем, что лежит в каталоге загрузки: при
// совпавшем sha256 Ustanovit файлов не шлёт, и каталог пуст. Сокета в
// каталоге не создать, поэтому пульт проверяется по наличию файла.
func ustanovka(t *testing.T, k koren, env ...string) ([]string, int) {
	t.Helper()
	if err := os.MkdirAll(k.f("/var/lib/affory/zagruzka"), 0o700); err != nil {
		t.Fatal(err)
	}
	return k.prognat(t, strings.ReplaceAll(skriptUstanovki, " -S ", " -e "), env, "AAAAnash1")
}

// doKonca говорит, что скрипт дошёл до конца: последняя строка «gotovo da».
func doKonca(vyvod []string) bool { return len(vyvod) > 0 && vyvod[len(vyvod)-1] == "gotovo da" }

func preduprezhdeniya(vyvod []string) []string {
	var p []string
	for _, z := range vyvod {
		if s, ok := strings.CutPrefix(z, "preduprezhdenie "); ok {
			p = append(p, s)
		}
	}
	return p
}

// Очередь живому интерфейсу пересаживает ядро, как после перезагрузки;
// корень, поставленный человеком, не трогается.
func TestUstanovkaSkript(t *testing.T) {
	pfifo := []string{"qdisc pfifo 8001: root refcnt 2 limit 1000p"}
	cake := []string{"qdisc cake 8001: root refcnt 2 bandwidth 100Mbit"}
	// Снимок прошлой установки: очередь была ядерной.
	bylSnimok := []string{"qdisc_dev=eth0", "qdisc=fq_codel", "qdisc_nomer=0:"}
	for _, s := range []struct {
		imya string
		tc   []string
		// bylSnimok это строки очереди в снимке прошлой установки, без них
		// снимок снимает эта; otmetka - отметка её оборванной пересадки.
		bylSnimok       []string
		otmetka         bool
		snimok          []string
		vyzovy          []string
		preduprezhdenie string
	}{
		{"одна очередь ядра", []string{"qdisc fq_codel 0: root refcnt 2 limit 10240p"}, nil, false, []string{"qdisc=fq_codel", "qdisc_nomer=0:"}, peresadka, ""},
		{"mq ядра", []string{"qdisc mq 0: root", "qdisc fq_codel 0: parent :1 limit 10240p", "qdisc fq_codel 0: parent :2 limit 10240p"}, nil, false,
			[]string{"qdisc=mq", "qdisc_nomer=0:"}, peresadka, ""},
		{"mq уже с fq", []string{"qdisc mq 0: root", "qdisc fq 0: parent :1 limit 10000p", "qdisc ingress ffff: parent ffff:fff1 ----------------"}, nil, false,
			[]string{"qdisc=mq", "qdisc_nomer=0:"}, nil, ""},
		{"корень руками", cake, nil, false, []string{"qdisc=cake", "qdisc_nomer=8001:"}, nil, "очередь eth0 поставлена руками (cake), fq не ставлю"},
		{"fq руками", []string{"qdisc fq 8001: root refcnt 2 limit 10000p"}, nil, false, []string{"qdisc=fq", "qdisc_nomer=8001:"}, nil, ""},
		{"pfifo руками", pfifo, nil, false, []string{"qdisc=pfifo", "qdisc_nomer=8001:"}, nil, "очередь eth0 поставлена руками (pfifo), fq не ставлю"},
		{"pfifo руками после установки", pfifo, bylSnimok, false, []string{"qdisc=fq_codel", "qdisc_nomer=0:"}, nil,
			"очередь eth0 поставлена руками (pfifo), fq не ставлю"},
		{"pfifo оборванной пересадки", pfifo, bylSnimok, true, []string{"qdisc=fq_codel", "qdisc_nomer=0:"}, peresadka, ""},
		{"корень руками после установки", cake, bylSnimok, false, []string{"qdisc=fq_codel", "qdisc_nomer=0:"}, nil,
			"очередь eth0 поставлена руками (cake), fq не ставлю"},
		{"устаревшая отметка", []string{"qdisc fq 0: root refcnt 2 limit 10000p"}, bylSnimok, true, []string{"qdisc=fq_codel", "qdisc_nomer=0:"}, nil, ""},
		{"очередь не прочитана", nil, nil, false, []string{"qdisc=", "qdisc_nomer="}, nil, "очередь eth0 не прочитана, fq не ставлю"},
	} {
		t.Run(s.imya, func(t *testing.T) {
			k := mashinaDlyaUstanovki(t, s.tc...)
			if s.bylSnimok != nil {
				zapisat(t, k.f("/etc/affory/prezhnee"), append(chitat(t, k.sysctl), s.bylSnimok...)...)
			}
			if s.otmetka {
				zapisat(t, k.f("/etc/affory/peresadka"))
			}
			vyvod, kod := prognatUstanovku(t, k)
			if kod != 0 || !slices.Contains(vyvod, "perezapushchen programma yunit") || !slices.Contains(vyvod, "versiya vpn-uzel 0.6.0") || !doKonca(vyvod) {
				t.Fatalf("код %d %q", kod, vyvod)
			}
			var zhdu []string
			if s.preduprezhdenie != "" {
				zhdu = []string{s.preduprezhdenie}
			}
			if p := preduprezhdeniya(vyvod); !slices.Equal(p, zhdu) {
				t.Errorf("предупреждения %q", p)
			}
			if tc := k.vyzovyTC(t); !slices.Equal(tc, s.vyzovy) {
				t.Errorf("вызовы tc %q", tc)
			}
			if est(k.f("/etc/affory/peresadka")) {
				t.Error("отметка пересадки осталась")
			}
			prezhnee := chitat(t, k.f("/etc/affory/prezhnee"))
			for _, z := range append([]string{"qdisc_dev=eth0", "net.core.default_qdisc=fq_codel"}, s.snimok...) {
				if !slices.Contains(prezhnee, z) {
					t.Errorf("в снимке нет %q: %q", z, prezhnee)
				}
			}
			fsys := chitat(t, k.f("/etc/sysctl.d/90-affory.conf"))
			for _, z := range []string{"net.core.default_qdisc = fq", "net.ipv4.tcp_congestion_control = bbr", "net.core.rmem_default = 16777216"} {
				if !slices.Contains(fsys, z) {
					t.Errorf("в sysctl нет %q: %q", z, fsys)
				}
			}
			if est(k.f("/var/lib/affory")) || !est(k.f("/usr/local/sbin/vpn-uzel")) ||
				!strings.Contains(strings.Join(chitat(t, k.f("/etc/affory/sostav.json")), ""), `"sostoyanie":"gotovo"`) {
				t.Error("после установки машина не та")
			}
		})
	}

	// Повтор поверх работающей установки той же программой: каталог загрузки
	// пуст, снимок не переписывается (иначе в нём были бы наши значения),
	// служба не перезапускается, очередь уже fq.
	t.Run("повтор", func(t *testing.T) {
		k := mashinaDlyaUstanovki(t, "qdisc fq_codel 0: root refcnt 2 limit 10240p")
		if vyvod, kod := prognatUstanovku(t, k); kod != 0 {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		snimok := chitat(t, k.f("/etc/affory/prezhnee"))
		zapisat(t, k.tc, "qdisc fq 0: root refcnt 2 limit 10000p")
		vyvod, kod := ustanovka(t, k, "AKTIVNA=1")
		if kod != 0 || !doKonca(vyvod) || !slices.Contains(vyvod, "versiya vpn-uzel 0.6.0") ||
			slices.ContainsFunc(vyvod, func(s string) bool { return strings.HasPrefix(s, "perezapushchen") }) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		if est(k.f("/var/lib/affory")) || !est(k.f("/usr/local/sbin/vpn-uzel")) || !est(k.f("/etc/systemd/system/vpn-uzel.service")) {
			t.Fatal("после повтора машина не та")
		}
		if posle := chitat(t, k.f("/etc/affory/prezhnee")); !slices.Equal(posle, snimok) {
			t.Fatalf("снимок переписан: %q, был %q", posle, snimok)
		}
		if fsys := chitat(t, k.f("/etc/sysctl.d/90-affory.conf")); !slices.Contains(fsys, "net.core.default_qdisc = fq") {
			t.Fatalf("sysctl после повтора %q", fsys)
		}
		if tc := k.vyzovyTC(t); !slices.Equal(tc, peresadka) {
			t.Fatalf("вызовы tc %q", tc)
		}
	})

	// Снятие корня не прошло: корень остался pfifo с отметкой, и повтор
	// доводит пересадку. Не прошла замена: корень прежний, отметки нет.
	t.Run("пересадка оборвалась, повтор", func(t *testing.T) {
		k := mashinaDlyaUstanovki(t, "qdisc fq_codel 0: root refcnt 2 limit 10240p")
		vyvod, kod := prognatUstanovku(t, k, "TC_SBOY=del")
		if p := preduprezhdeniya(vyvod); kod != 0 || !doKonca(vyvod) || !slices.Equal(p, []string{"fq на eth0 не встал, корень остался pfifo"}) ||
			!est(k.f("/etc/affory/peresadka")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
		zapisat(t, k.tc, "qdisc pfifo 8001: root refcnt 2 limit 1000p")
		vyvod, kod = ustanovka(t, k, "AKTIVNA=1")
		if kod != 0 || !doKonca(vyvod) || len(preduprezhdeniya(vyvod)) > 0 || est(k.f("/etc/affory/peresadka")) {
			t.Fatalf("повтор: код %d %q", kod, vyvod)
		}
		if tc := k.vyzovyTC(t); !slices.Equal(tc, append(slices.Clone(peresadka), peresadka...)) {
			t.Fatalf("вызовы tc %q", tc)
		}
	})
	// Отметка не записалась (на её месте каталог): корень не трогается.
	t.Run("отметка не записалась", func(t *testing.T) {
		k := mashinaDlyaUstanovki(t, "qdisc pfifo 8001: root refcnt 2 limit 1000p")
		if err := os.MkdirAll(k.f("/etc/affory/peresadka"), 0o700); err != nil {
			t.Fatal(err)
		}
		vyvod, kod := prognatUstanovku(t, k)
		if p := preduprezhdeniya(vyvod); kod != 0 || !doKonca(vyvod) || !slices.Equal(p, []string{"отметка пересадки не записалась, fq на eth0 не ставлю"}) ||
			len(k.vyzovyTC(t)) > 0 {
			t.Fatalf("код %d %q, вызовы tc %q", kod, vyvod, k.vyzovyTC(t))
		}
	})
	t.Run("замена корня не прошла", func(t *testing.T) {
		k := mashinaDlyaUstanovki(t, "qdisc fq_codel 0: root refcnt 2 limit 10240p")
		vyvod, kod := prognatUstanovku(t, k, "TC_SBOY=replace")
		if p := preduprezhdeniya(vyvod); kod != 0 || !doKonca(vyvod) || !slices.Equal(p, []string{"fq на eth0 не встал"}) ||
			est(k.f("/etc/affory/peresadka")) {
			t.Fatalf("код %d %q", kod, vyvod)
		}
	})
}

// Адрес точка-точка ip пишет без маски: осмотр берёт его одним узлом, а не
// сбоем.
func TestOsmotrAdresa(t *testing.T) {
	d := t.TempDir()
	f4, f6 := filepath.Join(d, "4"), filepath.Join(d, "6")
	zapisat(t, f4, `2: eth0    inet 203.0.113.10 peer 203.0.113.1/32 scope global eth0\       valid_lft forever preferred_lft forever`,
		`3: eth1    inet 198.51.100.7/24 brd 198.51.100.255 scope global eth1\       valid_lft forever preferred_lft forever`)
	zapisat(t, f6, `2: eth0    inet6 2001:db8::10/64 scope global \       valid_lft forever preferred_lft forever`)
	podstava := `ip() {
    case "$*" in
    "-4 -o addr show scope global") cat "$F4" ;;
    "-6 -o addr show scope global") cat "$F6" ;;
    esac
}
sshd() { :; }
`
	out, kod := bashSkript(t, podstava+skriptOsmotra, []string{"F4=" + filepath.ToSlash(f4), "F6=" + filepath.ToSlash(f6)})
	o, konec := razobratOsmotr([]byte(strings.ReplaceAll(out, "\r", "")))
	if kod != 0 || !konec {
		t.Fatalf("код %d, конец %v:\n%s", kod, konec, out)
	}
	zhdu4 := []Adres{{"eth0", netip.MustParsePrefix("203.0.113.10/32")}, {"eth1", netip.MustParsePrefix("198.51.100.7/24")}}
	zhdu6 := []Adres{{"eth0", netip.MustParsePrefix("2001:db8::10/64")}}
	if !slices.Equal(o.IPv4, zhdu4) || !slices.Equal(o.IPv6, zhdu6) || slices.ContainsFunc(o.Sboi, func(s string) bool { return strings.HasPrefix(s, "ipv") }) {
		t.Fatalf("IPv4 %v, IPv6 %v, сбои %q", o.IPv4, o.IPv6, o.Sboi)
	}
}
