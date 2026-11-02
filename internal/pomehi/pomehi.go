// Package pomehi ищет на машине чужие программы, которые ломают VPN или могут
// ему мешать (1.10.0).
//
// Две группы, и решение о них принято владельцем 21.10.2026. «Ломает» это
// перехватчик пакетов (zapret, GoodbyeDPI, всё, что держит WinDivert.dll) и
// чужой VPN с маршрутом по умолчанию: окно говорит о них сразу, строкой на
// главном экране. «Может мешать» это антивирус, фильтр трафика и VPN-клиент
// без поднятого подключения: о них окно молчит до отказа «данные не пошли» и
// пишет их в выгрузку. Чужие процессы пакет не трогает, только называет.
//
// Разбор чужих клиентов и опыт в госте:
// vpn/docs/issledovaniya/2026-10-21-ustoychivost-windows.md.
package pomehi

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Gruppa решает, говорить о находке сразу или молчать до отказа.
type Gruppa string

const (
	Lomaet  Gruppa = "lomaet"
	Meshaet Gruppa = "meshaet"
)

// Vid выбирает фразу окна: перехватчик просят выключить, VPN отключить, а
// антивирусу добавить исключение.
type Vid string

const (
	VidObhod     Vid = "obhod"
	VidVPN       Vid = "vpn"
	VidAntivirus Vid = "antivirus"
	VidFiltr     Vid = "filtr"
)

// Nahodka это одна программа, а не один процесс: avp.exe и avpui.exe это один
// Kaspersky. Priznak говорит, чем найдено, и нужен выгрузке, а не экрану.
type Nahodka struct {
	Imya    string
	Vid     Vid
	Gruppa  Gruppa
	Priznak string
}

// Svoi это то, что пакет обязан не назвать чужим.
type Svoi struct {
	// KatalogProgrammy: процессы с образом отсюда наши. Сверка по пути, а не по
	// имени: v2rayN, Karing и Hiddify возят свой sing-box.exe.
	KatalogProgrammy string
	// Adaptery это индексы наших адаптеров.
	Adaptery []uint32
	// ImyaAdaptera задаёт служба, только пока сама поднимает или держит туннель:
	// индекс TUN она узнаёт не сразу, а фоновая проверка между созданием
	// адаптера и его запоминанием назвала бы наш туннель чужим. Выключенной
	// службе адаптер с этим именем не принадлежит.
	ImyaAdaptera string
}

type programma struct {
	imya   string
	vid    Vid
	gruppa Gruppa
}

