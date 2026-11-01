package ssh

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func zhdatPrichinu(t *testing.T, err error, nado Prichina) *Otkaz {
	t.Helper()
	var o *Otkaz
	if !errors.As(err, &o) {
		t.Fatalf("ждала отказ %q, получила %v", nado, err)
	}
	if o.Prichina != nado {
		t.Fatalf("ждала причину %q, получила %q (%v)", nado, o.Prichina, err)
	}
	return o
}

func voyti(t *testing.T, p *podstavnoy, v Vhod) *Soedinenie {
	t.Helper()
	s, err := Podklyuchit(kontekst(t), p.zapros(v))
	if err != nil {
		t.Fatalf("вход: %v", err)
	}
	t.Cleanup(s.Zakryt)
	return s
}

func TestParolRoot(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	if !s.Root || s.KlyuchHosta.Otpechatok() == "" || s.KlyuchHostaSmenilsya {
		t.Fatalf("root %v, отпечаток %q, сменился %v", s.Root, s.KlyuchHosta.Otpechatok(), s.KlyuchHostaSmenilsya)
	}
	it, err := s.Vypolnit(kontekst(t), "echo privet", nil)
	if err != nil || it.Kod != 0 || string(it.Vyvod) != "privet\n" {
		t.Fatalf("команда: %v, код %d, вывод %q, ошибки %q", err, it.Kod, it.Vyvod, it.Oshibki)
	}
	z := p.zapisi()
	if posl := z[len(z)-1].komanda; posl != "bash -c 'export LC_ALL=C\necho privet'" {
		t.Fatalf("root идёт без sudo и в bash -c с LC_ALL=C, ушло %q", posl)
	}
}

func TestNevernyyParolOdnaPopytka(t *testing.T) {
	// Сервер предлагает и password, и keyboard-interactive: неверный пароль
	// обязан стоить одну неудачу, а не две.
	p := zapustitPodstavnoy(t, nastroyki{
		paroli:   map[string]string{"root": "verno"},
		paroliKI: map[string]string{"root": "verno"},
	})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "neverno"}))
	zhdatPrichinu(t, err, PrichinaParolNePrinyat)
	if paroli, ki, podkl := p.schyot(); paroli != 1 || ki != 0 || podkl != 1 {
		t.Fatalf("попыток пароля %d, keyboard-interactive %d, подключений %d; ждала 1, 0, 1", paroli, ki, podkl)
	}
}

func TestTolkoKeyboardInteractive(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroliKI: map[string]string{"root": "verno"}})
	voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})

	p2 := zapustitPodstavnoy(t, nastroyki{paroliKI: map[string]string{"root": "verno"}})
	_, err := Podklyuchit(kontekst(t), p2.zapros(Vhod{Polzovatel: "root", Parol: "neverno"}))
	zhdatPrichinu(t, err, PrichinaParolNePrinyat)
	if _, ki, _ := p2.schyot(); ki != 1 {
		t.Fatalf("keyboard-interactive пробовался %d раз, ждала 1", ki)
	}
}

func TestTolkoKlyuch(t *testing.T) {
	k := novyyKlyuch(t)
	n := nastroyki{klyuchi: map[string]gossh.PublicKey{"root": k.PublicKey()}}
	p := zapustitPodstavnoy(t, n)

	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "lyuboy"}))
	zhdatPrichinu(t, err, PrichinaParolNeBeryot)

	s := voyti(t, p, Vhod{Polzovatel: "root", Podpis: k})
	if !s.Root {
		t.Fatal("вошли ключом root, а Root ложь")
	}
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Podpis: novyyKlyuch(t)}))
	zhdatPrichinu(t, err, PrichinaKlyuchNePrinyat)
}

func TestRootPoParolyuZapreshchyon(t *testing.T) {
	// PermitRootLogin prohibit-password: сервер просто не принимает пароль
	// root, по протоколу это неотличимо от неверного.
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"ivan": "verno"}})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaParolNePrinyat)
}

