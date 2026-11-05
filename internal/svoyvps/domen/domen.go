// Package domen проверяет свой домен человека перед режимом домена (ДМ1):
// ведёт ли его A-запись на сервер.
//
// Спрашиваются Google и Cloudflare через HTTPS, а не DNS компьютера: под
// туннелем DNS системы отвечает подставными адресами (урок 05.11), а DNS
// провайдера бывает подменён. Let's Encrypt проверяет домен из нескольких
// мест, поэтому запись должна быть видна обоим.
package domen

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/idna"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obhoddns"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
)

// Itog проверки, от худшего к лучшему: облако Cloudflare, чужой адрес, нет
// записи, запись видна не везде, всё сходится. NetSvyazi: ни один DNS не
// ответил, проверить нечем.
const (
	Cloudflare  = "cloudflare"
	DrugoyAdres = "drugoy_adres"
	NetZapisi   = "net_zapisi"
	NeVezde     = "ne_vezde"
	Sovpadaet   = "sovpadaet"
	NetSvyazi   = "net_svyazi"
)

// Itog это ответ проверки.
type Itog struct {
	Domen string `json:"domen"` // в ASCII, как пойдёт в сертификат
	Kod   string `json:"kod"`
	// Adresa это все адреса A, что назвали DNS, без повторов.
	Adresa []string `json:"adresa,omitempty"`
	// AAAA это адреса IPv6 домена: Let's Encrypt идёт по ним первым, и если
	// они ведут не на сервер, сертификат не выйдет.
	AAAA []string `json:"aaaa,omitempty"`
	// Otstayut у NeVezde: кто ещё не видит, что домен ведёт на сервер (записи
	// нет или в ней прежний адрес), хотя другой уже видит.
	Otstayut []string `json:"otstayut,omitempty"`
	// Podtverdili: кто видит, что домен ведёт только на сервер.
	Podtverdili []string `json:"podtverdili,omitempty"`
	// NeOtvetili: кто не ответил вовсе. У Sovpadaet запись подтвердил другой.
	NeOtvetili []string `json:"ne_otvetili,omitempty"`
	// Oshibka: почему не ответили те, кто в NeOtvetili. Окно её не
	// показывает (технический текст по-английски), служба пишет её в журнал.
	Oshibka string `json:"oshibka,omitempty"`
}

// istochnik это публичный DNS через HTTPS по IP: имя самого сервиса
// пришлось бы спрашивать у DNS провайдера.
type istochnik struct{ imya, adres string }

var istochniki = []istochnik{
	{"Google", "https://8.8.8.8/dns-query"},
	{"Cloudflare", "https://1.1.1.1/dns-query"},
}

// sprosit подменяют тесты.
var sprosit = obhoddns.SprositU

// ErrNegodnyy: введённое не имя сайта.
var ErrNegodnyy = errors.New("это не имя сайта")

// Normalizovat приводит ввод человека к имени в ASCII: без схемы, пути,
// порта и точки в конце, строчными, кириллица в punycode («кот.рф» это
// xn--j1aim.xn--p1ai).
func Normalizovat(vvod string) (string, error) {
	s := strings.TrimSpace(vvod)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" {
			return "", ErrNegodnyy
		}
		s = u.Hostname()
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	if _, err := netip.ParseAddr(s); err == nil {
		return "", fmt.Errorf("%w: это адрес, а нужен домен", ErrNegodnyy)
	}
	ascii, err := idna.Lookup.ToASCII(strings.ToLower(s))
	if err != nil || konfig.ProveritMasku(ascii) != nil {
		return "", ErrNegodnyy
	}
	return ascii, nil
}

type otvet struct {
	a, aaaa       []netip.Addr
	netImeni      bool
	errA, errAAAA error
}

// Proverit спрашивает записи домена у Google и Cloudflare и сверяет их с
// адресом сервера.
func Proverit(ctx context.Context, domen string, server netip.Addr) Itog {
	itog := Itog{Domen: domen}
	otvety := make([]otvet, len(istochniki))
	var wg sync.WaitGroup
	for i, ist := range istochniki {
		wg.Add(2)
		go func() {
			defer wg.Done()
			otvety[i].a, otvety[i].errA = sprosit(ctx, ist.adres, domen, dnsmessage.TypeA)
		}()
		go func() {
			defer wg.Done()
			otvety[i].aaaa, otvety[i].errAAAA = sprosit(ctx, ist.adres, domen, dnsmessage.TypeAAAA)
		}()
	}
	wg.Wait()

	server = server.Unmap()
	var oshibki []string
	// Каждый ответивший DNS: запись ведёт только на сервер, на сервер и
	// куда-то ещё (две записи A), только не на сервер, или записи нет.
	var smeshannye, chuzhie int
	for i, o := range otvety {
		for _, a := range o.aaaa {
			if !slices.Contains(itog.AAAA, a.String()) {
				itog.AAAA = append(itog.AAAA, a.String())
			}
		}
		imya := istochniki[i].imya
		if o.errA != nil && !errors.Is(o.errA, obhoddns.ErrNetImeni) {
			oshibki = append(oshibki, imya+": "+o.errA.Error())
			itog.NeOtvetili = append(itog.NeOtvetili, imya)
			continue
		}
		est, drugie := false, false
		for _, a := range o.a {
			a = a.Unmap()
			if !slices.Contains(itog.Adresa, a.String()) {
				itog.Adresa = append(itog.Adresa, a.String())
			}
			if a == server {
				est = true
			} else {
				drugie = true
			}
		}
		switch {
		case est && drugie:
			smeshannye++
		case est:
			itog.Podtverdili = append(itog.Podtverdili, imya)
		default:
			// Записи нет или в ней прежний адрес: этот DNS отстаёт, если
			// другой уже видит сервер.
			itog.Otstayut = append(itog.Otstayut, imya)
			if drugie {
				chuzhie++
			}
		}
	}
	itog.Oshibka = strings.Join(oshibki, "; ")
	sovpali := len(itog.Podtverdili)
	switch {
	case len(itog.NeOtvetili) == len(istochniki):
		itog.Kod = NetSvyazi
	case sovpali == 0 && slices.ContainsFunc(itog.Adresa, cloudflare):
		// Прокси Cloudflare у того, кто уже видит сервер, не бывает: адреса
		// облака у отстающего это память о записи до того, как его выключили.
		itog.Kod = Cloudflare
	case smeshannye > 0 || sovpali == 0 && chuzhie > 0:
		// Две записи A, одна не на сервер, или запись целиком не на сервер:
		// ожиданием это не лечится.
		itog.Kod = DrugoyAdres
	case sovpali == 0:
		itog.Kod = NetZapisi
	case len(itog.Otstayut) > 0:
		// Один DNS уже видит сервер, другой ещё нет или помнит прежний адрес:
		// Let's Encrypt может попасть на того, кто отстаёт.
		itog.Kod = NeVezde
	default:
		itog.Kod = Sovpadaet
	}
	if itog.Kod != NeVezde {
		itog.Otstayut = nil
	}
	return itog
}

// setiCloudflare это адреса прокси Cloudflare (cloudflare.com/ips-v4):
// «оранжевое облако» прячет сервер за ними, и reality с проверкой
// сертификата туда не доходят.
var setiCloudflare = func() []netip.Prefix {
	var p []netip.Prefix
	for _, s := range []string{"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22",
		"198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22"} {
		p = append(p, netip.MustParsePrefix(s))
	}
	return p
}()

func cloudflare(s string) bool {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(setiCloudflare, func(p netip.Prefix) bool { return p.Contains(a) })
}
