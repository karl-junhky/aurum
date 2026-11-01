package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
)

// Put это дорога до сервера: напрямую мимо туннеля (PutPryamo) или через
// локальный вход ядра (PutCherezTunnel).
type Put struct {
	// Imya для журнала: «напрямую через Ethernet», «через туннель».
	Imya string
	// Tunnel: соединение идёт через ядро туннеля. После входа слой сверяет,
	// что туннель выходит в интернет не с этого же сервера.
	Tunnel bool
	Dial   func(ctx context.Context, network, adres string) (net.Conn, error)
}

// Zapros это куда и как входить.
type Zapros struct {
	Adres string
	// Port SSH, 0 значит 22.
	Port int
	Vhod Vhod
	// Izvesten это записанный ключ хоста, nil при первой встрече.
	Izvesten *KlyuchHosta
	// Odobren это отпечаток нового ключа хоста, который человек одобрил
	// кнопкой «Я переустановил систему».
	Odobren string
	Put     Put
	// TolkoSmotrit: вход ничего не перезапускает, и туннелю через этот же
	// сервер он не опасен. Без этого сервер, до которого прямой SSH режут,
	// а VPN идёт через него же, нельзя было бы даже проверить.
	TolkoSmotrit bool
}

// Пределы. Переменные, а не константы: тесты сжимают паузы до миллисекунд.
var (
	// Одна попытка: соединение, рукопожатие и вход.
	srokPopytki = 30 * time.Second
	// Повтор только при обрыве до входа (пробы 01.11: сброс по дороге через
	// туннель 1 из 30). Отказ входа не повторяется никогда: это бан.
	pauzyPovtora = []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second}
	// Пульс соединения: без него мёртвый путь держал бы команду до её срока.
	intervalPulsa = 15 * time.Second
	// Проверки сразу после входа: uid, sudo, путь туннеля.
	srokProverok = 20 * time.Second
	// Команде без своего срока в ctx.
	srokKomandy = 10 * time.Minute
)

const (
	predelVyvoda  = 1 << 20
	predelOshibok = 16 << 10
	// Сколько интервалов пульс ждёт ответа, как ServerAliveCountMax 3 у
	// OpenSSH. Сразу после входа sshd сотни миллисекунд занят сеансом (PAM,
	// systemd-logind) и не отвечает ни на пульс, ни на открытие сессии
	// (живая проверка 01.11): ложное закрытие посреди установки дороже, чем
	// лишние полминуты на мёртвом пути.
	propuskovPulsa = 3
)

// Soedinenie это одно соединение на одну операцию.
type Soedinenie struct {
	KlyuchHosta          KlyuchHosta
	KlyuchHostaSmenilsya bool
	// Root: вошли root. Иначе команды идут через sudo.
	Root bool
	Put  string

	klient    *gossh.Client
	prava     prava
	parolSudo string
	stop      chan struct{}
	zakryto   sync.Once
	// Куда входили: укладка ключа root проверяет его новым входом тем же
	// путём. Пароля здесь нет.
	adres string
	port  int
	put   Put
	// otkuda это адрес, с которого сервер видит подключение (SSH_CLIENT),
	// svoi это адреса самого сервера (hostname -I).
	otkuda netip.Addr
	svoi   []string
	// pustoyShum это пустые строки, которые оболочка входа печатает раньше
	// каждой команды: zapustit срезает их с начала вывода.
	pustoyShum []byte
	// pulsZakryl: соединение закрыл пульс, сервер не ответил за интервал.
	// Обрыв тогда называет эту причину, а не голое «оборвалось».
	pulsZakryl atomic.Bool
}

type prava int

const (
	pravaRoot prava = iota
	// sudo без пароля.
	pravaSudo
	// sudo с паролем входа. Так идут только осмотр и укладка нашего ключа
	// root: ввод команды занят паролем, поэтому файлов здесь нет.
	pravaSudoSParolem
)

