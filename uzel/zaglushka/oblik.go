package zaglushka

import (
	"strings"
)

// Облик: палитры, шрифты, раскладки и форма подиума, из них собирается CSS.
// Кот единственное яркое место страницы, всё вокруг тихое.

type palitra struct {
	kod                                 string
	fon, tekst, tusk, akcent, naAkcente string
	podium, pol, liniya                 string
	klyuch                              string // ключевой свет на кота: тёплый или холодный
	temnaya                             bool
}

// Контраст проверен на глаз и по формуле WCAG: tekst и tusk на fon и на
// podium, naAkcente на akcent не ниже 4.5:1
var (
	palNochnoyMontazh = palitra{"nochnoy-montazh", "#1c2541", "#f2eee8", "#b7bcd0", "#ffb4a2", "#1c2541", "#2b3a67", "#3e5291", "#34406a", "#e8eeff", true}
	palRaskadrovka    = palitra{"raskadrovka", "#e9eef5", "#172033", "#4a556b", "#2f4fd8", "#ffffff", "#ffd9c7", "#ffc0a6", "#c9d3e2", "#fff1e2", false}
	palShalfey        = palitra{"shalfey", "#e3e9de", "#1e2a21", "#4c5a4f", "#2e6a4e", "#ffffff", "#f3d6a4", "#e8be79", "#c5d0c0", "#fff1e2", false}
	palGuash          = palitra{"guash", "#f8ecef", "#2a1c2c", "#5e4b5f", "#7a2e8c", "#ffffff", "#bfe2d5", "#93cdb9", "#e7d3d9", "#fff1e2", false}
	palPodsolnukh     = palitra{"podsolnukh", "#fff3c4", "#22305e", "#4c5680", "#c72c63", "#ffffff", "#8ccbf5", "#5fb2ee", "#f0de9a", "#fff4e6", false}
	palMyata          = palitra{"myata", "#ddf4ea", "#17314a", "#42596e", "#6447d1", "#ffffff", "#ffc9de", "#ffa9c9", "#bfe3d3", "#fff1e2", false}
	palLavanda        = palitra{"lavanda", "#eeeaf7", "#28233f", "#565071", "#4f43a8", "#ffffff", "#d6efe2", "#b5e2cb", "#d9d2ec", "#fff4ea", false}
	palSumerki        = palitra{"sumerki", "#2a2634", "#f0ecf6", "#bdb5cc", "#c2b3ff", "#2a2634", "#3a344b", "#514868", "#3f3950", "#eef0ff", true}
	palSliva          = palitra{"sliva", "#2a1a3e", "#fbefd9", "#cdbfd9", "#ffd45e", "#2a1a3e", "#43305f", "#5e4485", "#3d2b57", "#fff0dc", true}
	palArkada         = palitra{"arkada", "#0e3a39", "#e9f6f1", "#a9cfc6", "#ff92b5", "#0e3a39", "#15524f", "#1f6d69", "#1b4c4a", "#eefaff", true}
	palPryazha        = palitra{"pryazha", "#f6e3e1", "#3a2226", "#6a4d51", "#973055", "#ffffff", "#f3cb66", "#edb63e", "#ebcfcb", "#fff1e2", false}
	palLen            = palitra{"len", "#dde6ee", "#2b3440", "#536070", "#276f6c", "#ffffff", "#f1dcc4", "#e8c6a0", "#c6d2dd", "#fff1e2", false}
)

// Только системные шрифты: своих файлов шрифтов сайт не тянет, а характер
// задают начертание, насыщенность, ширина и плотность строк
type shrift struct {
	kod                string
	telo, zagolovok    string
	ves                string
	razryadka, shirina string
	vysota, strochka   string
}

