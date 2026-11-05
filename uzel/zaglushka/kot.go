package zaglushka

// Настройки кота для сцены: окрас, сложение, танцы, темп, ракурс и свет.
// Уходят в JS строкой JSON, поля совпадают с OKRAS в kot/kot.js.

type okras struct {
	Osnova     string  `json:"osnova"`
	Svetlyy    string  `json:"svetlyy"`
	Polosy     string  `json:"polosy"`
	Rozovyy    string  `json:"rozovyy"`
	Pyatno     string  `json:"pyatno"`
	Pyatno2    string  `json:"pyatno2"`
	Glaza      string  `json:"glaza"`
	Blesk      string  `json:"blesk"`
	PolosyVes  float64 `json:"polosyVes"`
	PyatnaVes  float64 `json:"pyatnaVes"`
	Pyatna2Ves float64 `json:"pyatna2Ves"`
	SvetlyyVes float64 `json:"svetlyyVes"`
	Sid        float64 `json:"sid"`
	temnyy     bool    // тёмного кота не ставим на тёмный подиум
}

// Окрасы котов из сайта-подарка. Незаданное в исходнике взято из OKRAS
func osnovaOkrasa() okras {
	return okras{
		Osnova: "#f7922f", Svetlyy: "#ffe2bd", Polosy: "#d0601d", Rozovyy: "#ffa0af", Pyatno: "#2a2522",
		Pyatno2: "#2a2522", Glaza: "#6cc23e", Blesk: "#ffd2a0", PolosyVes: 1, SvetlyyVes: 1,
	}
}

func okrasy() []okras {
	s := func(f func(*okras)) okras {
		o := osnovaOkrasa()
		f(&o)
		return o
	}
	return []okras{
		osnovaOkrasa(),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.Polosy, o.PolosyVes, o.Rozovyy, o.Glaza, o.Blesk = "#e6d8e4", "#f0e9f0", "#c9a9d4", 0.35, "#ff9ec4", "#59c6f0", "#ffe6f6"
		}),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.Polosy, o.PolosyVes, o.Rozovyy, o.Glaza = "#f3efe8", "#ffffff", "#e2dccf", 0, "#ffb0bd", "#5aa7e8"
		}),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.Polosy, o.PolosyVes, o.SvetlyyVes, o.Rozovyy, o.Glaza = "#1f1c24", "#34303c", "#1f1c24", 0, 0.5, "#c97a8a", "#f2c84b"
			o.temnyy = true
		}),
		s(func(o *okras) {
			o.Osnova, o.Pyatno, o.PyatnaVes, o.Svetlyy, o.PolosyVes, o.Glaza, o.Sid = "#e98a3c", "#26201c", 1, "#fbf4ea", 0, "#7cc24a", 3.1
		}),
		s(func(o *okras) {
			o.Osnova, o.Polosy, o.PolosyVes, o.Svetlyy, o.Glaza = "#f2c48d", "#d49a5a", 0.7, "#fff1dc", "#8a6cff"
		}),
		s(func(o *okras) {
			o.Osnova, o.Polosy, o.PolosyVes, o.Svetlyy, o.Glaza = "#8d8f96", "#6e7078", 0.3, "#d9d9dc", "#e0b04a"
		}),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.SvetlyyVes, o.PolosyVes, o.Rozovyy, o.Glaza = "#1f1d22", "#f7f5f2", 1, 0, "#ff9fb4", "#5fd28a"
			o.temnyy = true
		}),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.Polosy, o.PolosyVes, o.Glaza = "#f6b99a", "#fde6dc", "#e89c80", 0.4, "#6fb2e6"
		}),
		s(func(o *okras) {
			o.Osnova, o.Polosy, o.PolosyVes, o.Svetlyy, o.Glaza = "#8e8a86", "#4f4b48", 1, "#d8d2ca", "#9acb3c"
		}),
		s(func(o *okras) {
			o.Osnova, o.Pyatno, o.PyatnaVes, o.PolosyVes, o.Svetlyy, o.Glaza, o.Sid = "#d9b77e", "#8a5a2b", 1, 0, "#f4e6c8", "#c98a2e", 7.4
		}),
		s(func(o *okras) {
			o.Osnova, o.Svetlyy, o.Polosy, o.PolosyVes, o.Glaza = "#f3e3c3", "#fff8ea", "#e6cfa3", 0.25, "#7aa0d8"
		}),
		s(func(o *okras) {
			o.Osnova, o.Polosy, o.PolosyVes, o.Svetlyy, o.Glaza = "#b8bcc4", "#5d6470", 1, "#eef0f4", "#4fc0b0"
		}),
		s(func(o *okras) {
			o.Osnova, o.Polosy, o.PolosyVes, o.Svetlyy, o.SvetlyyVes, o.Glaza = "#ef8a32", "#d06a1e", 0.35, "#ffffff", 1, "#58b84a"
		}),
	}
}

