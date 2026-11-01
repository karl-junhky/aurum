package proverka

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
)

func TestMain(m *testing.M) {
	pauza = 50 * time.Millisecond
	tishina = 600 * time.Millisecond
	srokKlyucha = 5 * time.Second
	srokPodyoma = 2 * time.Second
	os.Exit(m.Run())
}

// povedenie это как ведёт себя вход сервера на порту.
type povedenie string

const (
	vezyot       povedenie = "vezyot"
	zamerz       povedenie = "zamerz"
	molchit      povedenie = "molchit"
	chuzhoy      povedenie = "chuzhoy"
	kod          povedenie = "kod"
	bezZagolovka povedenie = "bez-zagolovka"
	bezAdresa    povedenie = "bez-adresa"
	lyuboy       povedenie = "lyuboy" // везёт с любым секретом
	medlenno     povedenie = "medlenno"
	smert        povedenie = "smert"  // ядро проверки падает на этом ключе
	mapped       povedenie = "mapped" // адрес выхода IPv4 в записи IPv6
	// visitKontrol: свой ключ везёт, испорченная копия висит без ответа.
	visitKontrol povedenie = "visit-kontrol"
	obryv        povedenie = "obryv"
)

// podstava это ядро проверки вместе с сервером и целью: читает конфиг, как
// ядро, и отвечает так, как ответил бы вход сервера с поведением порта.
// Секрет исходящего сверяется с настоящим, поэтому испорченная копия
// умирает сама.
type podstava struct {
	t         *testing.T
	sekrety   map[int]string
	povedenie map[int]povedenie

	mu          sync.Mutex
	parol       string
	vyhody      map[string]map[string]any
	zaprosy     []string
	nachala     []time.Time
	zapuskov    int
	ostanovleno int
	bezVhoda    bool
	// ubit гасит ядро посреди прогона, priKontrole зовётся, когда повис
	// испорченный ключ.
	ubit        func()
	priKontrole func()
}

func novayaPodstava(t *testing.T, klyuchi []protokol.Server, pov ...povedenie) *podstava {
	p := &podstava{t: t, sekrety: map[int]string{}, povedenie: map[int]povedenie{}}
	for i, k := range klyuchi {
		p.sekrety[k.Port] = k.PublicKey + k.Parol
		p.povedenie[k.Port] = pov[i]
	}
	return p
}

func (p *podstava) zapusk(ctx context.Context, telo []byte) (func(), error) {
	var k struct {
		Inbounds []struct {
			Type       string `json:"type"`
			Listen     string `json:"listen"`
			ListenPort int    `json:"listen_port"`
			Users      []struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"users"`
		} `json:"inbounds"`
		Outbounds []map[string]any `json:"outbounds"`
		Route     struct {
			Rules []struct {
				Inbound  []string `json:"inbound"`
				AuthUser []string `json:"auth_user"`
				Outbound string   `json:"outbound"`
				Action   string   `json:"action"`
			} `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		p.t.Fatal(err)
	}
	if len(k.Inbounds) != 1 || k.Inbounds[0].Type != "mixed" || k.Inbounds[0].Listen != "127.0.0.1" {
		p.t.Fatalf("входы ядра проверки: %s", telo)
	}
	pravila := k.Route.Rules
	if len(pravila) == 0 || pravila[len(pravila)-1].Action != "reject" || len(pravila[len(pravila)-1].Inbound) != 0 {
		p.t.Fatalf("последнее правило не отбивает чужих: %s", telo)
	}
	po := map[string]map[string]any{}
	for _, o := range k.Outbounds {
		if o["bind_interface"] != "Ethernet" {
			p.t.Errorf("исходящий %v не привязан к адаптеру", o["tag"])
		}
		if o["type"] == "vless" {
			if fp := o["tls"].(map[string]any)["utls"].(map[string]any)["fingerprint"]; fp != "firefox" {
				p.t.Errorf("reality %v идёт с отпечатком %v", o["tag"], fp)
			}
		}
		po[o["tag"].(string)] = o
	}
	p.mu.Lock()
	p.zapuskov++
	p.vyhody = map[string]map[string]any{}
	for _, pr := range pravila[:len(pravila)-1] {
		if len(pr.AuthUser) != 1 || len(pr.Inbound) != 1 || pr.Inbound[0] != tegVhoda || po[pr.Outbound] == nil {
			p.t.Fatalf("правило %+v", pr)
		}
		p.vyhody[pr.AuthUser[0]] = po[pr.Outbound]
	}
	for _, u := range k.Inbounds[0].Users {
		if p.parol != "" && u.Password != p.parol {
			p.t.Fatal("у логинов разные пароли")
		}
		p.parol = u.Password
	}
	bezVhoda := p.bezVhoda
	p.mu.Unlock()
	if bezVhoda {
		return func() { p.mu.Lock(); p.ostanovleno++; p.mu.Unlock() }, nil
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(k.Inbounds[0].ListenPort)))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: p, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, kluchLogina{}, c.(*socksSoedinenie).login)
	}}
	go srv.Serve(&socksVhod{Listener: l, p: p})
	p.mu.Lock()
	p.ubit = func() { srv.Close() }
	p.mu.Unlock()
	return func() {
		srv.Close()
		p.mu.Lock()
		p.ostanovleno++
		p.mu.Unlock()
	}, nil
}

