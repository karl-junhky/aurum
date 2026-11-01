package lovushka

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"testing"

	"golang.org/x/net/bpf"
)

func ip4(proto byte, dlina int, smeshchenie uint16, otkogo string, dalshe []byte) []byte {
	b := make([]byte, dlina*4)
	b[0] = 0x40 | byte(dlina)
	binary.BigEndian.PutUint16(b[6:8], smeshchenie)
	b[9] = proto
	a := netip.MustParseAddr(otkogo).As4()
	copy(b[12:16], a[:])
	return append(b, dalshe...)
}

func ip6(proto byte, otkogo string, dalshe []byte) []byte {
	b := make([]byte, 40)
	b[0] = 0x60
	b[6] = proto
	a := netip.MustParseAddr(otkogo).As16()
	copy(b[8:24], a[:])
	return append(b, dalshe...)
}

func tcp(port uint16, flagi byte) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b[0:2], 51000)
	binary.BigEndian.PutUint16(b[2:4], port)
	b[12] = 5 << 4
	b[13] = flagi
	return b
}

func udp(port uint16, dannyh int) []byte {
	b := make([]byte, 8+dannyh)
	binary.BigEndian.PutUint16(b[0:2], 51000)
	binary.BigEndian.PutUint16(b[2:4], port)
	return b
}

const (
	syn    = 0x02
	synAck = 0x12
	ack    = 0x10
)

// Фильтр в ядре и разбор в программе обязаны решать одинаково: фильтр
// прогоняется здесь той же виртуальной машиной BPF, что описывает ядро.
func TestFiltrIRazbor(t *testing.T) {
	sluchai := []struct {
		chto   string
		proto  int
		paket  []byte
		zhdali string // отправитель или пусто, если мимо
	}{
		{"SYN на 443", protoTCP, ip4(6, 5, 0, "198.51.100.7", tcp(443, syn)), "198.51.100.7"},
		{"SYN с опциями IP", protoTCP, ip4(6, 7, 0, "198.51.100.8", tcp(443, syn)), "198.51.100.8"},
		{"SYN-ACK", protoTCP, ip4(6, 5, 0, "198.51.100.7", tcp(443, synAck)), ""},
		{"ACK", protoTCP, ip4(6, 5, 0, "198.51.100.7", tcp(443, ack)), ""},
		{"SYN на другой порт", protoTCP, ip4(6, 5, 0, "198.51.100.7", tcp(80, syn)), ""},
		{"фрагмент", protoTCP, ip4(6, 5, 0x0010, "198.51.100.7", tcp(443, syn)), ""},
		{"UDP вместо TCP", protoTCP, ip4(17, 5, 0, "198.51.100.7", udp(443, 20)), ""},
		{"SYN по IPv6", protoTCP, ip6(6, "2001:db8::7", tcp(443, syn)), "2001:db8::7"},
		{"IPv6 ACK", protoTCP, ip6(6, "2001:db8::7", tcp(443, ack)), ""},
		{"датаграмма", protoUDP, ip4(17, 5, 0, "198.51.100.9", udp(443, 1200)), "198.51.100.9"},
		{"пустая датаграмма", protoUDP, ip4(17, 5, 0, "198.51.100.9", udp(443, 0)), "198.51.100.9"},
		{"датаграмма по IPv6", protoUDP, ip6(17, "2001:db8::9", udp(443, 3)), "2001:db8::9"},
		{"датаграмма мимо порта", protoUDP, ip4(17, 5, 0, "198.51.100.9", udp(8443, 20)), ""},
		{"TCP вместо UDP", protoUDP, ip4(6, 5, 0, "198.51.100.9", tcp(443, syn)), ""},
		{"не IP", protoTCP, []byte{0x00, 1, 2, 3}, ""},
		{"обрезанный TCP", protoTCP, ip4(6, 5, 0, "198.51.100.7", tcp(443, syn)[:10]), ""},
	}
	vm := map[int]*bpf.VM{}
	for _, proto := range []int{protoTCP, protoUDP} {
		syroe, err := sobratFiltr(proto, 443)
		if err != nil {
			t.Fatal(err)
		}
		instrukcii := make([]bpf.Instruction, len(syroe))
		for n, r := range syroe {
			instrukcii[n] = r.Disassemble()
		}
		if vm[proto], err = bpf.NewVM(instrukcii); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range sluchai {
		a, est := razobrat(s.paket, s.proto, 443)
		if got := map[bool]string{true: a.String(), false: ""}[est]; got != s.zhdali {
			t.Errorf("%s: разбор дал %q, ждали %q", s.chto, got, s.zhdali)
		}
		vzyato, err := vm[s.proto].Run(s.paket)
		if err != nil {
			t.Fatalf("%s: фильтр: %v", s.chto, err)
		}
		if (vzyato > 0) != (s.zhdali != "") {
			t.Errorf("%s: фильтр взял %d байт, разбор решил %v", s.chto, vzyato, s.zhdali != "")
		}
	}
}

func TestProveritProsbu(t *testing.T) {
	for _, s := range []struct {
		proto        string
		port, sekund int
	}{{"icmp", 443, 5}, {"tcp", 0, 5}, {"udp", 65536, 5}, {"tcp", 443, 0}, {"tcp", 443, PredelSekund + 1}} {
		if Proverit(s.proto, s.port, s.sekund) == nil {
			t.Errorf("%+v прошла", s)
		}
	}
	if err := Proverit("udp", 443, PredelSekund); err != nil {
		t.Fatal(err)
	}
}

func TestPredelAdresov(t *testing.T) {
	i := Itog{Adresa: map[string]int64{}}
	for n := 0; n < predelAdresov+5; n++ {
		i.dobavit(netip.MustParseAddr(fmt.Sprintf("198.51.100.%d", n)))
	}
	i.dobavit(netip.MustParseAddr("::ffff:198.51.100.0"))
	if i.Vsego != predelAdresov+6 || len(i.Adresa) != predelAdresov+1 || i.Adresa["*"] != 5 || i.Adresa["198.51.100.0"] != 2 {
		t.Fatalf("всего %d, адресов %d, сверх %d", i.Vsego, len(i.Adresa), i.Adresa["*"])
	}
}
