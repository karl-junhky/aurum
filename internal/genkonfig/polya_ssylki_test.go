package genkonfig

import (
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Находка 17. Разбор ссылки вынимал fp, alpn и host, клал их в модель, а
// генератор о них не знал вовсе. Практическое последствие: fp=firefox в чужой
// ссылке игнорируется, отпечаток TLS уходит дефолтный, то есть НЕ тот, на
// который рассчитывал сервер, а REALITY отпечаток замечает.
func ishodyashchiyProby(t *testing.T, s protokol.Server) map[string]any {
	t.Helper()
	o, err := ishodyashchiy(Vhod{Server: s}, s, "srv-proba")
	if err != nil {
		t.Fatalf("исходящий не собран: %v", err)
	}
	b, _ := json.Marshal(o)
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// У reality отпечаток всегда chrome, даже когда ссылка просит firefox.
//
// Xray с 26.9.8 отвергает приветствие reality без ключа X25519MLKEM768, а в
// пресетах utls ядра его несёт только chrome. Официальный клиент Xray с
// fp=firefox к такому серверу подключается, поэтому послушаться ссылки значило
// бы молча сломать чужой рабочий ключ. Пустой отпечаток здесь тоже нельзя: без
// utls рукопожатие Go, и REALITY его отвергнет.
func TestRealityVsegdaSChrome(t *testing.T) {
	for _, transport := range []string{"reality-tcp", "reality-grpc"} {
		for _, fp := range []string{"firefox", "edge", ""} {
			t.Run(transport+"/"+fp, func(t *testing.T) {
				s := protokol.Server{
					Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
					Uuid: "11111111-2222-3333-4444-555555555555",
					// 43 символа base64url: проверка ключа сильна в значениях.
					PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
					Sni: "www.example.com", Fp: fp,
				}
				utls := ishodyashchiyProby(t, s)["tls"].(map[string]any)["utls"].(map[string]any)
				if utls["fingerprint"] != "chrome" {
					t.Fatalf("отпечаток reality %v: без X25519MLKEM768 свежий Xray его отвергнет", utls["fingerprint"])
				}
			})
		}
	}
}

// Свой сервер (раздел «Свой VPN») это sing-box, а не Xray: его reality идёт с
// firefox, с которым ключ проверялся при установке. Пометка, а не отпечаток
// ссылки: в ссылке своего ключа стоит edge для телефона.
func TestSvoyRealitySFirefox(t *testing.T) {
	for _, transport := range []string{"reality-tcp", "reality-grpc"} {
		t.Run(transport, func(t *testing.T) {
			s := protokol.Server{
				Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
				Uuid:      "11111111-2222-3333-4444-555555555555",
				PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
				Sni: "www.example.com", Fp: "edge", Svoy: true,
			}
			utls := ishodyashchiyProby(t, s)["tls"].(map[string]any)["utls"].(map[string]any)
			if utls["fingerprint"] != "firefox" {
				t.Fatalf("отпечаток своего reality %v, проверялся firefox", utls["fingerprint"])
			}
		})
	}
}

// Обычный TLS без проверенного fp в ссылке ходит с firefox.
//
// Приветствие chrome в ядре 1757 байт, двумя TCP-сегментами, и залп из шести
// соединений разом замораживал адрес сервера на 70-150 с (02.10.2026, запись
// пакетов на сервере). firefox 663 байта одним сегментом, залп проходил.
// Непроверенный отпечаток (edge) уходит туда же: ядро примет его молча, а
// несёт ли он, никто не мерил.
func TestObychnyyTlsPoUmolchaniyuSFirefox(t *testing.T) {
	for _, transport := range []string{"trojan", "trojan-ws", "anytls", "tls-tcp", "ws", "httpupgrade", "grpc", "vmess"} {
		for _, fp := range []string{"", "edge", "firefox"} {
			t.Run(transport+"/"+fp, func(t *testing.T) {
				s := protokol.Server{
					Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
					Uuid: "11111111-2222-3333-4444-555555555555", Parol: "p",
					Sni: "a.example", Fp: fp,
				}
				utls, est := ishodyashchiyProby(t, s)["tls"].(map[string]any)["utls"].(map[string]any)
				if !est {
					t.Fatal("utls не появился")
				}
				if utls["fingerprint"] != "firefox" {
					t.Fatalf("отпечаток %v, ожидался firefox", utls["fingerprint"])
				}
			})
		}
	}
}

func TestAlpnIzSsylkiDoezzhaetDoYadra(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555",
		Sni:  "a.example", Alpn: "h2,http/1.1",
	}
	v := ishodyashchiyProby(t, s)
	tls := v["tls"].(map[string]any)
	alpn, est := tls["alpn"].([]any)
	if !est {
		t.Fatalf("alpn до ядра не доехал: %v", tls)
	}
	if len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Fatalf("alpn разобран неверно: %v", alpn)
	}
}

// КОНТРОЛЬ: без alpn в ссылке поля быть не должно. Пустой список ALPN это не то
// же самое, что его отсутствие: сервер выберет по нему и не найдёт ничего.
func TestBezAlpnPolyaNet(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
	}
	v := ishodyashchiyProby(t, s)
	if _, est := v["tls"].(map[string]any)["alpn"]; est {
		t.Fatal("пустой alpn всё равно уехал в конфиг")
	}
}

func TestZagolovokHostIzSsylkiDoezzhaetDoYadra(t *testing.T) {
	for _, transport := range []string{"ws", "httpupgrade"} {
		t.Run(transport, func(t *testing.T) {
			s := protokol.Server{
				Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
				Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
				PublicKey:     "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
				HostZagolovka: "front.example",
			}
			v := ishodyashchiyProby(t, s)
			tr, est := v["transport"].(map[string]any)
			if !est {
				t.Fatalf("нет транспорта вовсе: %v", v)
			}
			zag, est := tr["headers"].(map[string]any)
			if !est {
				t.Fatalf("заголовок Host до ядра не доехал: %v", tr)
			}
			if zag["Host"] != "front.example" {
				t.Fatalf("Host %v, в ссылке был front.example", zag["Host"])
			}
		})
	}
}

