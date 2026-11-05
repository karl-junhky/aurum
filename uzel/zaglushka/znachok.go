package zaglushka

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// Значок сайта: голова кота из эллипса и двух ушей-треугольников в поле
// 64×64. Одна геометрия рисуется и в SVG (знак в шапке, favicon.svg,
// запасной рисунок на подиуме), и в PNG внутри favicon.ico.

type tochka struct{ x, y float64 }

type golova struct {
	rx, ry      float64
	ushi        float64 // высота ушей над макушкой
	naklon      float64 // насколько уши разведены наружу
	fon         string  // krug, kvadrat или net
	cvetFona    string
	cvetGolovy  string
	cvetGlaz    string
	cvetNosa    string
	masshtab    float64
	centr       tochka
	levoeUho    [3]tochka
	pravoeUho   [3]tochka
	glazaCentry [2]tochka
}

const kraiGolovy = 64.0

func novayaGolova(k kosti, pal palitra, glaza string) golova {
	g := golova{
		rx:         20 + 4*k.dolya("znak/rx"),
		ry:         16 + 3*k.dolya("znak/ry"),
		ushi:       11 + 6*k.dolya("znak/ushi"),
		naklon:     -1 + 5*k.dolya("znak/naklon"),
		fon:        vybor(k, "znak/fon", []string{"krug", "kvadrat", "net"}),
		cvetFona:   pal.podium,
		cvetGolovy: pal.tekst,
		cvetGlaz:   glaza,
		cvetNosa:   pal.akcent,
		masshtab:   1,
	}
	if g.fon != "net" {
		// С фоном голова мельче, иначе кончики ушей вылезают за круг
		g.masshtab = 0.74
	}
	// Голова стоит так, чтобы вместе с ушами быть по центру поля
	verh := g.ry + g.ushi
	g.centr = tochka{32, 32 + (verh-g.ry)/2}
	c := g.centr
	l := [3]tochka{
		{c.x - g.rx*0.95, c.y - g.ry*0.25},
		{c.x - g.rx*0.72 - g.naklon, c.y - g.ry - g.ushi},
		{c.x - g.rx*0.12, c.y - g.ry*0.92},
	}
	for i, t := range l {
		g.levoeUho[i] = t
		g.pravoeUho[i] = tochka{2*c.x - t.x, t.y}
	}
	g.glazaCentry = [2]tochka{{c.x - g.rx*0.42, c.y - 1}, {c.x + g.rx*0.42, c.y - 1}}
	return g
}

// sdvig переводит точку головы в поле значка с учётом масштаба
func (g golova) sdvig(t tochka) tochka {
	const cx, cy = 32, 32
	return tochka{cx + (t.x-cx)*g.masshtab, cy + (t.y-cy)*g.masshtab}
}

