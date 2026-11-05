package zaglushka

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func zerno(i int) []byte { return []byte("zerno-" + strconv.Itoa(i)) }

type otvet struct {
	kod       int
	zagolovki http.Header
	telo      []byte
}

func zapros(t *testing.T, h http.Handler, metod, put string, zag map[string]string) otvet {
	t.Helper()
	r := httptest.NewRequest(metod, put, nil)
	for k, v := range zag {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return otvet{w.Code, w.Result().Header, w.Body.Bytes()}
}

// puti сайта в постоянном порядке
func putiSayta(s *sayt) []string {
	var p []string
	for k := range s.puti {
		p = append(p, k)
	}
	sort.Strings(p)
	return p
}

// unikalnye: каждый ресурс один раз (главная живёт под двумя путями) и 404
func unikalnye(s *sayt) []*resurs {
	vidno := map[*resurs]bool{}
	var out []*resurs
	for _, p := range putiSayta(s) {
		if r := s.puti[p]; !vidno[r] {
			vidno[r] = true
			out = append(out, r)
		}
	}
	return append(out, s.nenaydeno)
}

func TestOdnoZernoOdinSayt(t *testing.T) {
	for i := range 20 {
		a, b := sobrat(zerno(i)), sobrat(zerno(i))
		pa, pb := putiSayta(a), putiSayta(b)
		if strings.Join(pa, " ") != strings.Join(pb, " ") {
			t.Fatalf("зерно %d: разные пути %v и %v", i, pa, pb)
		}
		for _, put := range append(pa, "/net-takogo") {
			for _, zag := range []map[string]string{nil, {"Accept-Encoding": "gzip"}} {
				x := zapros(t, a, http.MethodGet, put, zag)
				y := zapros(t, b, http.MethodGet, put, zag)
				if x.kod != y.kod || !bytes.Equal(x.telo, y.telo) || x.zagolovki.Get("ETag") != y.zagolovki.Get("ETag") {
					t.Fatalf("зерно %d, %s, %v: ответы двух сборок различаются", i, put, zag)
				}
			}
		}
	}
	// Тот же ответ и через публичный конструктор
	x := zapros(t, Novyy([]byte("x")), http.MethodGet, "/", nil)
	y := zapros(t, Novyy([]byte("x")), http.MethodGet, "/", nil)
	if !bytes.Equal(x.telo, y.telo) {
		t.Fatal("Novyy с одним зерном отдал разные страницы")
	}
}

// Страницы разных зёрен различаются и без случайных хвостов в именах файлов
// и id: тексты, раскладка или значок другие
func TestRaznyeZernaRaznyeSayty(t *testing.T) {
	const n = 400
	html := map[string]int{}
	// Только хвосты имён файлов и id; цвета палитры остаются в сравнении
	sluchaynoe := regexp.MustCompile(`[.-][0-9a-f]{6,10}\.(?:css|js|svg)|id="[kp][0-9a-f]{5}"`)
	for i := range n {
		stranica := sluchaynoe.ReplaceAllString(string(sobrat(zerno(i)).puti["/"].telo), "")
		if j, est := html[stranica]; est {
			t.Fatalf("зёрна %d и %d дали одинаковую страницу", j, i)
		}
		html[stranica] = i
	}
}

// Сколько различимых вариантов на деле: считается по выборам без сборки
// сайта, поэтому зёрен можно взять много
func TestRaznoobrazie(t *testing.T) {
	const n = 5000
	oblikov, imen, kotov, tekstov := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	miryV, raskladkiV, palitryV := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range n {
		k := kosti{zerno: zerno(i)}
		m := vybor(k, "mir", miry)
		pal := vybor(k, "palitra", m.palitry)
		sh := vybor(k, "shrift", m.shrifty)
		r := vybor(k, "raskladka", raskladki)
		f := vybor(k, "forma", formyRaskladok[r])
		zerk := r == rDvoyka && k.dolya("zerkalo") < 0.5
		oblikov[fmt.Sprint(m.kod, pal.kod, sh.kod, r, f.css, zerk)] = true
		imen[m.kod+vybor(k, "imya", m.imena)] = true
		tekstov[fmt.Sprint(vybor(k, "imya", m.imena), vybor(k, "zagolovok", m.zagolovki), vybor(k, "vvod", m.vvod))] = true
		nk := vybratKota(k, pal)
		kotov[fmt.Sprint(nk.Okras, nk.Tancy)] = true
		miryV[m.kod], raskladkiV[r], palitryV[pal.kod] = true, true, true
	}
	t.Logf("на %d зёрнах: обликов (мир, палитра, шрифт, раскладка, форма) %d, названий %d, связок «название, заголовок, вступление» %d, котов (окрас и пара танцев) %d",
		n, len(oblikov), len(imen), len(tekstov), len(kotov))
	if len(miryV) != len(miry) || len(raskladkiV) != len(raskladki) || len(palitryV) != 12 {
		t.Fatalf("выпали не все варианты: миров %d, раскладок %d, палитр %d", len(miryV), len(raskladkiV), len(palitryV))
	}
	if len(oblikov) < 350 {
		t.Fatalf("обликов всего %d, ожидалось больше 350", len(oblikov))
	}
}

func findPut(s *sayt, okonchanie string) string {
	for _, p := range putiSayta(s) {
		if strings.HasSuffix(p, okonchanie) {
			return p
		}
	}
	return ""
}

// Тот же список, что в kot/sobrat.mjs
var zapretnye = []string{
	"affory", "vpn", "впн", "proxy", "прокси", "reality", "sing-box", "singbox", "xray", "vless",
	"vmess", "hysteria", "trojan", "shadowsocks", "tunnel", "туннел", "обход блок",
}

// kirillica возвращает кусок вокруг первой русской буквы или пусто
func kirillica(b []byte) string {
	i := bytes.IndexFunc(b, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) })
	if i < 0 {
		return ""
	}
	return string(b[max(0, i-30):min(len(b), i+30)])
}

