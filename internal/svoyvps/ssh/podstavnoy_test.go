package ssh

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// Подставной SSH-сервер на x/crypto/ssh. Команды он не исполняет, а
// узнаёт: пробы прав и путей слоя по точной строке, скрипты слоя по
// содержимому после снятия обёртки sudo и bash. Сами скрипты bash проверяются
// живьём на машинах, здесь проверяется разговор слоя с сервером.

type sudoRezhim int

const (
	sudoNet sudoRezhim = iota
	sudoBezParolya
	sudoSParolem
	sudoNeVSudoers
)

type nastroyki struct {
	// Кто входит паролем: пользователь -> пароль. Пусто - password не
	// предлагается.
	paroli map[string]string
	// Тот же словарь через keyboard-interactive.
	paroliKI map[string]string
	// Кто входит ключом.
	klyuchi map[string]gossh.PublicKey
	// Просроченный пароль: вход проходит, команды просят сменить пароль.
	prosrochen bool
	// Просроченный пароль через keyboard-interactive: после верного пароля
	// PAM просит текущий и новый.
	prosrochenKI bool
	uid          map[string]string
	sudo         sudoRezhim
	// Что отвечает sshd -T на permitrootlogin.
	permitRootLogin string
	// Какой адрес сервер называет в SSH_CLIENT; пусто - настоящий.
	sshClient string
	// Что печатает hostname -I.
	adresaServera string
	// Первые столько подключений закрываются без приветствия.
	zakrytBezPrivetstviya int
	// Первые столько подключений сбрасываются (RST), как рвёт путь.
	sbrosBezPrivetstviya int
	// Подключения не получают ни байта, пока клиент не уйдёт сам.
	molchat bool
	// Вместо SSH отвечает HTTP. С molchat держит соединение после ответа.
	neSSH bool
	// Не отвечать на keepalive: путь умер, а TCP этого не знает.
	gluhoyPuls bool
	// Ключи хоста; пусто - один свежий ed25519.
	klyuchiHosta []gossh.Signer
	// root не входит ключом, даже лежащим в его authorized_keys: так ведут
	// себя AllowUsers без root и свой AuthorizedKeysFile.
	rootKlyuchomNelzya bool
	// Уборка строки ключа из authorized_keys root отказывает.
	uborkaPadaet bool
	// Что оболочка входа печатает раньше любой команды (echo в .bashrc).
	shumVhoda string
}

const putKlyuchaRoot = "/root/.ssh/authorized_keys"

type zapis struct {
	polzovatel string
	komanda    string
}

type fayl struct {
	dannye []byte
	rezhim string
}

type podstavnoy struct {
	t  *testing.T
	ln net.Listener
	n  nastroyki

	mu             sync.Mutex
	komandy        []zapis
	popytkiParolya int
	popytkiKI      int
	podklyucheniy  int
	fs             map[string]fayl
	otkrytye       map[net.Conn]bool
	wg             sync.WaitGroup
}

func novyyKlyuch(t *testing.T) gossh.Signer {
	t.Helper()
	_, zakrytyy, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p, err := gossh.NewSignerFromKey(zakrytyy)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func zapustitPodstavnoy(t *testing.T, n nastroyki) *podstavnoy {
	t.Helper()
	if len(n.klyuchiHosta) == 0 {
		n.klyuchiHosta = []gossh.Signer{novyyKlyuch(t)}
	}
	// Заслон помнит сервер по адресу и порту, а порт закрытого подставного
	// система отдаёт следующему: чужие неудачи не должны доставаться ему.
	obshchiyZaslon.mu.Lock()
	obshchiyZaslon.zapisi = map[string]neudachi{}
	obshchiyZaslon.mu.Unlock()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &podstavnoy{t: t, ln: ln, n: n, fs: map[string]fayl{}, otkrytye: map[net.Conn]bool{}}
	p.wg.Add(1)
	go p.prinimat()
	t.Cleanup(func() {
		ln.Close()
		p.mu.Lock()
		for c := range p.otkrytye {
			c.Close()
		}
		p.mu.Unlock()
		p.wg.Wait()
	})
	return p
}

func (p *podstavnoy) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *podstavnoy) zapros(v Vhod) Zapros {
	return Zapros{Adres: "127.0.0.1", Port: p.port(), Vhod: v, Put: Put{Imya: "тест", Dial: (&net.Dialer{}).DialContext}}
}

func (p *podstavnoy) zapisi() []zapis {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]zapis(nil), p.komandy...)
}

func (p *podstavnoy) schyot() (paroli, ki, podkl int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.popytkiParolya, p.popytkiKI, p.podklyucheniy
}

