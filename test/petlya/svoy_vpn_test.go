package petlya

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
)

// Свой VPN в петле. Сервер поднимается на конфиге ядра прямо из пакета,
// который клиент отдаёт программе сервера, а ключи из ссылок продукта гоняет
// проверка ключей продукта на настоящем ядре. Тестового здесь то, без чего
// петля не встанет на рабочей машине: адреса и порты на 127.0.0.1, своя
// маска, сертификат на адрес от корня петли и привязка исходящих к петлевому
// адаптеру вместо физического.

// adresSvoego это адрес сервера в ключах. Разбор ссылок отвергает петлю,
// поэтому ключи ведут на адрес из документационного блока, а ядро проверки
// перенаправляется на 127.0.0.1.
const adresSvoego = "198.51.100.10"

// vhodSvoego это вход пакета: тип и порт, как в пакете. Номера у reality и
// hy2 бывают одинаковые, поэтому порт без типа вход не называет.
type vhodSvoego struct {
	tip  string // vless или hysteria2, у входа сервера и исходящего клиента
	port int
}

// stendSvoego это всё, что нужно и серверу, и проверке: секреты, маска,
// сертификат на адрес и цель проверки.
type stendSvoego struct {
	sekrety konfig.Sekrety
	maska   Maska
	sert    Sertifikat
	opcii   proverka.Opcii
}

func novyyStend(t *testing.T) stendSvoego {
	t.Helper()
	PutKYadru(t)
	maska := NovayaMaska(t)
	s, err := konfig.NovyeSekrety(maska.Imya)
	if err != nil {
		t.Fatal(err)
	}
	cel, korni := mishenProverki(t)
	return stendSvoego{
		sekrety: s, maska: maska, sert: NovyySertifikat(t, adresSvoego),
		opcii: proverka.Opcii{
			Adapter:     petlevoyAdapter(t),
			Vyhody:      []netip.Addr{netip.MustParseAddr("127.0.0.1")},
			CelZagruzki: cel + "/__down",
			CelVyhoda:   cel + "/ip",
			Korni:       korni,
		},
	}
}

// portyServera даёт каждому входу пакета свой порт на 127.0.0.1.
func portyServera(t *testing.T, v konfig.Vhody) map[vhodSvoego]int {
	t.Helper()
	p := map[vhodSvoego]int{}
	for _, port := range v.Reality {
		p[vhodSvoego{"vless", port}] = SvobodnyyPort(t)
	}
	for _, port := range v.Hy2 {
		p[vhodSvoego{"hysteria2", port}] = SvobodnyyPortUDP(t)
	}
	return p
}

// zapusk поднимает ядро проверки, направив каждый исходящий на 127.0.0.1 и
// порт из kuda. Ядро живёт в подтесте t: и провал, и гашение его.
func (s stendSvoego) zapusk(t *testing.T, kuda map[vhodSvoego]int) proverka.Zapusk {
	return func(_ context.Context, telo []byte) (func(), error) {
		var kk map[string]any
		if err := json.Unmarshal(telo, &kk); err != nil {
			return nil, err
		}
		for _, x := range kk["outbounds"].([]any) {
			m := x.(map[string]any)
			if m["server"] != adresSvoego {
				t.Fatalf("исходящий %v ведёт на %v", m["tag"], m["server"])
			}
			port, est := kuda[vhodSvoego{m["type"].(string), int(m["server_port"].(float64))}]
			if !est {
				t.Fatalf("исходящему %v %v:%v некуда идти", m["tag"], m["type"], m["server_port"])
			}
			m["server"], m["server_port"] = "127.0.0.1", port
			dobavitKoren(t, m, s.sert.CaPEM)
		}
		return PodnyatYadro(t, kk).Pogasit, nil
	}
}

func klyuchiSvoego(t *testing.T, s konfig.Sekrety, v konfig.Vhody) []protokol.Server {
	t.Helper()
	k, err := konfig.Klyuchi(s, v)
	if err != nil {
		t.Fatal(err)
	}
	var klyuchi []protokol.Server
	for _, x := range k {
		klyuchi = append(klyuchi, x.Server)
	}
	return klyuchi
}