var cvetaGlaz = []string{"#6cc23e", "#59c6f0", "#f2c84b", "#8a6cff", "#e0b04a", "#4fc0b0", "#c98a2e", "#5aa7e8"}

// Ключи TANCY из kot/tancy.js
var tancy = []string{
	"kach", "mashet", "kruzhitsya", "prygaet", "kivaet", "tryaset", "volna",
	"plechi", "shag", "hlopaet", "vazhno", "pruzhinit", "imeninnica",
}

type slozhenie struct {
	Rost      float64 `json:"rost"`
	Golova    float64 `json:"golova"`
	Tolschina float64 `json:"tolschina"`
}

type svet struct {
	Klyuch string `json:"klyuch"`
	Kontur string `json:"kontur"`
	Nebo   string `json:"nebo"`
	Zemlya string `json:"zemlya"`
}

type nastroykaKota struct {
	ID        string    `json:"id"`
	Okras     okras     `json:"okras"`
	Slozhenie slozhenie `json:"slozhenie"`
	Tancy     []string  `json:"tancy"`
	Temp      int       `json:"temp"`
	Povorot   float64   `json:"povorot"`
	Kamera    float64   `json:"kamera"`
	Zapas     float64   `json:"zapas"`
	Svet      svet      `json:"svet"`
	Pochta    string    `json:"pochta"`
	PochtaID  string    `json:"pochtaId"`
}

// vybratKota заполняет всё, что зависит только от зерна и палитры; id,
// запас кадра и почту дописывает сборка страницы
func vybratKota(k kosti, pal palitra) nastroykaKota {
	var mozhno []okras
	for _, o := range okrasy() {
		if !(o.temnyy && pal.temnaya) {
			mozhno = append(mozhno, o)
		}
	}
	ok := vybor(k, "okras", mozhno)
	if k.dolya("glaza-svoi") < 0.5 {
		ok.Glaza = vybor(k, "glaza", cvetaGlaz)
	}
	if ok.PyatnaVes > 0 {
		ok.Sid = k.mezhdu("sid", 0, 20)
	}
	povorot := k.mezhdu("povorot", 0.15, 0.5)
	if k.dolya("povorot-znak") < 0.5 {
		povorot = -povorot
	}
	nebo := pal.fon
	if pal.temnaya {
		nebo = pal.tekst
	}
	return nastroykaKota{
		Okras: ok,
		Slozhenie: slozhenie{
			Rost:      k.mezhdu("rost", 0.92, 1.08),
			Golova:    k.mezhdu("golova", 0.95, 1.15),
			Tolschina: k.mezhdu("tolschina", 0.9, 1.2),
		},
		Tancy:   neskolko(k, "tancy", tancy, 2),
		Temp:    96 + 2*k.iz("temp", 17),
		Povorot: povorot,
		Kamera:  k.mezhdu("kamera", -0.1, 0.3),
		Svet:    svet{Klyuch: pal.klyuch, Kontur: pal.akcent, Nebo: nebo, Zemlya: pal.podium},
	}
}
