package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Osmotr это то, что увидел скрипт осмотра. Поля как есть с машины, вывод
// делает Zamechaniya.
type Osmotr struct {
	OS        string `json:"os"`
	OSVersiya string `json:"os_versiya"`
	OSImya    string `json:"os_imya"`
	Arch      string `json:"arch"`
	PID1      string `json:"pid1"`
	// Virt это ответ systemd-detect-virt: kvm, none, lxc, openvz.
	Virt string `json:"virt"`
	// PamyatMB это MemAvailable, DiskMB это свободное на /var/lib; -1 - не
	// узнали.
	PamyatMB int `json:"pamyat_mb"`
	DiskMB   int `json:"disk_mb"`
	// Chasy это время сервера, Unix-секунды.
	Chasy     int64  `json:"chasy"`
	NTP       string `json:"ntp,omitempty"`
	CloudInit string `json:"cloud_init,omitempty"`
	// NetUtilit это утилиты, которых на машине нет.
	NetUtilit []string `json:"net_utilit,omitempty"`
	// Sboi это проверки, которые упали, хотя утилита есть («ss tcp»), и
	// строки, которые не разобрались («ipv4 eth0 ?»).
	Sboi []string `json:"sboi,omitempty"`
	IPv4 []Adres  `json:"ipv4,omitempty"`
	IPv6 []Adres  `json:"ipv6,omitempty"`
	// Marshrut4 и Marshrut6 это интерфейсы маршрута по умолчанию.
	Marshrut4 string `json:"marshrut4,omitempty"`
	Marshrut6 string `json:"marshrut6,omitempty"`
	// Qdisc это очередь интерфейса Marshrut4.
	Qdisc  string            `json:"qdisc,omitempty"`
	Sysctl map[string]string `json:"sysctl,omitempty"`
	// BBR: zagruzhen, modul (подгрузится при включении) или net.
	BBR       string      `json:"bbr"`
	Slushayut []Slushatel `json:"slushayut,omitempty"`
	// UFW: active, inactive или net.
	UFW string `json:"ufw"`
	// NftInput это цепочки nft с hook input: «семейство таблица цепочка
	// политика».
	NftInput []string `json:"nft_input,omitempty"`
	// IptablesInput это политика INPUT в iptables.
	IptablesInput string `json:"iptables_input,omitempty"`
	// SshdFayly это файлы /etc/ssh/sshd_config.d, Sshd это итог sshd -T по
	// port, permitrootlogin, passwordauthentication,
	// kbdinteractiveauthentication; несколько портов идут через пробел.
	SshdFayly []string          `json:"sshd_fayly,omitempty"`
	Sshd      map[string]string `json:"sshd,omitempty"`
	// Otmetka это своя отметка установки, если она есть.
	Otmetka *Sostav `json:"otmetka,omitempty"`
	// VpnUzel это ответ vpn-uzel versiya, если программа стоит.
	VpnUzel string `json:"vpn_uzel,omitempty"`
	// Centr это признаки сервера центра: klyuch, nablyudenie.
	Centr []string `json:"centr,omitempty"`
	// Chuzhoe это найденное чужое: «programma nginx», «katalog /etc/xray»,
	// «yunit xray.service».
	Chuzhoe []string `json:"chuzhoe,omitempty"`
}

// Adres это адрес на интерфейсе.
type Adres struct {
	Interfeys string       `json:"interfeys"`
	Set       netip.Prefix `json:"set"`
}

// Slushatel это открытый на приём сокет.
type Slushatel struct {
	Proto string `json:"proto"`
	// Adres как его пишет ss: 0.0.0.0, [::], *, 127.0.0.53%lo.
	Adres string `json:"adres"`
	Port  int    `json:"port"`
	// Process это имя процесса из ss -p, пусто - ss его не назвал.
	Process string `json:"process,omitempty"`
}