type kluchLogina struct{}

// ServeHTTP отвечает за цель: логин уже проверен рукопожатием SOCKS5.
func (p *podstava) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	login := r.Context().Value(kluchLogina{}).(string)
	p.mu.Lock()
	o := p.vyhody[login]
	p.zaprosy = append(p.zaprosy, login)
	p.nachala = append(p.nachala, time.Now())
	p.mu.Unlock()
	port := int(o["server_port"].(float64))
	sekret, _ := o["password"].(string)
	if o["type"] == "vless" {
		sekret = o["tls"].(map[string]any)["reality"].(map[string]any)["public_key"].(string)
	}
	pov := p.povedenie[port]
	if sekret != p.sekrety[port] && pov == visitKontrol {
		if p.priKontrole != nil {
			p.priKontrole()
		}
		<-r.Context().Done()
		return
	}
	if sekret != p.sekrety[port] && pov != lyuboy {
		pov = molchit
	}
	adres := o["server"].(string)
	if r.Host != "vyhod.example" && r.Host != "cel.example" {
		p.t.Errorf("запрос не к цели проверки: %s", r.Host)
	}
	if r.Host == "vyhod.example" {
		if pov == bezAdresa {
			pov = molchit
		} else {
			fmt.Fprint(w, adres)
			return
		}
	}
	switch pov {
	case smert:
		p.mu.Lock()
		ubit := p.ubit
		p.mu.Unlock()
		ubit()
		return
	case molchit:
		c, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			c.Close()
		}
		return
	case kod:
		w.WriteHeader(http.StatusTooManyRequests)
		return
	case chuzhoy:
		adres = "198.51.100.7"
	case bezZagolovka, bezAdresa:
		adres = ""
	case mapped:
		adres = "::ffff:" + adres
	}
	if adres != "" {
		w.Header().Set("cf-meta-ip", adres)
	}
	switch pov {
	case zamerz, obryv:
		w.Write(make([]byte, 100_000))
		w.(http.Flusher).Flush()
		if pov == obryv {
			panic(http.ErrAbortHandler)
		}
		<-r.Context().Done()
	case medlenno:
		// Каждый кусок приходит раньше срока тишины, весь поток позже него:
		// это медленный канал, а не заморозка.
		time.Sleep(tishina / 3)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for range 10 {
			time.Sleep(tishina / 3)
			w.Write(make([]byte, razmer/10))
			w.(http.Flusher).Flush()
		}
	default:
		// Лишнее сверх 2 МБ проверка не дочитывает.
		w.Write(make([]byte, razmer+10_000))
	}
}

// socksVhod это вход ядра проверки: SOCKS5 с логином и паролем.
// Неудачное рукопожатие закрывает соединение и ждёт следующего.
type socksVhod struct {
	net.Listener
	p *podstava
}

type socksSoedinenie struct {
	net.Conn
	r     *bufio.Reader
	login string
}

func (s *socksSoedinenie) Read(b []byte) (int, error) { return s.r.Read(b) }

