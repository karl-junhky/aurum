package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// sNovoyProgrammoy: свой сервер с программой 0.6.0 и ключом управления, а
// сборка ставит программу novaya.
func sNovoyProgrammoy(t *testing.T, novaya string) (*Sluzhba, string, []string) {
	t.Helper()
	s := sPustymNaborom(t)
	s.vshityyUzel = func() (vypuskUzla, error) {
		return vypuskUzla{Versiya: novaya, Sha256: strings.Repeat("a", 64)}, nil
	}
	id, ids := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader([]byte("vpn-uzel"))}, nil
	}
	return s, id, ids
}

func obnovlenieNeZhdali(t *testing.T) func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
	return func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		t.Error("обновление не ждали")
		return upravlenie.ObnovlenieItog{}, errors.New("не ждали")
	}
}

func TestSvoyVpsObnovit(t *testing.T) {
	s, id, ids := sNovoyProgrammoy(t, "0.7.0")
	// hy2 из сети человека не проходил: его ключ обновление не проверяет.
	// Прошлая проверка не прошла: удачное обновление тоже визит и снимает её.
	if err := s.otmetitProverku(id, func(x *SvoyServer) {
		x.NeIzSeti, x.Proveren, x.ProverkaOtkaz = []string{"hy2"}, chasyProverki.Add(-time.Hour), "сервер молчал"
	}); err != nil {
		t.Fatal(err)
	}
	s.seychas = func() time.Time { return chasyProverki }
	prog := &programmaDlyaTesta{Reader: bytes.NewReader([]byte("vpn-uzel"))}
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) { return prog, nil }
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	var o upravlenie.Obnovlenie
	s.vpsObnovit = func(_ context.Context, x upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		o = x
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagPerezapusk})
		return upravlenie.ObnovlenieItog{Vhod: upravlenie.Svedeniya{Put: "через VPN"}, Bylo: "0.6.0", Versiya: "0.7.0"}, nil
	}
	if otv := s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Vid != vidObnovlenie || h.Faza != fazaGotovo || h.Server != id || h.Put != "через VPN" ||
		h.Shag != string(upravlenie.ShagPerezapusk) || h.Obnovlenie == nil ||
		!reflect.DeepEqual(*h.Obnovlenie, ObnovlenieVps{Bylo: "0.6.0", Versiya: "0.7.0"}) || len(h.Preduprezhdeniya) != 0 {
		t.Fatalf("обновление: %+v %+v", h, h.Obnovlenie)
	}
	if o.Versiya != "0.7.0" || o.Programma != io.ReadSeeker(prog) || !prog.zakryta || o.Zapusk == nil {
		t.Fatalf("задание: версия %q, программа своя %v, закрыта %v", o.Versiya, o.Programma == io.ReadSeeker(prog), prog.zakryta)
	}
	if o.Vhod.Adres != adresSvoego || o.Vhod.Port != 2222 || o.Vhod.Vhod.Podpis == nil ||
		!bytes.Equal(o.Vhod.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) {
		t.Fatalf("вход: %+v", o.Vhod)
	}
	if len(o.Klyuchi) != 1 || o.Klyuchi[0].Transport == "hy2" || o.Klyuchi[0].Id != ids[0] || o.Proverka.Adapter != "Ethernet" {
		t.Fatalf("ключи для проверки: %+v, адаптер %q", o.Klyuchi, o.Proverka.Adapter)
	}
	if z := zapisPoId(t, s, id); z.Versiya != "0.7.0" || !z.Proveren.Equal(chasyProverki) || z.ProverkaOtkaz != "" {
		t.Fatalf("запись после обновления: версия %q, визит %v, отказ проверки %q", z.Versiya, z.Proveren, z.ProverkaOtkaz)
	}
}

// Ни один вид ключей не проходил из сети: проверять после обновления нечем,
// и о новой программе судит она сама, без адаптера.
func TestSvoyVpsObnovitBezKlyuchey(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.7.0")
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.NeIzSeti = []string{"reality-tcp", "hy2"} }); err != nil {
		t.Fatal(err)
	}
	var o upravlenie.Obnovlenie
	s.vpsObnovit = func(_ context.Context, x upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		o = x
		return upravlenie.ObnovlenieItog{Bylo: "0.6.0", Versiya: "0.7.0"}, nil
	}
	s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo {
		t.Fatalf("обновление: %+v", h)
	}
	if len(o.Klyuchi) != 0 || o.Proverka.Adapter != "" {
		t.Fatalf("ключи для проверки: %+v, адаптер %q", o.Klyuchi, o.Proverka.Adapter)
	}
}

// Новее на сервере нет: команда отказывает сразу, на сервер не ходит.
func TestSvoyVpsObnovitNeNovee(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.5.9")
	s.vpsObnovit = obnovlenieNeZhdali(t)
	otv := s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	if kodOtkaza(otv) != protokol.KodTeloNegodno || !strings.Contains(otv.Oshib.Tekst, "уже программа 0.6.0") {
		t.Fatalf("ответ: %+v", otv.Oshib)
	}
}

