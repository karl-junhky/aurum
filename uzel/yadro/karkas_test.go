package yadro

import (
	"strings"
	"testing"
)

func TestKarkas(t *testing.T) {
	bazovyy := `{"inbounds":[
		{"type":"vless","tag":"v","listen_port":110,"users":[{"name":"a","uuid":"u1"}]},
		{"type":"tuic","tag":"t","listen_port":443,"users":[{"name":"a","uuid":"u1","password":"p"}]}
	],"route":{"final":"pryamo"}}`
	k0, err := Karkas([]byte(bazovyy))
	if err != nil {
		t.Fatal(err)
	}
	sluchai := []struct {
		chto     string
		konfig   string
		prezhniy bool
	}{
		{"другие люди во входе vless", `{"route":{"final":"pryamo"},"inbounds":[
			{"type":"vless","tag":"v","listen_port":110,"users":[{"name":"b","uuid":"u2"},{"name":"a","uuid":"u1"}]},
			{"type":"tuic","tag":"t","listen_port":443,"users":[{"name":"a","uuid":"u1","password":"p"}]}]}`, true},
		{"другой порт", `{"inbounds":[
			{"type":"vless","tag":"v","listen_port":111,"users":[{"name":"a","uuid":"u1"}]},
			{"type":"tuic","tag":"t","listen_port":443,"users":[{"name":"a","uuid":"u1","password":"p"}]}],"route":{"final":"pryamo"}}`, false},
		{"люди во входе не нашего типа", `{"inbounds":[
			{"type":"vless","tag":"v","listen_port":110,"users":[{"name":"a","uuid":"u1"}]},
			{"type":"tuic","tag":"t","listen_port":443,"users":[]}],"route":{"final":"pryamo"}}`, false},
		{"другой маршрут", `{"inbounds":[
			{"type":"vless","tag":"v","listen_port":110,"users":[{"name":"a","uuid":"u1"}]},
			{"type":"tuic","tag":"t","listen_port":443,"users":[{"name":"a","uuid":"u1","password":"p"}]}],"route":{"final":"drugoy"}}`, false},
	}
	for _, s := range sluchai {
		k, err := Karkas([]byte(s.konfig))
		if err != nil {
			t.Fatalf("%s: %v", s.chto, err)
		}
		if (k == k0) != s.prezhniy {
			t.Fatalf("%s: каркас прежний = %v, ждали %v", s.chto, k == k0, s.prezhniy)
		}
	}
	if _, err := Karkas([]byte(`{"inbounds":[1]}`)); err == nil {
		t.Fatal("вход-число прошёл")
	}
}