// izvestnye: имя exe в нижнем регистре. Список Happ 4.5.4 (разбор 21.10.2026),
// клиенты, чьи exe видны на диске, и сверенные по исходникам и справке
// производителя 21.10.2026 (Hiddify, NekoBox, Throne, Clash Verge, AmneziaVPN,
// Outline, AdGuard, Proxifier, Kaspersky VPN). Имя, подтверждённое только
// косвенно, сюда не добавляется: ложная находка хуже пропущенной.
var izvestnye = map[string]programma{
	// Перехватчики пакетов. Оба держат драйвер WinDivert и ломают туннель
	// всегда, а не только в некоторых сетях.
	"winws.exe":      {"zapret", VidObhod, Lomaet},
	"goodbyedpi.exe": {"GoodbyeDPI", VidObhod, Lomaet},
	// Обход через локальный прокси. Туннелю сам не мешает, мешает только
	// системным прокси, а его служба называет отдельно (foreign-registry-hijack).
	"ciadpi.exe":   {"ByeDPI", VidObhod, Meshaet},
	"byedpi.exe":   {"ByeDPI", VidObhod, Meshaet},
	"spoofdpi.exe": {"SpoofDPI", VidObhod, Meshaet},
	"tpws.exe":     {"zapret (tpws)", VidObhod, Meshaet},
	// VPN-клиенты. Без поднятого подключения не мешают, с поднятым их находит
	// проверка адаптеров, и тогда они в группе «ломает».
	"warp-svc.exe":            {"Cloudflare WARP", VidVPN, Meshaet},
	"cloudflare warp.exe":     {"Cloudflare WARP", VidVPN, Meshaet},
	"happ.exe":                {"Happ", VidVPN, Meshaet},
	"happd.exe":               {"Happ", VidVPN, Meshaet},
	"karing.exe":              {"Karing", VidVPN, Meshaet},
	"karingservice.exe":       {"Karing", VidVPN, Meshaet},
	"v2rayn.exe":              {"v2rayN", VidVPN, Meshaet},
	"sotavpn.exe":             {"Sota", VidVPN, Meshaet},
	"sota-daemon.exe":         {"Sota", VidVPN, Meshaet},
	"hiddify.exe":             {"Hiddify", VidVPN, Meshaet},
	"hiddifycli.exe":          {"Hiddify", VidVPN, Meshaet},
	"nekobox.exe":             {"NekoBox", VidVPN, Meshaet},
	"nekobox_core.exe":        {"NekoBox", VidVPN, Meshaet},
	"throne.exe":              {"Throne", VidVPN, Meshaet},
	"thronecore.exe":          {"Throne", VidVPN, Meshaet},
	"clash-verge.exe":         {"Clash Verge", VidVPN, Meshaet},
	"clash-verge-service.exe": {"Clash Verge", VidVPN, Meshaet},
	"verge-mihomo.exe":        {"Clash Verge", VidVPN, Meshaet},
	"verge-mihomo-alpha.exe":  {"Clash Verge", VidVPN, Meshaet},
	"amneziavpn.exe":          {"AmneziaVPN", VidVPN, Meshaet},
	"amneziavpn-service.exe":  {"AmneziaVPN", VidVPN, Meshaet},
	"outline.exe":             {"Outline", VidVPN, Meshaet},
	"outlineservice.exe":      {"Outline", VidVPN, Meshaet},
	"adguardvpnsvc.exe":       {"AdGuard VPN", VidVPN, Meshaet},
	"ksdeui.exe":              {"Kaspersky VPN", VidVPN, Meshaet},
	// Фильтры трафика: пропускают соединения через себя и бывают причиной
	// «VPN включился, а данные не пошли».
	"adguard.exe":        {"AdGuard", VidFiltr, Meshaet},
	"adguardsvc.exe":     {"AdGuard", VidFiltr, Meshaet},
	"proxifier.exe":      {"Proxifier", VidFiltr, Meshaet},
	"avp.exe":            {"Kaspersky", VidAntivirus, Meshaet},
	"avpui.exe":          {"Kaspersky", VidAntivirus, Meshaet},
	"ekrn.exe":           {"ESET", VidAntivirus, Meshaet},
	"egui.exe":           {"ESET", VidAntivirus, Meshaet},
	"dwengine.exe":       {"Dr.Web", VidAntivirus, Meshaet},
	"dwnetfilter.exe":    {"Dr.Web", VidAntivirus, Meshaet},
	"drweb32.exe":        {"Dr.Web", VidAntivirus, Meshaet},
	"avastsvc.exe":       {"Avast", VidAntivirus, Meshaet},
	"avastui.exe":        {"Avast", VidAntivirus, Meshaet},
	"aswengsrv.exe":      {"Avast", VidAntivirus, Meshaet},
	"bdagent.exe":        {"Bitdefender", VidAntivirus, Meshaet},
	"bdservicehost.exe":  {"Bitdefender", VidAntivirus, Meshaet},
	"nortonsecurity.exe": {"Norton", VidAntivirus, Meshaet},
	"ccsvchst.exe":       {"Norton", VidAntivirus, Meshaet},
	"mcshield.exe":       {"McAfee", VidAntivirus, Meshaet},
	"mfemms.exe":         {"McAfee", VidAntivirus, Meshaet},
	"mbam.exe":           {"Malwarebytes", VidAntivirus, Meshaet},
	"mbamservice.exe":    {"Malwarebytes", VidAntivirus, Meshaet},
	"savservice.exe":     {"Sophos", VidAntivirus, Meshaet},
	"cmdagent.exe":       {"Comodo", VidAntivirus, Meshaet},
	"cfp.exe":            {"Comodo", VidAntivirus, Meshaet},
	"vsmon.exe":          {"ZoneAlarm", VidAntivirus, Meshaet},
	"zlclient.exe":       {"ZoneAlarm", VidAntivirus, Meshaet},
	"fortitray.exe":      {"FortiClient", VidAntivirus, Meshaet},
	"fmon.exe":           {"FortiClient", VidAntivirus, Meshaet},
}

// Protsess это строка снимка процессов. Put пуст, если процесс не отдал путь
// образа (защищённый, уже завершился): тогда решает имя из снимка.
type Protsess struct {
	Pid  uint32
	Imya string
	Put  string
}