// Запись уже знает эту версию (проверка застала неподтверждённую), а
// повтор обновления всё равно идёт: довести его решает сервер.
func TestSvoyVpsObnovitTaZhe(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.6.0")
	poshlo := false
	s.vpsObnovit = func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		poshlo = true
		return upravlenie.ObnovlenieItog{Bylo: "0.6.0", Versiya: "0.6.0"}, nil
	}
	s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || !poshlo {
		t.Fatalf("обновление: %+v, пошло %v", h, poshlo)
	}
}

func TestSvoyVpsObnovitOtkaz(t *testing.T) {
	for _, c := range []struct {
		imya  string
		err   error
		tekst []string
	}{
		{"ключи не везут", &upravlenie.NeObnovilsya{Prichina: upravlenie.ErrKlyuchiNeVezut},
			[]string{"программа сервера не обновилась: с ней ни один ключ не провёз из твоей сети", "Прежняя программа возвращена"}},
		{"не поднялась, прежняя не вернулась", &upravlenie.NeObnovilsya{
			Prichina: &upravlenie.NePodnyalas{Posledneye: errors.New("ядро не отвечает")}, Vozvrat: errors.New("обрыв")},
			[]string{"новая не поднялась за 2 минуты (ядро не отвечает)", "сервер вернёт её сам через 10 минут"}},
		{"замена не встала", &upravlenie.NeObnovilsya{Prichina: upravlenie.ErrZamenaNeVstala},
			[]string{"новая так и не встала на место"}},
		{"подтверждение не дошло", &upravlenie.NePodtverzhdeno{Err: errors.New("обрыв")},
			[]string{"подтверждение до сервера не дошло", "Повтори обновление"}},
		{"откатился сам", upravlenie.ErrOtkatilsyaSam, []string{"вернул прежнюю программу раньше"}},
		{"скрипт обновления", &server.Oshibka{Shag: "obnovlenie", Kod: 1, Tekst: "новая программа не приняла состояние"},
			[]string{"программа сервера не обновилась: новая программа не приняла состояние", "На сервере работает прежняя"}},
	} {
		t.Run(c.imya, func(t *testing.T) {
			s, id, _ := sNovoyProgrammoy(t, "0.7.0")
			s.vpsObnovit = func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
				return upravlenie.ObnovlenieItog{}, c.err
			}
			s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
			h := dozhdatsyaVps(t, s)
			if h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsObnovlenie || h.Obnovlenie != nil {
				t.Fatalf("отказ: %+v %+v", h, h.Otkaz)
			}
			for _, tk := range c.tekst {
				if !strings.Contains(h.Otkaz.Tekst, tk) {
					t.Fatalf("в тексте нет %q: %s", tk, h.Otkaz.Tekst)
				}
			}
			if v := zapisPoId(t, s, id).Versiya; v != "0.6.0" {
				t.Fatalf("версия в записи после отказа: %q", v)
			}
		})
	}
}

func TestSvoyVpsSpisokNovayaVersiya(t *testing.T) {
	for _, c := range []struct{ vshitaya, zhdali string }{{"0.7.0", "0.7.0"}, {"0.6.0", ""}} {
		s, _, _ := sNovoyProgrammoy(t, c.vshitaya)
		otv := s.svoyVpsSpisok(kadrVps(t, "svoyVpsSpisok", map[string]string{}))
		if otv.Oshib != nil {
			t.Fatal(otv.Oshib)
		}
		var telo struct {
			Servery []struct {
				NovayaVersiya string `json:"novaya_versiya"`
			} `json:"servery"`
		}
		if err := json.Unmarshal(otv.Telo, &telo); err != nil {
			t.Fatal(err)
		}
		if len(telo.Servery) != 1 || telo.Servery[0].NovayaVersiya != c.zhdali {
			t.Fatalf("сборка %s: %+v", c.vshitaya, telo)
		}
	}
}

// Та же программа уже стояла: сервер не проверялся, и прошлый отказ
// проверки с отметкой визита остаются.
func TestSvoyVpsObnovitUzhe(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.7.0")
	if err := s.otmetitProverku(id, func(x *SvoyServer) {
		x.Proveren, x.ProverkaOtkaz = chasyProverki.Add(-time.Hour), "программа сервера не ответила"
	}); err != nil {
		t.Fatal(err)
	}
	bylo := zapisPoId(t, s, id)
	s.vpsObnovit = func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		return upravlenie.ObnovlenieItog{Bylo: "0.7.0", Versiya: "0.7.0", Uzhe: true}, nil
	}
	s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || h.Obnovlenie == nil || !h.Obnovlenie.Uzhe {
		t.Fatalf("обновление: %+v", h)
	}
	if z := zapisPoId(t, s, id); z.Versiya != "0.7.0" || !z.Proveren.Equal(bylo.Proveren) || z.ProverkaOtkaz != bylo.ProverkaOtkaz {
		t.Fatalf("запись: версия %q, визит %v, отказ %q", z.Versiya, z.Proveren, z.ProverkaOtkaz)
	}
}

