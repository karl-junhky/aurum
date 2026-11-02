package upravlenie

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var (
	dozhdatsya  = server.DozhdatsyaNastroyki
	ustanovit   = server.Ustanovit
	primenit    = server.Primenit
	sostoyanie  = server.Sostoyanie
	otkrytPorty = server.OtkrytPorty
	lovit       = server.Lovit
	podobrat    = porty.Podobrat
	proverit    = proverka.Proverit
	detektor    = blok.Proverit

	// Первый сертификат клиент ждёт не дольше srokSerta (§3.8, шаг 7),
	// спрашивая программу сервера раз в shagSerta.
	srokSerta = 3 * time.Minute
	shagSerta = 5 * time.Second
)

// Shag это шаг установки, как его видит ход в окне.
type Shag string

const (
	ShagVhod       Shag = "vhod"
	ShagNastroyka  Shag = "nastroyka"
	ShagOsmotr     Shag = "osmotr"
	ShagKlyuch     Shag = "klyuch"
	ShagProgramma  Shag = "programma"
	ShagMaska      Shag = "maska"
	ShagSertifikat Shag = "sertifikat"
	ShagPodbor     Shag = "podbor"
	ShagPaket      Shag = "paket"
	ShagPorty      Shag = "porty"
	ShagDetektor   Shag = "detektor"
	ShagProverka   Shag = "proverka"
)

// Hod это весть о ходе, в той же горутине.
type Hod struct {
	Shag Shag
	// Klyuch: на подборе и проверке ключ, который проверяется (Vid пуст),
	// или его итог.
	Klyuch *proverka.Itog
	// Otsev: кандидат подбора, выпавший до проверки.
	Otsev *porty.Otsev
	Krug  *blok.Krug
}

// Ustanovka это всё, что нужно установке.
type Ustanovka struct {
	Puti Puti
	// Vhod: адрес (IP), порт, вход и Izvesten, ключ хоста с осмотра.
	Vhod ssh.Zapros
	// Pustoy это галочка «Сервер пустой»: без неё на машине не меняется
	// ничего.
	Pustoy bool
	// Imya это имя сервера в списке клиента.
	Imya string
	// Upravlenie это ключ управления этого компьютера к этому серверу.
	Upravlenie  gossh.Signer
	Kommentariy string
	Programma   io.ReadSeeker
	// Maski это список масок reality; пусто - Maski пакета.
	Maski   []string
	Ruchnye porty.Ruchnye
	Opcii   konfig.Opcii
	// Proverka: адаптер, цели. Vyhody и Hod ставит установка.
	Proverka proverka.Opcii
	Zapusk   proverka.Zapusk
	// Pryamo это соединение мимо туннеля для детектора блокировки.
	Pryamo blok.Dial
	Hod    func(Hod)
}

// VidPreduprezhdeniya это что вышло не так, а установка всё же дошла до
// ключей.
type VidPreduprezhdeniya string

const (
	// NetSerta: сертификат на адрес не выпущен (закрыты 80 и 443/tcp), hy2
	// не ставится.
	NetSerta VidPreduprezhdeniya = "net-serta"
	// NeProshyol: ни один кандидат вида не прошёл проверку из сети человека.
	// Поставлен первый свободный, Detektor говорит почему.
	NeProshyol VidPreduprezhdeniya = "ne-proshyol"
	// NeVezyot: ключ окончательного пакета не провёз проверку.
	NeVezyot VidPreduprezhdeniya = "ne-vezyot"
	// NeProvereno: проверка ключей не состоялась.
	NeProvereno VidPreduprezhdeniya = "ne-provereno"
)

// Preduprezhdenie это одно такое «не так».
type Preduprezhdenie struct {
	Vid VidPreduprezhdeniya
	// Transport и Port называют ключ: reality-tcp или hy2.
	Transport string
	Port      int
	// Otkaz для журнала: отказ выпуска или проверки, без секретов.
	Otkaz    string
	Detektor *blok.Itog
}

