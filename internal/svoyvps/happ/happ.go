// Package happ собирает правила Happ «российские сайты напрямую» (РФ1 плана
// 1.12.0): голая ссылка vless:// или hy2:// правил не несёт, Happ берёт их
// отдельной ссылкой happ://routing/onadd/{base64} или заголовком routing
// подписки.
//
// Формат взят 05.11.2026 из документации happ.su (dev-docs/routing) и из
// генератора самого Happ на routing.happ.su: ключи с большой буквы, флаги
// строками "true"/"false", base64 без дополнения в алфавите для адресов.
package happ

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
)

// imya это имя профиля в Happ. Профиль с тем же именем Happ перезаписывает:
// так выключение заменяет включение, а выпуски обновляют профиль, а не
// копят новые. Латиница без знаков: что Happ делает с именем внутри, не
// проверено.
const imya = "Affory"

// Штатные geo-файлы Happ (Loyalsoldier). С этими адресами Happ ничего не
// качает при добавлении профиля, а режет свои встроенные файлы под правила.
const (
	geoip   = "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat"
	geosite = "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"
)

// cherezVpn это сайты, которые идут через VPN раньше правил «напрямую».
// geoip:ru забирает кэши Google у российских провайдеров, и без этого
// YouTube ушёл бы напрямую. Своего списка заблокированного у штатных файлов
// нет, поэтому здесь сервисы, заблокированные или замедленные в РФ. Каждое
// имя есть в списках v2fly, из которых собраны файлы Loyalsoldier (сверено
// 05.11.2026): имя, которого в файле нет, не даст ядру Happ запуститься.
var cherezVpn = []string{
	"geosite:youtube", "geosite:google", "geosite:telegram", "geosite:whatsapp",
	"geosite:facebook", "geosite:instagram", "geosite:twitter", "geosite:discord",
	"geosite:linkedin",
}

// Мимо VPN всегда идут локальные сети, а при включённых правилах ещё всё в
// зоне .ru, российские сервисы вне неё (category-ru) и адреса РФ.
var (
	lokalnyeSayty  = []string{"geosite:private"}
	lokalnyeAdresa = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "224.0.0.0/4", "255.255.255.255"}
	rossiyaSayty   = []string{"geosite:category-ru"}
	rossiyaAdresa  = []string{"geoip:ru"}
)

// profil это профиль маршрутизации Happ. Порядок полей тот же, что у
// генератора Happ. DnsHosts нет: Happ берёт его из своего профиля по
// умолчанию, а ссылка короче и код QR читается легче.
type profil struct {
	Name              string   `json:"Name"`
	GlobalProxy       string   `json:"GlobalProxy"`
	RouteOrder        string   `json:"RouteOrder"`
	RemoteDNSType     string   `json:"RemoteDNSType"`
	RemoteDNSDomain   string   `json:"RemoteDNSDomain"`
	RemoteDNSIP       string   `json:"RemoteDNSIP"`
	DomesticDNSType   string   `json:"DomesticDNSType"`
	DomesticDNSDomain string   `json:"DomesticDNSDomain"`
	DomesticDNSIP     string   `json:"DomesticDNSIP"`
	Geoipurl          string   `json:"Geoipurl"`
	Geositeurl        string   `json:"Geositeurl"`
	LastUpdated       string   `json:"LastUpdated"`
	DirectSites       []string `json:"DirectSites"`
	DirectIp          []string `json:"DirectIp"`
	ProxySites        []string `json:"ProxySites"`
	ProxyIp           []string `json:"ProxyIp"`
	BlockSites        []string `json:"BlockSites"`
	BlockIp           []string `json:"BlockIp"`
	DomainStrategy    string   `json:"DomainStrategy"`
	FakeDNS           string   `json:"FakeDNS"`
	UseChunkFiles     string   `json:"UseChunkFiles"`
}

// sobrat собирает JSON профиля: всё, чего правила не назвали, идёт через
// VPN. Без российских сайтов профиль пускает мимо VPN только локальные сети.
//
// RouteOrder block-proxy-direct ставит правила «через VPN» раньше
// «напрямую». DNS тот же, что у Happ по умолчанию, оба через DoH: открытый
// DNS провайдер подменяет, и адрес его заглушки из сети РФ увёл бы
// заблокированный сайт по правилу geoip:ru мимо VPN. LastUpdated пуст: с ним
// Happ перекачивал бы geo-файлы с GitHub при каждом добавлении профиля, а
// штатных хватает.
func sobrat(rossiya bool) ([]byte, error) {
	p := profil{
		Name:              imya,
		GlobalProxy:       "true",
		RouteOrder:        "block-proxy-direct",
		RemoteDNSType:     "DoH",
		RemoteDNSDomain:   "https://cloudflare-dns.com/dns-query",
		RemoteDNSIP:       "1.1.1.1",
		DomesticDNSType:   "DoH",
		DomesticDNSDomain: "https://dns.google/dns-query",
		DomesticDNSIP:     "8.8.8.8",
		Geoipurl:          geoip,
		Geositeurl:        geosite,
		DirectSites:       lokalnyeSayty,
		DirectIp:          lokalnyeAdresa,
		ProxySites:        []string{},
		ProxyIp:           []string{},
		BlockSites:        []string{},
		BlockIp:           []string{},
		DomainStrategy:    "IPIfNonMatch",
		FakeDNS:           "false",
		UseChunkFiles:     "true",
	}
	if rossiya {
		p.DirectSites = append(append([]string{}, rossiyaSayty...), lokalnyeSayty...)
		p.DirectIp = append(append([]string{}, rossiyaAdresa...), lokalnyeAdresa...)
		p.ProxySites = cherezVpn
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("профиль Happ не собран: %w", err)
	}
	return b, nil
}

func ssylka(rossiya bool) (string, error) {
	b, err := sobrat(rossiya)
	if err != nil {
		return "", err
	}
	return "happ://routing/onadd/" + base64.RawURLEncoding.EncodeToString(b), nil
}

// Профиль не меняется за жизнь службы: ссылки собираются один раз.
var (
	vklyuchit  = sync.OnceValues(func() (string, error) { return ssylka(true) })
	vyklyuchit = sync.OnceValues(func() (string, error) { return ssylka(false) })
)

// Vklyuchit это ссылка, которая добавляет в Happ профиль «российские сайты
// напрямую» и сразу его включает; профиль с тем же именем перезаписывается.
func Vklyuchit() (string, error) { return vklyuchit() }

// Vyklyuchit ставит под тем же именем профиль, где всё, кроме локальных
// сетей, идёт через VPN. happ://routing/off сюда не годится: он выключает
// маршрутизацию в Happ целиком, вместе с правилами чужих подписок.
func Vyklyuchit() (string, error) { return vyklyuchit() }
