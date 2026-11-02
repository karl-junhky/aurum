package konfig

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func sekrety(t *testing.T) Sekrety {
	t.Helper()
	s, err := NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNovyeSekrety(t *testing.T) {
	a, b := sekrety(t), sekrety(t)
	if a.Uuid == b.Uuid || a.RealityZakr == b.RealityZakr || a.ShortId == b.ShortId || a.Parol == b.Parol || a.ObfsParol == b.ObfsParol {
		t.Fatalf("две установки с общим секретом:\n%+v\n%+v", a, b)
	}
}

func TestSekretyProverit(t *testing.T) {
	chuzhoy := sekrety(t)
	for _, s := range []struct {
		imya  string
		porch func(*Sekrety)
	}{
		{"uuid", func(s *Sekrety) { s.Uuid = strings.ToUpper(s.Uuid) }},
		{"short_id нечётной длины", func(s *Sekrety) { s.ShortId = "abc" }},
		{"short_id длинный", func(s *Sekrety) { s.ShortId = strings.Repeat("ab", 9) }},
		{"короткий пароль", func(s *Sekrety) { s.Parol = "abc" }},
		{"пароль с плюсом", func(s *Sekrety) { s.ObfsParol = s.ObfsParol[:20] + "+" }},
		{"маска со схемой", func(s *Sekrety) { s.Maska = "https://dl.google.com" }},
		{"маска без точки", func(s *Sekrety) { s.Maska = "localhost" }},
		{"открытый не пара", func(s *Sekrety) { s.RealityOtkr = chuzhoy.RealityOtkr }},
		{"закрытый не ключ", func(s *Sekrety) { s.RealityZakr = "abc" }},
	} {
		t.Run(s.imya, func(t *testing.T) {
			x := sekrety(t)
			s.porch(&x)
			if err := x.Proverit(); !errors.Is(err, errNegodno) {
				t.Fatalf("приняты негодные: %v", err)
			}
		})
	}
}

type razobrannyy struct {
	Versiya int `json:"versiya"`
	Yadro   struct {
		Inbounds []struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			ListenPort int    `json:"listen_port"`
			Users      []struct {
				Name     string `json:"name"`
				Uuid     string `json:"uuid"`
				Flow     string `json:"flow"`
				Password string `json:"password"`
			} `json:"users"`
			IgnoreClientBandwidth bool `json:"ignore_client_bandwidth"`
			Masquerade            struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"masquerade"`
			TLS struct {
				ServerName string   `json:"server_name"`
				Alpn       []string `json:"alpn"`
				CertPath   string   `json:"certificate_path"`
				KeyPath    string   `json:"key_path"`
				Reality    struct {
					Handshake struct {
						Server string `json:"server"`
						Port   int    `json:"server_port"`
					} `json:"handshake"`
					PrivateKey string   `json:"private_key"`
					ShortId    []string `json:"short_id"`
				} `json:"reality"`
			} `json:"tls"`
			Obfs struct {
				Type     string `json:"type"`
				Password string `json:"password"`
			} `json:"obfs"`
		} `json:"inbounds"`
		Route struct {
			Rules []map[string]any `json:"rules"`
			Final string           `json:"final"`
		} `json:"route"`
	} `json:"yadro"`
	SertIP    *Sert `json:"sert_ip"`
	SertIPSam *struct {
		IP      string `json:"ip"`
		Staging bool   `json:"staging"`
	} `json:"sert_ip_sam"`
	Proba *struct {
		Srok int `json:"srok"`
	} `json:"proba"`
}

func razobrat(t *testing.T, b []byte) razobrannyy {
	t.Helper()
	var r razobrannyy
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPaketOkonchatelnyy(t *testing.T) {
	s := sekrety(t)
	kopiya := &Sert{Cert: "C", Klyuch: "K"}
	b, err := Paket(s, Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{8443}}, Opcii{Staging: true, Kopiya: kopiya})
	if err != nil {
		t.Fatal(err)
	}
	p := razobrat(t, b)
	if p.Versiya != 3 || p.Proba != nil || p.SertIPSam == nil || p.SertIPSam.IP != "203.0.113.10" || !p.SertIPSam.Staging ||
		p.SertIP == nil || *p.SertIP != *kopiya || len(p.Yadro.Inbounds) != 2 {
		t.Fatalf("пакет %s", b)
	}
	r, h := p.Yadro.Inbounds[0], p.Yadro.Inbounds[1]
	if r.Type != "vless" || r.Tag != "reality-443" || r.ListenPort != 443 || len(r.Users) != 1 || r.Users[0].Uuid != s.Uuid ||
		r.Users[0].Flow != flowVision || r.TLS.ServerName != s.Maska || r.TLS.Reality.Handshake.Server != s.Maska || r.TLS.Reality.Handshake.Port != 443 ||
		r.TLS.Reality.PrivateKey != s.RealityZakr || len(r.TLS.Reality.ShortId) != 1 || r.TLS.Reality.ShortId[0] != s.ShortId {
		t.Fatalf("вход reality %+v", r)
	}
	if h.Type != "hysteria2" || h.Tag != "hy2-8443" || h.ListenPort != 8443 || len(h.Users) != 1 || h.Users[0].Password != s.Parol ||
		h.Obfs.Type != "salamander" || h.Obfs.Password != s.ObfsParol || h.TLS.ServerName != "203.0.113.10" ||
		h.TLS.CertPath != "/var/lib/vpn-uzel/sert-ip/fullchain.pem" || h.TLS.KeyPath != "/var/lib/vpn-uzel/sert-ip/privkey.pem" ||
		len(h.TLS.Alpn) != 1 || h.TLS.Alpn[0] != "h3" || !h.IgnoreClientBandwidth ||
		h.Masquerade.Type != "proxy" || h.Masquerade.URL != "https://"+s.Maska {
		t.Fatalf("вход hy2 %+v", h)
	}
	// Через свой VPN нельзя достучаться до частных сетей сервера и его
	// хостера: правило стоит после разрешения имён, иначе имя в частный адрес
	// прошло бы мимо.
	pravila := p.Yadro.Route.Rules
	if p.Yadro.Route.Final != "direct" || len(pravila) < 2 {
		t.Fatalf("маршрут %+v", p.Yadro.Route)
	}
	if posl, pred := pravila[len(pravila)-1], pravila[len(pravila)-2]; posl["ip_is_private"] != true || posl["action"] != "reject" ||
		pred["action"] != "resolve" {
		t.Fatalf("маршрут %+v", pravila)
	}
}

// Без hy2 сертификат не нужен: ни выпуска, ни копии.
func TestPaketBezHy2(t *testing.T) {
	b, err := Paket(sekrety(t), Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{465}}, Opcii{Kopiya: &Sert{Cert: "C", Klyuch: "K"}})
	if err != nil {
		t.Fatal(err)
	}
	if p := razobrat(t, b); p.SertIPSam != nil || p.SertIP != nil || len(p.Yadro.Inbounds) != 1 {
		t.Fatalf("пакет %s", b)
	}
	// По прямой просьбе выпуск идёт и без hy2, входы те же.
	b, err = Paket(sekrety(t), Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{465}}, Opcii{Vypusk: true, Staging: true, Proba: 360})
	if err != nil {
		t.Fatal(err)
	}
	if p := razobrat(t, b); p.SertIPSam == nil || p.SertIPSam.IP != "203.0.113.10" || !p.SertIPSam.Staging || len(p.Yadro.Inbounds) != 1 || p.Proba == nil {
		t.Fatalf("пакет с выпуском %s", b)
	}
}

// Пакет «только сертификат»: ни одного входа, выпуск на адрес и копия
// прошлой установки, если она есть.
func TestPaketSerta(t *testing.T) {
	kopiya := &Sert{Cert: "C", Klyuch: "K"}
	b, err := PaketSerta("::ffff:203.0.113.10", Opcii{Staging: true, Kopiya: kopiya})
	if err != nil {
		t.Fatal(err)
	}
	p := razobrat(t, b)
	if p.Versiya != versiyaPaketa || p.Proba != nil || p.SertIPSam == nil || p.SertIPSam.IP != "203.0.113.10" || !p.SertIPSam.Staging ||
		p.SertIP == nil || *p.SertIP != *kopiya || len(p.Yadro.Inbounds) != 0 || p.Yadro.Route.Final != "direct" {
		t.Fatalf("пакет %s", b)
	}
	if !strings.Contains(string(b), `"inbounds":[]`) {
		t.Fatalf("входы не пустым списком: %s", b)
	}
	for _, c := range []struct {
		adres string
		o     Opcii
	}{{"10.0.0.1", Opcii{}}, {"203.0.113.10%eth0", Opcii{}}, {"сервер", Opcii{}}, {"203.0.113.10", Opcii{Proba: 600}}} {
		if _, err := PaketSerta(c.adres, c.o); !errors.Is(err, errNegodno) {
			t.Errorf("%q %+v принят: %v", c.adres, c.o, err)
		}
	}
}

func TestPaketProbnyy(t *testing.T) {
	s := sekrety(t)
	v := Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443, 8443, 465}, Hy2: []int{443, 11444}}
	b, err := Paket(s, v, Opcii{Proba: 600})
	if err != nil {
		t.Fatal(err)
	}
	p := razobrat(t, b)
	if p.Proba == nil || p.Proba.Srok != 600 || len(p.Yadro.Inbounds) != 5 {
		t.Fatalf("пакет %s", b)
	}
	var tegi []string
	for _, vh := range p.Yadro.Inbounds {
		tegi = append(tegi, vh.Tag)
	}
	if strings.Join(tegi, " ") != "reality-443 reality-8443 reality-465 hy2-443 hy2-11444" {
		t.Fatalf("теги %q", tegi)
	}
	for _, srok := range []int{probaMin - 1, probaMax + 1, -1} {
		if _, err := Paket(s, v, Opcii{Proba: srok}); !errors.Is(err, errNegodno) {
			t.Errorf("срок пробы %d принят: %v", srok, err)
		}
	}
}

func TestVhodyNegodny(t *testing.T) {
	s := sekrety(t)
	for _, c := range []struct {
		imya  string
		v     Vhody
		proba bool
	}{
		{"без имени", Vhody{Adres: "203.0.113.10", Reality: []int{443}}, false},
		{"имя вместо адреса", Vhody{Imya: "Мой", Adres: "vpn.example.net", Reality: []int{443}}, false},
		{"адрес с зоной", Vhody{Imya: "Мой", Adres: "fe80::1%eth0", Reality: []int{443}}, false},
		{"петля", Vhody{Imya: "Мой", Adres: "127.0.0.1", Reality: []int{443}}, true},
		{"неуказанный", Vhody{Imya: "Мой", Adres: "::", Reality: []int{443}}, true},
		{"частный", Vhody{Imya: "Мой", Adres: "10.0.0.5", Reality: []int{443}}, true},
		{"частный в записи IPv6", Vhody{Imya: "Мой", Adres: "::ffff:192.168.1.5", Reality: []int{443}}, true},
		{"частный IPv6", Vhody{Imya: "Мой", Adres: "fd00::5", Reality: []int{443}}, true},
		{"локальный IPv6", Vhody{Imya: "Мой", Adres: "fe80::1", Reality: []int{443}}, true},
		{"групповой", Vhody{Imya: "Мой", Adres: "224.0.0.1", Reality: []int{443}}, true},
		{"без портов", Vhody{Imya: "Мой", Adres: "203.0.113.10"}, true},
		{"два reality в окончательном", Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443, 465}}, false},
		{"два hy2 в окончательном", Vhody{Imya: "Мой", Adres: "203.0.113.10", Hy2: []int{443, 8443}}, false},
		{"порт 0", Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{0}}, true},
		{"порт 65536", Vhody{Imya: "Мой", Adres: "203.0.113.10", Hy2: []int{65536}}, true},
		{"порт дважды", Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443, 465, 443}}, true},
	} {
		t.Run(c.imya, func(t *testing.T) {
			o := Opcii{}
			if c.proba {
				o.Proba = 60
			}
			if _, err := Paket(s, c.v, o); !errors.Is(err, errNegodno) {
				t.Errorf("пакет собран: %v", err)
			}
			if c.proba {
				if _, err := Klyuchi(s, c.v); !errors.Is(err, errNegodno) {
					t.Errorf("ключи собраны: %v", err)
				}
			}
		})
	}
	// Один и тот же номер у reality и hy2 это разные порты: TCP и UDP.
	if _, err := Paket(s, Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{443}}, Opcii{}); err != nil {
		t.Fatal(err)
	}
}

func TestKlyuchi(t *testing.T) {
	s := sekrety(t)
	k, err := Klyuchi(s, Vhody{Imya: "Мой сервер", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{8443}})
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != 2 {
		t.Fatalf("ключей %d", len(k))
	}
	r, h := k[0], k[1]
	if r.Server.Imya != "Мой сервер, reality" || r.Server.Transport != "reality-tcp" || r.Server.Host != "203.0.113.10" ||
		r.Server.Port != 443 || r.Server.Uuid != s.Uuid || r.Server.PublicKey != s.RealityOtkr || r.Server.ShortId != s.ShortId ||
		r.Server.Flow != flowVision || r.Server.Sni != s.Maska || r.Server.Fp != "edge" || r.Server.Id != ssylki.Id("203.0.113.10", 443, "reality-tcp") {
		t.Fatalf("reality %+v", r.Server)
	}
	if !r.Server.Svoy || !k[1].Server.Svoy {
		t.Fatal("ключи без пометки своего сервера: ядро поставит reality chrome вместо проверенного firefox")
	}
	if h.Server.Imya != "Мой сервер, hysteria2" || h.Server.Transport != "hy2" || h.Server.Port != 8443 || h.Server.Parol != s.Parol ||
		h.Server.Obfs != "salamander" || h.Server.ObfsParol != s.ObfsParol || h.Server.Sni != "203.0.113.10" || h.Server.Pin != "" {
		t.Fatalf("hy2 %+v", h.Server)
	}
	for _, c := range []struct {
		ssylka string
		chasti []string
	}{
		{r.Ssylka, []string{"vless://" + s.Uuid + "@203.0.113.10:443?", "security=reality", "pbk=" + s.RealityOtkr, "sid=" + s.ShortId,
			"flow=xtls-rprx-vision", "fp=edge", "sni=dl.google.com", "type=tcp", "#%D0%9C%D0%BE%D0%B9%20%D1%81%D0%B5%D1%80%D0%B2%D0%B5%D1%80%2C%20reality"}},
		{h.Ssylka, []string{"hy2://" + s.Parol + "@203.0.113.10:8443?", "obfs=salamander", "obfs-password=" + s.ObfsParol, "sni=203.0.113.10"}},
	} {
		for _, ch := range c.chasti {
			if !strings.Contains(c.ssylka, ch) {
				t.Errorf("в %q нет %q", c.ssylka, ch)
			}
		}
		if strings.Contains(c.ssylka, "insecure") || strings.Contains(c.ssylka, "pinSHA256") {
			t.Errorf("ссылка без проверки сертификата: %q", c.ssylka)
		}
	}
}

// Адрес IPv4, записанный как IPv6, идёт в ключи и сертификат обычным IPv4:
// иначе ссылка разошлась бы с сертификатом на IP.
func TestKlyuchiIPv4VIPv6(t *testing.T) {
	s := sekrety(t)
	v := Vhody{Imya: "Мой", Adres: "::ffff:203.0.113.10", Hy2: []int{8443}}
	k, err := Klyuchi(s, v)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Paket(s, v, Opcii{})
	if err != nil {
		t.Fatal(err)
	}
	if p := razobrat(t, b); k[0].Server.Host != "203.0.113.10" || k[0].Server.Sni != "203.0.113.10" || p.SertIPSam.IP != "203.0.113.10" ||
		p.Yadro.Inbounds[0].TLS.ServerName != "203.0.113.10" {
		t.Fatalf("ключ %+v, пакет %s", k[0].Server, b)
	}
}

// Кандидаты подбора различаются по имени портом, адрес IPv6 идёт в скобках.
func TestKlyuchiKandidatov(t *testing.T) {
	k, err := Klyuchi(sekrety(t), Vhody{Imya: "Мой", Adres: "2001:db8::10", Reality: []int{443, 8443}, Hy2: []int{11444}})
	if err != nil {
		t.Fatal(err)
	}
	var imena []string
	for _, x := range k {
		imena = append(imena, x.Server.Imya)
	}
	if strings.Join(imena, "|") != "Мой, reality 443|Мой, reality 8443|Мой, hysteria2" {
		t.Fatalf("имена %q", imena)
	}
	if !strings.Contains(k[0].Ssylka, "@[2001:db8::10]:443?") || k[2].Server.Sni != "2001:db8::10" {
		t.Fatalf("IPv6: %q %+v", k[0].Ssylka, k[2].Server)
	}
}

// Захват собирает из пакета сервера те же ключи, что ставила установка.
func TestIzPaketa(t *testing.T) {
	s := sekrety(t)
	for _, v := range []Vhody{
		{Imya: "Дача", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{8443}},
		{Imya: "Дача", Adres: "203.0.113.10", Reality: []int{8444}},
	} {
		p, err := Paket(s, v, Opcii{})
		if err != nil {
			t.Fatal(err)
		}
		s2, v2, err := IzPaketa(p, v.Adres, v.Imya)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(v2.Reality, v.Reality) || !slices.Equal(v2.Hy2, v.Hy2) || v2.Adres != v.Adres || v2.Imya != v.Imya {
			t.Fatalf("входы %+v, а ставились %+v", v2, v)
		}
		bylo, err := Klyuchi(s, v)
		if err != nil {
			t.Fatal(err)
		}
		stalo, err := Klyuchi(s2, v2)
		if err != nil {
			t.Fatal(err)
		}
		for i := range bylo {
			if bylo[i].Ssylka != stalo[i].Ssylka {
				t.Fatalf("ключ %d после захвата другой:\n%s\n%s", i, bylo[i].Ssylka, stalo[i].Ssylka)
			}
		}
	}
}

func TestIzPaketaNegodnyy(t *testing.T) {
	s := sekrety(t)
	v := Vhody{Imya: "x", Adres: "203.0.113.10", Reality: []int{443, 8443}, Hy2: []int{8443}}
	proba, err := Paket(s, v, Opcii{Proba: 60})
	if err != nil {
		t.Fatal(err)
	}
	sert, err := PaketSerta("203.0.113.10", Opcii{})
	if err != nil {
		t.Fatal(err)
	}
	okonch, err := Paket(s, Vhody{Imya: "x", Adres: "203.0.113.10", Reality: []int{443}}, Opcii{})
	if err != nil {
		t.Fatal(err)
	}
	chuzhaya := strings.Replace(string(okonch), `"versiya":3`, `"versiya":2`, 1)
	for imya, b := range map[string][]byte{
		"пробный":           proba,
		"только сертификат": sert,
		"другая версия":     []byte(chuzhaya),
		"не json":           []byte("{"),
	} {
		if _, _, err := IzPaketa(b, "203.0.113.10", "x"); !errors.Is(err, errNegodno) {
			t.Errorf("%s: %v", imya, err)
		}
	}
}