// Podklyuchit входит на сервер. Пароль после входа не хранится, кроме
// случая sudo с паролем: там он нужен до укладки ключа root.
func Podklyuchit(ctx context.Context, z Zapros) (s *Soedinenie, err error) {
	var k *gossh.Client
	// Отказ, в том числе из паники, не оставляет за собой соединение и
	// пульс. Стоит первым, чтобы сработать после zashchita.
	defer func() {
		if err == nil {
			return
		}
		if s != nil {
			s.Zakryt()
		} else if k != nil {
			k.Close()
		}
		s = nil
	}()
	defer zashchita("Podklyuchit", &err)
	if err := z.Vhod.proverit(); err != nil {
		return nil, err
	}
	if z.Put.Dial == nil {
		return nil, otkaz(PrichinaSboy, "путь до сервера не задан", nil)
	}
	port := z.Port
	if port == 0 {
		port = 22
	}
	adres := strings.TrimSpace(z.Adres)
	if !godnyyAdres(adres) || port < 1 || port > 65535 {
		return nil, otkaz(PrichinaSboy, fmt.Sprintf("негодный адрес %q или порт %d", Obrezat(adres, 80), port), nil)
	}
	server := net.JoinHostPort(adres, strconv.Itoa(port))
	if zhdat := obshchiyZaslon.mozhno(server); zhdat > 0 {
		o := otkaz(PrichinaRanoPovtoryat, server, nil)
		o.Podozhdat = int(math.Ceil(zhdat.Seconds()))
		return nil, o
	}
	podpis := z.Vhod.Podpis
	if len(z.Vhod.Klyuch) > 0 {
		if podpis, err = podpisKlyucha(z.Vhod.Klyuch, z.Vhod.ParolKlyucha); err != nil {
			return nil, err
		}
	}

	var sv *sverkaKlyuchaHosta
	for i := 0; ; i++ {
		var povtor bool
		k, sv, povtor, err = popytka(ctx, z, server, podpis)
		if err == nil {
			break
		}
		if !povtor || i == len(pauzyPovtora) {
			return nil, err
		}
		t := time.NewTimer(pauzyPovtora[i])
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, otkaz(PrichinaSrok, server, ctx.Err())
		case <-t.C:
		}
	}
	obshchiyZaslon.udacha(server)
	if sv.predyavlen == nil {
		return nil, otkaz(PrichinaSboy, server+": вход без ключа хоста", nil)
	}

	s = &Soedinenie{
		KlyuchHosta:          *sv.predyavlen,
		KlyuchHostaSmenilsya: sv.smenilsya,
		Put:                  z.Put.Imya,
		klient:               k,
		stop:                 make(chan struct{}),
		adres:                adres,
		port:                 port,
		put:                  z.Put,
	}
	fon.Zapustit("svoyvps/ssh.puls", s.puls)
	pctx, otmena := context.WithTimeout(ctx, srokProverok)
	defer otmena()
	if err := s.uznatPrava(pctx, z.Vhod.Parol); err != nil {
		return nil, err
	}
	if z.Put.Tunnel && !z.TolkoSmotrit {
		if err := s.proveritTunnel(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Имя хоста: буквы, цифры, точки и дефисы, без них по краям.
var imyaHosta = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// godnyyAdres: IP или имя хоста. Адрес с опечаткой иначе дошёл бы до сети и
// вернулся «сервер молчит».
func godnyyAdres(adres string) bool {
	if _, err := netip.ParseAddr(adres); err == nil {
		return true
	}
	return imyaHosta.MatchString(adres)
}

// popytka это одно соединение до входа включительно. povtor значит обрыв до
// входа, который стоит повторить.
func popytka(ctx context.Context, z Zapros, server string, podpis gossh.Signer) (
	k *gossh.Client, sv *sverkaKlyuchaHosta, povtor bool, err error) {
	pctx, otmena := context.WithTimeout(ctx, srokPopytki)
	defer otmena()

	soed, err := z.Put.Dial(pctx, "tcp", server)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return nil, nil, false, otkaz(PrichinaSrok, server, ctx.Err())
		case netVhodaTunnelya(err):
			return nil, nil, false, otkaz(PrichinaNetVhodaTunnelya, z.Put.Imya, err)
		case otkazPorta(err):
			return nil, nil, false, otkaz(PrichinaPortZakryt, server, err)
		}
		return nil, nil, false, otkaz(PrichinaTishina, server+" "+z.Put.Imya, err)
	}
	sch := &schyotchik{Conn: soed}
	stop := context.AfterFunc(pctx, func() { soed.Close() })

	sv = &sverkaKlyuchaHosta{izvesten: z.Izvesten, odobren: z.Odobren}
	vb := &vybor{parol: z.Vhod.Parol, podpis: podpis}
	konfig := &gossh.ClientConfig{
		User:              z.Vhod.Polzovatel,
		HostKeyCallback:   sv.proverit,
		HostKeyAlgorithms: algoritmyKlyuchaHosta(z.Izvesten),
		AuthCallback:      vb.sleduyushchiy,
	}
	c, kanaly, zaprosy, err := gossh.NewClientConn(sch, server, konfig)
	// Вход состоялся: дальше соединение живёт своими сессиями, у каждой свой
	// срок. Если срок попытки сработал в ту же минуту, соединение уже
	// закрыто, и отдавать его нельзя.
	if !stop() && err == nil {
		c.Close()
		return nil, nil, true, otkaz(PrichinaObryv, server+": срок вышел на самом входе", pctx.Err())
	}
	if err == nil {
		return gossh.NewClient(c, kanaly, zaprosy), sv, false, nil
	}
	soed.Close()

	var o *Otkaz
	switch {
	case ctx.Err() != nil:
		return nil, nil, false, otkaz(PrichinaSrok, server, ctx.Err())
	case vb.dostignut:
		// Неудача, где пароль или ключ серверу ушёл, считается и сервером, и
		// заслоном. Свой отказ без попытки (сервер не берёт этот способ)
		// сервер неудачей не считает.
		if vb.poprobovano {
			obshchiyZaslon.neudacha(server)
		}
		switch {
		case errors.As(err, &o):
			return nil, nil, false, o
		case pctx.Err() != nil:
			return nil, nil, false, otkaz(PrichinaObryv, server+": срок попытки вышел посреди входа", err)
		}
		return nil, nil, false, otkaz(PrichinaObryv, server+": оборвалось на входе", err)
	case errors.As(err, &o):
		// Ключ хоста: до входа дело не дошло.
		return nil, nil, false, o
	case sv.predyavlen != nil:
		return nil, nil, false, otkaz(PrichinaObryv, server+": оборвалось после обмена ключами", err)
	case sch.prochitano() > 0 && !sch.bylSSH():
		// Раньше срока попытки: сервис, ответивший не SSH и державший
		// соединение (HTTP keep-alive), иначе выглядел бы молчанием.
		return nil, nil, false, otkaz(PrichinaNeSSH, server, err)
	case pctx.Err() != nil:
		// Соединение открылось, а приветствия нет до срока. Это не обрыв, и
		// повтор длиной ещё в полминуты ничего не даст.
		return nil, nil, false, otkaz(PrichinaTishina, server+" "+z.Put.Imya+": соединение открылось, а сервер молчит", err)
	case sch.prochitano() == 0 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)):
		// Через туннель ядро принимает соединение у себя и закрывает его без
		// единого байта, если сервер не ответил: это молчание сервера, а не
		// штраф.
		if z.Put.Tunnel {
			return nil, nil, true, otkaz(PrichinaTishina, server+" "+z.Put.Imya+": закрыто без приветствия", err)
		}
		return nil, nil, true, otkaz(PrichinaPodozhdi, server+": закрыто без приветствия", err)
	}
	return nil, nil, true, otkaz(PrichinaObryv, server+": оборвалось до входа", err)
}

