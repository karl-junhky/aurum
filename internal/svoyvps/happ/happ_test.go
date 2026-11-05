package happ

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// razobrat разбирает ссылку тем же путём, что генератор Happ: base64 в
// алфавите для адресов без дополнения, внутри JSON с ключами с большой
// буквы.
func razobrat(t *testing.T, s string) map[string]any {
	t.Helper()
	telo, ok := strings.CutPrefix(s, "happ://routing/onadd/")
	if !ok {
		t.Fatalf("ссылка %q не добавляет и не включает профиль", s)
	}
	if strings.ContainsAny(telo, "+/=") {
		t.Fatalf("в ссылке знаки, которые ломают адрес: %q", telo)
	}
	b, err := base64.RawURLEncoding.DecodeString(telo)
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func spisok(p map[string]any, k string) []string {
	out := []string{}
	for _, x := range p[k].([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestVklyuchit(t *testing.T) {
	s, err := Vklyuchit()
	if err != nil {
		t.Fatal(err)
	}
	// Потолок одного кода QR в окне (potolokKoda в cmd/affory-ui): длиннее
	// телефон с монитора читает плохо.
	if len(s) > 1400 {
		t.Fatalf("ссылка %d знаков, код QR не прочтётся", len(s))
	}
	p := razobrat(t, s)
	for k, want := range map[string]string{
		"Name": imya, "GlobalProxy": "true", "RouteOrder": "block-proxy-direct",
		"DomainStrategy": "IPIfNonMatch", "FakeDNS": "false", "UseChunkFiles": "true", "LastUpdated": "",
		"Geoipurl": geoip, "Geositeurl": geosite, "RemoteDNSType": "DoH", "DomesticDNSType": "DoH",
	} {
		if p[k] != want {
			t.Errorf("%s = %v, нужно %q", k, p[k], want)
		}
	}
	// YouTube через VPN, хотя кэши Google у провайдера лежат в адресах РФ:
	// правила «через VPN» стоят раньше «напрямую».
	if !slices.Contains(spisok(p, "ProxySites"), "geosite:youtube") || !slices.Contains(spisok(p, "ProxySites"), "geosite:telegram") {
		t.Errorf("через VPN %v", spisok(p, "ProxySites"))
	}
	if !slices.Contains(spisok(p, "DirectSites"), "geosite:category-ru") || !slices.Contains(spisok(p, "DirectIp"), "geoip:ru") {
		t.Errorf("напрямую %v %v", spisok(p, "DirectSites"), spisok(p, "DirectIp"))
	}
	// Пустые списки уходят массивами: null Happ может не принять.
	for _, k := range []string{"ProxyIp", "BlockSites", "BlockIp"} {
		if v, ok := p[k].([]any); !ok || len(v) != 0 {
			t.Errorf("%s = %v, нужен пустой массив", k, p[k])
		}
	}
}

// Выключение ставит профиль под тем же именем, а не выключает маршрутизацию
// Happ целиком: правила чужих подписок остаются.
func TestVyklyuchit(t *testing.T) {
	s, err := Vyklyuchit()
	if err != nil {
		t.Fatal(err)
	}
	p := razobrat(t, s)
	if p["Name"] != imya || p["GlobalProxy"] != "true" {
		t.Fatalf("профиль выключения: %v", p)
	}
	if got := spisok(p, "ProxySites"); len(got) != 0 {
		t.Errorf("через VPN %v", got)
	}
	if !slices.Equal(spisok(p, "DirectSites"), lokalnyeSayty) || !slices.Equal(spisok(p, "DirectIp"), lokalnyeAdresa) {
		t.Errorf("напрямую %v %v", spisok(p, "DirectSites"), spisok(p, "DirectIp"))
	}
	// Сборка включения не задела списки выключения.
	if _, err := Vklyuchit(); err != nil || !slices.Equal(lokalnyeSayty, []string{"geosite:private"}) {
		t.Fatalf("списки после сборки: %v %v", lokalnyeSayty, err)
	}
}

// Каждое имя списка есть в файлах Loyalsoldier: имя, которого там нет, не
// даст ядру Happ запуститься, и VPN на телефоне не поднимется вовсе. Новое
// имя добавляется сюда только после сверки со списками v2fly.
func TestImenaSpiskovSvereny(t *testing.T) {
	sovereny := []string{
		"geosite:youtube", "geosite:google", "geosite:telegram", "geosite:whatsapp", "geosite:facebook",
		"geosite:instagram", "geosite:twitter", "geosite:discord", "geosite:linkedin",
		"geosite:category-ru", "geosite:private", "geoip:ru",
	}
	for _, x := range slices.Concat(cherezVpn, lokalnyeSayty, lokalnyeAdresa, rossiyaSayty, rossiyaAdresa) {
		if (strings.HasPrefix(x, "geosite:") || strings.HasPrefix(x, "geoip:")) && !slices.Contains(sovereny, x) {
			t.Errorf("список %s не сверен с файлами Loyalsoldier", x)
		}
	}
}