// Osmotret осматривает машину. На ней ничего не меняется.
func Osmotret(ctx context.Context, m Mashina) (o *Osmotr, err error) {
	defer zashchita("Osmotret", &err)
	it, err := m.Skript(ctx, skriptOsmotra)
	if err != nil {
		return nil, err
	}
	if it.Kod != 0 {
		return nil, oshibkaSkripta("osmotr", it)
	}
	o, konec := razobratOsmotr(it.Vyvod)
	if !konec {
		// Пустые поля оборванного вывода читались бы как «нет»: нет
		// чужого, нет отметки.
		return nil, &Oshibka{Shag: "osmotr", Tekst: "вывод осмотра оборван"}
	}
	return o, nil
}

func razobratOsmotr(vyvod []byte) (*Osmotr, bool) {
	o := &Osmotr{PamyatMB: -1, DiskMB: -1, Sysctl: map[string]string{}, Sshd: map[string]string{}}
	konec := false
	stroki(vyvod, func(k, z string) {
		switch k {
		case "otmetka":
			// Отметка не чистится до разбора: обрезка сломала бы JSON.
			s, err := razobratOtmetku([]byte(z))
			if err != nil {
				s = Sostav{Sostoyanie: "isporchena"}
			}
			o.Otmetka = &s
			return
		case "konec":
			konec = true
			return
		}
		z = chisto(z)
		switch k {
		case "os":
			o.OS = z
		case "os_versiya":
			o.OSVersiya = z
		case "os_imya":
			o.OSImya = z
		case "arch":
			o.Arch = z
		case "pid1":
			o.PID1 = z
		case "virt":
			o.Virt = z
		case "pamyat_mb":
			o.PamyatMB = chislo(z)
		case "disk_mb":
			o.DiskMB = chislo(z)
		case "chasy":
			if n, err := strconv.ParseInt(z, 10, 64); err == nil {
				o.Chasy = n
			}
		case "ntp":
			o.NTP = z
		case "cloud_init":
			o.CloudInit = z
		case "net_utility":
			o.NetUtilit = append(o.NetUtilit, z)
		case "sboy":
			o.Sboi = append(o.Sboi, z)
		case "ipv4", "ipv6":
			dev, set, _ := strings.Cut(z, " ")
			p, err := netip.ParsePrefix(set)
			if err != nil {
				// Пропуск читался бы как «адреса нет», вплоть до отказа
				// «только IPv6» при живом IPv4.
				o.Sboi = append(o.Sboi, k+" "+z)
				return
			}
			a := Adres{Interfeys: dev, Set: p}
			if k == "ipv4" {
				o.IPv4 = append(o.IPv4, a)
			} else {
				o.IPv6 = append(o.IPv6, a)
			}
		case "marshrut4":
			o.Marshrut4 = z
		case "marshrut6":
			o.Marshrut6 = z
		case "qdisc":
			o.Qdisc = z
		case "sysctl":
			if kk, zz, ok := strings.Cut(z, "="); ok {
				o.Sysctl[kk] = zz
			}
		case "bbr":
			o.BBR = z
		case "slushaet":
			if s, ok := razobratSlushatelya(z); ok {
				o.Slushayut = append(o.Slushayut, s)
			}
		case "ufw":
			o.UFW = z
		case "nft_input":
			o.NftInput = append(o.NftInput, z)
		case "iptables_input":
			o.IptablesInput = z
		case "sshd_fayl":
			o.SshdFayly = append(o.SshdFayly, z)
		case "vpn_uzel":
			o.VpnUzel = z
		case "centr":
			o.Centr = append(o.Centr, z)
		case "chuzhoe":
			o.Chuzhoe = append(o.Chuzhoe, z)
		default:
			if imya, ok := strings.CutPrefix(k, "sshd_"); ok {
				// Портов у sshd бывает несколько, каждый своей строкой.
				if bylo := o.Sshd[imya]; bylo != "" {
					z = bylo + " " + z
				}
				o.Sshd[imya] = z
			}
		}
	})
	return o, konec
}

func razobratOtmetku(b []byte) (Sostav, error) {
	var s Sostav
	if err := json.Unmarshal(b, &s); err != nil {
		return Sostav{}, fmt.Errorf("отметка не разобрана: %w", err)
	}
	s.Sostoyanie, s.VpnUzel = chisto(s.Sostoyanie), chisto(s.VpnUzel)
	for i, k := range s.Klyuchi {
		s.Klyuchi[i] = chisto(k)
	}
	return s, nil
}