const (
	stekGrotesk = `system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif`
	stekSerif   = `"Iowan Old Style","Palatino Linotype",Palatino,"Book Antiqua",Georgia,"Noto Serif",serif`
	stekUzkiy   = `Bahnschrift,"DIN Alternate","Roboto Condensed","Arial Narrow",sans-serif`
	stekKruglyy = `Nunito,"Varela Round","Trebuchet MS","Segoe UI",system-ui,sans-serif`
	stekMono    = `"Cascadia Mono","SF Mono",ui-monospace,"JetBrains Mono","Roboto Mono",Consolas,monospace`
)

var (
	shGrotesk = shrift{"grotesk", stekGrotesk, stekGrotesk, "800", "-0.03em", "100%", "1.02", "1.6"}
	shSerif   = shrift{"serif", stekSerif, stekSerif, "500", "-0.01em", "100%", "1.08", "1.7"}
	shUzkiy   = shrift{"uzkiy", stekGrotesk, stekUzkiy, "700", "0", "75%", "1", "1.6"}
	shKruglyy = shrift{"kruglyy", stekKruglyy, stekKruglyy, "800", "-0.01em", "100%", "1.06", "1.65"}
	shMono    = shrift{"mono", stekGrotesk, stekMono, "700", "-0.04em", "100%", "1.05", "1.6"}
)

// Раскладки главного экрана
const (
	rDvoyka  = "dvoyka"  // текст и подиум рядом
	rAfisha  = "afisha"  // заголовок по центру, широкий подиум под ним
	rPolosa  = "polosa"  // цветная полоса во всю ширину, подиум на ней
	rVizitka = "vizitka" // узкая колонка как визитная карточка, имя крупно
)

var raskladki = []string{rDvoyka, rAfisha, rPolosa, rVizitka}

// Форма подиума и запас кадра: в круге ноги кота ближе к краю, его надо
// показывать мельче
type forma struct {
	kod   string
	zapas float64
	css   string
}

var (
	fKrug    = forma{"krug", 1.32, `.podium{border-radius:50%;aspect-ratio:1}`}
	fArka    = forma{"arka", 1.1, `.podium{border-radius:999px 999px var(--r-podium) var(--r-podium)}`}
	fOkno    = forma{"okno", 1.08, `.podium{border-radius:var(--r-podium)}`}
	fKaplya1 = forma{"kaplya", 1.25, `.podium{border-radius:46% 54% 50% 50%/55% 52% 48% 45%;aspect-ratio:1}`}
	fKaplya2 = forma{"kaplya", 1.25, `.podium{border-radius:58% 42% 47% 53%/45% 55% 45% 55%;aspect-ratio:1}`}
)

var formyRaskladok = map[string][]forma{
	rDvoyka:  {fArka, fOkno, fKaplya1, fKrug},
	rAfisha:  {fOkno},
	rPolosa:  {fArka, fKrug, fKaplya2},
	rVizitka: {fKrug, fKaplya1, fKaplya2, fArka},
}

var (
	radiusyPodiuma = []string{"1rem", "1.75rem", "2.75rem"}
	radiusyKnopki  = []string{"999px", ".55rem", ".2rem"}
)

type oblik struct {
	pal       palitra
	sh        shrift
	raskladka string
	forma     forma
	zerkalo   bool // в двойке подиум слева
	rPodium   string
	rKnopka   string
}

