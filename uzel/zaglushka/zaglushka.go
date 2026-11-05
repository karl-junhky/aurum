// Package zaglushka отдаёт маленький сайт-визитку с танцующим котом.
//
// Всё, что видно снаружи (кто владелец сайта, тексты, палитра, шрифты,
// раскладка, окрас и танец кота, имена файлов), выбирается по зерну: одно
// зерно всегда даёт тот же сайт байт в байт, разные зёрна дают разные
// сайты. Страница не делает внешних запросов, всё отдаётся отсюда.
//
// Кот собран из kot/ скриптом kot/sobrat.mjs в vshito/kot.js; настройки
// конкретного сайта подставляются в этот файл при сборке сайта.
package zaglushka

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed vshito/kot.js
var sobrannyyKot string

const metkaNastroyki = "__NASTROYKA_KOTA__"

const (
	tipHTML = "text/html; charset=utf-8"
	// Свои файлы со случайной частью в имени можно хранить долго, остальное
	// сутки
	keshDolgiy = "public, max-age=604800"
	keshDen    = "public, max-age=86400"
	// Страница берёт только своё: скрипт, стиль и значки с этого же сайта
	politika = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

type resurs struct {
	tip      string
	kesh     string
	telo     []byte
	gz       []byte // nil, если сжимать незачем
	etag     string
	etagGz   string
	stranica bool
}

type sayt struct {
	puti      map[string]*resurs
	nenaydeno *resurs
}

// Novyy возвращает обработчик всего сайта для зерна: главная страница,
// стили, скрипт кота, значки, robots.txt и своя страница 404 на любой
// неизвестный путь. Обработчик неизменяем и безопасен для одновременных
// запросов.
func Novyy(zerno []byte) http.Handler {
	return sobrat(zerno)
}

func novyyResurs(tip, kesh string, telo []byte, stranica bool) *resurs {
	r := &resurs{tip: tip, kesh: kesh, telo: telo, stranica: stranica}
	sum := sha256.Sum256(telo)
	r.etag = `"` + hex.EncodeToString(sum[:8]) + `"`
	// Картинка значка уже сжата внутри, маленькие файлы сжимать незачем
	if tip != "image/x-icon" && len(telo) > 512 {
		if gz := szhat(telo); len(gz) < len(telo) {
			r.gz = gz
			r.etagGz = `"` + hex.EncodeToString(sum[:8]) + `-gz"`
		}
	}
	return r
}

func szhat(b []byte) []byte {
	var buf bytes.Buffer
	z, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		panic("zaglushka: gzip: " + err.Error())
	}
	// Запись в bytes.Buffer не отказывает; заголовок gzip без времени и
	// имени файла, поэтому сжатое тоже повторяется байт в байт
	if _, err := z.Write(b); err != nil {
		panic("zaglushka: gzip: " + err.Error())
	}
	if err := z.Close(); err != nil {
		panic("zaglushka: gzip: " + err.Error())
	}
	return buf.Bytes()
}

// podstavitNastroyku кладёт настройки кота строкой JSON на место метки в
// собранном коде. Метка там ровно одна, это проверяет и сборка кота
func podstavitNastroyku(n nastroykaKota) []byte {
	obekt, err := json.Marshal(n)
	if err != nil {
		panic("zaglushka: настройки кота: " + err.Error())
	}
	// Строка JSON годится и как строка JS: json.Marshal экранирует кавычки,
	// обратную косую черту и < > &
	stroka, err := json.Marshal(string(obekt))
	if err != nil {
		panic("zaglushka: настройки кота: " + err.Error())
	}
	for _, k := range []string{`"`, `'`, "`"} {
		metka := k + metkaNastroyki + k
		if c := strings.Count(sobrannyyKot, metka); c == 1 {
			return []byte(strings.Replace(sobrannyyKot, metka, string(stroka), 1))
		} else if c > 1 {
			break
		}
	}
	panic("zaglushka: в vshito/kot.js метка настроек не одна или её нет, пересоберите kot/sobrat.mjs")
}