// schyotchik помнит начало потока от сервера: по нему видно, ответил ли
// сервер хоть что-то и было ли это SSH.
//
// И держит нашу строку версии, пока не придёт первый байт сервера. Штраф
// OpenSSH 9.8 и новее закрывает соединение сразу после accept, не читая.
// Успей наша строка дойти раньше, закрытие с непрочитанными данными ушло бы
// сбросом (RST), и «подожди минуту» выглядело бы обрывом по дороге. RFC 4253
// разрешает клиенту ждать строку сервера; OpenSSH сервер шлёт её первым.
type schyotchik struct {
	net.Conn
	n          atomic.Int64
	mu         sync.Mutex
	nachalo    []byte
	otlozheno  []byte
	otpravleno bool
}

const dlinaNachala = 2048

func (s *schyotchik) Write(b []byte) (int, error) {
	s.mu.Lock()
	if !s.otpravleno {
		s.otlozheno = append(s.otlozheno, b...)
		s.mu.Unlock()
		return len(b), nil
	}
	s.mu.Unlock()
	return s.Conn.Write(b)
}

func (s *schyotchik) Read(b []byte) (int, error) {
	n, err := s.Conn.Read(b)
	if n > 0 {
		s.n.Add(int64(n))
		s.mu.Lock()
		if ostalos := dlinaNachala - len(s.nachalo); ostalos > 0 {
			s.nachalo = append(s.nachalo, b[:min(n, ostalos)]...)
		}
		// Отложенное уходит под замком: запись из другой горутины не
		// обгонит строку версии.
		if !s.otpravleno {
			s.otpravleno = true
			if len(s.otlozheno) > 0 {
				if _, werr := s.Conn.Write(s.otlozheno); werr != nil && err == nil {
					err = werr
				}
			}
			s.otlozheno = nil
		}
		s.mu.Unlock()
	}
	return n, err
}