func chislo(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func (g golova) treugolnik(t [3]tochka) string {
	var b strings.Builder
	for i, p := range t {
		p = g.sdvig(p)
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(chislo(p.x) + "," + chislo(p.y))
	}
	return b.String()
}

// svg: polnyy добавляет глаза и нос (рисунок на подиуме); svoy ставит
// xmlns для отдельного файла
func (g golova) svg(polnyy, svoy bool, klass string) string {
	var b strings.Builder
	b.WriteString(`<svg`)
	if svoy {
		b.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
	}
	if klass != "" {
		b.WriteString(` class="` + klass + `"`)
	}
	b.WriteString(` viewBox="0 0 64 64"`)
	if !svoy {
		b.WriteString(` aria-hidden="true" focusable="false"`)
	}
	b.WriteString(`>`)
	if !polnyy {
		switch g.fon {
		case "krug":
			b.WriteString(`<circle cx="32" cy="32" r="32" fill="` + g.cvetFona + `"/>`)
		case "kvadrat":
			b.WriteString(`<rect width="64" height="64" rx="14" fill="` + g.cvetFona + `"/>`)
		}
	}
	m := g
	if polnyy {
		m.masshtab = 1
	}
	c := m.sdvig(m.centr)
	fmt.Fprintf(&b, `<g fill="%s"><ellipse cx="%s" cy="%s" rx="%s" ry="%s"/><polygon points="%s"/><polygon points="%s"/></g>`,
		m.cvetGolovy, chislo(c.x), chislo(c.y), chislo(m.rx*m.masshtab), chislo(m.ry*m.masshtab),
		m.treugolnik(m.levoeUho), m.treugolnik(m.pravoeUho))
	if polnyy {
		for _, gl := range m.glazaCentry {
			fmt.Fprintf(&b, `<ellipse cx="%s" cy="%s" rx="2.6" ry="3.4" fill="%s"/>`, chislo(gl.x), chislo(gl.y), m.cvetGlaz)
		}
		n := tochka{c.x, c.y + 4}
		fmt.Fprintf(&b, `<polygon points="%s,%s %s,%s %s,%s" fill="%s"/>`,
			chislo(n.x-2.4), chislo(n.y), chislo(n.x+2.4), chislo(n.y), chislo(n.x), chislo(n.y+2.4), m.cvetNosa)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// --- растр для favicon.ico ---

func vTreugolnike(p tochka, t [3]tochka) bool {
	s := func(a, b, c tochka) float64 { return (a.x-c.x)*(b.y-c.y) - (b.x-c.x)*(a.y-c.y) }
	d1, d2, d3 := s(p, t[0], t[1]), s(p, t[1], t[2]), s(p, t[2], t[0])
	otr := d1 < 0 || d2 < 0 || d3 < 0
	pol := d1 > 0 || d2 > 0 || d3 > 0
	return !(otr && pol)
}

func (g golova) vGolove(p tochka) bool {
	// Обратный сдвиг: точку поля переводим в координаты головы
	const cx, cy = 32, 32
	q := tochka{cx + (p.x-cx)/g.masshtab, cy + (p.y-cy)/g.masshtab}
	dx, dy := (q.x-g.centr.x)/g.rx, (q.y-g.centr.y)/g.ry
	return dx*dx+dy*dy <= 1 || vTreugolnike(q, g.levoeUho) || vTreugolnike(q, g.pravoeUho)
}

func (g golova) vFone(p tochka) bool {
	switch g.fon {
	case "krug":
		dx, dy := p.x-32, p.y-32
		return dx*dx+dy*dy <= 32*32
	case "kvadrat":
		const r = 14.0
		x := math.Max(math.Abs(p.x-32)-(32-r), 0)
		y := math.Max(math.Abs(p.y-32)-(32-r), 0)
		return x*x+y*y <= r*r
	}
	return false
}

func razobratCvet(s string) color.NRGBA {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil || len(s) != 7 {
		// Цвета берутся только из своих таблиц; кривой цвет это ошибка в коде
		panic("zaglushka: плохой цвет " + s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// png рисует значок размером n×n со сглаживанием 4×4 подточки на пиксель
func (g golova) png(n int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	fon, kot := razobratCvet(g.cvetFona), razobratCvet(g.cvetGolovy)
	const pod = 4
	shag := kraiGolovy / float64(n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var af, ak float64
			for sy := 0; sy < pod; sy++ {
				for sx := 0; sx < pod; sx++ {
					p := tochka{(float64(x) + (float64(sx)+0.5)/pod) * shag, (float64(y) + (float64(sy)+0.5)/pod) * shag}
					if g.vFone(p) {
						af++
					}
					if g.vGolove(p) {
						ak++
					}
				}
			}
			af /= pod * pod
			ak /= pod * pod
			a := ak + af*(1-ak)
			if a == 0 {
				continue
			}
			sm := func(k, f uint8) uint8 {
				return uint8(math.Round((float64(k)*ak + float64(f)*af*(1-ak)) / a))
			}
			img.SetNRGBA(x, y, color.NRGBA{sm(kot.R, fon.R), sm(kot.G, fon.G), sm(kot.B, fon.B), uint8(math.Round(a * 255))})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		// В память png.Encode падает только на неверном изображении
		panic("zaglushka: значок не закодирован: " + err.Error())
	}
	return b.Bytes()
}

// ico собирает favicon.ico из PNG-картинок (так умеют все браузеры с Vista)
func (g golova) ico() []byte {
	razmery := []int{16, 32, 48}
	kartinki := make([][]byte, len(razmery))
	for i, n := range razmery {
		kartinki[i] = g.png(n)
	}
	var b bytes.Buffer
	zapis := func(v any) {
		// bytes.Buffer не возвращает ошибок записи
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	zapis(uint16(0))
	zapis(uint16(1))
	zapis(uint16(len(razmery)))
	smeshchenie := 6 + 16*len(razmery)
	for i, n := range razmery {
		b.WriteByte(byte(n))
		b.WriteByte(byte(n))
		b.WriteByte(0) // палитры нет
		b.WriteByte(0)
		zapis(uint16(1))  // плоскостей
		zapis(uint16(32)) // бит на точку
		zapis(uint32(len(kartinki[i])))
		zapis(uint32(smeshchenie))
		smeshchenie += len(kartinki[i])
	}
	for _, k := range kartinki {
		b.Write(k)
	}
	return b.Bytes()
}