// Itog это поставленный сервер.
type Itog struct {
	Adres     string
	Sekrety   konfig.Sekrety
	Vhody     konfig.Vhody
	Klyuchi   []konfig.Klyuch
	Proverki  []proverka.Itog
	Prodlenie porty.Prodlenie
	Versiya   string
	Vhod      Svedeniya
	Porty     server.Porty
	// Sert это выпуск сертификата глазами программы сервера на конце
	// ожидания; nil - программа о нём ничего не сказала.
	Sert             *server.SertUzla
	Preduprezhdeniya []Preduprezhdenie
	// ParolVyklyuchen: на сервере лежит наш файл sshd, вход по паролю
	// выключали из программы (З6).
	ParolVyklyuchen bool
}

// Ostanovka: осмотр нашёл то, с чем установка не идёт. Своя отметка
// установки останавливает, только если в ней нет ключа этого компьютера:
// незаконченная своя установка продолжается, чужая берётся под управление.
type Ostanovka struct {
	Zamechaniya []server.Zamechanie
}

func (o *Ostanovka) Error() string {
	kody := make([]string, len(o.Zamechaniya))
	for i, z := range o.Zamechaniya {
		kody[i] = string(z.Kod)
	}
	return "установка не идёт: " + strings.Join(kody, ", ")
}

func (u Ustanovka) proverit() (netip.Addr, error) {
	a, err := netip.ParseAddr(u.Vhod.Adres)
	switch {
	case err != nil:
		return netip.Addr{}, fmt.Errorf("установка: адрес %q не IP", ssh.Obrezat(u.Vhod.Adres, 80))
	case !u.Pustoy:
		return netip.Addr{}, errors.New("установка: галочка «Сервер пустой» не стоит")
	case u.Upravlenie == nil || u.Programma == nil || u.Zapusk == nil || u.Pryamo == nil:
		return netip.Addr{}, errors.New("установка: не задан ключ управления, программа или запуск проверки")
	case u.Imya == "":
		return netip.Addr{}, errors.New("установка: нет имени сервера")
	}
	return a.Unmap(), nil
}