func TestProsrochennyyParol(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, prosrochen: true})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaParolProsrochen)

	p2 := zapustitPodstavnoy(t, nastroyki{paroliKI: map[string]string{"root": "verno"}, prosrochenKI: true})
	_, err = Podklyuchit(kontekst(t), p2.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaParolProsrochen)
}

func TestSudoBezParolya(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{
		paroli: map[string]string{"ivan": "verno"}, uid: map[string]string{"ivan": "1000"},
		sudo: sudoBezParolya, permitRootLogin: "prohibit-password",
	})
	s := voyti(t, p, Vhod{Polzovatel: "ivan", Parol: "verno"})
	if s.Root {
		t.Fatal("ivan не root")
	}
	it, err := s.Skript(kontekst(t), "argumenty", "a b", "c'd")
	if err != nil || strings.TrimSpace(string(it.Vyvod)) != "a b|c'd" {
		t.Fatalf("скрипт: %v, %q, %q", err, it.Vyvod, it.Oshibki)
	}
	z := p.zapisi()
	if posl := z[len(z)-1].komanda; !strings.HasPrefix(posl, "sudo -n -- bash -s -- ") {
		t.Fatalf("скрипт не-root идёт через sudo -n в bash -s, ушло %q", posl)
	}
	if err := s.OtpravitFayl(kontekst(t), bytes.NewReader([]byte("dannye")), "/var/lib/affory/zagruzka/f", 0o644); err != nil {
		t.Fatalf("файл через sudo без пароля: %v", err)
	}
}

func TestSudoSParolem(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{
		paroli: map[string]string{"ivan": "verno"}, uid: map[string]string{"ivan": "1000"},
		sudo: sudoSParolem, permitRootLogin: "prohibit-password",
	})
	s := voyti(t, p, Vhod{Polzovatel: "ivan", Parol: "verno"})
	it, err := s.Skript(kontekst(t), "argumenty", "x")
	if err != nil || strings.TrimSpace(string(it.Vyvod)) != "x" {
		t.Fatalf("скрипт под sudo с паролем: %v, %q, %q", err, it.Vyvod, it.Oshibki)
	}
	z := p.zapisi()
	if posl := z[len(z)-1].komanda; !strings.HasPrefix(posl, "sudo -k -S -p '' -- bash -c 'export LC_ALL=C\nargumenty' bash 'x'") {
		t.Fatalf("под sudo с паролем скрипт идёт аргументом bash -c, ушло %q", posl)
	}
	for _, zp := range z {
		if strings.Contains(zp.komanda, "verno") {
			t.Fatalf("пароль попал в строку команды: %q", zp.komanda)
		}
	}
	err = s.OtpravitFayl(kontekst(t), bytes.NewReader([]byte("x")), "/var/lib/affory/f", 0o644)
	zhdatPrichinu(t, err, PrichinaSboy)

	podpis := novyyKlyuch(t)
	klyuch := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(podpis.PublicKey()))) + " affory-PC-2026-11-01"
	if r, err := s.PolozhitKlyuchRoot(kontekst(t), podpis, "affory-PC-2026-11-01", false); err == nil {
		r.Zakryt()
		t.Fatal("ключ лёг без галочки «Сервер пустой»")
	}
	if _, est := p.fayl(putKlyuchaRoot); est {
		t.Fatal("без галочки сервер тронут")
	}
	for i := range 2 {
		r, err := s.PolozhitKlyuchRoot(kontekst(t), podpis, "affory-PC-2026-11-01", true)
		if err != nil {
			t.Fatalf("ключ root, раз %d: %v", i+1, err)
		}
		if !r.Root || r.KlyuchHosta.Otpechatok() != s.KlyuchHosta.Otpechatok() {
			t.Fatalf("вход ключом: root %v, ключ хоста %s против %s", r.Root, r.KlyuchHosta.Otpechatok(), s.KlyuchHosta.Otpechatok())
		}
		r.Zakryt()
	}
	f, _ := p.fayl(putKlyuchaRoot)
	if string(f.dannye) != klyuch+"\n" {
		t.Fatalf("authorized_keys %q, ждала одну строку %q", f.dannye, klyuch)
	}
}