// Вид, что провозил до обновления, а после него нет, попадает в запись и в
// итог: обновление подтверждено другим видом.
func TestSvoyVpsObnovitVidPerestal(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.7.0")
	s.vpsObnovit = func(context.Context, upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		return upravlenie.ObnovlenieItog{Bylo: "0.6.0", Versiya: "0.7.0", Proverki: []proverka.Itog{
			{Server: protokol.Server{Transport: "reality-tcp"}, Vid: proverka.Vezyot},
			{Server: protokol.Server{Transport: "hy2"}, Vid: proverka.NetOtveta, Otkaz: "за 10 секунд ни байта"},
		}}, nil
	}
	s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Obnovlenie == nil || !slices.Equal(h.Obnovlenie.NeProshli, []string{"hy2"}) {
		t.Fatalf("обновление: %+v %+v", h, h.Obnovlenie)
	}
	z := zapisPoId(t, s, id)
	if !slices.Equal(z.NeIzSeti, []string{"hy2"}) || z.SetPoVidam != nil ||
		z.SetTekst != "после обновления программы сервера ключ hysteria2 не провёз проверку из твоей сети: за 10 секунд ни байта" {
		t.Fatalf("запись: %q %q %v", z.NeIzSeti, z.SetTekst, z.SetPoVidam)
	}
}

func TestDobavitNeIzSeti(t *testing.T) {
	// Был один вид с общей причиной: она становится его своей.
	x := SvoyServer{NeIzSeti: []string{"reality-tcp"}, SetTekst: "порт 443 закрыт у хостера"}
	dobavitNeIzSeti(&x, map[string]string{"hy2": "hy2 не провёз"})
	if !slices.Equal(x.NeIzSeti, []string{"reality-tcp", "hy2"}) || x.SetPoVidam["reality-tcp"] != "порт 443 закрыт у хостера" ||
		x.SetPoVidam["hy2"] != "hy2 не провёз" || x.SetTekst != "порт 443 закрыт у хостера. Hy2 не провёз" {
		t.Fatalf("%+v", x)
	}
	y := SvoyServer{}
	dobavitNeIzSeti(&y, nil)
	if y.NeIzSeti != nil || y.SetTekst != "" || y.SetPoVidam != nil {
		t.Fatalf("пустое добавление тронуло запись: %+v", y)
	}
}

// Отменённое обновление, чья прежняя программа сразу не вернулась, говорит
// об этом предупреждением: причины у отмены нет.
func TestSvoyVpsObnovitOtmenaBezVozvrata(t *testing.T) {
	s, id, _ := sNovoyProgrammoy(t, "0.7.0")
	otmena := kadrVps(t, "svoyVpsOtmenit", map[string]string{})
	s.vpsObnovit = func(ctx context.Context, _ upravlenie.Obnovlenie) (upravlenie.ObnovlenieItog, error) {
		go s.svoyVpsOtmenit(otmena)
		<-ctx.Done()
		return upravlenie.ObnovlenieItog{}, &upravlenie.NeObnovilsya{Prichina: ctx.Err(), Vozvrat: errors.New("обрыв")}
	}
	s.svoyVpsObnovit(kadrVps(t, "svoyVpsObnovit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaOtmeneno || len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Kod != protokol.KodVpsObnovlenie ||
		!strings.Contains(h.Preduprezhdeniya[0].Tekst, "сервер вернёт её сам через 10 минут") {
		t.Fatalf("отмена: %+v", h)
	}
}

func TestTrevogaStarayaProgramma(t *testing.T) {
	n := Nabor{SvoiServery: []SvoyServer{
		{Id: "staraya", Imya: "A", Prodlenie: "net", Versiya: "0.5.0"},
		{Id: "staraya-bez-klyucha", Imya: "B", Prodlenie: "net", Versiya: "0.5.0"},
		{Id: "svezhaya", Imya: "C", Prodlenie: "net", Versiya: bezopasnayaVersiyaUzla},
		{Id: "ne-znaem", Imya: "D", Prodlenie: "net"},
	}}
	tr := trevogiSvoih(sertySvoih(n, func(id string) bool { return id != "staraya-bez-klyucha" }), chasyProverki)
	if len(tr) != 1 || tr[0].Id != "staraya" || tr[0].Imya != "A" ||
		!strings.HasPrefix(tr[0].Tekst, "на сервере программа 0.5.0 с уязвимостью") {
		t.Fatalf("тревоги: %+v", tr)
	}
	// Карточка берёт первую тревогу сервера: вышедший срок сертификата
	// впереди старой программы.
	n = Nabor{SvoiServery: []SvoyServer{{Id: "oba", Imya: "A", Prodlenie: "80", Versiya: "0.5.0",
		SertDo: chasyProverki.Add(-time.Minute).Unix(), SertProveren: chasyProverki.Add(-time.Hour)}}}
	tr = trevogiSvoih(sertySvoih(n, func(string) bool { return true }), chasyProverki)
	if len(tr) != 2 || !strings.HasPrefix(tr[0].Tekst, "срок сертификата вышел") || !strings.Contains(tr[1].Tekst, "с уязвимостью") {
		t.Fatalf("порядок тревог: %+v", tr)
	}
}