func TestSvoyVPN(t *testing.T) {
	st := novyyStend(t)
	v := konfig.Vhody{Imya: "Петля", Adres: adresSvoego, Reality: []int{443}, Hy2: []int{8443}}
	paket, err := konfig.Paket(st.sekrety, v, konfig.Opcii{})
	if err != nil {
		t.Fatal(err)
	}
	klyuchi := klyuchiSvoego(t, st.sekrety, v)
	porty := portyServera(t, v)

	// Мишень на 127.0.0.1, а сервер свой VPN в частные сети не пускает:
	// ключи обязаны не довезти. Отличие от прогона ниже ровно в этом правиле.
	t.Run("частные сети закрыты", func(t *testing.T) {
		server := podnyatSvoy(t, paket, st, porty, true)
		defer server.Pogasit()
		itogi, err := proverka.Proverit(context.Background(), klyuchi, st.opcii, st.zapusk(t, porty))
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range itogi {
			if it.Vid != proverka.NetOtveta {
				t.Errorf("%s: %s %s", it.Server.Imya, it.Vid, it.Otkaz)
			}
		}
	})
	t.Run("везёт", func(t *testing.T) {
		server := podnyatSvoy(t, paket, st, porty, false)
		defer server.Pogasit()
		itogi, err := proverka.Proverit(context.Background(), klyuchi, st.opcii, st.zapusk(t, porty))
		if err != nil {
			t.Fatal(err)
		}
		if len(itogi) != 2 {
			t.Fatalf("итогов %d", len(itogi))
		}
		for _, it := range itogi {
			if it.Vid != proverka.Vezyot || it.Bayt < 2_000_000 || it.Vyhod != netip.MustParseAddr("127.0.0.1") {
				t.Errorf("%s: %s %s, %d байт", it.Server.Imya, it.Vid, it.Otkaz, it.Bayt)
			}
		}
	})
}

// Подбор портов на пробном пакете: первый кандидат reality закрыт, второй
// пропускает рукопожатие и первые килобайты и замерзает, как заморозка
// провайдера после ~16 КБ; первый кандидат hy2 замерзает так же. Подбор
// обязан взять третий reality и второй hy2.
func TestPodborPortov(t *testing.T) {
	st := novyyStend(t)
	v := konfig.Vhody{Imya: "Петля", Adres: adresSvoego, Reality: []int{443, 8443, 465}, Hy2: []int{443, 8443}}
	paket, err := konfig.Paket(st.sekrety, v, konfig.Opcii{Proba: porty.SrokProby(v)})
	if err != nil {
		t.Fatal(err)
	}
	naServere := portyServera(t, v)
	server := podnyatSvoy(t, paket, st, naServere, false)
	defer server.Pogasit()

	const porog = 16_000
	kuda := map[vhodSvoego]int{
		{"vless", 443}:      SvobodnyyPort(t), // никто не слушает
		{"vless", 8443}:     zamorazhivayushchiyTCP(t, naServere[vhodSvoego{"vless", 8443}], porog),
		{"vless", 465}:      naServere[vhodSvoego{"vless", 465}],
		{"hysteria2", 443}:  zamorazhivayushchiyUDP(t, naServere[vhodSvoego{"hysteria2", 443}], porog),
		{"hysteria2", 8443}: naServere[vhodSvoego{"hysteria2", 8443}],
	}
	o := st.opcii
	var hod []string
	o.Hod = func(it proverka.Itog) {
		hod = append(hod, fmt.Sprintf("%s %d %s", it.Server.Transport, it.Server.Port, it.Vid))
	}
	vybor, err := porty.Podobrat(context.Background(), st.sekrety, v, o, st.zapusk(t, kuda))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range vybor.Proverki {
		t.Logf("%s: %s %s, %d байт за %s", it.Server.Imya, it.Vid, it.Otkaz, it.Bayt, it.Vremya)
	}
	if vybor.Reality != 465 || vybor.Hy2 != 8443 {
		t.Fatalf("выбраны reality %d, hy2 %d", vybor.Reality, vybor.Hy2)
	}
	zhdu := []string{
		"reality-tcp 443 ", "reality-tcp 443 net-otveta",
		"reality-tcp 8443 ", "reality-tcp 8443 zamerz",
		"reality-tcp 465 ", "reality-tcp 465 vezyot",
		"hy2 443 ", "hy2 443 ?",
		"hy2 8443 ", "hy2 8443 vezyot",
	}
	// Замёрзший hy2 бывает и без ответа: QUIC встаёт раньше заголовков.
	if len(hod) == len(zhdu) && (hod[7] == "hy2 443 zamerz" || hod[7] == "hy2 443 net-otveta") {
		hod[7] = "hy2 443 ?"
	}
	if !slices.Equal(hod, zhdu) {
		t.Fatalf("ход подбора\n%s", strings.Join(hod, "\n"))
	}
}