func TestKlyuchRootNePustilOtkat(t *testing.T) {
	// AllowUsers без root: PermitRootLogin пускает, а вход ключом нет.
	chuzhaya := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(novyyKlyuch(t).PublicKey()))) + " chuzhoy\n"
	n := nastroyki{
		paroli: map[string]string{"ivan": "verno"}, uid: map[string]string{"ivan": "1000"},
		sudo: sudoBezParolya, permitRootLogin: "prohibit-password", rootKlyuchomNelzya: true,
	}
	p := zapustitPodstavnoy(t, n)
	p.polozhit(putKlyuchaRoot, fayl{dannye: []byte(chuzhaya), rezhim: "644"})
	s := voyti(t, p, Vhod{Polzovatel: "ivan", Parol: "verno"})
	ostalos := func(nado string) {
		t.Helper()
		if f, _ := p.fayl(putKlyuchaRoot); string(f.dannye) != nado || f.rezhim != "644" {
			t.Fatalf("authorized_keys %q, права %s; ждала %q, 644", f.dannye, f.rezhim, nado)
		}
	}

	// Положенная сейчас строка убирается, чужая остаётся.
	_, err := s.PolozhitKlyuchRoot(kontekst(t), novyyKlyuch(t), "affory", true)
	if o := zhdatPrichinu(t, err, PrichinaRootZapreshchyon); o.KlyuchOstalsya {
		t.Fatal("строка убрана, а отказ говорит, что осталась")
	}
	ostalos(chuzhaya)

	// Строка нашего ключа от прошлой, оборванной попытки тоже бесполезна.
	podpis := novyyKlyuch(t)
	nasha := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(podpis.PublicKey()))) + " affory\n"
	p.polozhit(putKlyuchaRoot, fayl{dannye: []byte(chuzhaya + nasha), rezhim: "644"})
	_, err = s.PolozhitKlyuchRoot(kontekst(t), podpis, "affory", true)
	zhdatPrichinu(t, err, PrichinaRootZapreshchyon)
	ostalos(chuzhaya)

	// Вход сорвался не отказом sshd, а заслоном: прежняя строка могла
	// работать и остаётся, положенная сейчас убирается.
	obshchiyZaslon.mu.Lock()
	obshchiyZaslon.zapisi[p.ln.Addr().String()] = neudachi{podryad: neudachPodryad, poslednyaya: obshchiyZaslon.seychas()}
	obshchiyZaslon.mu.Unlock()
	p.polozhit(putKlyuchaRoot, fayl{dannye: []byte(chuzhaya + nasha), rezhim: "644"})
	_, err = s.PolozhitKlyuchRoot(kontekst(t), podpis, "affory", true)
	zhdatPrichinu(t, err, PrichinaRanoPovtoryat)
	ostalos(chuzhaya + nasha)
	_, err = s.PolozhitKlyuchRoot(kontekst(t), novyyKlyuch(t), "affory", true)
	zhdatPrichinu(t, err, PrichinaRanoPovtoryat)
	ostalos(chuzhaya + nasha)

	// Убрать не вышло: отказ это называет.
	n.uborkaPadaet = true
	p = zapustitPodstavnoy(t, n)
	s = voyti(t, p, Vhod{Polzovatel: "ivan", Parol: "verno"})
	_, err = s.PolozhitKlyuchRoot(kontekst(t), novyyKlyuch(t), "affory", true)
	if o := zhdatPrichinu(t, err, PrichinaRootZapreshchyon); !o.KlyuchOstalsya || !strings.Contains(o.Podrobno, "не убрана") {
		t.Fatalf("неубранная строка не названа: %v", err)
	}
}

func TestShumVhoda(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, shumVhoda: "Добро пожаловать\n"})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	if o := zhdatPrichinu(t, err, PrichinaShumVhoda); !strings.Contains(o.Podrobno, "Добро пожаловать") {
		t.Fatalf("шум не назван: %v", err)
	}
	// Слова про пароль в шуме не делают пароль просроченным.
	p = zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, shumVhoda: "Please change your password after first login\n"})
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaShumVhoda)
	// Пустые строки терпимы и срезаются с вывода каждой команды.
	p = zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, shumVhoda: "\n\n"})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	if it, err := s.Vypolnit(kontekst(t), "echo privet", nil); err != nil || string(it.Vyvod) != "privet\n" {
		t.Fatalf("вывод после пустого шума: %v, %q", err, it.Vyvod)
	}
}

