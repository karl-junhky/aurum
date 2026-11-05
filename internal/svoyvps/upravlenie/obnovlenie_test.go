package upravlenie

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// stendObnovleniya подменяет вход и шаги обновления на сервере. Входы
// выдаются по очереди из vhody, кончились - сервер молчит. Шаги пишутся
// с именем входа.
type stendObnovleniya struct {
	nash      gossh.Signer
	chuzhaya  bool
	vhody     []*svyaz
	vydano    int
	zaprosy   []ssh.Zapros
	shagi     []string
	zamena    server.Zamena
	errZameny error
	// naMeste и uzel отвечают по номеру вызова, последний ответ повторяется.
	naMeste    []string
	errNaMeste error
	uzel       []server.SostoyanieUzla
	errUzla    []error
	proverki   []proverka.Itog
	errProv    error
	podtv      string
	errPodtv   error
	// errOtkata отвечает по номеру вызова, последний ответ повторяется.
	errOtkata []error
	ctxOtkata error
	hody      []Shag
	// sdvig двигает часы seychas: так проверка видит, сколько прошло от замены.
	sdvig        time.Duration
	srokProverki time.Duration
	posleZameny  func()
}

func (st *stendObnovleniya) podmenit(t *testing.T) {
	t.Helper()
	bP, bZ, bN, bS, bPv, bPd, bO, bOs := podklyuchit, zamenitProgrammu, programmaNaMeste, sostoyanie, proverit, podtverditObnovlenie, otkatitObnovlenie, osmotret
	bSrok, bShag, bVoz, bCh := srokPodyoma, shagPodyoma, srokVozvrata, seychas
	t.Cleanup(func() {
		podklyuchit, zamenitProgrammu, programmaNaMeste, sostoyanie, proverit, podtverditObnovlenie, otkatitObnovlenie, osmotret = bP, bZ, bN, bS, bPv, bPd, bO, bOs
		srokPodyoma, shagPodyoma, srokVozvrata, seychas = bSrok, bShag, bVoz, bCh
	})
	srokPodyoma, shagPodyoma, srokVozvrata = 200*time.Millisecond, time.Millisecond, 300*time.Millisecond
	seychas = func() time.Time { return time.Now().Add(st.sdvig) }
	st.nash = klyuchUpravleniya(t)
	imya := func(m server.Mashina) string {
		if s, ok := m.(*svyaz); ok {
			return s.imya
		}
		return "?"
	}
	podklyuchit = func(_ context.Context, z ssh.Zapros) (Svyaz, error) {
		st.zaprosy = append(st.zaprosy, z)
		if z.Put.Tunnel || st.vydano >= len(st.vhody) {
			return nil, otkaz(ssh.PrichinaTishina)
		}
		st.vydano++
		return st.vhody[st.vydano-1], nil
	}
	osmotret = func(_ context.Context, m server.Mashina) (*server.Osmotr, error) {
		st.shagi = append(st.shagi, "osmotr "+imya(m))
		o := chistyyOsmotr()
		k := st.nash
		if st.chuzhaya {
			k = klyuchUpravleniya(t)
		}
		o.Otmetka = &server.Sostav{Sostoyanie: "gotovo", Klyuchi: []string{base64.StdEncoding.EncodeToString(k.PublicKey().Marshal())}}
		return o, nil
	}
	zamenitProgrammu = func(_ context.Context, m server.Mashina, _ io.ReadSeeker) (server.Zamena, error) {
		st.shagi = append(st.shagi, "zamenit "+imya(m))
		if st.posleZameny != nil {
			st.posleZameny()
		}
		return st.zamena, st.errZameny
	}
	nNaMeste, nUzla := 0, 0
	programmaNaMeste = func(_ context.Context, m server.Mashina) (server.ProgrammaNaMeste, error) {
		st.shagi = append(st.shagi, "na-meste "+imya(m))
		if st.errNaMeste != nil {
			return server.ProgrammaNaMeste{}, st.errNaMeste
		}
		v := st.naMeste[min(nNaMeste, len(st.naMeste)-1)]
		nNaMeste++
		return server.ProgrammaNaMeste{Sha256: "x", Versiya: v}, nil
	}
	sostoyanie = func(context.Context, server.Mashina) (server.SostoyanieUzla, error) {
		i := min(nUzla, len(st.uzel)-1)
		nUzla++
		if i < len(st.errUzla) && st.errUzla[i] != nil {
			return server.SostoyanieUzla{}, st.errUzla[i]
		}
		return st.uzel[i], nil
	}
	proverit = func(ctx context.Context, _ []protokol.Server, o proverka.Opcii, _ proverka.Zapusk) ([]proverka.Itog, error) {
		st.shagi = append(st.shagi, "proverit")
		if len(o.Vyhody) == 0 {
			t.Error("проверке не дали адресов выхода")
		}
		if dl, ok := ctx.Deadline(); ok {
			st.srokProverki = time.Until(dl)
		}
		// Настоящая проверка с вышедшим сроком ничего не провозит.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return st.proverki, st.errProv
	}
	podtverditObnovlenie = func(_ context.Context, m server.Mashina) (string, error) {
		st.shagi = append(st.shagi, "podtverdit "+imya(m))
		return st.podtv, st.errPodtv
	}
	nOtkata := 0
	otkatitObnovlenie = func(ctx context.Context, m server.Mashina) (string, error) {
		st.shagi = append(st.shagi, "otkatit "+imya(m))
		st.ctxOtkata = ctx.Err()
		var err error
		if len(st.errOtkata) > 0 {
			err = st.errOtkata[min(nOtkata, len(st.errOtkata)-1)]
		}
		nOtkata++
		return "vpn-uzel 0.6.0, sing-box 1.14.2", err
	}
}