// zamorazhivayushchiyTCP стоит перед входом сервера kuda: к серверу
// пропускает всё, от сервера первые porog байт, дальше держит соединение
// молча. Отдаёт свой порт.
func zamorazhivayushchiyTCP(t *testing.T, kuda, porog int) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				s, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", kuda))
				if err != nil {
					return
				}
				defer s.Close()
				go func() {
					io.Copy(s, c)
					s.Close()
				}()
				io.CopyN(c, s, int64(porog))
				io.Copy(io.Discard, s)
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// zamorazhivayushchiyUDP то же для UDP: от сервера первые porog байт
// каждому адресу клиента, дальше пакеты сервера выбрасываются.
func zamorazhivayushchiyUDP(t *testing.T, kuda, porog int) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: kuda}
	var mu sync.Mutex
	k := map[string]*net.UDPConn{}
	t.Cleanup(func() {
		pc.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range k {
			c.Close()
		}
	})
	go func() {
		b := make([]byte, 65536)
		for {
			n, klient, err := pc.ReadFrom(b)
			if err != nil {
				return
			}
			mu.Lock()
			c := k[klient.String()]
			if c == nil {
				if c, err = net.DialUDP("udp", nil, server); err != nil {
					mu.Unlock()
					// Иначе сломанный посредник прошёл бы за «нет ответа».
					t.Errorf("посредник UDP не достал сервер: %v", err)
					return
				}
				k[klient.String()] = c
				go func() {
					otvet := make([]byte, 65536)
					otdano := 0
					for {
						n, err := c.Read(otvet)
						if err != nil {
							return
						}
						if otdano < porog {
							otdano += n
							pc.WriteTo(otvet[:n], klient)
						}
					}
				}()
			}
			mu.Unlock()
			c.Write(b[:n])
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// podnyatSvoy поднимает сервер на конфиге ядра из пакета. Правится только
// место: адреса, порты из porty, маска, файлы сертификата. sChastnymi
// оставляет правило, которое не пускает в частные сети.
func podnyatSvoy(t *testing.T, paket []byte, st stendSvoego, porty map[vhodSvoego]int, sChastnymi bool) *Yadro {
	t.Helper()
	var p struct {
		Yadro map[string]any `json:"yadro"`
	}
	if err := json.Unmarshal(paket, &p); err != nil {
		t.Fatal(err)
	}
	for _, x := range p.Yadro["inbounds"].([]any) {
		m := x.(map[string]any)
		port, est := porty[vhodSvoego{m["type"].(string), int(m["listen_port"].(float64))}]
		if !est {
			t.Fatalf("входу %v:%v нет порта", m["type"], m["listen_port"])
		}
		m["listen"], m["listen_port"] = "127.0.0.1", port
		tls := m["tls"].(map[string]any)
		switch m["type"] {
		case "vless":
			tls["reality"].(map[string]any)["handshake"] = map[string]any{"server": "127.0.0.1", "server_port": st.maska.Port}
		case "hysteria2":
			tls["certificate_path"], tls["key_path"] = st.sert.PutSert, st.sert.PutKlyuch
			m["masquerade"].(map[string]any)["url"] = fmt.Sprintf("https://127.0.0.1:%d", st.maska.Port)
		default:
			t.Fatalf("вход %v", m["type"])
		}
	}
	marshrut := p.Yadro["route"].(map[string]any)
	pravila := marshrut["rules"].([]any)
	i := slices.IndexFunc(pravila, func(x any) bool { return x.(map[string]any)["ip_is_private"] == true })
	if i < 0 {
		t.Fatal("в пакете нет правила для частных сетей")
	}
	if !sChastnymi {
		marshrut["rules"] = slices.Delete(pravila, i, i+1)
	}
	return PodnyatServer(t, p.Yadro)
}

// mishenProverki отвечает как цель проверки ключей: тело на 2 МБ с адресом
// запроса в cf-meta-ip и голый адрес на /ip. По https, как Cloudflare: vision
// с голым HTTP внутри изредка встаёт (см. proverka.Opcii.Korni).
func mishenProverki(t *testing.T) (string, *x509.CertPool) {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adres, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/__down":
			w.Header().Set("cf-meta-ip", adres)
			w.Write(make([]byte, 2_000_000))
		case "/ip":
			io.WriteString(w, adres)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	korni := x509.NewCertPool()
	korni.AddCert(s.Certificate())
	return s.URL, korni
}

func petlevoyAdapter(t *testing.T) string {
	t.Helper()
	spisok, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range spisok {
		if a.Flags&net.FlagLoopback != 0 && a.Flags&net.FlagUp != 0 {
			return a.Name
		}
	}
	t.Fatal("петлевого адаптера нет")
	return ""
}
