package domen

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obhoddns"
)

func TestNormalizovat(t *testing.T) {
	for vvod, zhdali := range map[string]string{
		"kot.example.net":                "kot.example.net",
		"  Kot.Example.NET. ":            "kot.example.net",
		"https://kot.example.net/":       "kot.example.net",
		"https://kot.example.net:8443/x": "kot.example.net",
		"kot.example.net/sub?x=1":        "kot.example.net",
		"кот.рф":                         "xn--j1aim.xn--p1ai",
	} {
		if got, err := Normalizovat(vvod); err != nil || got != zhdali {
			t.Errorf("%q: %q %v", vvod, got, err)
		}
	}
	for _, vvod := range []string{"", "localhost", "203.0.113.10", "https://203.0.113.10/", "kot", "kot..net", "*.kot.net", "kot net.ru"} {
		if got, err := Normalizovat(vvod); !errors.Is(err, ErrNegodnyy) {
			t.Errorf("%q принят: %q", vvod, got)
		}
	}
}

// podmenit отвечает за оба DNS: по адресу источника - A и AAAA.
func podmenit(t *testing.T, otvety map[string][2][]string, oshibki map[string]error) {
	t.Helper()
	bylo := sprosit
	t.Cleanup(func() { sprosit = bylo })
	sprosit = func(_ context.Context, adres, host string, tip dnsmessage.Type) ([]netip.Addr, error) {
		if host != "kot.example.net" {
			t.Fatalf("спросили %q", host)
		}
		if err := oshibki[adres]; err != nil {
			return nil, err
		}
		i := 0
		if tip == dnsmessage.TypeAAAA {
			i = 1
		}
		var a []netip.Addr
		for _, s := range otvety[adres][i] {
			a = append(a, netip.MustParseAddr(s))
		}
		return a, nil
	}
}

const (
	google = "https://8.8.8.8/dns-query"
	cf     = "https://1.1.1.1/dns-query"
)

func TestProverit(t *testing.T) {
	server := netip.MustParseAddr("203.0.113.10")
	netImeni := obhoddns.ErrNetImeni
	for _, c := range []struct {
		imya       string
		otvety     map[string][2][]string
		oshibki    map[string]error
		kod        string
		aaaa       bool
		otstayut   []string
		neOtvetili []string
	}{
		{"сходится", map[string][2][]string{google: {{"203.0.113.10"}}, cf: {{"203.0.113.10"}}}, nil, Sovpadaet, false, nil, nil},
		{"облако", map[string][2][]string{google: {{"104.21.3.4"}}, cf: {{"104.21.3.4", "172.67.1.2"}}}, nil, Cloudflare, false, nil, nil},
		{"чужой адрес", map[string][2][]string{google: {{"198.51.100.7"}}, cf: {{"198.51.100.7"}}}, nil, DrugoyAdres, false, nil, nil},
		{"лишний адрес", map[string][2][]string{google: {{"203.0.113.10", "198.51.100.7"}}, cf: {{"203.0.113.10"}}}, nil, DrugoyAdres, false, nil, nil},
		{"нет имени", nil, map[string]error{google: netImeni, cf: netImeni}, NetZapisi, false, nil, nil},
		{"нет записи A", map[string][2][]string{google: {nil}, cf: {nil}}, nil, NetZapisi, false, nil, nil},
		{"видит один", map[string][2][]string{google: {{"203.0.113.10"}}, cf: {nil}}, nil, NeVezde, false, []string{"Cloudflare"}, nil},
		// Запись перенесли с прежней машины: один DNS ещё помнит её адрес.
		// Это ожидание, а не чужой адрес.
		{"помнит прежний адрес", map[string][2][]string{google: {{"198.51.100.7"}}, cf: {{"203.0.113.10"}}}, nil, NeVezde, false,
			[]string{"Google"}, nil},
		// Облако выключили только что: один DNS ещё помнит адреса прокси.
		{"помнит облако", map[string][2][]string{google: {{"104.21.3.4"}}, cf: {{"203.0.113.10"}}}, nil, NeVezde, false,
			[]string{"Google"}, nil},
		// Не ответил - не значит «ещё не видит»: запись подтвердил второй.
		{"второй не ответил", map[string][2][]string{google: {{"203.0.113.10"}}}, map[string]error{cf: errors.New("таймаут")}, Sovpadaet, false,
			nil, []string{"Cloudflare"}},
		{"нет связи", nil, map[string]error{google: errors.New("a"), cf: errors.New("b")}, NetSvyazi, false, nil, []string{"Google", "Cloudflare"}},
		{"есть AAAA", map[string][2][]string{google: {{"203.0.113.10"}, {"2001:db8::1"}}, cf: {{"203.0.113.10"}, {"2001:db8::1"}}}, nil, Sovpadaet, true, nil, nil},
	} {
		t.Run(c.imya, func(t *testing.T) {
			podmenit(t, c.otvety, c.oshibki)
			it := Proverit(context.Background(), "kot.example.net", server)
			if it.Kod != c.kod || (len(it.AAAA) > 0) != c.aaaa || it.Domen != "kot.example.net" ||
				!slices.Equal(it.Otstayut, c.otstayut) || !slices.Equal(it.NeOtvetili, c.neOtvetili) {
				t.Fatalf("%+v", it)
			}
			if c.kod == Sovpadaet && len(it.Podtverdili)+len(it.NeOtvetili) != 2 {
				t.Fatalf("подтвердили %v, не ответили %v", it.Podtverdili, it.NeOtvetili)
			}
			if (len(it.NeOtvetili) > 0) != (it.Oshibka != "") {
				t.Fatalf("не ответили %v, а причина %q", it.NeOtvetili, it.Oshibka)
			}
			if c.kod == DrugoyAdres && !slices.Contains(it.Adresa, "198.51.100.7") {
				t.Fatalf("чужой адрес не назван: %v", it.Adresa)
			}
		})
	}
}