// Ustanovit ставит свой сервер по §3.8 плана: вход, ожидание первой
// настройки машины, осмотр, ключ управления, программа, сертификат, подбор
// портов, окончательный пакет, порты в ufw, проверка ключей. Ключи в набор
// кладёт служба.
func Ustanovit(ctx context.Context, u Ustanovka) (itog Itog, err error) {
	defer zashchita("Ustanovit", &err)
	adres, err := u.proverit()
	if err != nil {
		return Itog{}, err
	}
	hod := func(h Hod) {
		if u.Hod != nil {
			u.Hod(h)
		}
	}

	hod(Hod{Shag: ShagVhod})
	s, err := Voyti(ctx, u.Puti, u.Vhod)
	if err != nil {
		return Itog{}, err
	}
	// s сменится соединением root по ключу: закрывается последнее.
	defer func() { s.Zakryt() }()
	itog.Vhod = s.Svedeniya()

	hod(Hod{Shag: ShagNastroyka})
	if err := dozhdatsya(ctx, s); err != nil {
		return Itog{}, err
	}
	hod(Hod{Shag: ShagOsmotr})
	o, err := osmotret(ctx, s)
	if err != nil {
		return Itog{}, err
	}
	if p := Prepyatstviya(o, u.Upravlenie.PublicKey()); p != nil {
		return Itog{}, p
	}
	itog.ParolVyklyuchen = slices.Contains(o.SshdFayly, server.FaylParolya)

	hod(Hod{Shag: ShagKlyuch})
	r, err := s.KlyuchRoot(ctx, u.Upravlenie, u.Kommentariy, u.Pustoy)
	if err != nil {
		return Itog{}, err
	}
	s.Zakryt()
	s = r

	hod(Hod{Shag: ShagProgramma})
	ust, err := ustanovit(ctx, s, u.Programma, u.Upravlenie.PublicKey())
	if err != nil {
		return Itog{}, err
	}
	itog.Versiya = ust.Versiya

	hod(Hod{Shag: ShagMaska})
	spisokMasok := u.Maski
	if len(spisokMasok) == 0 {
		spisokMasok = Maski
	}
	maska, err := vybratMasku(ctx, s, spisokMasok)
	if err != nil {
		return Itog{}, err
	}

	hod(Hod{Shag: ShagSertifikat})
	pr, otkazSerta, err := vypustitSert(ctx, s, adres, u.Opcii, &itog.Sert)
	if err != nil {
		return Itog{}, err
	}
	itog.Prodlenie = pr
	if pr == porty.BezSerta {
		itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, Preduprezhdenie{Vid: NetSerta, Otkaz: otkazSerta})
	}

	hod(Hod{Shag: ShagPodbor})
	spisok, err := porty.Kandidaty(o, pr, u.Ruchnye)
	if err != nil {
		return Itog{}, err
	}
	for i := range spisok.Otseyany {
		hod(Hod{Shag: ShagPodbor, Otsev: &spisok.Otseyany[i]})
	}
	sek, err := konfig.NovyeSekrety(maska)
	if err != nil {
		return Itog{}, err
	}
	v := konfig.Vhody{Imya: u.Imya, Adres: adres.String(), Reality: spisok.Reality, Hy2: spisok.Hy2}
	if v, err = primenitProbu(ctx, s, sek, v, u.Opcii, hod); err != nil {
		return Itog{}, err
	}
	op := u.Proverka
	op.Vyhody = vyhody(o)
	op.Hod = func(it proverka.Itog) { hod(Hod{Shag: ShagPodbor, Klyuch: &it}) }
	vybor, err := podobrat(ctx, sek, v, op, u.Zapusk)
	if err != nil {
		return Itog{}, err
	}

	// Не прошёл ни один кандидат вида: ставится первый свободный, а почему
	// не прошёл, скажет детектор (§3.3, шаг 6).
	final := konfig.Vhody{Imya: u.Imya, Adres: v.Adres}
	var neProshli []string
	for _, vid := range []struct {
		transport string
		kandidaty []int
		vybran    int
		kuda      *[]int
	}{
		{"reality-tcp", v.Reality, vybor.Reality, &final.Reality},
		{"hy2", v.Hy2, vybor.Hy2, &final.Hy2},
	} {
		if len(vid.kandidaty) == 0 {
			continue
		}
		port := vid.vybran
		if port == 0 {
			port = vid.kandidaty[0]
			neProshli = append(neProshli, vid.transport)
		}
		*vid.kuda = []int{port}
	}

	hod(Hod{Shag: ShagPaket})
	paket, err := konfig.Paket(sek, final, u.Opcii)
	if err != nil {
		return Itog{}, err
	}
	if _, err := primenit(ctx, s, paket); err != nil {
		return Itog{}, err
	}
	hod(Hod{Shag: ShagPorty})
	if itog.Porty, err = otkrytPorty(ctx, s, final.Reality, final.Hy2); err != nil {
		return Itog{}, err
	}

	if len(neProshli) > 0 {
		hod(Hod{Shag: ShagDetektor})
		d, err := detektor(ctx, opciiDetektora(u, adres, final, sek.Maska, itog.Vhod.Tunnel, s, hod))
		if err != nil {
			return Itog{}, err
		}
		for _, t := range neProshli {
			itog.Preduprezhdeniya = append(itog.Preduprezhdeniya,
				Preduprezhdenie{Vid: NeProshyol, Transport: t, Port: portVida(final, t), Detektor: &d})
		}
	}

	hod(Hod{Shag: ShagProverka})
	klyuchi, err := konfig.Klyuchi(sek, final)
	if err != nil {
		return Itog{}, err
	}
	servery := make([]protokol.Server, len(klyuchi))
	for i, k := range klyuchi {
		servery[i] = k.Server
	}
	op.Hod = func(it proverka.Itog) { hod(Hod{Shag: ShagProverka, Klyuch: &it}) }
	op.PervyyNaVid = false
	itogi, err := proverit(ctx, servery, op, u.Zapusk)
	if err != nil {
		// Сервер поставлен, ключи собраны: несостоявшаяся проверка их не
		// отменяет, и отменённая тоже, а человек видит, что они не
		// проверены. Иначе сервер раздавал бы ключи, которых нет ни у кого.
		itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, Preduprezhdenie{Vid: NeProvereno, Otkaz: err.Error()})
	}
	for _, it := range itogi {
		if it.Vid != proverka.Vezyot && !slices.Contains(neProshli, it.Server.Transport) {
			itog.Preduprezhdeniya = append(itog.Preduprezhdeniya, Preduprezhdenie{Vid: NeVezyot,
				Transport: it.Server.Transport, Port: it.Server.Port, Otkaz: it.Otkaz})
		}
	}

	itog.Adres, itog.Sekrety, itog.Vhody, itog.Klyuchi, itog.Proverki = adres.String(), sek, final, klyuchi, itogi
	return itog, nil
}