func (v *socksVhod) Accept() (net.Conn, error) {
	for {
		c, err := v.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if s, err := v.rukopozhatie(c); err == nil {
			return s, nil
		}
		c.Close()
	}
}

func (v *socksVhod) rukopozhatie(c net.Conn) (*socksSoedinenie, error) {
	if err := c.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	br := bufio.NewReader(c)
	var oshibka error
	chitat := func(n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(br, b); err != nil && oshibka == nil {
			oshibka = err
		}
		return b
	}
	pisat := func(b ...byte) {
		if _, err := c.Write(b); err != nil && oshibka == nil {
			oshibka = err
		}
	}
	privet := chitat(2)
	metody := chitat(int(privet[1]))
	if oshibka != nil || privet[0] != 5 || !bytes.Contains(metody, []byte{2}) {
		pisat(5, 0xff)
		return nil, errors.New("socks: нет входа по паролю")
	}
	pisat(5, 2)
	login := string(chitat(int(chitat(2)[1])))
	parol := string(chitat(int(chitat(1)[0])))
	v.p.mu.Lock()
	svoy := parol == v.p.parol && v.p.vyhody[login] != nil
	v.p.mu.Unlock()
	if oshibka != nil || !svoy {
		pisat(1, 1)
		return nil, errors.New("socks: чужой логин")
	}
	pisat(1, 0)
	zapros := chitat(4)
	dlina := map[byte]int{1: 4, 4: 16}[zapros[3]]
	if zapros[3] == 3 {
		dlina = int(chitat(1)[0])
	}
	if oshibka != nil || zapros[1] != 1 || dlina == 0 {
		pisat(5, 7, 0, 1, 0, 0, 0, 0, 0, 0)
		return nil, errors.New("socks: не CONNECT")
	}
	chitat(dlina + 2)
	pisat(5, 0, 0, 1, 0, 0, 0, 0, 0, 0)
	if oshibka != nil {
		return nil, oshibka
	}
	if err := c.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return &socksSoedinenie{Conn: c, r: br, login: login}, nil
}

func klyuchi(t *testing.T, v konfig.Vhody) []protokol.Server {
	t.Helper()
	s, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	k, err := konfig.Klyuchi(s, v)
	if err != nil {
		t.Fatal(err)
	}
	var servery []protokol.Server
	for _, x := range k {
		servery = append(servery, x.Server)
	}
	return servery
}

func dva(t *testing.T) []protokol.Server {
	return klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443}, Hy2: []int{8443}})
}

var opcii = Opcii{Adapter: "Ethernet", CelZagruzki: "http://cel.example/__down", CelVyhoda: "http://vyhod.example/"}

func TestVseVezut(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, vezyot, vezyot)
	itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if err != nil {
		t.Fatal(err)
	}
	if len(itogi) != 2 {
		t.Fatalf("итогов %d", len(itogi))
	}
	for i, it := range itogi {
		if it.Server != k[i] || it.Vid != Vezyot || it.Vyhod != netip.MustParseAddr("203.0.113.10") || it.Bayt != razmer ||
			it.Otkaz != "" || it.Vremya <= 0 {
			t.Errorf("итог %d: %+v", i, it)
		}
	}
	// Ключи по одному в своём порядке, за ними контроль, с паузой между всеми.
	if strings.Join(p.zaprosy, " ") != "klyuch1 klyuch2 kontrol1 kontrol2" {
		t.Fatalf("порядок %q", p.zaprosy)
	}
	for i := 1; i < len(p.nachala); i++ {
		if d := p.nachala[i].Sub(p.nachala[i-1]); d < pauza {
			t.Errorf("между %s и %s всего %s", p.zaprosy[i-1], p.zaprosy[i], d)
		}
	}
	if p.zapuskov != 1 || p.ostanovleno != 1 {
		t.Fatalf("запусков %d, остановок %d", p.zapuskov, p.ostanovleno)
	}
}