// razobratSlushatelya разбирает «tcp 0.0.0.0:443 users:(("nginx",pid=1,fd=6))».
func razobratSlushatelya(z string) (Slushatel, bool) {
	polya := strings.Fields(z)
	if len(polya) < 2 {
		return Slushatel{}, false
	}
	i := strings.LastIndexByte(polya[1], ':')
	if i < 0 {
		return Slushatel{}, false
	}
	port, err := strconv.Atoi(polya[1][i+1:])
	if err != nil || port < 1 || port > 65535 {
		return Slushatel{}, false
	}
	s := Slushatel{Proto: polya[0], Adres: polya[1][:i], Port: port}
	if len(polya) > 2 {
		if _, ostatok, ok := strings.Cut(polya[2], `(("`); ok {
			s.Process, _, _ = strings.Cut(ostatok, `"`)
		}
	}
	return s, true
}

// svoyProcess это наша программа: её порты для подбора свободны.
const svoyProcess = "vpn-uzel"

// Zanyat называет чужой процесс, который слушает порт: proto tcp или udp.
// Пусто - порт свободен или его держит наша программа. Слушатель на любом
// адресе занимает порт: привязка ко всем адресам с ним не уживётся.
func (o *Osmotr) Zanyat(proto string, port int) string {
	for _, s := range o.Slushayut {
		if s.Proto != proto || s.Port != port || s.Process == svoyProcess {
			continue
		}
		if s.Process == "" {
			return "?"
		}
		return s.Process
	}
	return ""
}

// SlushaetSvoy: порт proto держит наша программа. Слушатель без имени
// процесса не в счёт: подтвердить нечем.
func (o *Osmotr) SlushaetSvoy(proto string, port int) bool {
	return slices.ContainsFunc(o.Slushayut, func(s Slushatel) bool {
		return s.Proto == proto && s.Port == port && s.Process == svoyProcess
	})
}

// Kod различает замечания осмотра: каждому свой текст и совет в службе.
type Kod string

const (
	KodNeUbuntu Kod = "ne-ubuntu"
	// Ниже 20.04.
	KodStarayaUbuntu Kod = "staraya-ubuntu"
	// 20.04: стандартная поддержка кончилась в мае 2025, обновления
	// безопасности только по Ubuntu Pro.
	KodUbuntu2004 Kod = "ubuntu-20.04"
	// Версия, которую живьём не проверяли (новее 26.04 или промежуточная).
	KodUbuntuNeProverena Kod = "ubuntu-ne-proverena"
	KodNeX86             Kod = "ne-x86"
	KodNeSystemd         Kod = "ne-systemd"
	// Контейнер (OpenVZ, LXC и прочие): ни модулей ядра, ни своей сети.
	KodKonteyner   Kod = "konteyner"
	KodMaloPamyati Kod = "malo-pamyati"
	KodMaloDiska   Kod = "malo-diska"
	// Нет нужной утилиты; в Chto их имена.
	KodNetUtility Kod = "net-utility"
	// Нет утилиты, без которой обойтись можно (tc, openssl, timeout).
	KodNetUtilityZhelatelno Kod = "net-utility-zhelatelno"
	// Проверка осмотра упала, хотя утилита есть: без неё вывод неполный
	// (без ss все порты сочлись бы свободными). В Chto что упало.
	KodSboyOsmotra Kod = "sboy-osmotra"
	// Ни одного глобального IPv4: только IPv6.
	KodNetIPv4 Kod = "net-ipv4"
	// IPv4 на интерфейсе только частный: облако с NAT один к одному или
	// NAT хостера. Различает проба порта с компьютера человека.
	KodChastnyyIPv4 Kod = "chastnyy-ipv4"
	// Часы расходятся больше чем на 5 минут: выпуск сертификата может не
	// пройти. В Chto разница.
	KodChasy Kod = "chasy"
	// cloud-init ещё работает: машина настраивается после создания.
	KodNastraivaetsya Kod = "nastraivaetsya"
	// На машине чужое: в Chto что найдено.
	KodNePustoy Kod = "ne-pustoy"
	// Сервер уже раздаёт VPN под управлением центра.
	KodPodCentrom Kod = "pod-centrom"
	// Своя установка уже стоит.
	KodUzheStoit Kod = "uzhe-stoit"
	// Входящие режет брандмауэр, который ufw и временные правила программы
	// не открывают: своя таблица nft или политика iptables без ufw.
	KodChuzhoyBrandmauer Kod = "chuzhoy-brandmauer"
	// bbr в ядре нет: reality без ровной скорости на дальних каналах, hy2
	// это не задевает.
	KodNetBBR Kod = "net-bbr"
)

