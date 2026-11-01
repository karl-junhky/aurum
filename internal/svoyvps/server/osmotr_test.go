package server

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// chistyy это вывод осмотра чистой 24.04 того же вида, что на тестовых
// машинах 01.11, с адресами из документационных сетей.
const chistyy = `os	ubuntu
os_versiya	24.04
os_imya	Ubuntu 24.04.5 LTS
arch	x86_64
pid1	systemd
virt	kvm
pamyat_mb	644
disk_mb	6370
chasy	1793549503
ntp	yes
cloud_init	done
ipv4	ens3 203.0.113.10/24
ipv6	ens3 2001:db8:9::2a08/128
marshrut4	ens3
marshrut6	ens3
qdisc	fq_codel
sysctl	net.core.rmem_max=212992
sysctl	net.core.default_qdisc=fq_codel
sysctl	net.ipv4.tcp_congestion_control=cubic
bbr	modul
slushaet	tcp 0.0.0.0:22 users:(("sshd",pid=863,fd=3),("systemd",pid=1,fd=91))
slushaet	tcp 127.0.0.53%lo:53 users:(("systemd-resolve",pid=520,fd=15))
slushaet	tcp [::]:22 users:(("sshd",pid=863,fd=4))
slushaet	udp 127.0.0.53%lo:53 users:(("systemd-resolve",pid=520,fd=14))
slushaet	udp 203.0.113.10%ens3:68 users:(("systemd-network",pid=634,fd=21))
slushaet	udp [fe80::f816:3eff:fe2f:99dc]%ens3:546 users:(("systemd-network",pid=634,fd=23))
ufw	inactive
iptables_input	ACCEPT
sshd_fayl	00-billpanel.conf
sshd_fayl	50-cloud-init.conf
sshd_port	22
sshd_permitrootlogin	yes
sshd_passwordauthentication	yes
sshd_kbdinteractiveauthentication	no
konec	da
`

var chasyChistogo = time.Unix(1793549503, 0)

func osmotret(t *testing.T, vyvod string) *Osmotr {
	t.Helper()
	o, konec := razobratOsmotr([]byte(vyvod))
	if !konec {
		t.Fatal("нет строки konec")
	}
	return o
}