func TestRazobratPaket(t *testing.T) {
	plohie := []string{
		`{"versiya":1}`,
		`{"versiya":1,"yadro":{},"lishnee":1}`,
		`{"versiya":4,"yadro":{}}`,
		`{"versiya":0,"yadro":{}}`,
		`{"versiya":2,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7"}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"vpn.example.net"}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","lishnee":1}}`,
		`{"versiya":2,"yadro":{},"proba":{"srok":60}}`,
		`{"versiya":3,"yadro":{},"proba":{"srok":5}}`,
		`{"versiya":3,"yadro":{},"proba":{"srok":3601}}`,
		`{"versiya":3,"yadro":{},"proba":{}}`,
		`{"versiya":1,"yadro":{},"podpiska":{"slushat":":8443","lyudi":{}}}`,
		`{"versiya":2,"yadro":{},"podpiska":{"slushat":":8443","lyudi":{}}}`,
		`{"versiya":2,"yadro":{},"sert":{"cert":"x","klyuch":"y"}}`,
		`{"versiya":2,"yadro":{},"podpiska":{"slushat":":8443","lyudi":{},"lishnee":1}}`,
		`не json`,
		// Режим домена: сайт только с выпуском на тот же домен, копия на IP
		// при выпуске на домен не годится.
		`{"versiya":3,"yadro":{},"sayt":` + saytTest + `}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7"},"sayt":` + saytTest + `}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"drugoy.example.net"},"sayt":` + saytTest + `}`,
		`{"versiya":2,"yadro":{},"sayt":` + saytTest + `}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"Kot.example.net"}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"kot.example.net"},"sert_ip":{"cert":"x","klyuch":"y"}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"kot.example.net"},"sayt":` +
			strings.Replace(saytTest, `"127.0.0.1:10443"`, `"0.0.0.0:10443"`, 1) + `}`,
	}
	for _, p := range plohie {
		if _, err := razobratPaket([]byte(p)); err == nil {
			t.Fatalf("%s прошёл", p)
		}
	}
	for _, p := range []string{`{"versiya":1,"yadro":{"log":{}}}`, `{"versiya":2,"yadro":{"log":{}}}`,
		`{"versiya":3,"yadro":{}}`, `{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","staging":true}}`,
		`{"versiya":3,"yadro":{},"proba":{"srok":600}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"kot.example.net"}}`,
		`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","domen":"kot.example.net"},"sayt":` + saytTest + `}`,
		`{"versiya":3,"yadro":{},"proba":{"srok":600},"sert_ip_sam":{"ip":"198.51.100.7","domen":"kot.example.net"},"sayt":` + saytTest + `}`} {
		if _, err := razobratPaket([]byte(p)); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

const saytTest = `{"domen":"kot.example.net","port":443,"slushat":"127.0.0.1:10443",` +
	`"zerno":"0123456789abcdef0123456789abcdef","podpiska":{"put":"fedcba9876543210fedcba9876543210",` +
	`"zagolovki":{"profile-update-interval":"12"},"telo":"x"}}`

func TestMaskaRealityIzKonfiga(t *testing.T) {
	konfig := []byte(`{"inbounds":[
		{"type":"hysteria2","tag":"hy2-8443","listen_port":8443,"tls":{"enabled":true,"server_name":"203.0.113.10"}},
		{"type":"vless","tag":"reality-443","listen_port":443,"tls":{"enabled":true,"server_name":"sosed.example.net",
			"reality":{"enabled":true,"handshake":{"server":"sosed.example.net","server_port":443}}}},
		{"type":"vless","tag":"vless-tls","listen_port":2053,"tls":{"enabled":true,"server_name":"vpn.example.net",
			"reality":{"enabled":false,"handshake":{"server":"ne.example.net"}}}}]}`)
	vh := opisatVhody(konfig)
	maski := map[string]string{}
	for _, v := range vh {
		maski[v.Teg] = v.Maska
	}
	if maski["reality-443"] != "sosed.example.net" || maski["hy2-8443"] != "" || maski["vless-tls"] != "" {
		t.Fatalf("маски входов %v", maski)
	}
	y := &Yadro{}
	if m := y.Maska(); m != "" {
		t.Fatalf("ядро не поднято, а маска %q", m)
	}
	y.rabotaet = &zapushchennoe{vhody: vh}
	if m := y.Maska(); m != "sosed.example.net" {
		t.Fatalf("маска ядра %q", m)
	}
	y.rabotaet = &zapushchennoe{vhody: opisatVhody([]byte(`{"inbounds":[{"type":"hysteria2","tag":"hy2-8443"}]}`))}
	if m := y.Maska(); m != "" {
		t.Fatalf("без reality маска %q", m)
	}
	// Режим домена: reality ведёт на свой сайт на петле, сторожить нечего.
	y.rabotaet = &zapushchennoe{vhody: opisatVhody([]byte(`{"inbounds":[
		{"type":"vless","tag":"reality-443","listen_port":443,"tls":{"enabled":true,"server_name":"kot.example.net",
			"reality":{"enabled":true,"handshake":{"server":"127.0.0.1","server_port":10443}}}}]}`))}
	if m := y.Maska(); m != "" {
		t.Fatalf("в режиме домена маска %q", m)
	}
}