func TestVidy(t *testing.T) {
	for _, c := range []struct {
		pov   povedenie
		vid   Vid
		otkaz string
	}{
		{zamerz, Zamerz, "поток встал: доехало 100 КБ из 2000 КБ"},
		{obryv, Zamerz, "поток встал: доехало 100 КБ из 2000 КБ: "},
		{medlenno, Vezyot, ""},
		{mapped, Vezyot, ""},
		{molchit, NetOtveta, "нет ответа через ключ"},
		{chuzhoy, ChuzhoyVyhod, "выход с 198.51.100.7, а не с адреса сервера"},
		{kod, Cel, "цель проверки ответила кодом 429"},
		{bezZagolovka, Vezyot, ""},
		{bezAdresa, Cel, "адрес выхода не назван"},
	} {
		t.Run(string(c.pov), func(t *testing.T) {
			k := dva(t)
			p := novayaPodstava(t, k, c.pov, vezyot)
			itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
			if err != nil {
				t.Fatal(err)
			}
			if it := itogi[0]; it.Vid != c.vid || !strings.HasPrefix(it.Otkaz, c.otkaz) || (c.otkaz == "") != (it.Otkaz == "") {
				t.Fatalf("итог %+v", it)
			}
			if it := itogi[1]; it.Vid != Vezyot {
				t.Fatalf("соседний ключ: %+v", it)
			}
		})
	}
}

// Сервер выходит в интернет с другого своего адреса: годен, если адрес
// назван.
func TestVyhodyServera(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, chuzhoy, vezyot)
	o := opcii
	// Адрес самого ключа годен и без списка, адрес списка в записи IPv6
	// сверяется как IPv4.
	o.Vyhody = []netip.Addr{netip.MustParseAddr("::ffff:198.51.100.7")}
	itogi, err := Proverit(context.Background(), k, o, p.zapusk)
	if err != nil {
		t.Fatal(err)
	}
	if itogi[0].Vid != Vezyot || itogi[0].Vyhod != netip.MustParseAddr("198.51.100.7") || itogi[1].Vid != Vezyot {
		t.Fatalf("итоги %+v", itogi)
	}
}

func TestKontrolProshyol(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, vezyot, lyuboy)
	itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if !errors.Is(err, ErrNegoden) || itogi != nil || !strings.Contains(err.Error(), "hy2") {
		t.Fatalf("итоги %+v, ошибка %v", itogi, err)
	}
	if p.ostanovleno != 1 {
		t.Fatalf("остановок %d", p.ostanovleno)
	}
}

func vidyItogov(itogi []Itog) string {
	var vidy []string
	for _, it := range itogi {
		vidy = append(vidy, string(it.Vid))
	}
	return strings.Join(vidy, " ")
}

// Контроль один на вид ключа и стоит на первом, что везёт, иначе на первом
// с ответом; вид без единого ответа контроля не требует.
func TestKontrolNaVid(t *testing.T) {
	for _, c := range []struct {
		imya          string
		reality, hy2  []int
		pov           []povedenie
		vidy, zaprosy string
	}{
		{"первые везут", []int{443, 465}, []int{8443, 11444}, []povedenie{vezyot, molchit, vezyot, zamerz},
			"vezyot net-otveta vezyot zamerz", "klyuch1 klyuch2 klyuch3 klyuch4 kontrol1 kontrol3"},
		{"везёт второй", []int{443, 465}, []int{8443}, []povedenie{molchit, vezyot, molchit},
			"net-otveta vezyot net-otveta", "klyuch1 klyuch2 klyuch3 kontrol2"},
		{"везущий важнее ответа", []int{443, 465, 993}, nil, []povedenie{molchit, zamerz, vezyot},
			"net-otveta zamerz vezyot", "klyuch1 klyuch2 klyuch3 kontrol3"},
		{"только ответ", []int{443, 465}, nil, []povedenie{molchit, zamerz},
			"net-otveta zamerz", "klyuch1 klyuch2 kontrol2"},
		{"никто не ответил", []int{443}, []int{8443}, []povedenie{molchit, molchit},
			"net-otveta net-otveta", "klyuch1 klyuch2"},
	} {
		t.Run(c.imya, func(t *testing.T) {
			k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: c.reality, Hy2: c.hy2})
			p := novayaPodstava(t, k, c.pov...)
			itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
			if err != nil {
				t.Fatal(err)
			}
			if vidyItogov(itogi) != c.vidy || strings.Join(p.zaprosy, " ") != c.zaprosy {
				t.Fatalf("виды %q, запросы %q", vidyItogov(itogi), p.zaprosy)
			}
		})
	}
}