const cssOsnova = `:root{--fon:{fon};--tekst:{tekst};--tusk:{tusk};--akcent:{akcent};--na-akcente:{naAkcente};--podium:{podium};--pol:{pol};--liniya:{liniya};--shrift:{telo};--shrift-z:{zagolovok};--z-ves:{ves};--z-razryadka:{razryadka};--z-shirina:{shirina};--z-vysota:{vysota};--strochka:{strochka};--r-podium:{rPodium};--r-knopka:{rKnopka};color-scheme:{shema}}
*,*::before,*::after{box-sizing:border-box}
html{-webkit-text-size-adjust:100%}
body{margin:0;background:var(--fon);color:var(--tekst);font-family:var(--shrift);font-size:1.0625rem;line-height:var(--strochka)}
svg,canvas{display:block}
a{color:inherit;text-decoration-thickness:1px;text-underline-offset:.2em}
a:hover{text-decoration-thickness:2px}
:focus-visible{outline:3px solid var(--akcent);outline-offset:3px}
.ramka{width:100%;max-width:70rem;margin:0 auto;padding:0 1rem}
@media(min-width:48rem){.ramka{padding:0 2rem}}
.shapka{display:flex;align-items:center;justify-content:space-between;gap:1rem;padding-top:1.25rem;padding-bottom:1.25rem}
.znak{display:flex;align-items:center;gap:.7rem;text-decoration:none;line-height:1.2}
.znak svg{width:2.5rem;height:2.5rem;flex:none}
.znak b{display:block;font-family:var(--shrift-z);font-weight:var(--z-ves);font-stretch:var(--z-shirina);letter-spacing:var(--z-razryadka);font-size:1.15rem}
.znak small{display:block;color:var(--tusk);font-size:.85rem}
.menyu{display:flex;gap:1.5rem;list-style:none;margin:0;padding:0}
.menyu a{text-decoration:none;color:var(--tusk)}
.menyu a:hover{color:var(--tekst);text-decoration:underline}
@media(max-width:44rem){.menyu{display:none}}
h1,h2{font-family:var(--shrift-z);font-weight:var(--z-ves);font-stretch:var(--z-shirina);letter-spacing:var(--z-razryadka);line-height:var(--z-vysota);text-wrap:balance}
h1{font-size:clamp(2.25rem,3.8vw + 1.2rem,4.25rem);margin:0 0 1.25rem;max-width:15ch}
h2{font-size:clamp(1.5rem,1.1vw + 1.15rem,2.1rem);margin:0 0 1.25rem}
h3{font-size:1.125rem;line-height:1.3;margin:0 0 .3rem}
p{margin:0 0 1rem;max-width:62ch}
.vvod{font-size:clamp(1.125rem,.4vw + 1rem,1.3rem);max-width:36ch;margin-bottom:1.75rem}
.knopka{display:inline-block;background:var(--akcent);color:var(--na-akcente);font-weight:700;text-decoration:none;padding:.85rem 1.4rem;border-radius:var(--r-knopka);line-height:1.2}
.knopka:hover{filter:brightness(1.08)}
.stsena{margin:0}
.podium{position:relative;overflow:hidden;isolation:isolate;width:100%;aspect-ratio:4/5;background:var(--podium)}
.podium::before{content:"";position:absolute;left:18%;right:18%;bottom:5%;height:16%;z-index:-1;background:radial-gradient(closest-side,var(--pol),transparent)}
.podium canvas{position:absolute;inset:0;width:100%;height:100%;opacity:0;transition:opacity .5s ease}
.podium.zhivoy canvas{opacity:1}
.risunok{position:absolute;left:50%;bottom:16%;width:50%;transform:translateX(-50%)}
.podium.zhivoy .risunok{display:none}
.stsena figcaption{margin-top:.85rem;color:var(--tusk);font-size:.95rem;max-width:38ch}
.razdel{padding:3.5rem 0;border-top:1px solid var(--liniya)}
.uslugi{list-style:none;margin:0;padding:0;display:grid;gap:1.75rem 2.5rem;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr))}
.uslugi li{position:relative;padding-left:1.25rem}
.uslugi li::before{content:"";position:absolute;left:0;top:.5em;width:.55rem;height:.55rem;border-radius:50%;background:var(--akcent)}
.uslugi p{margin:0;color:var(--tusk)}
.pochta a{font-weight:700}
.podval{border-top:1px solid var(--liniya);padding:1.75rem 0 2.5rem;color:var(--tusk);font-size:.9rem}
.podval p{margin:0}
.oshibka{padding-top:5rem;padding-bottom:6rem}
.oshibka svg{width:5rem;height:5rem;margin-bottom:1.5rem}
@media(prefers-reduced-motion:reduce){.podium canvas{transition:none}}
`