// panikuyushchiyKlyuch падает на первом же вопросе сверки.
type panikuyushchiyKlyuch struct{ gossh.PublicKey }

func (panikuyushchiyKlyuch) Type() string { panic("проверка") }

func TestSverkaKlyuchaHostaBezPaniki(t *testing.T) {
	// Сверку x/crypto зовёт из своей горутины: паника там прошла бы мимо
	// перехвата Podklyuchit и уронила бы службу.
	zhdatPrichinu(t, (&sverkaKlyuchaHosta{}).proverit("", nil, panikuyushchiyKlyuch{}), PrichinaSboy)
}

func TestOchistkaChuzhogoTeksta(t *testing.T) {
	o := otkaz(PrichinaObryv, "a\nb", errors.New("ssh: disconnect, reason 2: x\x1b[2Jy‮z"))
	if s := o.Error(); strings.ContainsAny(s, "\n\x1b‮") {
		t.Fatalf("в тексте отказа остались управляющие символы: %q", s)
	}
	if s := Obrezat("a⁦b\tc", 10); s != "a b c" {
		t.Fatalf("obrezat: %q", s)
	}
}

func TestSudoOtkazy(t *testing.T) {
	baza := func(r sudoRezhim) nastroyki {
		return nastroyki{paroli: map[string]string{"ivan": "verno"}, uid: map[string]string{"ivan": "1000"}, sudo: r}
	}
	p := zapustitPodstavnoy(t, baza(sudoNet))
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "ivan", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaNetSudo)

	p = zapustitPodstavnoy(t, baza(sudoNeVSudoers))
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "ivan", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaSudoNePustil)

	k := novyyKlyuch(t)
	n := baza(sudoSParolem)
	n.paroli, n.klyuchi = nil, map[string]gossh.PublicKey{"ivan": k.PublicKey()}
	p = zapustitPodstavnoy(t, n)
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "ivan", Podpis: k}))
	zhdatPrichinu(t, err, PrichinaSudoPrositParol)
}

func TestPermitRootLoginNo(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{
		paroli: map[string]string{"ivan": "verno"}, uid: map[string]string{"ivan": "1000"},
		sudo: sudoBezParolya, permitRootLogin: "no",
	})
	s := voyti(t, p, Vhod{Polzovatel: "ivan", Parol: "verno"})
	_, err := s.PolozhitKlyuchRoot(kontekst(t), novyyKlyuch(t), "", true)
	zhdatPrichinu(t, err, PrichinaRootZapreshchyon)
	if _, est := p.fayl(putKlyuchaRoot); est {
		t.Fatal("при PermitRootLogin no ключ всё равно лёг")
	}
}

func TestObryvPosrediKomandy(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	_, err := s.Vypolnit(kontekst(t), "obryv", nil)
	zhdatPrichinu(t, err, PrichinaObryv)
}

func TestKlyuchHostaSmenilsya(t *testing.T) {
	staryy := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, staryy, Vhod{Polzovatel: "root", Parol: "verno"})
	zapisan := s.KlyuchHosta

	novyy := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	z := novyy.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Izvesten = &zapisan
	_, err := Podklyuchit(kontekst(t), z)
	o := zhdatPrichinu(t, err, PrichinaKlyuchHostaSmenilsya)
	if o.Predyavlen == nil || o.Predyavlen.Otpechatok() == zapisan.Otpechatok() {
		t.Fatal("в отказе нет нового ключа хоста")
	}
	// До отказа серверу не ушло ничего: ни пароля, ни команды.
	if paroli, _, _ := novyy.schyot(); paroli != 0 || len(novyy.zapisi()) != 0 {
		t.Fatalf("после смены ключа ушли пароли (%d) или команды (%d)", paroli, len(novyy.zapisi()))
	}

	z.Odobren = o.Predyavlen.Otpechatok()
	s2, err := Podklyuchit(kontekst(t), z)
	if err != nil {
		t.Fatalf("одобренный ключ: %v", err)
	}
	defer s2.Zakryt()
	if !s2.KlyuchHostaSmenilsya {
		t.Fatal("одобренная смена ключа не отмечена")
	}
}