// Подбор портов: после первого везущего ключа вид дальше не проверяется, ход
// виден до и после каждого ключа.
func TestPervyyNaVid(t *testing.T) {
	k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443, 465, 993}, Hy2: []int{8443, 11444}})
	p := novayaPodstava(t, k, molchit, vezyot, vezyot, vezyot, vezyot)
	o := opcii
	o.PervyyNaVid = true
	var hod []string
	o.Hod = func(it Itog) { hod = append(hod, strconv.Itoa(it.Server.Port)+":"+string(it.Vid)) }
	itogi, err := Proverit(context.Background(), k, o, p.zapusk)
	if err != nil {
		t.Fatal(err)
	}
	if vidyItogov(itogi) != "net-otveta vezyot vezyot" || itogi[1].Server != k[1] || itogi[2].Server != k[3] {
		t.Fatalf("итоги %+v", itogi)
	}
	if strings.Join(p.zaprosy, " ") != "klyuch1 klyuch2 klyuch4 kontrol2 kontrol4" {
		t.Fatalf("запросы %q", p.zaprosy)
	}
	if strings.Join(hod, " ") != "443: 443:net-otveta 465: 465:vezyot 8443: 8443:vezyot" {
		t.Fatalf("ход %q", hod)
	}
}

func TestPredel(t *testing.T) {
	if d := Predel(2, 2); d < srokPodyoma+4*srokKlyucha {
		t.Fatalf("предел двух ключей двух видов %s", d)
	}
	// Контроль один на вид: видов больше ключей не бывает.
	if Predel(3, 1) >= Predel(3, 3) || Predel(1, 2) != Predel(1, 1) {
		t.Fatal("предел считает контроль не по видам")
	}
}

// Испорченная копия стоит на везущем кандидате, а не на первом ключе вида:
// на мёртвом первом порту она «не ответила» бы и ничего не доказала.
func TestKontrolNaVezushchem(t *testing.T) {
	k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443, 465}})
	p := novayaPodstava(t, k, molchit, lyuboy)
	itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if !errors.Is(err, ErrNegoden) || itogi != nil || !strings.Contains(err.Error(), "порту 465") {
		t.Fatalf("итоги %+v, ошибка %v", itogi, err)
	}
}

func TestOtkazyDoZapuska(t *testing.T) {
	k := dva(t)
	dubl := []protokol.Server{k[0], k[0]}
	bezId := []protokol.Server{k[0]}
	bezId[0].Id = ""
	chuzhoy := []protokol.Server{{Id: "x", Imya: "ws", Transport: "ws", Host: "203.0.113.10", Port: 80}}
	v6 := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "2001:db8::10", Reality: []int{443}})
	for _, c := range []struct {
		imya    string
		klyuchi []protokol.Server
		o       Opcii
	}{
		{"без адаптера", k, Opcii{}},
		{"без ключей", nil, opcii},
		{"Id дважды", dubl, opcii},
		{"без Id", bezId, opcii},
		{"нечем испортить", chuzhoy, opcii},
		{"IPv6 без адресов выхода", v6, opcii},
	} {
		t.Run(c.imya, func(t *testing.T) {
			zapushcheno := false
			_, err := Proverit(context.Background(), c.klyuchi, c.o, func(context.Context, []byte) (func(), error) {
				zapushcheno = true
				return func() {}, nil
			})
			if err == nil || zapushcheno {
				t.Fatalf("ошибка %v, ядро запущено %v", err, zapushcheno)
			}
		})
	}
}

func TestYadroNePodnyalos(t *testing.T) {
	_, err := Proverit(context.Background(), dva(t), opcii, func(context.Context, []byte) (func(), error) {
		return nil, errors.New("файл ядра пропал")
	})
	if err == nil || !strings.Contains(err.Error(), "файл ядра пропал") {
		t.Fatal(err)
	}
}

func TestVhodNeOtkrylsya(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, vezyot, vezyot)
	p.bezVhoda = true
	_, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if err == nil || !strings.Contains(err.Error(), "не открыло вход") || p.ostanovleno != 1 {
		t.Fatalf("ошибка %v, остановок %d", err, p.ostanovleno)
	}
}