// svoy: образ из каталога программы. Сравнение без регистра, потому что
// Windows отдаёт путь в том регистре, в каком его записал установщик.
func svoy(p Protsess, katalog string) bool {
	if p.Put == "" || katalog == "" {
		return false
	}
	k := strings.ToLower(filepath.Clean(katalog)) + `\`
	return strings.HasPrefix(strings.ToLower(p.Put), k)
}

// poImenam называет известные программы среди процессов.
func poImenam(protsessy []Protsess, svoi Svoi) []Nahodka {
	var itog []Nahodka
	for _, p := range protsessy {
		pr, est := izvestnye[strings.ToLower(p.Imya)]
		if !est || svoy(p, svoi.KatalogProgrammy) {
			continue
		}
		itog = append(itog, Nahodka{Imya: pr.imya, Vid: pr.vid, Gruppa: pr.gruppa, Priznak: priznakProtsessa(p)})
	}
	return itog
}

func priznakProtsessa(p Protsess) string {
	if p.Put != "" {
		return p.Put
	}
	return p.Imya
}

// derzhatelWinDivert называет процесс, у которого загружена WinDivert.dll.
// Известное имя даёт своё название, неизвестное остаётся именем exe: так
// находится и переименованный zapret.
func derzhatelWinDivert(p Protsess) Nahodka {
	n := Nahodka{Imya: p.Imya, Vid: VidObhod, Gruppa: Lomaet,
		Priznak: priznakProtsessa(p) + " держит WinDivert.dll"}
	if pr, est := izvestnye[strings.ToLower(p.Imya)]; est && pr.vid == VidObhod {
		n.Imya = pr.imya
	}
	return n
}

// ifTypePropVirtual это IF_TYPE_PROP_VIRTUAL: такой тип у адаптеров Wintun и
// WireGuard. В golang.org/x/sys/windows константы нет.
const ifTypePropVirtual = 53

// pohozhNaTunnel отделяет адаптер VPN от физического канала. Одного маршрута
// по умолчанию мало: его несёт и Wi-Fi, и Ethernet, и внешний коммутатор
// Hyper-V.
func pohozhNaTunnel(a set.Adapter) bool {
	switch a.Tip {
	// IF_TYPE_PPP это встроенный VPN Windows: IKEv2, SSTP, L2TP.
	case ifTypePropVirtual, windows.IF_TYPE_PPP, windows.IF_TYPE_TUNNEL:
		return true
	}
	o := strings.ToLower(a.Opisanie)
	for _, slovo := range []string{"wintun", "wireguard", "tap-windows", "openvpn", "warp", "tunnel"} {
		if strings.Contains(o, slovo) {
			return true
		}
	}
	return false
}

// PohozhNaTunnel отдаёт то же правило своему VPN: SSH к серверу человека
// идёт через физический адаптер, а не через какой-либо туннель.
func PohozhNaTunnel(a set.Adapter) bool { return pohozhNaTunnel(a) }

// imyaTunnelya называет чужой VPN по описанию адаптера. Wintun возят многие
// клиенты сразу, и по нему одному не сказать чей: тогда название берётся у
// единственного запущенного VPN-клиента, а если их нет или несколько,
// остаётся имя адаптера.
func imyaTunnelya(a set.Adapter, klienty []string) string {
	o := strings.ToLower(a.Opisanie)
	switch {
	case strings.Contains(o, "cloudflare"), strings.Contains(o, "warp"):
		return "Cloudflare WARP"
	case strings.Contains(o, "wireguard"):
		return "WireGuard"
	case strings.Contains(o, "tap-windows"), strings.Contains(o, "openvpn"):
		return "OpenVPN"
	case a.Tip == windows.IF_TYPE_PPP:
		return fmt.Sprintf("VPN Windows «%s»", a.Imya)
	case len(klienty) == 1:
		return klienty[0]
	}
	return fmt.Sprintf("VPN «%s»", a.Imya)
}

// chuzhieTunneli находит поднятые чужие адаптеры VPN с маршрутом по умолчанию:
// 0.0.0.0/0 или пара половинок. Radmin, Hamachi и Tailscale без выхода в
// интернет умолчания не несут и сюда не попадают.
func chuzhieTunneli(adaptery []set.Adapter, polovinki map[uint32]bool, svoi Svoi, klienty []string) []Nahodka {
	var itog []Nahodka
	for _, a := range adaptery {
		switch {
		case slices.Contains(svoi.Adaptery, a.Indeks),
			svoi.ImyaAdaptera != "" && strings.EqualFold(a.Imya, svoi.ImyaAdaptera),
			a.Sostoyanie != windows.IfOperStatusUp,
			!a.Umolchanie && !polovinki[a.Indeks],
			!pohozhNaTunnel(a):
			continue
		}
		itog = append(itog, Nahodka{
			Imya: imyaTunnelya(a, klienty), Vid: VidVPN, Gruppa: Lomaet,
			Priznak: fmt.Sprintf("адаптер «%s» (%s) несёт маршрут по умолчанию", a.Imya, a.Opisanie),
		})
	}
	return itog
}

// klientyVPN отдаёт названия запущенных VPN-клиентов без повторов.
func klientyVPN(n []Nahodka) []string {
	var itog []string
	for _, x := range n {
		if x.Vid == VidVPN && !slices.Contains(itog, x.Imya) {
			itog = append(itog, x.Imya)
		}
	}
	return itog
}

// svesti склеивает находки одной программы: признаки копятся, а группа берётся
// старшая. Подключённый WARP находится и адаптером («ломает»), и процессом
// («может мешать»), а назван должен быть один раз.
func svesti(vse []Nahodka) []Nahodka {
	var itog []Nahodka
	for _, n := range vse {
		i := slices.IndexFunc(itog, func(b Nahodka) bool { return b.Imya == n.Imya })
		if i < 0 {
			itog = append(itog, n)
			continue
		}
		b := &itog[i]
		if n.Gruppa == Lomaet && b.Gruppa != Lomaet {
			b.Gruppa, b.Vid = Lomaet, n.Vid
		}
		if !strings.Contains(b.Priznak, n.Priznak) {
			b.Priznak += "; " + n.Priznak
		}
	}
	slices.SortStableFunc(itog, func(a, b Nahodka) int {
		if a.Gruppa != b.Gruppa {
			if a.Gruppa == Lomaet {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Imya, b.Imya)
	})
	return itog
}

// Lomayushchie отдаёт находки группы «ломает»: только о них окно говорит сразу.
func Lomayushchie(n []Nahodka) []Nahodka {
	var itog []Nahodka
	for _, x := range n {
		if x.Gruppa == Lomaet {
			itog = append(itog, x)
		}
	}
	return itog
}

// perehvatPortov это TCP-порты, на которых программа встаёт посреди
// соединения со своей проверкой почты, и reality через них не проходит.
// Только снятое живьём: Avast Free режет 993, 995 и 110 (1.10.0,
// 21.10.2026) и 465 (стенд, 02.11.2026). Порт, не проверенный руками, сюда
// не идёт: отказ на нём назвал бы Avast зря.
var perehvatPortov = map[string][]int{"Avast": {110, 465, 993, 995}}

// PerehvatPorta называет программу из n, которая перехватывает TCP-порт
// port. Пусто - такой среди n нет.
func PerehvatPorta(n []Nahodka, port int) string {
	for _, x := range n {
		if slices.Contains(perehvatPortov[x.Imya], port) {
			return x.Imya
		}
	}
	return ""
}

// Podskazka это фраза для отказа «данные не пошли» по находкам группы «может
// мешать». Пусто, если сказать нечего. Не больше трёх программ: длинный
// список в отказе читается как сбой, а полный лежит в выгрузке.
//
// Без «: » внутри: текст отказа служба режет по этому разделителю на звенья
// (sboi.ObrezatTehniku), и звено с латинским названием программы могло бы
// уйти вместе с технической частью.
func Podskazka(n []Nahodka, katalogProgrammy string) string {
	var zashchita, vpn []string
	for _, x := range n {
		if x.Gruppa != Meshaet {
			continue
		}
		switch x.Vid {
		case VidAntivirus, VidFiltr:
			zashchita = append(zashchita, x.Imya)
		case VidVPN:
			vpn = append(vpn, x.Imya)
		}
	}
	var frazy []string
	if len(zashchita) > 0 {
		frazy = append(frazy, fmt.Sprintf("Может мешать %s. Если интернета нет, добавь папку %s в исключения его проверки трафика",
			perechen(zashchita), katalogProgrammy))
	}
	if len(vpn) > 0 {
		frazy = append(frazy, fmt.Sprintf("Запущен %s. Если в нём включено подключение, отключи его", perechen(vpn)))
	}
	return strings.Join(frazy, ". ")
}

func perechen(imena []string) string {
	if len(imena) > 3 {
		imena = imena[:3]
	}
	return strings.Join(imena, ", ")
}