// Zamechanie это вывод осмотра.
type Zamechanie struct {
	Kod Kod `json:"kod"`
	// Stop: установка дальше не идёт.
	Stop bool `json:"stop"`
	// Chto это подробности для текста: имена найденного, версия, разница
	// часов.
	Chto []string `json:"chto,omitempty"`
}

const (
	porogPamyatiMB = 128
	porogDiskaMB   = 256
	porogChasov    = 5 * time.Minute
)

var (
	// Проверенные живьём версии Ubuntu.
	proverennyeVersii = []string{"20.04", "22.04", "24.04", "26.04"}
	// Без этих утилит установка или осмотр не работают.
	nuzhnyeUtility = []string{"bash", "systemctl", "sysctl", "ip", "ss", "awk", "sed", "grep", "cmp", "sha256sum"}
	// Контейнеры по systemd-detect-virt.
	konteynery = []string{"openvz", "lxc", "lxc-libvirt", "systemd-nspawn", "docker", "podman", "rkt", "wsl", "proot", "pouch"}
	cgnat      = netip.MustParsePrefix("100.64.0.0/10")
)

// Zamechaniya выносит вывод по осмотру. seychas это часы компьютера.
func (o *Osmotr) Zamechaniya(seychas time.Time) []Zamechanie {
	var z []Zamechanie
	dobavit := func(kod Kod, stop bool, chto ...string) {
		z = append(z, Zamechanie{Kod: kod, Stop: stop, Chto: chto})
	}

	if len(o.Centr) > 0 {
		dobavit(KodPodCentrom, true, o.Centr...)
	}
	if o.Otmetka != nil {
		dobavit(KodUzheStoit, false, o.Otmetka.Sostoyanie, o.Otmetka.VpnUzel)
	}
	if kod, stop, ok := o.versiya(); ok {
		dobavit(kod, stop, o.OSImya)
	}
	if o.Arch != "x86_64" {
		dobavit(KodNeX86, true, o.Arch)
	}
	if o.PID1 != "systemd" {
		dobavit(KodNeSystemd, true, o.PID1)
	}
	if slices.Contains(konteynery, o.Virt) {
		dobavit(KodKonteyner, true, o.Virt)
	}
	if o.PamyatMB >= 0 && o.PamyatMB < porogPamyatiMB {
		dobavit(KodMaloPamyati, true, strconv.Itoa(o.PamyatMB))
	}
	if o.DiskMB >= 0 && o.DiskMB < porogDiskaMB {
		dobavit(KodMaloDiska, true, strconv.Itoa(o.DiskMB))
	}
	var nuzhnye, zhelatelnye []string
	for _, u := range o.NetUtilit {
		if slices.Contains(nuzhnyeUtility, u) {
			nuzhnye = append(nuzhnye, u)
		} else {
			zhelatelnye = append(zhelatelnye, u)
		}
	}
	if len(nuzhnye) > 0 {
		dobavit(KodNetUtility, true, nuzhnye...)
	}
	if len(zhelatelnye) > 0 {
		dobavit(KodNetUtilityZhelatelno, false, zhelatelnye...)
	}
	if len(o.Sboi) > 0 {
		dobavit(KodSboyOsmotra, true, o.Sboi...)
	}
	switch o.publichnyyIPv4() {
	case ipv4Net:
		dobavit(KodNetIPv4, true)
	case ipv4Chastnyy:
		dobavit(KodChastnyyIPv4, false, o.IPv4[0].Set.Addr().String())
	}
	if o.Chasy > 0 {
		raznica := time.Unix(o.Chasy, 0).Sub(seychas)
		if raznica > porogChasov || raznica < -porogChasov {
			dobavit(KodChasy, false, raznica.Round(time.Second).String())
		}
	}
	if o.CloudInit == "running" {
		dobavit(KodNastraivaetsya, true)
	}
	if chuzhoe := o.chuzhoe(); len(chuzhoe) > 0 {
		dobavit(KodNePustoy, true, chuzhoe...)
	}
	if b := o.chuzhoyBrandmauer(); len(b) > 0 {
		dobavit(KodChuzhoyBrandmauer, false, b...)
	}
	if o.BBR == "net" {
		dobavit(KodNetBBR, false)
	}
	return z
}

