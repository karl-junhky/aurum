package ssh

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	gossh "golang.org/x/crypto/ssh"
)

// Vhod это чем входить. Задан ровно один способ: пароль, ключ человека
// содержимым или подпись (ключ управления, который держит служба).
//
// Ключ человека приходит содержимым, а не путём: служба SYSTEM не читает
// файлы по просьбе процесса сеанса.
type Vhod struct {
	Polzovatel   string
	Parol        string
	Klyuch       []byte
	ParolKlyucha string
	Podpis       gossh.Signer
}

// predelKlyucha: файл ключа в десятки килобайт уже не ключ.
const predelKlyucha = 64 << 10

func (v Vhod) proverit() error {
	if strings.TrimSpace(v.Polzovatel) == "" || strings.ContainsAny(v.Polzovatel, " \t\r\n'\"") {
		return otkaz(PrichinaSboy, "пустое или негодное имя пользователя", nil)
	}
	zadano := 0
	for _, est := range []bool{v.Parol != "", len(v.Klyuch) > 0, v.Podpis != nil} {
		if est {
			zadano++
		}
	}
	if zadano != 1 {
		return otkaz(PrichinaSboy, fmt.Sprintf("способов входа задано %d, нужен один", zadano), nil)
	}
	return nil
}

// podpisKlyucha разбирает ключ человека: OpenSSH или PEM любого типа, пароль
// ключа только если он нужен.
func podpisKlyucha(klyuch []byte, parol string) (gossh.Signer, error) {
	if len(klyuch) > predelKlyucha {
		return nil, otkaz(PrichinaKlyuchNeChitaetsya, fmt.Sprintf("файл ключа больше %d КБ", predelKlyucha>>10), nil)
	}
	// Блокнот Windows сохраняет UTF-8 с BOM, а разбор PEM его не пропускает.
	klyuch = bytes.TrimPrefix(klyuch, []byte("\xef\xbb\xbf"))
	if bytes.HasPrefix(bytes.TrimSpace(klyuch), []byte("PuTTY-User-Key-File-")) {
		return nil, otkaz(PrichinaKlyuchPpk, "", nil)
	}
	p, err := gossh.ParsePrivateKey(klyuch)
	var nuzhenParol *gossh.PassphraseMissingError
	switch {
	case err == nil:
		return p, nil
	case !errors.As(err, &nuzhenParol):
		return nil, otkaz(PrichinaKlyuchNeChitaetsya, "", err)
	case parol == "":
		return nil, otkaz(PrichinaNuzhenParolKlyucha, "", nil)
	}
	p, err = gossh.ParsePrivateKeyWithPassphrase(klyuch, []byte(parol))
	switch {
	case errors.Is(err, x509.IncorrectPasswordError):
		return nil, otkaz(PrichinaParolKlyuchaNeveren, "", nil)
	case err != nil:
		return nil, otkaz(PrichinaKlyuchNeChitaetsya, "", err)
	}
	return p, nil
}

// vybor ведёт вход одного соединения. Каждый способ пробуется один раз:
// неверный пароль, повторённый keyboard-interactive, это уже две неудачи за
// соединение, а штраф OpenSSH 9.8 и новее считает каждую.
type vybor struct {
	parol  string
	podpis gossh.Signer

	dostignut bool // сервер дошёл до выбора способа, то есть до входа
	// poprobovano: способ ушёл серверу, и его отказ сервер посчитал неудачей.
	poprobovano    bool
	parolOtdan     bool
	prosrochen     bool
	strannyyVopros string
}

func (v *vybor) sleduyushchiy(ctx *gossh.ClientAuthContext) (gossh.AuthMethod, error) {
	v.dostignut = true
	sposoby := strings.Join(ctx.AllowedMethods, ",")
	if v.podpis != nil {
		if slices.Contains(ctx.TriedMethods, "publickey") {
			return nil, otkaz(PrichinaKlyuchNePrinyat, "способы сервера: "+sposoby, nil)
		}
		if !slices.Contains(ctx.AllowedMethods, "publickey") {
			return nil, otkaz(PrichinaKlyuchNePrinyat, "сервер не берёт ключи, способы: "+sposoby, nil)
		}
		v.poprobovano = true
		return gossh.PublicKeys(v.podpis), nil
	}
	switch {
	case v.prosrochen:
		return nil, otkaz(PrichinaParolProsrochen, "сервер просит сменить пароль", nil)
	case v.strannyyVopros != "":
		return nil, otkaz(PrichinaParolNeBeryot, "сервер спросил не пароль: "+v.strannyyVopros, nil)
	case slices.Contains(ctx.TriedMethods, "password"), slices.Contains(ctx.TriedMethods, "keyboard-interactive"):
		return nil, otkaz(PrichinaParolNePrinyat, "", nil)
	case slices.Contains(ctx.AllowedMethods, "password"):
		v.poprobovano = true
		return gossh.Password(v.parol), nil
	case slices.Contains(ctx.AllowedMethods, "keyboard-interactive"):
		// Только если password сервер не предложил: иначе неверный пароль
		// дал бы две неудачи за одно соединение.
		v.poprobovano = true
		return gossh.KeyboardInteractive(v.otvetit), nil
	}
	return nil, otkaz(PrichinaParolNeBeryot, "способы сервера: "+sposoby, nil)
}

