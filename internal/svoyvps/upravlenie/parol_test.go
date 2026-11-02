package upravlenie

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

// podmenitParol подменяет шаги пароля на сервере и пишет, каким входом и в
// каком порядке их позвали.
func podmenitParol(t *testing.T, root *svyaz, errVykl, errPodtv error, posle server.Vklyuchenie, errVkl error) *[]string {
	t.Helper()
	vykl, podtv, vkl := vyklyuchitParol, podtverditParol, vklyuchitParol
	t.Cleanup(func() { vyklyuchitParol, podtverditParol, vklyuchitParol = vykl, podtv, vkl })
	var shagi []string
	vhod := func(m server.Mashina) {
		if m != root {
			t.Error("шаг пароля шёл не тем входом")
		}
	}
	vyklyuchitParol = func(_ context.Context, m server.Mashina) error {
		vhod(m)
		shagi = append(shagi, "vyklyuchit")
		return errVykl
	}
	podtverditParol = func(_ context.Context, m server.Mashina) error {
		vhod(m)
		shagi = append(shagi, "podtverdit")
		return errPodtv
	}
	vklyuchitParol = func(ctx context.Context, m server.Mashina) (server.Vklyuchenie, error) {
		vhod(m)
		if ctx.Err() != nil {
			t.Error("возврат пароля шёл с отменённым ctx")
		}
		shagi = append(shagi, "vklyuchit")
		return posle, errVkl
	}
	return &shagi
}

func TestVyklyuchitParol(t *testing.T) {
	nash := klyuchUpravleniya(t)
	zapros := ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: nash}}
	novyy := &svyaz{imya: "новый", sv: Svedeniya{Root: true}}
	t.Run("выключен", func(t *testing.T) {
		root := &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "через туннель", Tunnel: true}, novyy: novyy}
		v := &vhody{tunnel: root, errPryamo: otkaz(ssh.PrichinaTishina)}
		v.podmenit(t)
		shagi := podmenitParol(t, root, nil, nil, server.Vklyuchenie{}, nil)
		it, err := VyklyuchitParol(context.Background(), puti(true), zapros)
		if err != nil || it.Vhod.Put != "через туннель" || !slices.Equal(*shagi, []string{"vyklyuchit", "podtverdit"}) {
			t.Fatalf("итог %+v, ошибка %v, шаги %q", it, err, *shagi)
		}
		// Туннель через этот же сервер годится; новый вход был и закрыт.
		if len(v.zaprosy) != 2 || !v.zaprosy[1].BezPerezapuska || root.novyhVhodov != 1 || novyy.zakryt != 1 || root.zakryt != 1 {
			t.Fatalf("входы %+v, новых %d, закрыт новый %d, старый %d", v.zaprosy, root.novyhVhodov, novyy.zakryt, root.zakryt)
		}
	})
	t.Run("вход ключом не прошёл", func(t *testing.T) {
		for imya, errVozvrat := range map[string]error{"пароль вернулся": nil, "пароль не вернулся": errors.New("обрыв")} {
			root := &svyaz{imya: "root", sv: Svedeniya{Root: true}, errNovyy: otkaz(ssh.PrichinaKlyuchNePrinyat)}
			v := &vhody{pryamo: root}
			v.podmenit(t)
			ctx, otmena := context.WithCancel(context.Background())
			shagi := podmenitParol(t, root, nil, nil, server.Vklyuchenie{Parol: "vklyuchen"}, errVozvrat)
			// Отмена посреди дела: пароль всё равно возвращается.
			vykl := vyklyuchitParol
			vyklyuchitParol = func(c context.Context, m server.Mashina) error {
				otmena()
				return vykl(c, m)
			}
			_, err := VyklyuchitParol(ctx, puti(false), zapros)
			var knp *KlyuchNePustil
			if !errors.As(err, &knp) || ssh.PrichinaIz(knp.Vhod) != ssh.PrichinaKlyuchNePrinyat || !errors.Is(knp.Vozvrat, errVozvrat) ||
				!slices.Equal(*shagi, []string{"vyklyuchit", "vklyuchit"}) || root.novyhVhodov != 1 {
				t.Fatalf("%s: %v, шаги %q, входов %d", imya, err, *shagi, root.novyhVhodov)
			}
		}
	})
	t.Run("sshd перезапускается после перечитывания", func(t *testing.T) {
		bylo := pauzaPerevhoda
		t.Cleanup(func() { pauzaPerevhoda = bylo })
		pauzaPerevhoda = time.Millisecond
		for _, c := range []struct {
			imya     string
			prichina ssh.Prichina
			raz      int
			udacha   bool
			vhodov   int
		}{
			{"порт ожил", ssh.PrichinaPortZakryt, 2, true, 3},
			{"порт так и закрыт", ssh.PrichinaPortZakryt, 0, false, 3},
			// Отказ ключа повтором не лечится.
			{"ключ не принят", ssh.PrichinaKlyuchNePrinyat, 0, false, 1},
		} {
			novyy := &svyaz{imya: "новый", sv: Svedeniya{Root: true}}
			root := &svyaz{imya: "root", sv: Svedeniya{Root: true}, novyy: novyy, errNovyy: otkaz(c.prichina), errNovyyRaz: c.raz}
			v := &vhody{pryamo: root}
			v.podmenit(t)
			shagi := podmenitParol(t, root, nil, nil, server.Vklyuchenie{Parol: "vklyuchen"}, nil)
			_, err := VyklyuchitParol(context.Background(), puti(false), zapros)
			if (err == nil) != c.udacha || root.novyhVhodov != c.vhodov {
				t.Fatalf("%s: %v, входов %d, шаги %q", c.imya, err, root.novyhVhodov, *shagi)
			}
		}
		// Отмена посреди паузы: повторов больше нет, пароль возвращается.
		pauzaPerevhoda = time.Hour
		root := &svyaz{imya: "root", sv: Svedeniya{Root: true}, errNovyy: otkaz(ssh.PrichinaPortZakryt)}
		v := &vhody{pryamo: root}
		v.podmenit(t)
		ctx, otmena := context.WithCancel(context.Background())
		shagi := podmenitParol(t, root, nil, nil, server.Vklyuchenie{Parol: "vklyuchen"}, nil)
		vykl := vyklyuchitParol
		vyklyuchitParol = func(c context.Context, m server.Mashina) error {
			time.AfterFunc(10*time.Millisecond, otmena)
			return vykl(c, m)
		}
		var knp *KlyuchNePustil
		if _, err := VyklyuchitParol(ctx, puti(false), zapros); !errors.As(err, &knp) || root.novyhVhodov != 1 ||
			!slices.Equal(*shagi, []string{"vyklyuchit", "vklyuchit"}) {
			t.Fatalf("отмена: %v, входов %d, шаги %q", err, root.novyhVhodov, *shagi)
		}
	})
	t.Run("сервер отказал", func(t *testing.T) {
		chuzhoy := &server.ParolChuzhoy{Fayly: []string{"/etc/ssh/sshd_config.d/00-billpanel.conf"}}
		root := &svyaz{imya: "root", sv: Svedeniya{Root: true}, novyy: novyy}
		v := &vhody{pryamo: root}
		v.podmenit(t)
		shagi := podmenitParol(t, root, chuzhoy, nil, server.Vklyuchenie{}, nil)
		if _, err := VyklyuchitParol(context.Background(), puti(false), zapros); !errors.Is(err, chuzhoy) || root.novyhVhodov != 0 || !slices.Equal(*shagi, []string{"vyklyuchit"}) {
			t.Fatalf("%v, шаги %q", err, *shagi)
		}
		// Таймер не снялся: ошибка наружу.
		root = &svyaz{imya: "root", sv: Svedeniya{Root: true}, novyy: &svyaz{}}
		v = &vhody{pryamo: root}
		v.podmenit(t)
		otkazPodtv := &server.Oshibka{Shag: "parol", Kod: 1, Tekst: "таймер отката успел вернуть пароль"}
		podmenitParol(t, root, nil, otkazPodtv, server.Vklyuchenie{}, nil)
		if _, err := VyklyuchitParol(context.Background(), puti(false), zapros); !errors.Is(err, otkazPodtv) {
			t.Fatal(err)
		}
	})
	t.Run("без ключа управления", func(t *testing.T) {
		v := &vhody{}
		v.podmenit(t)
		if _, err := VyklyuchitParol(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"}); err == nil || len(v.zaprosy) != 0 {
			t.Fatalf("%v, входы %d", err, len(v.zaprosy))
		}
		if _, err := VklyuchitParol(context.Background(), puti(false), ssh.Zapros{Adres: "203.0.113.10"}); err == nil || len(v.zaprosy) != 0 {
			t.Fatalf("включение: %v, входы %d", err, len(v.zaprosy))
		}
	})
}