// Prepyatstviya судит по осмотру, пойдёт ли установка: nil - пойдёт. klyuch
// это ключ управления этого компьютера к серверу, nil - его здесь нет, и любая
// отметка установки чужая.
func Prepyatstviya(o *server.Osmotr, klyuch gossh.PublicKey) *Ostanovka {
	var stop []server.Zamechanie
	for _, z := range o.Zamechaniya(seychas()) {
		switch {
		case z.Kod == server.KodUzheStoit:
			if !nashaOtmetka(o.Otmetka, klyuch) {
				z.Stop = true
				stop = append(stop, z)
			}
		case z.Stop:
			stop = append(stop, z)
		}
	}
	if len(stop) > 0 {
		return &Ostanovka{Zamechaniya: stop}
	}
	return nil
}

// nashaOtmetka: в отметке установки есть ключ управления этого компьютера.
func nashaOtmetka(s *server.Sostav, klyuch gossh.PublicKey) bool {
	return s != nil && klyuch != nil && slices.Contains(s.Klyuchi, base64.StdEncoding.EncodeToString(klyuch.Marshal()))
}

// vypustitSert ставит пакет «только сертификат» и ждёт выпуска (§3.8, шаг
// 7). Сертификат уже есть (продолжение своей установки, копия) - он и
// годится. Попытка программы сервера идёт обоими путями проверки, поэтому её
// отказ значит, что закрыты оба: hy2 тогда не ставится, а otkaz называет
// причину.
func vypustitSert(ctx context.Context, m server.Mashina, adres netip.Addr, o konfig.Opcii, sert **server.SertUzla) (pr porty.Prodlenie, otkaz string, err error) {
	do, err := sostoyanie(ctx, m)
	if err != nil {
		return 0, "", err
	}
	// Попытка считается новой, только если она позже последней до пакета,
	// на какой бы адрес та ни шла: учёт попыток у программы общий, и отказ
	// прошлого адреса иначе читался бы отказом этого.
	var byla int64
	if do.Sert != nil {
		byla = do.Sert.Popytka
	}
	paket, err := konfig.PaketSerta(adres.String(), o)
	if err != nil {
		return 0, "", err
	}
	if _, err := primenit(ctx, m, paket); err != nil {
		return 0, "", err
	}
	konec := seychas().Add(srokSerta)
	for {
		st, err := sostoyanie(ctx, m)
		if err != nil {
			return 0, "", err
		}
		if c := st.Sert; c != nil && c.IP == adres.String() {
			*sert = c
			switch {
			case c.Est:
				return prodlenieIz(c.Sposob), "", nil
			case !c.Idyot && c.Popytka > byla && c.Otkaz != "":
				return porty.BezSerta, c.Otkaz, nil
			}
		}
		if !seychas().Before(konec) {
			return porty.BezSerta, fmt.Sprintf("сертификат не выпущен за %v", srokSerta), nil
		}
		t := time.NewTimer(shagSerta)
		select {
		case <-ctx.Done():
			t.Stop()
			return 0, "", ctx.Err()
		case <-t.C:
		}
	}
}