func TestTipKlyuchaHosta(t *testing.T) {
	// golang/go#29286: у сервера два ключа, записан RSA. Исправная машина
	// обязана предъявить именно его, а не ed25519.
	rsaZakrytyy, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPodpis, err := gossh.NewSignerFromKey(rsaZakrytyy)
	if err != nil {
		t.Fatal(err)
	}
	p := zapustitPodstavnoy(t, nastroyki{
		paroli:       map[string]string{"root": "verno"},
		klyuchiHosta: []gossh.Signer{novyyKlyuch(t), rsaPodpis},
	})
	zapisan := klyuchHostaIz(rsaPodpis.PublicKey())
	z := p.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Izvesten = &zapisan
	s, err := Podklyuchit(kontekst(t), z)
	if err != nil {
		t.Fatalf("записанный RSA: %v", err)
	}
	defer s.Zakryt()
	if s.KlyuchHosta.Tip != gossh.KeyAlgoRSA || s.KlyuchHostaSmenilsya {
		t.Fatalf("предъявлен %s, сменился %v", s.KlyuchHosta.Tip, s.KlyuchHostaSmenilsya)
	}
	if v, err := KlyuchHostaIzZapisi(zapisan.Tip, zapisan.TeloBase64()); err != nil || v.Otpechatok() != zapisan.Otpechatok() {
		t.Fatalf("запись ключа хоста туда и обратно: %v", err)
	}
	if _, err := KlyuchHostaIzZapisi(gossh.KeyAlgoED25519, zapisan.TeloBase64()); err == nil {
		t.Fatal("тип в записи не сверяется с телом")
	}
}

func TestPovtorPriObryveDoVhoda(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, zakrytBezPrivetstviya: 2})
	voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	if _, _, podkl := p.schyot(); podkl != 3 {
		t.Fatalf("подключений %d, ждала 3", podkl)
	}

	p = zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, zakrytBezPrivetstviya: 100})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaPodozhdi)
	if _, _, podkl := p.schyot(); podkl != 1+len(pauzyPovtora) {
		t.Fatalf("подключений %d, ждала %d", podkl, 1+len(pauzyPovtora))
	}

	// Через туннель то же самое значит, что сервер не ответил ядру.
	z := p.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Put.Tunnel = true
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaTishina)

	p = zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, sbrosBezPrivetstviya: 100})
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	zhdatPrichinu(t, err, PrichinaObryv)
	if _, _, podkl := p.schyot(); podkl != 1+len(pauzyPovtora) {
		t.Fatalf("сброс: подключений %d, ждала %d", podkl, 1+len(pauzyPovtora))
	}
}

func TestOtkazVhodaNePovtoryaetsya(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "neverno"}))
	zhdatPrichinu(t, err, PrichinaParolNePrinyat)
	if _, _, podkl := p.schyot(); podkl != 1 {
		t.Fatalf("после отказа входа подключений %d, ждала 1", podkl)
	}
}

func TestPortZakrytMolchitNeSSH(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	z := Zapros{Adres: "127.0.0.1", Port: port, Vhod: Vhod{Polzovatel: "root", Parol: "x"},
		Put: Put{Imya: "тест", Dial: (&net.Dialer{}).DialContext}}
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaPortZakryt)

	staryy := srokPopytki
	srokPopytki = 300 * time.Millisecond
	defer func() { srokPopytki = staryy }()
	p := zapustitPodstavnoy(t, nastroyki{molchat: true})
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "x"}))
	zhdatPrichinu(t, err, PrichinaTishina)
	if _, _, podkl := p.schyot(); podkl != 1 {
		t.Fatalf("молчащий сервер: подключений %d, ждала 1 без повторов", podkl)
	}

	p = zapustitPodstavnoy(t, nastroyki{neSSH: true})
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "x"}))
	zhdatPrichinu(t, err, PrichinaNeSSH)

	// Не SSH, и соединение держит до срока попытки (HTTP keep-alive).
	p = zapustitPodstavnoy(t, nastroyki{neSSH: true, molchat: true})
	_, err = Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "x"}))
	zhdatPrichinu(t, err, PrichinaNeSSH)
}