func (s *schyotchik) prochitano() int64 { return s.n.Load() }

func (s *schyotchik) bylSSH() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.HasPrefix(s.nachalo, []byte("SSH-")) || bytes.Contains(s.nachalo, []byte("\nSSH-"))
}

// puls шлёт keepalive раз в intervalPulsa. Нет ответа за propuskovPulsa
// интервалов - соединение закрывается, и команда на нём кончается обрывом, а
// не своим сроком в минуты.
func (s *Soedinenie) puls() {
	t := time.NewTicker(intervalPulsa)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
		}
		srok := time.AfterFunc(propuskovPulsa*intervalPulsa, func() {
			s.pulsZakryl.Store(true)
			s.klient.Close()
		})
		_, _, err := s.klient.SendRequest("keepalive@openssh.com", true, nil)
		srok.Stop()
		if err != nil {
			return
		}
	}
}

// Zakryt закрывает соединение. Повторный вызов ничего не делает.
func (s *Soedinenie) Zakryt() {
	s.zakryto.Do(func() {
		close(s.stop)
		s.klient.Close()
	})
}

// Itog это исход команды. Ненулевой код выхода ошибкой не считается: по нему
// решает вызывающий (sudo, осмотр).
type Itog struct {
	// Vyvod это stdout, не больше мегабайта.
	Vyvod []byte
	// Oshibki это хвост stderr, не больше 16 КБ.
	Oshibki string
	// Kod выхода, -1 если команду убил сигнал.
	Kod int
}

// Vypolnit выполняет команду с правами root: как есть, если вошли root,
// иначе через sudo. Команда идёт в bash -c с LC_ALL=C: ответы утилит
// разбираются по тексту и не должны зависеть от языка сервера.
func (s *Soedinenie) Vypolnit(ctx context.Context, komanda string, vhod io.Reader) (it Itog, err error) {
	// Паника оставит Kod -1, а не нулевой «успех».
	it.Kod = -1
	defer zashchita("Vypolnit", &err)
	stroka, stdin, err := s.obernut("bash -c "+kavychki("export LC_ALL=C\n"+komanda), vhod)
	if err != nil {
		return Itog{Kod: -1}, err
	}
	return s.zapustit(ctx, stroka, stdin)
}

// predelSkriptaSudo: скрипт под sudo с паролем идёт аргументом bash -c, а
// одна строка аргумента в Linux не длиннее 128 КБ.
const predelSkriptaSudo = 100 << 10

// Skript выполняет скрипт bash с правами root. Скрипт идёт в stdin
// `bash -s`, а не `sh`: на Ubuntu sh это dash. Поэтому сам скрипт stdin не
// читает. Под sudo с паролем stdin занят паролем, и скрипт идёт аргументом.
func (s *Soedinenie) Skript(ctx context.Context, tekst string, argumenty ...string) (it Itog, err error) {
	it.Kod = -1
	defer zashchita("Skript", &err)
	tekst = "export LC_ALL=C\n" + tekst
	args := make([]string, len(argumenty))
	for i, a := range argumenty {
		args[i] = kavychki(a)
	}
	hvostArg := strings.Join(args, " ")
	var stroka string
	var stdin io.Reader
	if s.prava == pravaSudoSParolem {
		if len(tekst) > predelSkriptaSudo {
			return Itog{Kod: -1}, otkaz(PrichinaSboy, fmt.Sprintf("скрипт %d байт длиннее предела sudo с паролем", len(tekst)), nil)
		}
		stroka, stdin, err = s.obernut("bash -c "+kavychki(tekst)+" bash "+hvostArg, nil)
	} else {
		stroka, stdin, err = s.obernut("bash -s -- "+hvostArg, strings.NewReader(tekst))
	}
	if err != nil {
		return Itog{Kod: -1}, err
	}
	return s.zapustit(ctx, stroka, stdin)
}