// prodlenieIz: выпущен проверкой HTTP-01 - продлению хватит 80, и 443/tcp
// может взять reality. TLS-ALPN-01 и неизвестный путь (копия) держат 443.
func prodlenieIz(sposob string) porty.Prodlenie {
	if sposob == "http-01" {
		return porty.Cherez80
	}
	return porty.Cherez443
}

// primenitProbu ставит пробный пакет со всеми кандидатами. Порт, который
// ядру не достался (его заняла чужая программа после осмотра), уходит из
// кандидатов, и пакет ставится снова.
func primenitProbu(ctx context.Context, m server.Mashina, sek konfig.Sekrety, v konfig.Vhody, o konfig.Opcii, hod func(Hod)) (konfig.Vhody, error) {
	for {
		o.Proba = porty.SrokProby(v)
		paket, err := konfig.Paket(sek, v, o)
		if err != nil {
			return v, err
		}
		k, err := primenit(ctx, m, paket)
		if err == nil {
			return v, nil
		}
		z := k.Zanyat
		if z == nil {
			return v, err
		}
		var spisok *[]int
		switch z.Proto {
		case "tcp":
			spisok = &v.Reality
		case "udp":
			spisok = &v.Hy2
		default:
			return v, err
		}
		i := slices.Index(*spisok, z.Port)
		if i < 0 {
			return v, err
		}
		*spisok = slices.Delete(slices.Clone(*spisok), i, i+1)
		hod(Hod{Shag: ShagPodbor, Otsev: &porty.Otsev{Proto: z.Proto, Port: z.Port, Prichina: porty.PrichinaZanyat, Process: "?"}})
		if len(*spisok) == 0 {
			return v, fmt.Errorf("%w: %s", porty.ErrNetKandidatov, z.Proto)
		}
	}
}

// vyhody это адреса сервера из осмотра: с любого из них он может выйти в
// интернет.
func vyhody(o *server.Osmotr) []netip.Addr {
	var a []netip.Addr
	for _, x := range slices.Concat(o.IPv4, o.IPv6) {
		a = append(a, x.Set.Addr())
	}
	return a
}

func portVida(v konfig.Vhody, transport string) int {
	p := v.Hy2
	if transport == "reality-tcp" {
		p = v.Reality
	}
	if len(p) == 0 {
		return 0
	}
	return p[0]
}

// opciiDetektora: стук в 80 и 443 и порты ключей, на порту reality
// приветствие TLS с маской; UDP в порт hy2. Вход через туннель уже показал,
// что сервер жив.
func opciiDetektora(u Ustanovka, adres netip.Addr, v konfig.Vhody, maska string, tunnel bool, m server.Mashina, hod func(Hod)) blok.Opcii {
	var tcp []blok.PortTCP
	for _, p := range []int{80, 443} {
		if !slices.Contains(v.Reality, p) {
			tcp = append(tcp, blok.PortTCP{Port: p})
		}
	}
	for _, p := range v.Reality {
		tcp = append(tcp, blok.PortTCP{Port: p, TLS: maska})
	}
	t := blok.TunnelNeProveren
	if tunnel {
		t = blok.TunnelOtvetil
	}
	portSsh := u.Vhod.Port
	if portSsh == 0 {
		portSsh = 22
	}
	return blok.Opcii{
		Adres: adres, PortSsh: portSsh, TCP: tcp, UDP: v.Hy2, Pryamo: u.Pryamo, Tunnel: t,
		Lovushka: func(ctx context.Context, proto string, port, sekund int) (int64, error) {
			return lovit(ctx, m, proto, port, sekund)
		},
		Hod: func(k blok.Krug) { hod(Hod{Shag: ShagDetektor, Krug: &k}) },
	}
}