// Приметы просьбы сменить пароль у PAM и OpenSSH, в нижнем регистре.
var primetySmenyParolya = []string{
	"password change", "change your password", "new password", "current password",
	"(current)", "password has expired", "password expired",
}

func trebuetSmenyParolya(tekst string) bool {
	t := strings.ToLower(tekst)
	for _, p := range primetySmenyParolya {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// otvetit отвечает на вопросы keyboard-interactive. Пароль отдаётся один раз
// и только на один скрытый вопрос; просьба сменить пароль, второй фактор и
// любой другой вопрос обрывают способ без ответа.
func (v *vybor) otvetit(imya, instrukciya string, voprosy []string, eho []bool) ([]string, error) {
	if trebuetSmenyParolya(imya + " " + instrukciya + " " + strings.Join(voprosy, " ")) {
		v.prosrochen = true
		return nil, errors.New("сервер просит сменить пароль")
	}
	switch {
	case len(voprosy) == 0:
		// Пустой раунд это сообщение без вопросов: OpenSSH шлёт его с PAM.
		return nil, nil
	case len(voprosy) == 1 && !eho[0] && !v.parolOtdan:
		v.parolOtdan = true
		return []string{v.parol}, nil
	}
	v.strannyyVopros = Obrezat(strings.Join(voprosy, " | "), 120)
	return nil, errors.New("вопрос не про пароль")
}

// ochistit готовит чужой текст для Podrobno и журнала: управляющие символы
// (перевод строки, escape-последовательности терминала) и символы
// форматирования (смена направления текста, невидимые) становятся пробелом.
func ochistit(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, "?"))
}

// Obrezat чистит чужой текст, как ochistit, и режет до n знаков.
func Obrezat(s string, n int) string {
	r := []rune(ochistit(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}

// zaslon считает неудачные входы подряд по серверу. Третий подряд ждёт
// минуту после последней неудачи: OpenSSH 9.8 и новее после четырёх неудач
// подряд запирает адрес, а через туннель адрес общий с соседями по выходу.
// Удачный вход счёт обнуляет.
type zaslon struct {
	mu      sync.Mutex
	zapisi  map[string]neudachi
	seychas func() time.Time
}

type neudachi struct {
	podryad     int
	poslednyaya time.Time
}

const (
	neudachPodryad = 2
	pauzaZaslona   = time.Minute
	// Запись старше этого забывается: штрафы OpenSSH живут минуты.
	zhizn = 10 * time.Minute
	// Потолок карты: служба живёт неделями, адреса копиться не должны.
	predelZapisey = 256
)

var obshchiyZaslon = &zaslon{zapisi: map[string]neudachi{}, seychas: time.Now}

// mozhno отвечает, сколько ещё ждать до попытки; 0 значит можно.
func (z *zaslon) mozhno(server string) time.Duration {
	z.mu.Lock()
	defer z.mu.Unlock()
	n, est := z.zapisi[server]
	if !est || n.podryad < neudachPodryad {
		return 0
	}
	ostalos := pauzaZaslona - z.seychas().Sub(n.poslednyaya)
	if ostalos <= 0 {
		return 0
	}
	return ostalos
}

func (z *zaslon) neudacha(server string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	t := z.seychas()
	for k, n := range z.zapisi {
		if t.Sub(n.poslednyaya) > zhizn {
			delete(z.zapisi, k)
		}
	}
	if _, est := z.zapisi[server]; !est && len(z.zapisi) >= predelZapisey {
		// Карта полна свежими записями: забывается самая старая, а не новая,
		// иначе заслон молча перестал бы работать для новых серверов.
		var staryy string
		var kogda time.Time
		for k, n := range z.zapisi {
			if staryy == "" || n.poslednyaya.Before(kogda) {
				staryy, kogda = k, n.poslednyaya
			}
		}
		delete(z.zapisi, staryy)
	}
	n := z.zapisi[server]
	n.podryad++
	n.poslednyaya = t
	z.zapisi[server] = n
}

func (z *zaslon) udacha(server string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	delete(z.zapisi, server)
}