func zapretnoe(b []byte) string {
	niz := bytes.ToLower(b)
	for _, s := range zapretnye {
		if bytes.Contains(niz, []byte(s)) {
			return s
		}
	}
	return ""
}

func TestNetZapretnyhSlov(t *testing.T) {
	for i := range 300 {
		s := sobrat(zerno(i))
		for _, p := range putiSayta(s) {
			if m := zapretnoe([]byte(p)); m != "" {
				t.Fatalf("зерно %d: в имени файла %s слово %q", i, p, m)
			}
		}
		for _, r := range unikalnye(s) {
			if r.tip == "image/x-icon" {
				// Внутри PNG сжатые байты, а не текст
				continue
			}
			if m := zapretnoe(r.telo); m != "" {
				t.Fatalf("зерно %d: в ресурсе %s слово %q", i, r.tip, m)
			}
			// Сайт европейский: русский текст где угодно, даже в сообщении
			// об ошибке внутри кода, выдал бы, откуда он
			if m := kirillica(r.telo); m != "" {
				t.Fatalf("зерно %d: в ресурсе %s кириллица: %q", i, r.tip, m)
			}
		}
	}
}

func TestVesSayta(t *testing.T) {
	const potolok = 1_500_000
	naibolshiy := 0
	for i := range 200 {
		ves := 0
		for _, r := range unikalnye(sobrat(zerno(i))) {
			ves += len(r.telo)
		}
		naibolshiy = max(naibolshiy, ves)
		if ves > potolok {
			t.Fatalf("зерно %d: сайт весит %d байт, потолок %d", i, ves, potolok)
		}
	}
	t.Logf("самый тяжёлый сайт из 200: %d байт", naibolshiy)
}

func TestNeizvestnyyPut404(t *testing.T) {
	h := Novyy([]byte("404"))
	for _, put := range []string{"/net-takogo", "/admin", "/.env", "/assets/", "/index.htm", "/favicon.ico/", "/robots.txt.bak", "/%2e%2e/etc/passwd"} {
		o := zapros(t, h, http.MethodGet, put, nil)
		if o.kod != http.StatusNotFound {
			t.Fatalf("%s: код %d, ожидался 404", put, o.kod)
		}
		if !strings.HasPrefix(o.zagolovki.Get("Content-Type"), "text/html") || !bytes.Contains(o.telo, []byte("<h1>")) {
			t.Fatalf("%s: 404 без своей страницы: %q", put, o.telo)
		}
		hd := zapros(t, h, http.MethodHead, put, nil)
		if hd.kod != http.StatusNotFound || len(hd.telo) != 0 {
			t.Fatalf("%s HEAD: код %d, тело %d байт", put, hd.kod, len(hd.telo))
		}
	}
	o := zapros(t, h, http.MethodPost, "/", nil)
	if o.kod != http.StatusMethodNotAllowed || o.zagolovki.Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST: код %d, Allow %q", o.kod, o.zagolovki.Get("Allow"))
	}
}