func (p *podstavnoy) prinimat() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.podklyucheniy++
		nomer := p.podklyucheniy
		p.otkrytye[c] = true
		p.mu.Unlock()
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer func() {
				c.Close()
				p.mu.Lock()
				delete(p.otkrytye, c)
				p.mu.Unlock()
			}()
			p.obsluzhit(c, nomer)
		}()
	}
}

func (p *podstavnoy) konfig() *gossh.ServerConfig {
	k := &gossh.ServerConfig{}
	if p.n.paroli != nil {
		k.PasswordCallback = func(m gossh.ConnMetadata, parol []byte) (*gossh.Permissions, error) {
			p.mu.Lock()
			p.popytkiParolya++
			p.mu.Unlock()
			if nado, est := p.n.paroli[m.User()]; est && nado == string(parol) {
				return nil, nil
			}
			return nil, errors.New("неверный пароль")
		}
	}
	if p.n.paroliKI != nil {
		k.KeyboardInteractiveCallback = func(m gossh.ConnMetadata, vopros gossh.KeyboardInteractiveChallenge) (*gossh.Permissions, error) {
			p.mu.Lock()
			p.popytkiKI++
			p.mu.Unlock()
			otvet, err := vopros("", "", []string{"Password: "}, []bool{false})
			if err != nil {
				return nil, err
			}
			if nado, est := p.n.paroliKI[m.User()]; !est || len(otvet) != 1 || otvet[0] != nado {
				return nil, errors.New("неверный пароль")
			}
			if p.n.prosrochenKI {
				_, err := vopros("", "You are required to change your password immediately (administrator enforced)",
					[]string{"Current password: "}, []bool{false})
				if err != nil {
					return nil, err
				}
				return nil, errors.New("пароль не сменён")
			}
			return nil, nil
		}
	}
	// Ключи предлагаются всегда, как у sshd: root входит и тем, что лежит
	// в его authorized_keys.
	k.PublicKeyCallback = func(m gossh.ConnMetadata, klyuch gossh.PublicKey) (*gossh.Permissions, error) {
		if nado, est := p.n.klyuchi[m.User()]; est && string(nado.Marshal()) == string(klyuch.Marshal()) {
			return nil, nil
		}
		if m.User() == "root" && !p.n.rootKlyuchomNelzya {
			telo := strings.Fields(string(gossh.MarshalAuthorizedKey(klyuch)))[1]
			if f, est := p.fayl(putKlyuchaRoot); est && strings.Contains(string(f.dannye), telo) {
				return nil, nil
			}
		}
		return nil, errors.New("ключ не тот")
	}
	for _, kh := range p.n.klyuchiHosta {
		k.AddHostKey(kh)
	}
	return k
}

func (p *podstavnoy) obsluzhit(c net.Conn, nomer int) {
	switch {
	case nomer <= p.n.zakrytBezPrivetstviya:
		return
	case nomer <= p.n.sbrosBezPrivetstviya:
		c.(*net.TCPConn).SetLinger(0)
		return
	case p.n.neSSH:
		io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		if p.n.molchat {
			io.Copy(io.Discard, c)
		}
		return
	case p.n.molchat:
		io.Copy(io.Discard, c)
		return
	}
	sc, kanaly, zaprosy, err := gossh.NewServerConn(c, p.konfig())
	if err != nil {
		return
	}
	defer sc.Close()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for z := range zaprosy {
			if !p.n.gluhoyPuls {
				z.Reply(false, nil)
			}
		}
	}()
	for nc := range kanaly {
		if nc.ChannelType() != "session" {
			nc.Reject(gossh.UnknownChannelType, "только session")
			continue
		}
		ch, zap, err := nc.Accept()
		if err != nil {
			return
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer ch.Close()
			for z := range zap {
				if z.Type != "exec" {
					z.Reply(false, nil)
					continue
				}
				var kom struct{ Komanda string }
				if err := gossh.Unmarshal(z.Payload, &kom); err != nil {
					z.Reply(false, nil)
					return
				}
				z.Reply(true, nil)
				kod := p.vypolnit(c, sc, kom.Komanda, ch)
				if kod < 0 {
					return
				}
				ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Kod uint32 }{uint32(kod)}))
				return
			}
		}()
	}
}

// snyatKavychki разбирает начало s как слово в одинарных кавычках POSIX,
// склеенное через '\”.
func snyatKavychki(s string) (slovo, ostatok string, ok bool) {
	var b strings.Builder
	for strings.HasPrefix(s, "'") {
		j := strings.IndexByte(s[1:], '\'')
		if j < 0 {
			return "", "", false
		}
		b.WriteString(s[1 : 1+j])
		s = s[2+j:]
		if !strings.HasPrefix(s, `\'`) {
			return b.String(), s, true
		}
		b.WriteByte('\'')
		s = s[2:]
	}
	return "", "", false
}