func sobrat(zerno []byte) *sayt {
	k := kosti{zerno: bytes.Clone(zerno)}
	m := vybor(k, "mir", miry)
	o := oblik{
		pal:       vybor(k, "palitra", m.palitry),
		sh:        vybor(k, "shrift", m.shrifty),
		raskladka: vybor(k, "raskladka", raskladki),
		zerkalo:   k.dolya("zerkalo") < 0.5,
		rPodium:   vybor(k, "r-podium", radiusyPodiuma),
		rKnopka:   vybor(k, "r-knopka", radiusyKnopki),
	}
	o.forma = vybor(k, "forma", formyRaskladok[o.raskladka])

	imya := vybor(k, "imya", m.imena)
	vid := vybor(k, "vid", m.vid)
	god := 2014 + k.iz("god", 10)
	gor := vybor(k, "gorod", goroda)
	podiumID := "k" + k.hex("id-podium", 5)
	pochtaID := "p" + k.hex("id-pochta", 5)

	uslugi := razdel{
		ID:        vybor(k, "id-uslugi", []string{"uslugi", "services", "chto-delaem"}),
		Zagolovok: vybor(k, "r-uslugi", m.razdely[0]),
		Uslugi:    neskolko(k, "uslugi", m.uslugi, 3+k.iz("skolko-uslug", 2)),
	}
	oNas := razdel{
		ID:        vybor(k, "id-o-nas", []string{"o-nas", "about", "kto-my"}),
		Zagolovok: vybor(k, "r-o-nas", m.razdely[1]),
		Abzacy:    []string{fmt.Sprintf(vybor(k, "o-nas", m.oNas), god)},
	}
	svyaz := razdel{
		ID:        vybor(k, "id-svyaz", []string{"kontakty", "contact", "svyaz"}),
		Zagolovok: vybor(k, "r-svyaz", m.razdely[2]),
		Abzacy:    []string{vybor(k, "kontakt", m.kontakt)},
		Pochta:    true,
	}
	razdely := []razdel{uslugi, oNas, svyaz}
	menyu := []punktMenyu{{uslugi.ID, m.menyu[0]}, {oNas.ID, m.menyu[1]}, {svyaz.ID, m.menyu[2]}}
	if k.dolya("poryadok") < 0.4 {
		razdely[0], razdely[1] = razdely[1], razdely[0]
		menyu[0], menyu[1] = menyu[1], menyu[0]
	}

	nk := vybratKota(k, o.pal)
	nk.ID = podiumID
	nk.Zapas = o.forma.zapas
	nk.Pochta = vybor(k, "pochta", m.pochta)
	nk.PochtaID = pochtaID

	g := novayaGolova(k, o.pal, nk.Okras.Glaza)
	// В шапке знак стоит на фоне страницы, в значке вкладки на любом фоне:
	// без своей подложки голова там берёт тёмный цвет палитры
	gZnachka := g
	if g.fon == "net" && o.pal.temnaya {
		gZnachka.cvetGolovy = o.pal.fon
	}

	css := []byte(o.css())
	js := podstavitNastroyku(nk)
	svgZnachka := []byte(gZnachka.svg(false, true, ""))

	// Имена файлов: папка, слово, разделитель и хвост от зерна и содержимого
	papka := vybor(k, "papka", []string{"/assets/", "/static/", "/s/", "/files/", "/"})
	razd := vybor(k, "razdelitel", []string{".", "-"})
	putCSS := papka + vybor(k, "imya-css", []string{"style", "main", "site", "styles", "app"}) + razd + k.hexSoderzhimogo("hex-css", 6+k.iz("dlina-css", 5), css) + ".css"
	putJS := papka + vybor(k, "imya-js", []string{"app", "main", "site", "bundle", "index", "script"}) + razd + k.hexSoderzhimogo("hex-js", 6+k.iz("dlina-js", 5), js) + ".js"
	putSVG := vybor(k, "imya-svg", []string{"/favicon.svg", "/icon.svg", papka + "icon" + razd + k.hexSoderzhimogo("hex-svg", 8, svgZnachka) + ".svg"})

	stranica := ispolnit(shablonStranicy, dannyeStranicy{
		Titul:        imya + " | " + vid,
		Opisanie:     vybor(k, "vvod", m.vvod),
		TemaCvet:     o.pal.fon,
		PutCSS:       putCSS,
		PutJS:        putJS,
		PutSVG:       putSVG,
		Raskladka:    o.raskladka,
		Imya:         imya,
		Vid:          vid,
		Zagolovok:    vybor(k, "zagolovok", m.zagolovki),
		Vvod:         vybor(k, "vvod", m.vvod),
		Knopka:       vybor(k, "knopka", m.knopki),
		SvyazID:      svyaz.ID,
		Menyu:        menyu,
		Znak:         template.HTML(g.svg(false, false, "")),
		Risunok:      template.HTML(g.svg(true, false, "risunok")),
		PodiumID:     podiumID,
		Podpis:       fmt.Sprintf(vybor(k, "podpis", m.podpisi), vybor(k, "kot", imenaKotov)),
		Razdely:      razdely,
		Podval:       fmt.Sprintf("© %s, since %d. %s", imya, god, strings.ReplaceAll(vybor(k, "gde", m.gde), "{г}", gor)),
		PochtaID:     pochtaID,
		PochtaPodpis: vybor(k, "pochta-podpis", podpisiPochty),
	})

	zagOshibki := vybor(k, "oshibka", oshibkaZagolovki)
	oshibka := ispolnit(shablonOshibki, dannyeOshibki{
		Titul:     zagOshibki,
		PutCSS:    putCSS,
		PutSVG:    putSVG,
		TemaCvet:  o.pal.fon,
		Znak:      template.HTML(g.svg(false, false, "")),
		Zagolovok: zagOshibki,
		Tekst:     vybor(k, "oshibka-tekst", oshibkaTeksty),
		Ssylka:    vybor(k, "oshibka-ssylka", oshibkaSsylki),
	})

	robots := vybor(k, "robots", []string{
		"User-agent: *\nDisallow:\n",
		"User-agent: *\nAllow: /\n",
		"User-agent: *\nAllow: /\n\nUser-agent: GPTBot\nDisallow: /\n",
	})

	glavnaya := novyyResurs(tipHTML, "", stranica, true)
	s := &sayt{
		puti: map[string]*resurs{
			"/":            glavnaya,
			"/index.html":  glavnaya,
			putCSS:         novyyResurs("text/css; charset=utf-8", keshDolgiy, css, false),
			putJS:          novyyResurs("text/javascript; charset=utf-8", keshDolgiy, js, false),
			putSVG:         novyyResurs("image/svg+xml", keshDen, svgZnachka, false),
			"/favicon.ico": novyyResurs("image/x-icon", keshDen, gZnachka.ico(), false),
			"/robots.txt":  novyyResurs("text/plain; charset=utf-8", keshDen, []byte(robots), false),
		},
		nenaydeno: novyyResurs(tipHTML, "", oshibka, true),
	}
	return s
}