// podstavnoySOCKS отвечает на любой CONNECT кодом otvet (RFC 1928, 6.).
func podstavnoySOCKS(t *testing.T, otvet byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 512)
			// Приветствие: версия, число способов, способы.
			if _, err := io.ReadFull(c, buf[:2]); err == nil {
				if _, err := io.ReadFull(c, buf[:buf[1]]); err == nil {
					c.Write([]byte{5, 0})
					// Запрос: 5 1 0, тип адреса, адрес, порт. Адрес IPv4.
					if _, err := io.ReadFull(c, buf[:10]); err == nil {
						c.Write([]byte{5, otvet, 0, 1, 0, 0, 0, 0, 0, 0})
					}
				}
			}
			c.Close()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		<-gotovo
	})
	return ln.Addr().String()
}

func TestPutCherezTunnelOtkazy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	zakrytyy := ln.Addr().String()
	ln.Close()
	z := Zapros{Adres: "198.51.100.7", Port: 22, Vhod: Vhod{Polzovatel: "root", Parol: "x"}, Put: PutCherezTunnel(zakrytyy)}
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaNetVhodaTunnelya)

	// Ядро есть, а сервер не ответил: 4 «host unreachable». Это молчание
	// пути, а не закрытый порт и не отсутствие ядра.
	z.Put = PutCherezTunnel(podstavnoySOCKS(t, 4))
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaTishina)
}

func TestZaslon(t *testing.T) {
	seychas := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	prezhnee := obshchiyZaslon.seychas
	obshchiyZaslon.seychas = func() time.Time { return seychas }
	defer func() { obshchiyZaslon.seychas = prezhnee }()

	// Отказ без попытки (сервер пароль не берёт) неудачей не считается.
	k := novyyKlyuch(t)
	p := zapustitPodstavnoy(t, nastroyki{klyuchi: map[string]gossh.PublicKey{"root": k.PublicKey()}})
	for range 3 {
		_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "x"}))
		zhdatPrichinu(t, err, PrichinaParolNeBeryot)
	}

	p = zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	plohoy := p.zapros(Vhod{Polzovatel: "root", Parol: "neverno"})
	for range 2 {
		_, err := Podklyuchit(kontekst(t), plohoy)
		zhdatPrichinu(t, err, PrichinaParolNePrinyat)
	}
	_, err := Podklyuchit(kontekst(t), p.zapros(Vhod{Polzovatel: "root", Parol: "verno"}))
	o := zhdatPrichinu(t, err, PrichinaRanoPovtoryat)
	if o.Podozhdat != 60 {
		t.Fatalf("ждать %d с, ждала 60", o.Podozhdat)
	}
	if _, _, podkl := p.schyot(); podkl != 2 {
		t.Fatalf("третья попытка дошла до сервера: подключений %d", podkl)
	}
	seychas = seychas.Add(61 * time.Second)
	voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	// Удача обнуляет счёт.
	_, err = Podklyuchit(kontekst(t), plohoy)
	zhdatPrichinu(t, err, PrichinaParolNePrinyat)
}