func razobratArgumenty(s string) []string {
	var args []string
	s = strings.TrimSpace(s)
	for s != "" {
		a, ost, ok := snyatKavychki(s)
		if !ok {
			args = append(args, strings.Fields(s)...)
			break
		}
		args = append(args, a)
		s = strings.TrimSpace(ost)
	}
	return args
}

var (
	reVremennyy = regexp.MustCompile(`(?m)^t='([^']+)'$`)
	reRezhim    = regexp.MustCompile(`chmod (\d+) -- "\$t"`)
	reNado      = regexp.MustCompile(`if \[ "\$h" != '([0-9a-f]+)' \]`)
	reKuda      = regexp.MustCompile(`mv -f -- "\$t" '([^']+)'`)
	reEsli      = regexp.MustCompile(`^if \[ -f '([^']+)' \]; then sha256sum -- '([^']+)'; fi$`)
	reChmod     = regexp.MustCompile(`^chmod (\d+) -- '([^']+)'$`)
)

// vypolnit отвечает на команду. Отрицательный код значит «соединение
// оборвано, статуса не будет».
func (p *podstavnoy) vypolnit(c net.Conn, sc *gossh.ServerConn, komanda string, ch gossh.Channel) int {
	p.mu.Lock()
	p.komandy = append(p.komandy, zapis{sc.User(), komanda})
	p.mu.Unlock()
	vhod := bufio.NewReader(ch)
	vyvod := func(s string) { io.WriteString(ch, s) }
	oshibka := func(s string) { io.WriteString(ch.Stderr(), s) }
	vyvod(p.n.shumVhoda)

	if p.n.prosrochen {
		oshibka("WARNING: Your password has expired.\nPassword change required but no TTY available.\n")
		return 1
	}
	otkuda := p.n.sshClient
	if otkuda == "" {
		otkuda, _, _ = net.SplitHostPort(sc.RemoteAddr().String())
	}
	switch komanda {
	case komandaProbyVhoda:
		uid, est := p.n.uid[sc.User()]
		if !est {
			uid = "0"
		}
		vyvod(metkaProby + "\n" + uid + "\n" + otkuda + "\n" + p.n.adresaServera + "\n")
		return 0
	case "sudo -n true":
		switch p.n.sudo {
		case sudoNet:
			oshibka("bash: sudo: command not found\n")
			return 127
		case sudoBezParolya:
			return 0
		}
		oshibka("sudo: a password is required\n")
		return 1
	case "sudo -k -S -p '' true":
		return p.sudoSParolem(sc.User(), vhod, oshibka)
	}

	// Обёртка прав.
	switch {
	case strings.HasPrefix(komanda, "sudo -n -- "):
		if p.n.sudo != sudoBezParolya {
			oshibka("sudo: a password is required\n")
			return 1
		}
		komanda = strings.TrimPrefix(komanda, "sudo -n -- ")
	case strings.HasPrefix(komanda, "sudo -k -S -p '' -- "):
		if kod := p.sudoSParolem(sc.User(), vhod, oshibka); kod != 0 {
			return kod
		}
		komanda = strings.TrimPrefix(komanda, "sudo -k -S -p '' -- ")
	default:
		if p.n.uid[sc.User()] != "" && p.n.uid[sc.User()] != "0" {
			oshibka("команда root без sudo у не-root: " + komanda + "\n")
			return 126
		}
	}

	var skript string
	var args []string
	switch {
	case strings.HasPrefix(komanda, "bash -c "):
		s, ost, ok := snyatKavychki(strings.TrimPrefix(komanda, "bash -c "))
		if !ok {
			oshibka("не разобрать bash -c\n")
			return 2
		}
		skript = s
		args = razobratArgumenty(strings.TrimPrefix(strings.TrimSpace(ost), "bash"))
	case strings.HasPrefix(komanda, "bash -s -- "):
		b, err := io.ReadAll(vhod)
		if err != nil {
			return -1
		}
		skript = string(b)
		args = razobratArgumenty(strings.TrimPrefix(komanda, "bash -s -- "))
	default:
		oshibka("неизвестная команда: " + komanda + "\n")
		return 127
	}
	if !strings.HasPrefix(skript, "export LC_ALL=C\n") {
		oshibka("скрипт без LC_ALL=C\n")
		return 2
	}
	skript = strings.TrimPrefix(skript, "export LC_ALL=C\n")

	switch {
	case strings.Contains(skript, "permitrootlogin"):
		vyvod(p.n.permitRootLogin + "\n")
		return 0
	case strings.Contains(skript, "authorized_keys"):
		if len(args) != 1 {
			oshibka(fmt.Sprintf("ждала одну строку ключа, пришло %d\n", len(args)))
			return 2
		}
		telo := strings.Fields(args[0])[1]
		p.mu.Lock()
		defer p.mu.Unlock()
		f, est := p.fs[putKlyuchaRoot]
		if strings.Contains(skript, "grep -vF") {
			// Уборка: строки с телом ключа долой, остальные на месте.
			if p.n.uborkaPadaet {
				oshibka("cp: cannot create regular file: Read-only file system\n")
				return 1
			}
			if !est {
				return 0
			}
			var ostalos []string
			for _, s := range strings.SplitAfter(string(f.dannye), "\n") {
				if s != "" && !strings.Contains(s, telo) {
					ostalos = append(ostalos, s)
				}
			}
			f.dannye = []byte(strings.Join(ostalos, ""))
			p.fs[putKlyuchaRoot] = f
			return 0
		}
		if !strings.Contains(string(f.dannye), telo) {
			f.dannye = append(f.dannye, args[0]+"\n"...)
			vyvod("dobavlen\n")
		}
		if !est {
			f.rezhim = "600"
		}
		p.fs[putKlyuchaRoot] = f
		return 0
	case strings.Contains(skript, `cat > "$t"`):
		return p.prinyatFayl(skript, vhod, oshibka)
	case reEsli.MatchString(skript):
		m := reEsli.FindStringSubmatch(skript)
		p.mu.Lock()
		f, est := p.fs[m[1]]
		p.mu.Unlock()
		if est {
			h := sha256.Sum256(f.dannye)
			vyvod(hex.EncodeToString(h[:]) + "  " + m[1] + "\n")
		}
		return 0
	case reChmod.MatchString(skript):
		m := reChmod.FindStringSubmatch(skript)
		p.mu.Lock()
		f := p.fs[m[2]]
		f.rezhim = m[1]
		p.fs[m[2]] = f
		p.mu.Unlock()
		return 0
	case strings.HasPrefix(skript, "obryv"):
		vyvod("начало вывода\n")
		c.Close()
		return -1
	case strings.HasPrefix(skript, "zhdat"):
		// Висит, пока соединение не закроют.
		sc.Wait()
		return -1
	case strings.HasPrefix(skript, "echo "):
		vyvod(strings.TrimPrefix(skript, "echo ") + "\n")
		return 0
	case strings.HasPrefix(skript, "exit "):
		kod, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(skript, "exit ")))
		oshibka("выход с кодом\n")
		return kod
	case strings.HasPrefix(skript, "argumenty"):
		vyvod(strings.Join(args, "|") + "\n")
		return 0
	case strings.HasPrefix(skript, "mnogo"):
		vyvod(strings.Repeat("x", predelVyvoda+10))
		return 0
	}
	oshibka("неизвестный скрипт\n")
	return 127
}