// zamenit меняет строки с ключом k на новые (пустая - убрать).
func zamenit(vyvod, k string, novye ...string) string {
	var b strings.Builder
	for s := range strings.Lines(vyvod) {
		if strings.HasPrefix(s, k+"\t") {
			continue
		}
		if strings.HasPrefix(s, "konec\t") {
			for _, n := range novye {
				b.WriteString(k + "\t" + n + "\n")
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func kody(z []Zamechanie) []string {
	var k []string
	for _, x := range z {
		s := string(x.Kod)
		if x.Stop {
			s += "!"
		}
		k = append(k, s)
	}
	return k
}

func TestOsmotrChistoy(t *testing.T) {
	o := osmotret(t, chistyy)
	if z := o.Zamechaniya(chasyChistogo); len(z) != 0 {
		t.Fatalf("на чистой машине замечания %v", kody(z))
	}
	if o.PamyatMB != 644 || o.DiskMB != 6370 || o.Qdisc != "fq_codel" || o.BBR != "modul" || o.UFW != "inactive" {
		t.Fatalf("поля разобраны не так: %+v", o)
	}
	if len(o.IPv4) != 1 || o.IPv4[0].Interfeys != "ens3" || o.IPv4[0].Set.String() != "203.0.113.10/24" {
		t.Fatalf("IPv4: %+v", o.IPv4)
	}
	if len(o.IPv6) != 1 || o.Sysctl["net.core.rmem_max"] != "212992" || o.Sshd["permitrootlogin"] != "yes" {
		t.Fatalf("IPv6, sysctl или sshd: %+v %v %v", o.IPv6, o.Sysctl, o.Sshd)
	}
	if !slices.Equal(o.SshdFayly, []string{"00-billpanel.conf", "50-cloud-init.conf"}) {
		t.Fatalf("файлы sshd: %v", o.SshdFayly)
	}
	if len(o.Slushayut) != 6 {
		t.Fatalf("слушателей %d: %+v", len(o.Slushayut), o.Slushayut)
	}
	if s := o.Slushayut[5]; s.Proto != "udp" || s.Port != 546 || s.Process != "systemd-network" {
		t.Fatalf("слушатель с адресом IPv6: %+v", s)
	}
}

func TestOsmotrOborvan(t *testing.T) {
	if _, konec := razobratOsmotr([]byte(strings.Split(chistyy, "slushaet")[0])); konec {
		t.Fatal("оборванный вывод принят")
	}
}

func TestZanyat(t *testing.T) {
	o := osmotret(t, zamenit(chistyy, "slushaet",
		`tcp *:443 users:(("nginx",pid=10,fd=6))`,
		`udp [::]:8443 users:(("vpn-uzel",pid=11,fd=7))`,
		`tcp 0.0.0.0:8443 users:(("vpn-uzel",pid=11,fd=8))`,
		`udp 127.0.0.53%lo:53 users:(("systemd-resolve",pid=520,fd=14))`,
		`tcp 0.0.0.0:2053`,
	))
	sluchai := []struct {
		proto string
		port  int
		zhdu  string
	}{
		{"tcp", 443, "nginx"},
		{"udp", 443, ""},
		{"udp", 8443, ""}, // своя программа
		{"tcp", 8443, ""},
		{"udp", 53, "systemd-resolve"}, // на петле тоже занят
		{"tcp", 53, ""},
		{"tcp", 2053, "?"}, // ss не назвал процесс
	}
	for _, s := range sluchai {
		if got := o.Zanyat(s.proto, s.port); got != s.zhdu {
			t.Errorf("%s/%d: %q, ждали %q", s.proto, s.port, got, s.zhdu)
		}
		// Своя программа слушает там, где порт не занят чужим, но не везде:
		// свободный порт не её.
		svoy := s.port == 8443
		if got := o.SlushaetSvoy(s.proto, s.port); got != svoy {
			t.Errorf("%s/%d слушает своя: %v, ждали %v", s.proto, s.port, got, svoy)
		}
	}
}

func TestZamechaniya(t *testing.T) {
	sluchai := []struct {
		imya    string
		vyvod   string
		seychas time.Time
		zhdu    []string
	}{
		{"debian", zamenit(chistyy, "os", "debian"), chasyChistogo, []string{"ne-ubuntu!"}},
		{"18.04", zamenit(chistyy, "os_versiya", "18.04"), chasyChistogo, []string{"staraya-ubuntu!"}},
		{"20.04", zamenit(chistyy, "os_versiya", "20.04"), chasyChistogo, []string{"ubuntu-20.04"}},
		{"22.04", zamenit(chistyy, "os_versiya", "22.04"), chasyChistogo, nil},
		{"26.04", zamenit(chistyy, "os_versiya", "26.04"), chasyChistogo, nil},
		{"25.10", zamenit(chistyy, "os_versiya", "25.10"), chasyChistogo, []string{"ubuntu-ne-proverena"}},
		{"28.04", zamenit(chistyy, "os_versiya", "28.04"), chasyChistogo, []string{"ubuntu-ne-proverena"}},
		{"версия не число", zamenit(chistyy, "os_versiya", ""), chasyChistogo, []string{"ubuntu-ne-proverena"}},
		{"arm", zamenit(chistyy, "arch", "aarch64"), chasyChistogo, []string{"ne-x86!"}},
		{"не systemd", zamenit(chistyy, "pid1", "init"), chasyChistogo, []string{"ne-systemd!"}},
		{"openvz", zamenit(chistyy, "virt", "openvz"), chasyChistogo, []string{"konteyner!"}},
		{"lxc", zamenit(chistyy, "virt", "lxc"), chasyChistogo, []string{"konteyner!"}},
		{"мало памяти", zamenit(chistyy, "pamyat_mb", "90"), chasyChistogo, []string{"malo-pamyati!"}},
		{"память не узнали", zamenit(chistyy, "pamyat_mb", ""), chasyChistogo, nil},
		{"мало диска", zamenit(chistyy, "disk_mb", "100"), chasyChistogo, []string{"malo-diska!"}},
		{"нет ss и tc", zamenit(chistyy, "net_utility", "ss", "tc"), chasyChistogo,
			[]string{"net-utility!", "net-utility-zhelatelno"}},
		{"ss упал", zamenit(chistyy, "sboy", "ss tcp"), chasyChistogo, []string{"sboy-osmotra!"}},
		// Нечитаемый адрес не превращается в «адреса нет».
		{"адрес не разобран", zamenit(chistyy, "ipv4", "eth0 203.0.113.10/xx"), chasyChistogo,
			[]string{"sboy-osmotra!", "net-ipv4!"}},
		{"только IPv6", zamenit(chistyy, "ipv4"), chasyChistogo, []string{"net-ipv4!"}},
		{"частный IPv4", zamenit(chistyy, "ipv4", "eth0 10.0.0.5/24"), chasyChistogo, []string{"chastnyy-ipv4"}},
		{"CGNAT", zamenit(chistyy, "ipv4", "eth0 100.64.1.5/24"), chasyChistogo, []string{"chastnyy-ipv4"}},
		{"частный и публичный", zamenit(chistyy, "ipv4", "eth1 10.0.0.5/24", "eth0 203.0.113.10/24"), chasyChistogo, nil},
		{"часы вперёд", chistyy, chasyChistogo.Add(-6 * time.Minute), []string{"chasy"}},
		{"часы назад", chistyy, chasyChistogo.Add(6 * time.Minute), []string{"chasy"}},
		{"часы в пределе", chistyy, chasyChistogo.Add(4 * time.Minute), nil},
		{"cloud-init идёт", zamenit(chistyy, "cloud_init", "running"), chasyChistogo, []string{"nastraivaetsya!"}},
		{"чужое", zamenit(chistyy, "chuzhoe", "programma docker", "yunit xray.service"), chasyChistogo, []string{"ne-pustoy!"}},
		{"nginx на 80", zamenit(chistyy, "slushaet", `tcp 0.0.0.0:80 users:(("nginx",pid=1,fd=6))`), chasyChistogo,
			[]string{"ne-pustoy!"}},
		{"vpn-uzel без отметки", zamenit(chistyy, "vpn_uzel", "vpn-uzel 0.6.0, sing-box 1.14.2"), chasyChistogo,
			[]string{"ne-pustoy!"}},
		{"под центром", zamenit(zamenit(chistyy, "vpn_uzel", "vpn-uzel 0.4.0"), "centr", "klyuch", "nablyudenie"),
			chasyChistogo, []string{"pod-centrom!"}},
		{"своя установка", zamenit(zamenit(zamenit(chistyy, "vpn_uzel", "vpn-uzel 0.6.0"),
			"otmetka", `{"format":1,"sostoyanie":"gotovo","vpn_uzel":"vpn-uzel 0.6.0","klyuchi":["AAAA"]}`),
			"slushaet", `tcp 0.0.0.0:443 users:(("vpn-uzel",pid=1,fd=6))`), chasyChistogo, []string{"uzhe-stoit"}},
		{"nft inet drop", zamenit(chistyy, "nft_input", "inet filter input drop"), chasyChistogo,
			[]string{"chuzhoy-brandmauer"}},
		{"nft ufw", zamenit(zamenit(chistyy, "nft_input", "ip filter INPUT drop", "ip6 filter INPUT drop"), "ufw", "active"),
			chasyChistogo, nil},
		{"nft accept", zamenit(chistyy, "nft_input", "inet filter input accept"), chasyChistogo, nil},
		{"iptables DROP без ufw", zamenit(chistyy, "iptables_input", "DROP"), chasyChistogo, []string{"chuzhoy-brandmauer"}},
		{"iptables DROP с ufw", zamenit(zamenit(chistyy, "iptables_input", "DROP"), "ufw", "active"), chasyChistogo, nil},
		{"без bbr", zamenit(chistyy, "bbr", "net"), chasyChistogo, []string{"net-bbr"}},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			if got := kody(osmotret(t, s.vyvod).Zamechaniya(s.seychas)); !slices.Equal(got, s.zhdu) {
				t.Fatalf("замечания %v, ждали %v", got, s.zhdu)
			}
		})
	}
}

// Оборванный sostav.json и каталог без него (установка оборвалась между
// ключом и отметкой) это своя установка, а не чистая машина.
func TestOtmetkaIsporchena(t *testing.T) {
	for _, otmetka := range []string{`{"format":1,"sost`, ""} {
		o := osmotret(t, zamenit(chistyy, "otmetka", otmetka))
		if o.Otmetka == nil || o.Otmetka.Sostoyanie != "isporchena" {
			t.Fatalf("%q: %+v", otmetka, o.Otmetka)
		}
		if got := kody(o.Zamechaniya(chasyChistogo)); !slices.Equal(got, []string{"uzhe-stoit"}) {
			t.Fatalf("%q: замечания %v", otmetka, got)
		}
	}
}

// Портов у sshd бывает несколько: каждый своей строкой, в разборе все.
func TestSshdNeskolkoPortov(t *testing.T) {
	o := osmotret(t, strings.Replace(chistyy, "sshd_port\t22\n", "sshd_port\t22\nsshd_port\t2222\n", 1))
	if o.Sshd["port"] != "22 2222" || o.Sshd["permitrootlogin"] == "" {
		t.Fatalf("%q", o.Sshd)
	}
}

// Чужой текст с сервера не несёт управляющих символов дальше.
func TestOsmotrChistitTekst(t *testing.T) {
	o := osmotret(t, zamenit(chistyy, "chuzhoe", "programma \x1b[31mnginx‮"))
	if o.Chuzhoe[0] != "programma  [31mnginx " {
		t.Fatalf("%q", o.Chuzhoe[0])
	}
}
