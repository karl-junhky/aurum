package yadro

import "testing"

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
	}
	for _, p := range plohie {
		if _, err := razobratPaket([]byte(p)); err == nil {
			t.Fatalf("%s прошёл", p)
		}
	}
	for _, p := range []string{`{"versiya":1,"yadro":{"log":{}}}`, `{"versiya":2,"yadro":{"log":{}}}`,
		`{"versiya":3,"yadro":{}}`, `{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"198.51.100.7","staging":true}}`,
		`{"versiya":3,"yadro":{},"proba":{"srok":600}}`} {
		if _, err := razobratPaket([]byte(p)); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
