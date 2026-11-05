package zaglushka

import (
	"bytes"
	"html/template"
)

type punktMenyu struct{ ID, Nazvanie string }

type razdel struct {
	ID, Zagolovok string
	Uslugi        []usluga
	Abzacy        []string
	Pochta        bool
}

type dannyeStranicy struct {
	Titul, Opisanie, TemaCvet  string
	PutCSS, PutJS, PutSVG      string
	Raskladka                  string
	Imya, Vid, Zagolovok, Vvod string
	Knopka, SvyazID            string
	Menyu                      []punktMenyu
	Znak, Risunok              template.HTML
	PodiumID, Podpis           string
	Razdely                    []razdel
	Podval                     string
	PochtaID, PochtaPodpis     string
}

var shablonStranicy = template.Must(template.New("stranica").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Titul}}</title>
<meta name="description" content="{{.Opisanie}}">
<meta name="theme-color" content="{{.TemaCvet}}">
<link rel="icon" href="/favicon.ico" sizes="any">
<link rel="icon" href="{{.PutSVG}}" type="image/svg+xml">
<link rel="stylesheet" href="{{.PutCSS}}">
<script type="module" src="{{.PutJS}}"></script>
</head>
<body>
{{- if ne .Raskladka "vizitka"}}
<header class="ramka shapka">
<a class="znak" href="/">{{.Znak}}<span><b>{{.Imya}}</b><small>{{.Vid}}</small></span></a>
<nav aria-label="Sections"><ul class="menyu">{{range .Menyu}}<li><a href="#{{.ID}}">{{.Nazvanie}}</a></li>{{end}}</ul></nav>
</header>
{{- end}}
<main>
<section class="glavnoe"><div class="ramka">
{{- if eq .Raskladka "vizitka"}}
{{template "stsena" .}}
<h1>{{.Imya}}</h1>
<p class="podzagolovok">{{.Zagolovok}}</p>
<p class="vvod">{{.Vvod}}</p>
<a class="knopka" href="#{{.SvyazID}}">{{.Knopka}}</a>
{{- else}}
<div class="tekst">
<h1>{{.Zagolovok}}</h1>
<p class="vvod">{{.Vvod}}</p>
<a class="knopka" href="#{{.SvyazID}}">{{.Knopka}}</a>
</div>
{{template "stsena" .}}
{{- end}}
</div></section>
{{- range .Razdely}}
<section class="razdel" id="{{.ID}}"><div class="ramka">
<h2>{{.Zagolovok}}</h2>
{{- if .Uslugi}}
<ul class="uslugi">
{{- range .Uslugi}}
<li><h3>{{.Nazvanie}}</h3><p>{{.Tekst}}</p></li>
{{- end}}
</ul>
{{- end}}
{{- range .Abzacy}}
<p>{{.}}</p>
{{- end}}
{{- if .Pochta}}
<p class="pochta" id="{{$.PochtaID}}" hidden>{{$.PochtaPodpis}} <a href="#{{.ID}}"></a></p>
{{- end}}
</div></section>
{{- end}}
</main>
<footer class="podval"><div class="ramka"><p>{{.Podval}}</p></div></footer>
</body>
</html>
{{define "stsena"}}<figure class="stsena">
<div class="podium" id="{{.PodiumID}}">{{.Risunok}}<canvas aria-hidden="true"></canvas></div>
<figcaption>{{.Podpis}}</figcaption>
</figure>{{end}}`))

type dannyeOshibki struct {
	Titul, PutCSS, PutSVG, TemaCvet string
	Znak                            template.HTML
	Zagolovok, Tekst, Ssylka        string
}

var shablonOshibki = template.Must(template.New("oshibka").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Titul}}</title>
<meta name="theme-color" content="{{.TemaCvet}}">
<link rel="icon" href="/favicon.ico" sizes="any">
<link rel="icon" href="{{.PutSVG}}" type="image/svg+xml">
<link rel="stylesheet" href="{{.PutCSS}}">
</head>
<body>
<main class="ramka oshibka">
{{.Znak}}
<h1>{{.Zagolovok}}</h1>
<p>{{.Tekst}}</p>
<p><a class="knopka" href="/">{{.Ssylka}}</a></p>
</main>
</body>
</html>
`))

func ispolnit(t *template.Template, d any) []byte {
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		// Шаблоны и данные свои; ошибка здесь это ошибка в коде пакета
		panic("zaglushka: шаблон " + t.Name() + ": " + err.Error())
	}
	return b.Bytes()
}