// Отмена посреди замёрзшего потока это отмена, а не итог «замёрз».
func TestOtmena(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, zamerz, vezyot)
	ctx, otm := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, otm)
	itogi, err := Proverit(ctx, k, opcii, p.zapusk)
	if !errors.Is(err, context.Canceled) || itogi != nil || p.ostanovleno != 1 {
		t.Fatalf("итоги %+v, ошибка %v, остановок %d", itogi, err, p.ostanovleno)
	}
}

// Поток идёт, но не укладывается в срок ключа: это не заморозка.
func TestMedlenneeSroka(t *testing.T) {
	bylo := srokKlyucha
	srokKlyucha = time.Second
	t.Cleanup(func() { srokKlyucha = bylo })
	k := dva(t)
	p := novayaPodstava(t, k, medlenno, vezyot)
	itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if err != nil {
		t.Fatal(err)
	}
	if it := itogi[0]; it.Vid != Zamerz || !strings.HasPrefix(it.Otkaz, "поток не уложился в срок: доехало ") {
		t.Fatalf("итог %+v", it)
	}
}

// Отмена, пока висит последняя испорченная копия, это отмена, а не мёртвый
// контроль и годная проверка.
func TestOtmenaNaKontrole(t *testing.T) {
	k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "203.0.113.10", Reality: []int{443}})
	p := novayaPodstava(t, k, visitKontrol)
	ctx, otm := context.WithCancel(context.Background())
	defer otm()
	p.priKontrole = otm
	itogi, err := Proverit(ctx, k, opcii, p.zapusk)
	if !errors.Is(err, context.Canceled) || itogi != nil {
		t.Fatalf("итоги %+v, ошибка %v, запросы %q", itogi, err, p.zaprosy)
	}
	if strings.Join(p.zaprosy, " ") != "klyuch1 kontrol1" {
		t.Fatalf("запросы %q", p.zaprosy)
	}
}

// Упавшее ядро проверки это отказ проверки, а не мёртвые ключи.
func TestYadroUpalo(t *testing.T) {
	k := dva(t)
	p := novayaPodstava(t, k, smert, vezyot)
	itogi, err := Proverit(context.Background(), k, opcii, p.zapusk)
	if err == nil || !strings.Contains(err.Error(), "перестало принимать") || itogi != nil {
		t.Fatalf("итоги %+v, ошибка %v", itogi, err)
	}
}

// Ключ на IPv6 сверяется с названными адресами выхода сервера.
func TestKlyuchNaIPv6(t *testing.T) {
	k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "2001:db8::10", Reality: []int{443}})
	p := novayaPodstava(t, k, chuzhoy)
	o := opcii
	o.Vyhody = []netip.Addr{netip.MustParseAddr("198.51.100.7")}
	itogi, err := Proverit(context.Background(), k, o, p.zapusk)
	if err != nil || itogi[0].Vid != Vezyot {
		t.Fatalf("итоги %+v, ошибка %v", itogi, err)
	}
}

func TestPustayaOstanovka(t *testing.T) {
	_, err := Proverit(context.Background(), dva(t), opcii, func(context.Context, []byte) (func(), error) { return nil, nil })
	if err == nil || strings.Contains(err.Error(), "паника") {
		t.Fatal(err)
	}
}

// Конфиг ядра проверки принимает ядро клиента.
func TestKonfigPrinimaetYadro(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX")
	}
	k := klyuchi(t, konfig.Vhody{Imya: "Мой", Adres: "2001:db8::10", Reality: []int{443, 8443}, Hy2: []int{8443}})
	var kontrol []protokol.Server
	for _, x := range k {
		x, err := isportit(x)
		if err != nil {
			t.Fatal(err)
		}
		kontrol = append(kontrol, x)
	}
	telo, err := sobrat(k, kontrol, "Ethernet", 18101, "parol")
	if err != nil {
		t.Fatal(err)
	}
	put := filepath.Join(t.TempDir(), "proverka.json")
	if err := os.WriteFile(put, telo, 0o600); err != nil {
		t.Fatal(err)
	}
	if vyvod, err := exec.Command(yadro, "check", "-c", put).CombinedOutput(); err != nil {
		t.Fatalf("ядро не приняло конфиг: %v\n%s", err, vyvod)
	}
}