// С1 аудита 1.6.1. Ws+tls за CDN часто записан адресом и host= без sni. Имя в
// TLS бралось из адреса, и фронт отвечал сертификатом не того сайта.
func TestImyaTLSBeryotZagolovokHostDoAdresa(t *testing.T) {
	for _, transport := range []string{"ws", "httpupgrade", "trojan-ws", "vmess-ws", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			s := protokol.Server{
				Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
				Uuid: "11111111-2222-3333-4444-555555555555", Parol: "p",
				HostZagolovka: "front.example",
			}
			if got := ishodyashchiyProby(t, s)["tls"].(map[string]any)["server_name"]; got != "front.example" {
				t.Fatalf("server_name %v, в ссылке host=front.example", got)
			}
			s.Sni = "sni.example"
			if got := ishodyashchiyProby(t, s)["tls"].(map[string]any)["server_name"]; got != "sni.example" {
				t.Fatalf("server_name %v: sni из ссылки главнее заголовка", got)
			}
		})
	}
	// У reality имя это маска, и заголовок Host к ней отношения не имеет.
	s := protokol.Server{
		Id: "p", Transport: "reality-grpc", Host: "203.0.113.20", Port: 443,
		Uuid:      "11111111-2222-3333-4444-555555555555",
		PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
		HostZagolovka: "front.example",
	}
	if got := ishodyashchiyProby(t, s)["tls"].(map[string]any)["server_name"]; got != "203.0.113.20" {
		t.Fatalf("reality взял имя %v из заголовка Host", got)
	}
}

// Без fp в ссылке utls на обычном TLS обязан появиться САМ.
//
// Перевёрнут 21.09.2026. До того дня проверка требовала обратного, и из-за неё
// trojan, anytls и httpupgrade ходили с рукопожатием Go: отпечаток стоял только
// в reality-ссылках. Снятый с сервера ClientHello живого пользователя не нёс ни
// server_name, ни ALPN, ни одного GREASE — то есть отличался от браузерного
// первым же пакетом.
func TestBezOtpechatkaNaObychnomTlsUtlsVsyoRavnoEst(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
	}
	v := ishodyashchiyProby(t, s)
	utls, est := v["tls"].(map[string]any)["utls"].(map[string]any)
	if !est {
		t.Fatal("utls не появился: рукопожатие Go узнаётся по отсутствию GREASE и ALPN")
	}
	if utls["fingerprint"] != "firefox" {
		t.Fatalf("отпечаток %v, без просьбы ссылки ожидался firefox", utls["fingerprint"])
	}
}

// У QUIC-протоколов utls быть НЕ должно.
//
// Ядро отвечает «unsupported usage for uTLS» на каждое соединение, а check
// принимает такой конфиг молча и с кодом 0 (проверено подъёмом 21.09.2026).
// Сторож живёт рядом с проверкой выше не случайно: она требует utls везде, где
// есть TLS, и без этой пары первый же «везде» унёс бы tuic целиком.
func TestUQuicProtokolovUtlsNet(t *testing.T) {
	dlya := []protokol.Server{
		{Id: "t", Transport: "tuic", Host: "203.0.113.20", Port: 443, Sni: "a.example",
			Uuid: "11111111-2222-3333-4444-555555555555", Parol: "p"},
		{Id: "h", Transport: "hy2", Host: "203.0.113.20", Port: 443, Sni: "a.example", Parol: "p"},
	}
	for _, s := range dlya {
		t.Run(s.Transport, func(t *testing.T) {
			v := ishodyashchiyProby(t, s)
			if _, est := v["tls"].(map[string]any)["utls"]; est {
				t.Fatal("utls уехал в QUIC: ядро откажет в каждом соединении, а check промолчит")
			}
		})
	}
}

// Зеркало: ПРОВЕРЕННЫЙ fp из ссылки на ws доезжает как есть, иначе проверка
// выше зелёная и на генераторе, который отпечаток ссылки игнорирует вовсе.
func TestSOtpechatkomNaObychnomTlsUtlsEst(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example", Fp: "chrome",
	}
	v := ishodyashchiyProby(t, s)
	utls, est := v["tls"].(map[string]any)["utls"].(map[string]any)
	if !est {
		t.Fatal("отпечаток из ссылки на ws не доехал")
	}
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("отпечаток %v, в ссылке был chrome", utls["fingerprint"])
	}
}

// AnyTLS обязан держать тёплые сессии.
//
// Умолчание ядра это ноль: проверка раз в 30 секунд закрывает все простаивающие
// сессии, и каждая следующая вкладка платит новым TLS-рукопожатием. Смысл
// протокола ровно обратный, а пачка рукопожатий к одному адресу это то, из-за
// чего адрес и замораживают.
func TestAnytlsDerzhitTyoplyeSessii(t *testing.T) {
	s := protokol.Server{
		Id: "a", Transport: "anytls", Host: "203.0.113.20", Port: 2087,
		Parol: "p", Sni: "a.example",
	}
	v := ishodyashchiyProby(t, s)
	n, est := v["min_idle_session"]
	if !est {
		t.Fatal("min_idle_session не задан: ядро закроет простаивающие сессии через 30 с")
	}
	// float64: число прошло через json.Unmarshal, как и всё остальное здесь.
	if n != float64(3) {
		t.Fatalf("min_idle_session = %v, ожидалось 3", n)
	}
}