func TestVklyuchitParol(t *testing.T) {
	nash := klyuchUpravleniya(t)
	zapros := ssh.Zapros{Adres: "203.0.113.10", Vhod: ssh.Vhod{Polzovatel: "root", Podpis: nash}}
	root := &svyaz{imya: "root", sv: Svedeniya{Root: true, Put: "напрямую"}}
	v := &vhody{pryamo: root}
	v.podmenit(t)
	posle := server.Vklyuchenie{Parol: "vyklyuchen_chuzhim", Fayly: []string{"/etc/ssh/sshd_config.d/60-cloudimg-settings.conf"}}
	shagi := podmenitParol(t, root, nil, nil, posle, nil)
	it, err := VklyuchitParol(context.Background(), puti(false), zapros)
	if err != nil || it.Posle.Parol != "vyklyuchen_chuzhim" || it.Vhod.Put != "напрямую" || !v.zaprosy[0].BezPerezapuska ||
		!slices.Equal(*shagi, []string{"vklyuchit"}) || root.zakryt != 1 {
		t.Fatalf("итог %+v, ошибка %v, шаги %q", it, err, *shagi)
	}
	osh := &server.Oshibka{Shag: "parol", Kod: 1}
	podmenitParol(t, root, nil, nil, server.Vklyuchenie{}, osh)
	if _, err := VklyuchitParol(context.Background(), puti(false), zapros); !errors.Is(err, osh) {
		t.Fatal(err)
	}
}