func (st *stendObnovleniya) obnovlenie(klyuchi bool) Obnovlenie {
	o := Obnovlenie{
		Puti:      puti(false),
		Vhod:      ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: st.nash}},
		Programma: strings.NewReader("programma"),
		Versiya:   "0.7.0",
		Zapusk:    func(context.Context, []byte) (func(), error) { return func() {}, nil },
		Hod:       func(h Hod) { st.hody = append(st.hody, h.Shag) },
	}
	if klyuchi {
		o.Klyuchi = []protokol.Server{{Id: "k-r", Transport: "reality-tcp"}, {Id: "k-h", Transport: "hy2"}}
	}
	return o
}

const (
	uzel060 = "vpn-uzel 0.6.0, sing-box 1.14.2"
	uzel070 = "vpn-uzel 0.7.0, sing-box 1.14.2"
)

// novyyStend: замена встала, после неё сервер отвечает новой программой с
// живым ядром, первый ключ провозит, подтверждение видит новую.
func novyyStend(t *testing.T) (*stendObnovleniya, *svyaz, *svyaz) {
	t.Helper()
	pervyy, vtoroy := &svyaz{imya: "первый", sv: Svedeniya{Root: true, Put: "напрямую"}}, &svyaz{imya: "второй"}
	st := &stendObnovleniya{
		vhody:    []*svyaz{pervyy, vtoroy},
		zamena:   server.Zamena{Bylo: server.ProgrammaNaMeste{Versiya: uzel060}, Versiya: uzel070},
		naMeste:  []string{uzel070},
		uzel:     []server.SostoyanieUzla{{}, {Versiya: uzel060, YadroZhivo: true}, {Versiya: uzel070, YadroZhivo: true}},
		errUzla:  []error{errors.New("пульт не открыт")},
		proverki: []proverka.Itog{{Server: protokol.Server{Transport: "reality-tcp"}, Vid: proverka.Vezyot}, {Server: protokol.Server{Transport: "hy2"}, Vid: proverka.NetOtveta}},
		podtv:    uzel070,
	}
	st.podmenit(t)
	return st, pervyy, vtoroy
}