// obernut ставит перед командой sudo, если вошли не root.
func (s *Soedinenie) obernut(komanda string, vhod io.Reader) (string, io.Reader, error) {
	switch s.prava {
	case pravaRoot:
		return komanda, vhod, nil
	case pravaSudo:
		return "sudo -n -- " + komanda, vhod, nil
	case pravaSudoSParolem:
		if vhod != nil {
			return "", nil, otkaz(PrichinaSboy, "под sudo с паролем ввод команды занят паролем", nil)
		}
		return "sudo -k -S -p '' -- " + komanda, strings.NewReader(s.parolSudo + "\n"), nil
	}
	return "", nil, otkaz(PrichinaSboy, fmt.Sprintf("неизвестные права %d", s.prava), nil)
}

// syroe выполняет команду оболочкой входа, без sudo и обёртки: так идут
// проверки сразу после входа.
func (s *Soedinenie) syroe(ctx context.Context, komanda string, vhod io.Reader) (Itog, error) {
	return s.zapustit(ctx, komanda, vhod)
}

func (s *Soedinenie) zapustit(ctx context.Context, stroka string, stdin io.Reader) (Itog, error) {
	it := Itog{Kod: -1}
	if _, est := ctx.Deadline(); !est {
		var otmena context.CancelFunc
		ctx, otmena = context.WithTimeout(ctx, srokKomandy)
		defer otmena()
	}
	sess, err := s.klient.NewSession()
	if err != nil {
		return it, otkaz(PrichinaObryv, s.chtoOborvalos("сессия не открылась"), err)
	}
	defer sess.Close()
	stop := context.AfterFunc(ctx, func() { sess.Close() })
	defer stop()

	vyvod := &golova{predel: predelVyvoda}
	oshibki := &hvost{predel: predelOshibok}
	sess.Stdout, sess.Stderr, sess.Stdin = vyvod, oshibki, stdin
	err = sess.Run(stroka)
	it.Vyvod, it.Oshibki = bytes.TrimPrefix(vyvod.bayty(), s.pustoyShum), oshibki.stroka()

	var vyhod *gossh.ExitError
	switch {
	case ctx.Err() != nil:
		return it, otkaz(PrichinaSrok, "команда не уложилась в срок", ctx.Err())
	case err == nil:
		it.Kod = 0
	case errors.As(err, &vyhod):
		if vyhod.Signal() == "" {
			it.Kod = vyhod.ExitStatus()
		}
	default:
		return it, otkaz(PrichinaObryv, s.chtoOborvalos("команда оборвалась"), err)
	}
	if vyvod.perepolnen() {
		return it, otkaz(PrichinaSboy, "вывод команды больше мегабайта", nil)
	}
	return it, nil
}

func (s *Soedinenie) chtoOborvalos(chto string) string {
	if s.pulsZakryl.Load() {
		return fmt.Sprintf("%s: сервер не ответил на пульс за %v", chto, propuskovPulsa*intervalPulsa)
	}
	return chto
}

// golova копит начало stdout до предела и помнит, что был перебор.
type golova struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	predel  int
	lishnee bool
}

func (g *golova) Write(b []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ostalos := g.predel - g.buf.Len()
	if len(b) > ostalos {
		g.lishnee = true
		g.buf.Write(b[:max(ostalos, 0)])
	} else {
		g.buf.Write(b)
	}
	return len(b), nil
}

func (g *golova) bayty() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return bytes.Clone(g.buf.Bytes())
}

func (g *golova) perepolnen() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lishnee
}

// hvost держит конец stderr: причина отказа обычно в последних строках.
type hvost struct {
	mu     sync.Mutex
	buf    []byte
	predel int
}

func (h *hvost) Write(b []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, b...)
	if len(h.buf) > h.predel {
		h.buf = append([]byte(nil), h.buf[len(h.buf)-h.predel:]...)
	}
	return len(b), nil
}