func (p *podstavnoy) sudoSParolem(polzovatel string, vhod *bufio.Reader, oshibka func(string)) int {
	stroka, err := vhod.ReadString('\n')
	if err != nil && stroka == "" {
		oshibka("sudo: no password was provided\n")
		return 1
	}
	switch p.n.sudo {
	case sudoNet:
		oshibka("bash: sudo: command not found\n")
		return 127
	case sudoBezParolya:
		return 0
	case sudoNeVSudoers:
		oshibka(polzovatel + " is not in the sudoers file.\n")
		return 1
	}
	if strings.TrimSuffix(stroka, "\n") != p.n.paroli[polzovatel] {
		oshibka("Sorry, try again.\n")
		return 1
	}
	return 0
}

func (p *podstavnoy) prinyatFayl(skript string, vhod io.Reader, oshibka func(string)) int {
	vremennyy := reVremennyy.FindStringSubmatch(skript)
	rezhim := reRezhim.FindStringSubmatch(skript)
	nado := reNado.FindStringSubmatch(skript)
	kuda := reKuda.FindStringSubmatch(skript)
	if vremennyy == nil || rezhim == nil || nado == nil || kuda == nil {
		oshibka("скрипт файла не разобран\n")
		return 2
	}
	if vremennyy[1] != kuda[1]+".affory-chast" {
		oshibka("временный файл не рядом с целью\n")
		return 2
	}
	dannye, err := io.ReadAll(vhod)
	if err != nil {
		return -1
	}
	h := sha256.Sum256(dannye)
	if fakt := hex.EncodeToString(h[:]); fakt != nado[1] {
		oshibka("на сервере sha256 " + fakt + "\n")
		return 3
	}
	p.mu.Lock()
	p.fs[kuda[1]] = fayl{dannye: dannye, rezhim: rezhim[1]}
	p.mu.Unlock()
	return 0
}

func (p *podstavnoy) polozhit(put string, f fayl) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fs[put] = f
}

func (p *podstavnoy) fayl(put string) (fayl, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, est := p.fs[put]
	return f, est
}

func kontekst(t *testing.T) context.Context {
	t.Helper()
	ctx, otmena := context.WithTimeout(context.Background(), 20e9)
	t.Cleanup(otmena)
	return ctx
}