func TestRobotsIZnachki(t *testing.T) {
	for i := range 30 {
		s := sobrat(zerno(i))
		o := zapros(t, s, http.MethodGet, "/robots.txt", nil)
		if o.kod != 200 || !strings.HasPrefix(o.zagolovki.Get("Content-Type"), "text/plain") || !bytes.HasPrefix(o.telo, []byte("User-agent: *\n")) {
			t.Fatalf("зерно %d: robots.txt %d %q", i, o.kod, o.telo)
		}

		o = zapros(t, s, http.MethodGet, "/favicon.ico", nil)
		if o.kod != 200 || o.zagolovki.Get("Content-Type") != "image/x-icon" {
			t.Fatalf("зерно %d: favicon.ico %d %s", i, o.kod, o.zagolovki.Get("Content-Type"))
		}
		proveritIco(t, o.telo)

		svg := findPut(s, ".svg")
		o = zapros(t, s, http.MethodGet, svg, nil)
		if o.kod != 200 || o.zagolovki.Get("Content-Type") != "image/svg+xml" || !bytes.HasPrefix(o.telo, []byte("<svg ")) {
			t.Fatalf("зерно %d: %s %d %q", i, svg, o.kod, o.telo)
		}
	}
}

func proveritIco(t *testing.T, b []byte) {
	t.Helper()
	if len(b) < 6 || binary.LittleEndian.Uint16(b[0:]) != 0 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		t.Fatalf("favicon.ico: не ICO")
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	for i := range n {
		z := b[6+16*i:]
		razmer := int(z[0])
		dlina, sdvig := binary.LittleEndian.Uint32(z[8:]), binary.LittleEndian.Uint32(z[12:])
		img, err := png.Decode(bytes.NewReader(b[sdvig : sdvig+dlina]))
		if err != nil {
			t.Fatalf("favicon.ico, картинка %d: %v", i, err)
		}
		if img.Bounds().Dx() != razmer || img.Bounds().Dy() != razmer {
			t.Fatalf("favicon.ico: в оглавлении %d, в картинке %v", razmer, img.Bounds())
		}
	}
}

var (
	ssylkaHTML = regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	idHTML     = regexp.MustCompile(`id="([^"]*)"`)
	adresVnesh = regexp.MustCompile(`(?i)https?://|//[a-z0-9]|url\(`)
)

// Только свои ссылки: каждая ведёт на файл этого сайта или якорь страницы,
// внешних адресов нет ни в разметке, ни в стилях, ни в коде
func TestTolkoSvoiResursy(t *testing.T) {
	for i := range 100 {
		s := sobrat(zerno(i))
		for _, stranica := range [][]byte{s.puti["/"].telo, s.nenaydeno.telo} {
			ids := map[string]bool{}
			for _, m := range idHTML.FindAllSubmatch(stranica, -1) {
				ids[string(m[1])] = true
			}
			for _, m := range ssylkaHTML.FindAllSubmatch(stranica, -1) {
				a := string(m[1])
				switch {
				case strings.HasPrefix(a, "#"):
					if !ids[a[1:]] {
						t.Fatalf("зерно %d: якорь %s без раздела", i, a)
					}
				case strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "//"):
					if _, est := s.puti[a]; !est {
						t.Fatalf("зерно %d: ссылка %s ведёт в 404", i, a)
					}
				default:
					t.Fatalf("зерно %d: чужая ссылка %q", i, a)
				}
			}
			if m := adresVnesh.Find(stranica); m != nil {
				t.Fatalf("зерно %d: внешний адрес в разметке: %q", i, m)
			}
		}
		if m := adresVnesh.Find(s.puti[findPut(s, ".css")].telo); m != nil {
			t.Fatalf("зерно %d: внешний адрес в стилях: %q", i, m)
		}
		svg := bytes.Replace(s.puti[findPut(s, ".svg")].telo, []byte(`xmlns="http://www.w3.org/2000/svg"`), nil, 1)
		if m := adresVnesh.Find(svg); m != nil {
			t.Fatalf("зерно %d: внешний адрес в значке: %q", i, m)
		}
		js := bytes.ReplaceAll(s.puti[findPut(s, ".js")].telo, []byte("http://www.w3.org/1999/xhtml"), nil)
		if m := regexp.MustCompile(`https?://`).Find(js); m != nil {
			t.Fatalf("зерно %d: внешний адрес в коде", i)
		}
	}
}