func (h *hvost) stroka() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.ToValidUTF8(string(h.buf), "?")
}

// kavychki берёт строку в одинарные кавычки POSIX.
func kavychki(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Путь на сервере: только простые символы, чтобы его можно было вставить в
// команду без сюрпризов.
var godnyyPut = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

func proveritPut(put string) error {
	if !godnyyPut.MatchString(put) || strings.HasSuffix(put, "/") {
		return otkaz(PrichinaSboy, fmt.Sprintf("негодный путь на сервере %q", put), nil)
	}
	for _, chast := range strings.Split(put[1:], "/") {
		if chast == "" || chast == "." || chast == ".." {
			return otkaz(PrichinaSboy, fmt.Sprintf("негодный путь на сервере %q", put), nil)
		}
	}
	return nil
}

// skriptFayla кладёт stdin во временный файл рядом с целью, сверяет sha256 и
// только потом переносит на место: на uutils 0.8.0 (Ubuntu 26.04)
// `install -m 0644 /dev/stdin <существующий>` падает и оставляет старый файл
// (uutils #11532). Обрыв посреди передачи оставляет прежний файл целым, а
// временный убирает ловушка.
const skriptFayla = `set -e
t=%s
trap 'rm -f -- "$t"' EXIT
umask 077
cat > "$t"
chmod %04o -- "$t"
h=$(sha256sum -- "$t")
h=${h%%%% *}
if [ "$h" != %s ]; then echo "на сервере sha256 $h" >&2; exit 3; fi
mv -f -- "$t" %s`

// OtpravitFayl кладёт файл на сервер с правами rezhim и сверяет sha256 на
// месте. Файл, который уже лежит с тем же sha256, не передаётся заново:
// повтор после обрыва докачивает только несошедшиеся. dannye читается с
// начала, где бы ни стоял: повтор отдаёт тот же читатель, уже дочитанный.
func (s *Soedinenie) OtpravitFayl(ctx context.Context, dannye io.ReadSeeker, kuda string, rezhim fs.FileMode) (err error) {
	defer zashchita("OtpravitFayl", &err)
	if s.prava == pravaSudoSParolem {
		return otkaz(PrichinaSboy, "файлы кладутся только после входа ключом root", nil)
	}
	if err := proveritPut(kuda); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := dannye.Seek(0, io.SeekStart); err != nil {
		return otkaz(PrichinaSboy, "файл не перемотан", err)
	}
	if _, err := io.Copy(h, dannye); err != nil {
		return otkaz(PrichinaSboy, "файл не прочитан", err)
	}
	if _, err := dannye.Seek(0, io.SeekStart); err != nil {
		return otkaz(PrichinaSboy, "файл не перемотан", err)
	}
	nado := hex.EncodeToString(h.Sum(nil))

	it, err := s.Vypolnit(ctx, fmt.Sprintf("if [ -f %[1]s ]; then sha256sum -- %[1]s; fi", kavychki(kuda)), nil)
	if err != nil {
		return err
	}
	if fakt, _, _ := strings.Cut(strings.TrimSpace(string(it.Vyvod)), " "); it.Kod == 0 && fakt == nado {
		it, err = s.Vypolnit(ctx, fmt.Sprintf("chmod %04o -- %s", rezhim.Perm(), kavychki(kuda)), nil)
		if err != nil {
			return err
		}
		if it.Kod != 0 {
			return otkaz(PrichinaSboy, fmt.Sprintf("chmod %s: код %d: %s", kuda, it.Kod, PoslednyayaStroka(it.Oshibki)), nil)
		}
		return nil
	}
	it, err = s.Vypolnit(ctx, fmt.Sprintf(skriptFayla, kavychki(kuda+".affory-chast"), rezhim.Perm(), kavychki(nado), kavychki(kuda)), dannye)
	if err != nil {
		return err
	}
	if it.Kod != 0 {
		return otkaz(PrichinaSboy, fmt.Sprintf("файл %s: код %d: %s", kuda, it.Kod, PoslednyayaStroka(it.Oshibki)), nil)
	}
	return nil
}

// PoslednyayaStroka берёт из stderr сервера последнюю строку, чистит её от
// управляющих символов и режет до 200 знаков: так она годится в журнал.
func PoslednyayaStroka(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return Obrezat(s, 200)
}