func TestObnovit(t *testing.T) {
	ctx := context.Background()

	t.Run("встала, поднялась, ключ провёз, подтверждена", func(t *testing.T) {
		st, pervyy, vtoroy := novyyStend(t)
		it, err := Obnovit(ctx, st.obnovlenie(true))
		if err != nil {
			t.Fatal(err)
		}
		if it.Bylo != "0.6.0" || it.Versiya != "0.7.0" || it.Uzhe || len(it.Proverki) != 2 || it.Vhod.Put != "напрямую" {
			t.Errorf("итог %+v", it)
		}
		zhdali := []string{"osmotr первый", "zamenit первый", "na-meste второй", "na-meste второй", "na-meste второй", "proverit", "podtverdit второй"}
		if !slices.Equal(st.shagi, zhdali) {
			t.Errorf("шаги %q", st.shagi)
		}
		if pervyy.zakryt != 1 || vtoroy.zakryt != 1 {
			t.Errorf("закрыты: первый %d, второй %d", pervyy.zakryt, vtoroy.zakryt)
		}
		// Перезапуск идёт без этого входа: туннель через этот же сервер годится.
		if !slices.ContainsFunc(st.zaprosy, func(z ssh.Zapros) bool { return z.BezPerezapuska }) {
			t.Error("вход без BezPerezapuska")
		}
		if !slices.Contains(st.hody, ShagPerezapusk) || !slices.Contains(st.hody, ShagProverka) {
			t.Errorf("ход %q", st.hody)
		}
	})

	t.Run("та же программа уже стоит", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.zamena = server.Zamena{Bylo: server.ProgrammaNaMeste{Versiya: uzel070}, Uzhe: true, Versiya: uzel070}
		it, err := Obnovit(ctx, st.obnovlenie(true))
		if err != nil || !it.Uzhe || it.Versiya != "0.7.0" || st.vydano != 1 {
			t.Errorf("итог %+v, %v, входов %d", it, err, st.vydano)
		}
	})

	t.Run("чужая отметка: замены нет", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.chuzhaya = true
		if _, err := Obnovit(ctx, st.obnovlenie(true)); !errors.Is(err, ErrChuzhayaUstanovka) || slices.Contains(st.shagi, "zamenit первый") {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("отказ замены: возвращать нечего", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.errZameny = &server.Oshibka{Shag: "obnovlenie", Kod: 1, Tekst: "новая программа не принимает пакет сервера"}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var o *server.Oshibka
		if !errors.As(err, &o) || slices.ContainsFunc(st.shagi, func(s string) bool { return strings.HasPrefix(s, "otkatit") }) {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("ключи не везут: прежняя возвращена тем же входом", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.proverki = []proverka.Itog{{Vid: proverka.NetOtveta}, {Vid: proverka.NetOtveta}}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || !errors.Is(err, ErrKlyuchiNeVezut) || no.Vozvrat != nil {
			t.Fatalf("%v", err)
		}
		if st.shagi[len(st.shagi)-1] != "otkatit второй" || slices.ContainsFunc(st.shagi, func(s string) bool { return strings.HasPrefix(s, "podtverdit") }) {
			t.Errorf("шаги %q", st.shagi)
		}
	})

	// Перезапуск прежней рвёт вход через туннель того же сервера: ответ
	// отката теряется, а сам откат прошёл.
	t.Run("ответ отката потерян, новый вход видит прежнюю", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.vhody = append(st.vhody, &svyaz{imya: "третий"})
		st.proverki = []proverka.Itog{{Vid: proverka.NetOtveta}, {Vid: proverka.NetOtveta}}
		st.errOtkata = []error{otkaz(ssh.PrichinaTishina)}
		st.naMeste = []string{uzel070, uzel070, uzel070, uzel060}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || !errors.Is(err, ErrKlyuchiNeVezut) || no.Vozvrat != nil {
			t.Fatalf("%v", err)
		}
		if !slices.Contains(st.shagi, "na-meste третий") || slices.Contains(st.shagi, "otkatit третий") {
			t.Errorf("шаги %q", st.shagi)
		}
	})

	t.Run("откат не дошёл: новый вход повторяет его", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.vhody = append(st.vhody, &svyaz{imya: "третий"})
		st.proverki = []proverka.Itog{{Vid: proverka.NetOtveta}, {Vid: proverka.NetOtveta}}
		st.errOtkata = []error{otkaz(ssh.PrichinaTishina), nil}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || no.Vozvrat != nil || st.shagi[len(st.shagi)-1] != "otkatit третий" {
			t.Fatalf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("отказ скрипта отката не повторяется", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.vhody = append(st.vhody, &svyaz{imya: "третий"})
		st.proverki = []proverka.Itog{{Vid: proverka.NetOtveta}, {Vid: proverka.NetOtveta}}
		st.errOtkata = []error{&server.Oshibka{Shag: "obnovlenie", Kod: 1, Tekst: "таймер отката не снялся"}}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || no.Vozvrat == nil || st.vydano != 2 {
			t.Fatalf("%v, входов %d", err, st.vydano)
		}
	})

	t.Run("проверка ключей укладывается в таймер отката", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.posleZameny = func() { st.sdvig = 8 * time.Minute }
		if _, err := Obnovit(ctx, st.obnovlenie(true)); err != nil {
			t.Fatal(err)
		}
		if ostalos := server.SrokOtkata - zapasPodtverzhdeniya - 8*time.Minute; st.srokProverki > ostalos || st.srokProverki < ostalos-5*time.Second {
			t.Errorf("срок проверки %v, от таймера оставалось %v", st.srokProverki, ostalos)
		}
	})

	t.Run("от таймера ничего не осталось: прежняя возвращена", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.posleZameny = func() { st.sdvig = server.SrokOtkata }
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || !errors.Is(err, ErrProverkaDolgo) || st.shagi[len(st.shagi)-1] != "otkatit второй" {
			t.Fatalf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("без ключей судит программа сервера", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		if _, err := Obnovit(ctx, st.obnovlenie(false)); err != nil || slices.Contains(st.shagi, "proverit") {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("ядро не поднялось за срок: прежняя возвращена", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.uzel, st.errUzla = []server.SostoyanieUzla{{Versiya: uzel070, Oshibka: "порт занят"}}, nil
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var np *NePodnyalas
		if !errors.As(err, &np) || !strings.Contains(err.Error(), "порт занят") || st.shagi[len(st.shagi)-1] != "otkatit второй" {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("обрыв на замене, а новая встала: дело идёт дальше", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.errZameny = &server.ObryvZameny{Err: errors.New("сброс")}
		it, err := Obnovit(ctx, st.obnovlenie(true))
		if err != nil || it.Versiya != "0.7.0" || it.Bylo != "0.6.0" {
			t.Errorf("итог %+v, %v", it, err)
		}
	})

	t.Run("обрыв на замене, а на месте прежняя: таймер снимается откатом", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.errZameny = &server.ObryvZameny{Err: errors.New("сброс")}
		st.naMeste = []string{uzel060}
		_, err := Obnovit(ctx, st.obnovlenie(true))
		if !errors.Is(err, ErrZamenaNeVstala) || st.shagi[len(st.shagi)-1] != "otkatit второй" {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})

	t.Run("отмена посреди ожидания всё равно возвращает прежнюю", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		c, otmena := context.WithCancel(ctx)
		st.posleZameny = otmena
		_, err := Obnovit(c, st.obnovlenie(true))
		var no *NeObnovilsya
		if !errors.As(err, &no) || !errors.Is(err, context.Canceled) || st.ctxOtkata != nil {
			t.Errorf("%v, ctx отката %v", err, st.ctxOtkata)
		}
		// Вход после замены ещё не открывался: откат входит сам.
		if st.shagi[len(st.shagi)-1] != "otkatit второй" {
			t.Errorf("шаги %q", st.shagi)
		}
	})

	t.Run("сервер молчит после замены: прежнюю вернёт таймер", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.vhody = st.vhody[:1]
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var no *NeObnovilsya
		var np *NePodnyalas
		if !errors.As(err, &no) || !errors.As(err, &np) || no.Vozvrat == nil {
			t.Errorf("%v", err)
		}
	})

	t.Run("подтверждение опоздало", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.podtv = uzel060
		if _, err := Obnovit(ctx, st.obnovlenie(true)); !errors.Is(err, ErrOtkatilsyaSam) {
			t.Errorf("%v", err)
		}
	})

	t.Run("подтверждение не дошло: отката нет, его сделает таймер", func(t *testing.T) {
		st, _, _ := novyyStend(t)
		st.errPodtv = errors.New("обрыв")
		_, err := Obnovit(ctx, st.obnovlenie(true))
		var np *NePodtverzhdeno
		if !errors.As(err, &np) || slices.ContainsFunc(st.shagi, func(s string) bool { return strings.HasPrefix(s, "otkatit") }) {
			t.Errorf("%v, шаги %q", err, st.shagi)
		}
	})
}