func TestFayly(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	ctx := kontekst(t)
	dannye := bytes.Repeat([]byte("vpn-uzel "), 1000)
	chitatel := bytes.NewReader(dannye)
	if err := s.OtpravitFayl(ctx, chitatel, "/var/lib/affory/zagruzka/vpn-uzel", 0o755); err != nil {
		t.Fatalf("файл: %v", err)
	}
	f, est := p.fayl("/var/lib/affory/zagruzka/vpn-uzel")
	if !est || !bytes.Equal(f.dannye, dannye) || f.rezhim != "0755" {
		t.Fatalf("на сервере %d байт, права %q", len(f.dannye), f.rezhim)
	}
	// Тот же читатель второй раз, уже дочитанный до конца: так повторяет
	// установка.
	do := len(p.zapisi())
	if err := s.OtpravitFayl(ctx, chitatel, "/var/lib/affory/zagruzka/vpn-uzel", 0o700); err != nil {
		t.Fatalf("тот же файл: %v", err)
	}
	z := p.zapisi()[do:]
	if len(z) != 2 || strings.Contains(z[1].komanda, `cat > "$t"`) {
		t.Fatalf("сошедшийся файл передан заново: %q", z)
	}
	if f, _ := p.fayl("/var/lib/affory/zagruzka/vpn-uzel"); f.rezhim != "0700" {
		t.Fatalf("права сошедшегося файла %q, ждала 0700", f.rezhim)
	}
	for _, plohoy := range []string{"var/x", "/var/../etc/x", "/var/x/", "/var/x y", "/var/x'y", "/var//x"} {
		zhdatPrichinu(t, s.OtpravitFayl(ctx, bytes.NewReader(dannye), plohoy, 0o644), PrichinaSboy)
	}
}

func TestKomandaKodIVyvod(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	it, err := s.Vypolnit(kontekst(t), "exit 3", nil)
	if err != nil || it.Kod != 3 || !strings.Contains(it.Oshibki, "выход с кодом") {
		t.Fatalf("ненулевой код: %v, %d, %q", err, it.Kod, it.Oshibki)
	}
	_, err = s.Vypolnit(kontekst(t), "mnogo", nil)
	zhdatPrichinu(t, err, PrichinaSboy)
}

func TestPulsZakryvaetMyortvoe(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}, gluhoyPuls: true})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	nachalo := time.Now()
	_, err := s.Vypolnit(kontekst(t), "zhdat", nil)
	o := zhdatPrichinu(t, err, PrichinaObryv)
	if !strings.Contains(o.Podrobno, "пульс") {
		t.Fatalf("обрыв не называет пульс: %v", err)
	}
	if proshlo := time.Since(nachalo); proshlo > (propuskovPulsa+3)*intervalPulsa {
		t.Fatalf("мёртвое соединение держало команду %v", proshlo)
	}
}

func TestSrokKomandy(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	s := voyti(t, p, Vhod{Polzovatel: "root", Parol: "verno"})
	ctx, otmena := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer otmena()
	_, err := s.Vypolnit(ctx, "zhdat", nil)
	zhdatPrichinu(t, err, PrichinaSrok)
}

func TestTunnelCherezEtotZheServer(t *testing.T) {
	n := nastroyki{paroli: map[string]string{"root": "verno"}, sshClient: "198.51.100.7", adresaServera: "203.0.113.10 10.0.0.5"}
	p := zapustitPodstavnoy(t, n)
	z := p.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Put.Tunnel = true
	s, err := Podklyuchit(kontekst(t), z)
	if err != nil {
		t.Fatalf("туннель через другой сервер: %v", err)
	}
	s.Zakryt()

	n.sshClient = "203.0.113.10"
	p = zapustitPodstavnoy(t, n)
	z = p.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Put.Tunnel = true
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaTunnelCherezNego)

	// Подставной сервер видит настоящий адрес, а это петля.
	n.sshClient = ""
	p = zapustitPodstavnoy(t, n)
	z = p.zapros(Vhod{Polzovatel: "root", Parol: "verno"})
	z.Put.Tunnel = true
	_, err = Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaTunnelCherezNego)
}