// Настройки кота подставлены, и сцена найдёт на странице свой подиум
func TestNastroykaKota(t *testing.T) {
	for i := range 50 {
		s := sobrat(zerno(i))
		js := s.puti[findPut(s, ".js")].telo
		if bytes.Contains(js, []byte(metkaNastroyki)) {
			t.Fatalf("зерно %d: метка настроек осталась в коде", i)
		}
		m := regexp.MustCompile(`id="(k[0-9a-f]{5})"`).FindSubmatch(s.puti["/"].telo)
		if m == nil {
			t.Fatalf("зерно %d: на странице нет подиума", i)
		}
		if !bytes.Contains(js, []byte(`{\"id\":\"`+string(m[1])+`\"`)) {
			t.Fatalf("зерно %d: в настройках кота не тот подиум %s", i, m[1])
		}
	}
}

func TestSzhatie(t *testing.T) {
	s := sobrat([]byte("gzip"))
	for _, put := range []string{"/", findPut(s, ".css"), findPut(s, ".js"), "/net-takogo"} {
		prosto := zapros(t, s, http.MethodGet, put, nil)
		szhato := zapros(t, s, http.MethodGet, put, map[string]string{"Accept-Encoding": "gzip, deflate, br"})
		if szhato.zagolovki.Get("Content-Encoding") != "gzip" || szhato.zagolovki.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%s: сжатие не включилось: %v", put, szhato.zagolovki)
		}
		zr, err := gzip.NewReader(bytes.NewReader(szhato.telo))
		if err != nil {
			t.Fatalf("%s: %v", put, err)
		}
		raspak, err := io.ReadAll(zr)
		if err != nil || !bytes.Equal(raspak, prosto.telo) {
			t.Fatalf("%s: распакованное не совпало с несжатым (%v)", put, err)
		}
		if prosto.zagolovki.Get("Content-Encoding") != "" {
			t.Fatalf("%s: сжато без Accept-Encoding", put)
		}
	}
	sluchai := map[string]bool{
		"gzip": true, "GZIP;q=0.5": true, "br, gzip;q=0": false, "*": true, "gzip;q=0, *": false,
		"br": false, "": false, "identity": false, "gzip;q=abc": false, "deflate, *;q=0.1": true,
	}
	for z, nado := range sluchai {
		if est := prinimaetGzip([]string{z}); est != nado {
			t.Fatalf("Accept-Encoding %q: %v, ожидалось %v", z, est, nado)
		}
	}
}

func TestPovtornyyZaprosPoETag(t *testing.T) {
	s := sobrat([]byte("etag"))
	o := zapros(t, s, http.MethodGet, "/", nil)
	etag := o.zagolovki.Get("ETag")
	if etag == "" {
		t.Fatal("нет ETag")
	}
	if p := zapros(t, s, http.MethodGet, "/", map[string]string{"If-None-Match": etag}); p.kod != http.StatusNotModified {
		t.Fatalf("If-None-Match: код %d", p.kod)
	}
	if o.zagolovki.Get("Content-Security-Policy") == "" {
		t.Fatal("у страницы нет политики безопасности")
	}
}

// Контраст текста по WCAG 2: не ниже 4.5:1 везде, где на цвете лежит текст
func TestKontrastPalitr(t *testing.T) {
	yarkost := func(s string) float64 {
		c := razobratCvet(s)
		lin := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
	}
	kontrast := func(a, b string) float64 {
		x, y := yarkost(a), yarkost(b)
		return (max(x, y) + 0.05) / (min(x, y) + 0.05)
	}
	vse := map[string]palitra{}
	for _, m := range miry {
		for _, p := range m.palitry {
			vse[p.kod] = p
		}
	}
	for _, p := range vse {
		for _, para := range [][3]string{
			{"текст на фоне", p.tekst, p.fon},
			{"второстепенный на фоне", p.tusk, p.fon},
			{"кнопка", p.naAkcente, p.akcent},
			{"текст и знак на подиуме и полосе", p.tekst, p.podium},
		} {
			if k := kontrast(para[1], para[2]); k < 4.5 {
				t.Errorf("%s: %s %.2f:1", p.kod, para[0], k)
			}
		}
	}
}