var cssRaskladok = map[string]string{
	rDvoyka: `.glavnoe .ramka{display:grid;gap:2.5rem;align-items:center;padding-top:1.5rem;padding-bottom:4rem}
@media(min-width:56rem){.glavnoe .ramka{grid-template-columns:minmax(0,1.1fr) minmax(0,.9fr);gap:4.5rem}}
`,
	rAfisha: `.glavnoe .ramka{text-align:center;padding-top:2rem;padding-bottom:4rem}
.glavnoe h1{max-width:20ch;margin-inline:auto;font-size:clamp(2.4rem,3.6vw + 1rem,4rem)}
.glavnoe .vvod,.stsena figcaption{margin-inline:auto}
.glavnoe .stsena{margin-top:2.25rem}
.podium{aspect-ratio:16/9}
@media(max-width:40rem){.podium{aspect-ratio:4/5}}
`,
	rPolosa: `.glavnoe{background:var(--podium)}
.glavnoe .ramka{display:grid;gap:2.5rem;align-items:center;padding-top:3rem;padding-bottom:3rem}
@media(min-width:56rem){.glavnoe .ramka{grid-template-columns:minmax(0,1.15fr) minmax(0,.85fr);gap:4rem}.glavnoe .stsena{order:-1}}
.podium{aspect-ratio:1;background:var(--pol)}
.podium::before{background:radial-gradient(closest-side,var(--podium),transparent)}
.glavnoe .stsena figcaption{color:var(--tekst)}
.glavnoe+.razdel{border-top:0}
`,
	rVizitka: `.ramka{max-width:40rem}
.glavnoe .ramka{text-align:center;padding-top:3rem;padding-bottom:3.5rem}
.glavnoe .stsena{width:min(20rem,78vw);margin:0 auto 2.25rem}
.podium{aspect-ratio:1}
.glavnoe h1{margin-inline:auto;max-width:none;font-size:clamp(2.25rem,3vw + 1.4rem,3.5rem);margin-bottom:.5rem}
.podzagolovok{font-size:1.25rem;margin-inline:auto;max-width:30ch}
.glavnoe .vvod,.stsena figcaption{margin-inline:auto}
.uslugi{grid-template-columns:1fr}
`,
}

// Подиум слева в двойке
const cssZerkalo = `@media(min-width:56rem){.glavnoe .ramka{grid-template-columns:minmax(0,.9fr) minmax(0,1.1fr)}.glavnoe .stsena{order:-1}}
`

func (o oblik) css() string {
	shema := "light"
	if o.pal.temnaya {
		shema = "dark"
	}
	r := strings.NewReplacer(
		"{fon}", o.pal.fon, "{tekst}", o.pal.tekst, "{tusk}", o.pal.tusk, "{akcent}", o.pal.akcent,
		"{naAkcente}", o.pal.naAkcente, "{podium}", o.pal.podium, "{pol}", o.pal.pol, "{liniya}", o.pal.liniya,
		"{telo}", o.sh.telo, "{zagolovok}", o.sh.zagolovok, "{ves}", o.sh.ves, "{razryadka}", o.sh.razryadka,
		"{shirina}", o.sh.shirina, "{vysota}", o.sh.vysota, "{strochka}", o.sh.strochka,
		"{rPodium}", o.rPodium, "{rKnopka}", o.rKnopka, "{shema}", shema,
	)
	var b strings.Builder
	b.WriteString(r.Replace(cssOsnova))
	// Форма до раскладки: раскладка может переопределить пропорции подиума
	b.WriteString(o.forma.css)
	b.WriteString("\n")
	b.WriteString(cssRaskladok[o.raskladka])
	if o.zerkalo && o.raskladka == rDvoyka {
		b.WriteString(cssZerkalo)
	}
	return b.String()
}
