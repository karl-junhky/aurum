package pomehi

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

const katalog = `C:\Program Files\Affory`

// skanerIz собирает сканер на швах: ни процессов, ни драйверов, ни адаптеров
// машины разработчика тест не читает.
func skanerIz(p []Protsess, drayvery []string, derzhat map[uint32]bool, a []set.Adapter, pol map[uint32]bool) (*Skaner, *int) {
	osmotrov := 0
	return &Skaner{
		osmotreny:      map[klyuchOsmotra]osmotr{},
		protsessy:      func() ([]Protsess, error) { return p, nil },
		drayveryWinDiv: func() ([]string, error) { return drayvery, nil },
		derzhitWinDivert: func(pid uint32) (bool, error) {
			osmotrov++
			return derzhat[pid], nil
		},
		adaptery:  func() ([]set.Adapter, error) { return a, nil },
		polovinki: func() (map[uint32]bool, error) { return pol, nil },
	}, &osmotrov
}

func fizicheskiy() set.Adapter {
	return set.Adapter{Indeks: 5, Imya: "Ethernet", Opisanie: "Intel(R) Ethernet Connection I219-V",
		Tip: windows.IF_TYPE_ETHERNET_CSMACD, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
}

func odna(t *testing.T, n []Nahodka) Nahodka {
	t.Helper()
	if len(n) != 1 {
		t.Fatalf("ожидалась одна находка, получено %d: %+v", len(n), n)
	}
	return n[0]
}

func TestChistayaMashinaMolchit(t *testing.T) {
	s, _ := skanerIz([]Protsess{{Pid: 100, Imya: "explorer.exe", Put: `C:\Windows\explorer.exe`}},
		nil, nil, []set.Adapter{fizicheskiy()}, nil)
	n, err := s.Proverit(Svoi{KatalogProgrammy: katalog})
	if err != nil || len(n) != 0 {
		t.Fatalf("на чистой машине находки %+v, ошибка %v", n, err)
	}
}

func TestZapretPoImeniLomaet(t *testing.T) {
	s, _ := skanerIz([]Protsess{{Pid: 200, Imya: "winws.exe", Put: `C:\zapret\bin\winws.exe`}},
		nil, nil, nil, nil)
	n, _ := s.Proverit(Svoi{KatalogProgrammy: katalog})
	x := odna(t, n)
	if x.Imya != "zapret" || x.Gruppa != Lomaet || x.Vid != VidObhod {
		t.Fatalf("zapret назван не так: %+v", x)
	}
}

// Имя exe из снимка бывает в любом регистре, а список хранит нижний.
func TestImyaBezRegistra(t *testing.T) {
	s, _ := skanerIz([]Protsess{{Pid: 300, Imya: "AvastSvc.exe"}}, nil, nil, nil, nil)
	n, _ := s.Proverit(Svoi{KatalogProgrammy: katalog})
	if x := odna(t, n); x.Imya != "Avast" || x.Gruppa != Meshaet {
		t.Fatalf("Avast назван не так: %+v", x)
	}
}

// Свой процесс не чужой, даже если его имя в списке: сверка по каталогу.
func TestSvoyProtsessNePomeha(t *testing.T) {
	s, _ := skanerIz([]Protsess{{Pid: 400, Imya: "Happ.exe", Put: `c:\program files\affory\Happ.exe`}},
		nil, nil, nil, nil)
	n, _ := s.Proverit(Svoi{KatalogProgrammy: katalog})
	if len(n) != 0 {
		t.Fatalf("процесс из каталога программы назван чужим: %+v", n)
	}
	// Каталог-сосед с тем же началом имени не свой.
	if svoy(Protsess{Put: `C:\Program Files\Affory2\x.exe`}, katalog) {
		t.Fatal("каталог Affory2 принят за Affory")
	}
}

// Драйвер без держателя это след прошлого запуска zapret: молчим.
func TestDrayverBezDerzhatelyaMolchit(t *testing.T) {
	s, osmotrov := skanerIz([]Protsess{{Pid: 500, Imya: "chrome.exe"}},
		[]string{"WinDivert"}, nil, nil, nil)
	n, _ := s.Proverit(Svoi{KatalogProgrammy: katalog})
	if len(n) != 0 {
		t.Fatalf("драйвер без процесса назван помехой: %+v", n)
	}
	if *osmotrov != 1 {
		t.Fatalf("осмотрено процессов %d, ожидался 1", *osmotrov)
	}
}

// Без драйвера модули не осматриваются вовсе: это снимок на каждый процесс.
func TestBezDrayveraModuliNeOsmatrivayutsya(t *testing.T) {
	s, osmotrov := skanerIz([]Protsess{{Pid: 500, Imya: "chrome.exe"}}, nil, nil, nil, nil)
	if _, err := s.Proverit(Svoi{}); err != nil {
		t.Fatal(err)
	}
	if *osmotrov != 0 {
		t.Fatalf("модули осмотрены при отсутствии драйвера: %d", *osmotrov)
	}
}

// Переименованный zapret находится по WinDivert.dll и осматривается один раз,
// пока жив; процесс без библиотеки осматривается дважды и больше нет.
func TestPereimenovannyyPerehvatchikIOsmotrOdinRaz(t *testing.T) {
	p := []Protsess{{Pid: 600, Imya: "myfilter.exe", Put: `D:\tools\myfilter.exe`}, {Pid: 601, Imya: "notepad.exe"}}
	s, osmotrov := skanerIz(p, []string{"WinDivert1.4"}, map[uint32]bool{600: true}, nil, nil)
	for range 3 {
		n, _ := s.Proverit(Svoi{KatalogProgrammy: katalog})
		x := odna(t, n)
		if x.Imya != "myfilter.exe" || x.Gruppa != Lomaet || !strings.Contains(x.Priznak, "WinDivert.dll") {
			t.Fatalf("держатель WinDivert назван не так: %+v", x)
		}
	}
	if *osmotrov != 3 {
		t.Fatalf("осмотров %d за три проверки, ожидалось 3: держатель один раз, прочий дважды", *osmotrov)
	}
}

// Первый осмотр мог прийтись до загрузки WinDivert.dll: второй её находит.
func TestPerehvatchikZagruzivshiyDllPozzheNayden(t *testing.T) {
	derzhat := map[uint32]bool{}
	s, _ := skanerIz([]Protsess{{Pid: 650, Imya: "svc-filter.exe"}}, []string{"WinDivert"}, derzhat, nil, nil)
	if n, _ := s.Proverit(Svoi{}); len(n) != 0 {
		t.Fatalf("находка до загрузки библиотеки: %+v", n)
	}
	derzhat[650] = true
	n, _ := s.Proverit(Svoi{})
	if x := odna(t, n); x.Imya != "svc-filter.exe" {
		t.Fatalf("держатель назван не так: %+v", x)
	}
}

// Тот же номер у другого процесса это новый процесс, и осматривается он заново.
func TestNomerPovtornoNeNasleduetOtvet(t *testing.T) {
	p := []Protsess{{Pid: 700, Imya: "winws.exe"}}
	s, osmotrov := skanerIz(p, []string{"WinDivert"}, map[uint32]bool{700: true}, nil, nil)
	if _, err := s.Proverit(Svoi{}); err != nil {
		t.Fatal(err)
	}
	p[0].Imya = "calc.exe"
	s.protsessy = func() ([]Protsess, error) { return p, nil }
	s.derzhitWinDivert = func(uint32) (bool, error) { *osmotrov++; return false, nil }
	n, _ := s.Proverit(Svoi{})
	if len(n) != 0 || *osmotrov != 2 {
		t.Fatalf("новый процесс под старым номером: находки %+v, осмотров %d", n, *osmotrov)
	}
}

func TestChuzhoyVPNSUmolchaniem(t *testing.T) {
	wireguard := set.Adapter{Indeks: 30, Imya: "wg0", Opisanie: "WireGuard Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
	s, _ := skanerIz(nil, nil, nil, []set.Adapter{fizicheskiy(), wireguard}, nil)
	n, _ := s.Proverit(Svoi{})
	if x := odna(t, n); x.Imya != "WireGuard" || x.Gruppa != Lomaet || x.Vid != VidVPN {
		t.Fatalf("WireGuard назван не так: %+v", x)
	}
}

// OpenVPN ставит умолчание парой 0.0.0.0/1 и 128.0.0.0/1, а не 0.0.0.0/0.
func TestPolovinkiTozheUmolchanie(t *testing.T) {
	tap := set.Adapter{Indeks: 31, Imya: "OpenVPN TAP", Opisanie: "TAP-Windows Adapter V9",
		Tip: windows.IF_TYPE_ETHERNET_CSMACD, Sostoyanie: windows.IfOperStatusUp}
	s, _ := skanerIz(nil, nil, nil, []set.Adapter{tap}, map[uint32]bool{31: true})
	n, _ := s.Proverit(Svoi{})
	if x := odna(t, n); x.Imya != "OpenVPN" {
		t.Fatalf("OpenVPN с половинками назван не так: %+v", x)
	}
}

// Radmin, Hamachi и Tailscale без выхода в интернет держат адаптер туннеля без
// умолчания: у человека всё работает, и тревога была бы ложной.
func TestTunnelBezUmolchaniyaNePomeha(t *testing.T) {
	tailscale := set.Adapter{Indeks: 40, Imya: "Tailscale", Opisanie: "Tailscale Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp}
	radmin := set.Adapter{Indeks: 41, Imya: "Radmin VPN", Opisanie: "Famatech Radmin VPN Ethernet Adapter",
		Tip: windows.IF_TYPE_ETHERNET_CSMACD, Sostoyanie: windows.IfOperStatusUp}
	s, _ := skanerIz(nil, nil, nil, []set.Adapter{fizicheskiy(), tailscale, radmin}, nil)
	if n, _ := s.Proverit(Svoi{}); len(n) != 0 {
		t.Fatalf("туннель без умолчания назван помехой: %+v", n)
	}
}

// Опущенный чужой адаптер не мешает: WARP установлен, но не подключён.
func TestOpushchennyyTunnelNePomeha(t *testing.T) {
	warp := set.Adapter{Indeks: 42, Imya: "CloudflareWARP", Opisanie: "Cloudflare WARP Interface Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusDown, Umolchanie: true}
	s, _ := skanerIz([]Protsess{{Pid: 800, Imya: "warp-svc.exe"}}, nil, nil, []set.Adapter{warp}, nil)
	n, _ := s.Proverit(Svoi{})
	if x := odna(t, n); x.Imya != "Cloudflare WARP" || x.Gruppa != Meshaet {
		t.Fatalf("неподключённый WARP: %+v", x)
	}
	if len(Lomayushchie(n)) != 0 {
		t.Fatal("неподключённый WARP попал в строку главного экрана")
	}
}

// Подключённый WARP находится и адаптером, и процессом, а назван один раз.
func TestPodklyuchennyyWARPOdnaNahodka(t *testing.T) {
	warp := set.Adapter{Indeks: 42, Imya: "CloudflareWARP", Opisanie: "Cloudflare WARP Interface Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
	s, _ := skanerIz([]Protsess{{Pid: 800, Imya: "warp-svc.exe"}}, nil, nil, []set.Adapter{warp}, nil)
	n, _ := s.Proverit(Svoi{})
	x := odna(t, n)
	if x.Gruppa != Lomaet || !strings.Contains(x.Priznak, "warp-svc.exe") || !strings.Contains(x.Priznak, "CloudflareWARP") {
		t.Fatalf("подключённый WARP: %+v", x)
	}
}

// Wintun возят многие клиенты, и название берётся у единственного запущенного.
func TestWintunNazyvaetsyaPoKlientu(t *testing.T) {
	wintun := set.Adapter{Indeks: 50, Imya: "happ-tun", Opisanie: "Wintun Userspace Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
	s, _ := skanerIz([]Protsess{{Pid: 900, Imya: "Happ.exe"}}, nil, nil, []set.Adapter{wintun}, nil)
	n, _ := s.Proverit(Svoi{})
	if x := odna(t, n); x.Imya != "Happ" || x.Gruppa != Lomaet {
		t.Fatalf("Wintun при запущенном Happ: %+v", x)
	}

	// Клиентов двое: кто из них поднял адаптер, не сказать, и честнее имя адаптера.
	s, _ = skanerIz([]Protsess{{Pid: 900, Imya: "Happ.exe"}, {Pid: 901, Imya: "v2rayN.exe"}},
		nil, nil, []set.Adapter{wintun}, nil)
	n, _ = s.Proverit(Svoi{})
	if l := Lomayushchie(n); len(l) != 1 || l[0].Imya != "VPN «happ-tun»" {
		t.Fatalf("Wintun при двух клиентах: %+v", n)
	}
}

// Свой туннель не чужой: по индексу всегда, по имени только пока служба
// сама его держит.
func TestSvoyTunnelNePomeha(t *testing.T) {
	nash := set.Adapter{Indeks: 60, Imya: "tun0", Opisanie: "Wintun Userspace Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
	s, _ := skanerIz(nil, nil, nil, []set.Adapter{nash}, nil)
	if n, _ := s.Proverit(Svoi{Adaptery: []uint32{60}}); len(n) != 0 {
		t.Fatalf("свой туннель по индексу назван чужим: %+v", n)
	}
	if n, _ := s.Proverit(Svoi{ImyaAdaptera: "tun0"}); len(n) != 0 {
		t.Fatalf("свой туннель по имени назван чужим: %+v", n)
	}
	if n, _ := s.Proverit(Svoi{}); len(n) != 1 {
		t.Fatalf("чужой tun0 при выключенной службе не найден: %+v", n)
	}
}

// Отказ одного источника не прячет находки другого.
func TestOtkazIstochnikaNePryachetDrugie(t *testing.T) {
	wireguard := set.Adapter{Indeks: 30, Imya: "wg0", Opisanie: "WireGuard Tunnel",
		Tip: ifTypePropVirtual, Sostoyanie: windows.IfOperStatusUp, Umolchanie: true}
	s, _ := skanerIz(nil, nil, nil, []set.Adapter{wireguard}, nil)
	s.protsessy = func() ([]Protsess, error) { return nil, errors.New("снимок не дали") }
	n, err := s.Proverit(Svoi{})
	if err == nil || len(n) != 1 {
		t.Fatalf("находки %+v, ошибка %v", n, err)
	}
}

func TestLomayushchieIdutPervymi(t *testing.T) {
	n := svesti([]Nahodka{
		{Imya: "Kaspersky", Vid: VidAntivirus, Gruppa: Meshaet, Priznak: "avp.exe"},
		{Imya: "zapret", Vid: VidObhod, Gruppa: Lomaet, Priznak: "winws.exe"},
		{Imya: "Kaspersky", Vid: VidAntivirus, Gruppa: Meshaet, Priznak: "avpui.exe"},
	})
	if len(n) != 2 || n[0].Imya != "zapret" || n[1].Priznak != "avp.exe; avpui.exe" {
		t.Fatalf("сведение: %+v", n)
	}
}

func TestPerehvatPorta(t *testing.T) {
	n := []Nahodka{
		{Imya: "Kaspersky", Vid: VidAntivirus, Gruppa: Meshaet},
		{Imya: "Avast", Vid: VidAntivirus, Gruppa: Meshaet},
	}
	for _, c := range []struct {
		n    []Nahodka
		port int
		kto  string
	}{
		{n, 993, "Avast"},
		{n, 465, "Avast"},
		{n, 443, ""},
		{n, 2053, ""},
		{n[:1], 993, ""},
		{nil, 993, ""},
	} {
		if kto := PerehvatPorta(c.n, c.port); kto != c.kto {
			t.Errorf("порт %d при %+v: %q, ждали %q", c.port, c.n, kto, c.kto)
		}
	}
	// Перехват ищется по названию находки: переименованная в izvestnye
	// программа молча перестала бы называться.
	for imya := range perehvatPortov {
		est := false
		for _, p := range izvestnye {
			est = est || p.imya == imya
		}
		if !est {
			t.Errorf("перехват порта у %q, а среди известных программ такого названия нет", imya)
		}
	}
}

func TestPodskazka(t *testing.T) {
	n := []Nahodka{
		{Imya: "zapret", Vid: VidObhod, Gruppa: Lomaet},
		{Imya: "Kaspersky", Vid: VidAntivirus, Gruppa: Meshaet},
		{Imya: "Happ", Vid: VidVPN, Gruppa: Meshaet},
		{Imya: "ByeDPI", Vid: VidObhod, Gruppa: Meshaet},
	}
	p := Podskazka(n, katalog)
	for _, nado := range []string{"Kaspersky", katalog, "Happ"} {
		if !strings.Contains(p, nado) {
			t.Errorf("в подсказке нет %q: %s", nado, p)
		}
	}
	for _, nelzya := range []string{"zapret", "ByeDPI", ": "} {
		if strings.Contains(p, nelzya) {
			t.Errorf("в подсказке лишнее %q: %s", nelzya, p)
		}
	}
	if Podskazka(n[:1], katalog) != "" {
		t.Error("подсказка без находок «может мешать» не пустая")
	}
}