// prinimaetGzip разбирает Accept-Encoding: явный gzip главнее звёздочки,
// q=0 значит отказ
func prinimaetGzip(znacheniya []string) bool {
	gzipQ, zvezdaQ := -1.0, -1.0
	for _, z := range znacheniya {
		for _, chast := range strings.Split(z, ",") {
			polya := strings.Split(chast, ";")
			imya := strings.ToLower(strings.TrimSpace(polya[0]))
			q := 1.0
			for _, p := range polya[1:] {
				kl, zn, est := strings.Cut(strings.TrimSpace(p), "=")
				if !est || !strings.EqualFold(strings.TrimSpace(kl), "q") {
					continue
				}
				f, err := strconv.ParseFloat(strings.TrimSpace(zn), 64)
				if err != nil {
					// Непонятный вес считаем отказом: лучше отдать без сжатия
					f = 0
				}
				q = f
			}
			switch imya {
			case "gzip", "x-gzip":
				gzipQ = q
			case "*":
				zvezdaQ = q
			}
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return zvezdaQ > 0
}

func (s *sayt) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	res, est := s.puti[r.URL.Path]
	if !est {
		res = s.nenaydeno
	}
	h := w.Header()
	h.Set("Content-Type", res.tip)
	h.Set("X-Content-Type-Options", "nosniff")
	if res.stranica {
		h.Set("Content-Security-Policy", politika)
	}
	if res.kesh != "" {
		h.Set("Cache-Control", res.kesh)
	}
	telo, etag := res.telo, res.etag
	if res.gz != nil {
		h.Add("Vary", "Accept-Encoding")
		if prinimaetGzip(r.Header.Values("Accept-Encoding")) {
			telo, etag = res.gz, res.etagGz
			h.Set("Content-Encoding", "gzip")
		}
	}
	if !est {
		h.Set("Content-Length", strconv.Itoa(len(telo)))
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			// Ошибка записи значит, что клиент ушёл; ответить ему уже нечем
			_, _ = w.Write(telo)
		}
		return
	}
	h.Set("ETag", etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(telo))
}