func TestKlyuchCheloveka(t *testing.T) {
	_, err := podpisKlyucha([]byte("PuTTY-User-Key-File-3: ssh-ed25519\n"), "")
	zhdatPrichinu(t, err, PrichinaKlyuchPpk)
	_, err = podpisKlyucha([]byte("не ключ"), "")
	zhdatPrichinu(t, err, PrichinaKlyuchNeChitaetsya)
	_, err = podpisKlyucha(bytes.Repeat([]byte("x"), predelKlyucha+1), "")
	zhdatPrichinu(t, err, PrichinaKlyuchNeChitaetsya)

	rsaZakrytyy, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	blok, err := gossh.MarshalPrivateKeyWithPassphrase(rsaZakrytyy, "", []byte("sekret"))
	if err != nil {
		t.Fatal(err)
	}
	zashifrovan := pem.EncodeToMemory(blok)
	_, err = podpisKlyucha(zashifrovan, "")
	zhdatPrichinu(t, err, PrichinaNuzhenParolKlyucha)
	_, err = podpisKlyucha(zashifrovan, "ne-tot")
	zhdatPrichinu(t, err, PrichinaParolKlyuchaNeveren)
	podpis, err := podpisKlyucha(zashifrovan, "sekret")
	if err != nil {
		t.Fatalf("верный пароль ключа: %v", err)
	}

	p := zapustitPodstavnoy(t, nastroyki{klyuchi: map[string]gossh.PublicKey{"root": podpis.PublicKey()}})
	voyti(t, p, Vhod{Polzovatel: "root", Klyuch: zashifrovan, ParolKlyucha: "sekret"})
}

func TestNegodnyyZapros(t *testing.T) {
	p := zapustitPodstavnoy(t, nastroyki{paroli: map[string]string{"root": "verno"}})
	for _, v := range []Vhod{
		{Polzovatel: "", Parol: "x"},
		{Polzovatel: "root"},
		{Polzovatel: "root", Parol: "x", Podpis: novyyKlyuch(t)},
		{Polzovatel: "ro ot", Parol: "x"},
	} {
		_, err := Podklyuchit(kontekst(t), p.zapros(v))
		zhdatPrichinu(t, err, PrichinaSboy)
	}
	for _, adres := range []string{"", "http://203.0.113.10", "203.0.113.10:22", "-x.example.net", "a b", "203.0.113.10 /"} {
		z := p.zapros(Vhod{Polzovatel: "root", Parol: "x"})
		z.Adres = adres
		_, err := Podklyuchit(kontekst(t), z)
		zhdatPrichinu(t, err, PrichinaSboy)
	}
	if _, _, podkl := p.schyot(); podkl != 0 {
		t.Fatalf("негодный запрос дошёл до сервера: %d", podkl)
	}
}

func TestPanikaStanovitsyaOtkazom(t *testing.T) {
	z := Zapros{Adres: "127.0.0.1", Vhod: Vhod{Polzovatel: "root", Parol: "x"},
		Put: Put{Imya: "тест", Dial: func(context.Context, string, string) (net.Conn, error) { panic("проверка") }}}
	_, err := Podklyuchit(kontekst(t), z)
	zhdatPrichinu(t, err, PrichinaSboy)
}

func TestStrokaKlyucha(t *testing.T) {
	pk := novyyKlyuch(t).PublicKey()
	telo := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pk)))
	if s, err := strokaKlyucha(pk, " affory-PC 2026-11-01 "); err != nil || s != telo+" affory-PC 2026-11-01" {
		t.Fatalf("годная строка: %q, %v", s, err)
	}
	if s, err := strokaKlyucha(pk, ""); err != nil || s != telo {
		t.Fatalf("без комментария: %q, %v", s, err)
	}
	for _, plohoy := range []string{"$(reboot)", "a\nb", `x"y`, "имя"} {
		if _, err := strokaKlyucha(pk, plohoy); err == nil {
			t.Fatalf("комментарий %q принят", plohoy)
		}
	}
}

func TestAlgoritmyKlyuchaHosta(t *testing.T) {
	rsaKlyuch := KlyuchHosta{Tip: gossh.KeyAlgoRSA}
	a := algoritmyKlyuchaHosta(&rsaKlyuch)
	if a[0] != gossh.KeyAlgoRSASHA512 || a[1] != gossh.KeyAlgoRSASHA256 || len(a) != len(algoritmyPoUmolchaniyu) {
		t.Fatalf("для RSA: %v", a)
	}
	ed := KlyuchHosta{Tip: gossh.KeyAlgoED25519}
	if a := algoritmyKlyuchaHosta(&ed); a[0] != gossh.KeyAlgoED25519 || len(a) != len(algoritmyPoUmolchaniyu) {
		t.Fatalf("для ed25519: %v", a)
	}
}