// versiya судит о системе: ok false - замечаний нет.
func (o *Osmotr) versiya() (kod Kod, stop, ok bool) {
	if o.OS != "ubuntu" {
		return KodNeUbuntu, true, true
	}
	god, mesyac, _ := strings.Cut(o.OSVersiya, ".")
	g, err1 := strconv.Atoi(god)
	m, err2 := strconv.Atoi(mesyac)
	switch {
	case err1 != nil || err2 != nil:
		return KodUbuntuNeProverena, false, true
	case g < 20:
		return KodStarayaUbuntu, true, true
	case o.OSVersiya == "20.04":
		return KodUbuntu2004, false, true
	case slices.Contains(proverennyeVersii, fmt.Sprintf("%d.%02d", g, m)):
		return "", false, false
	}
	return KodUbuntuNeProverena, false, true
}

type vidIPv4 int

const (
	ipv4Est vidIPv4 = iota
	ipv4Chastnyy
	ipv4Net
)

// publichnyyIPv4: есть ли на интерфейсах публичный IPv4. Частный и CGNAT
// (100.64.0.0/10) публичным не считаются.
func (o *Osmotr) publichnyyIPv4() vidIPv4 {
	if len(o.IPv4) == 0 {
		return ipv4Net
	}
	for _, a := range o.IPv4 {
		ip := a.Set.Addr()
		if !ip.IsPrivate() && !cgnat.Contains(ip) && ip.IsGlobalUnicast() {
			return ipv4Est
		}
	}
	return ipv4Chastnyy
}

// chuzhoe собирает то, из-за чего сервер не пустой: найденное скриптом,
// чужие слушатели 80 и 443/tcp (их ждёт проверка сертификата), vpn-uzel
// без своей отметки.
func (o *Osmotr) chuzhoe() []string {
	chto := slices.Clone(o.Chuzhoe)
	for _, port := range []int{80, 443} {
		if p := o.Zanyat("tcp", port); p != "" {
			chto = append(chto, fmt.Sprintf("port tcp/%d %s", port, p))
		}
	}
	if o.VpnUzel != "" && o.Otmetka == nil && len(o.Centr) == 0 {
		chto = append(chto, "programma vpn-uzel")
	}
	return chto
}

// chuzhoyBrandmauer: цепочки nft с hook input и запретом, которые не
// принадлежат iptables (таблицы ip filter и ip6 filter, там же ufw), и
// политика DROP в iptables без ufw. Правила ufw ставятся только при
// включённом ufw, временные правила программы - в iptables.
func (o *Osmotr) chuzhoyBrandmauer() []string {
	var b []string
	for _, c := range o.NftInput {
		polya := strings.Fields(c)
		if len(polya) != 4 || polya[3] == "accept" {
			continue
		}
		if polya[1] == "filter" && (polya[0] == "ip" || polya[0] == "ip6") {
			continue
		}
		b = append(b, "nft "+c)
	}
	if o.IptablesInput == "DROP" && o.UFW != "active" {
		b = append(b, "iptables INPUT DROP")
	}
	return b
}
