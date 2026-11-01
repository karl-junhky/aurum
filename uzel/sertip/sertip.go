// Пакет sertip выпускает и продлевает свой сертификат сервера на его IP без
// центра. Компьютер человека, который ставил сервер, может не включаться
// неделями, а сертификат Let's Encrypt на IP живёт около 160 часов (профиль
// shortlived), поэтому продлевает сама программа сервера, с половины срока.
//
// Владение адресом Let's Encrypt проверяет запросом на сам адрес: HTTP-01 на
// 80/tcp или TLS-ALPN-01 на 443/tcp, по очереди. Слушатель проверки
// поднимается только на время попытки, порт на это время открывается своим
// правилом в начале INPUT (Brandmauer). Правила ufw не трогаются: снос
// такого правила однажды оставил сервер без продления на двое суток.
package sertip

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

const (
	katalogLE        = "https://acme-v02.api.letsencrypt.org/directory"
	katalogLEStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
	profil           = acme.ProfileName("shortlived")
	alpnACME         = "acme-tls/1"

	SposobHTTP = "http-01"
	SposobTLS  = "tls-alpn-01"
	// SposobKopiya: сертификат не выпущен здесь, а прислан клиентом из
	// копии прошлой установки (лимит Let's Encrypt: пять на адрес в неделю).
	SposobKopiya = "копия"

	// krug: как часто выпуск смотрит, не пора ли.
	krug = 10 * time.Minute
	// Не больше двух попыток за час: Let's Encrypt терпит пять проваленных
	// проверок адреса в час, а за попытку их не больше двух, по способу.
	oknoPopytok  = time.Hour
	popytokVOkne = 2
	srokPopytki  = 3 * time.Minute
	// trevogaNeVyhodit: продлить пора, а не выходит дольше этого - тревога.
	trevogaNeVyhodit = 6 * time.Hour
	// zapasSroka: осталось меньше - тревога, сколько бы ни было отказов.
	zapasSroka = 24 * time.Hour

	imyaSerta   = "fullchain.pem"
	imyaKlyucha = "privkey.pem"
	imyaUcheta  = "uchet.json"
	cnZaglushki = "vpn-uzel placeholder"
)

// Nastroyka это поле sert_ip_sam пакета: выпускай сертификат сам, на этот
// адрес. Staging нужен прогонам приёмки: настоящий сертификат лимитирован.
type Nastroyka struct {
	IP      string `json:"ip"`
	Staging bool   `json:"staging,omitempty"`
}

// Proverit отвергает адрес, который не IP. Публичный ли он, решает Let's
// Encrypt.
func (n Nastroyka) Proverit() error {
	if net.ParseIP(n.IP) == nil {
		return fmt.Errorf("sert_ip_sam: %q не IP", n.IP)
	}
	return nil
}

// Pem это сертификат с ключом в том виде, как они едут в пакете.
type Pem struct {
	Cert   string
	Klyuch string
}

// Brandmauer открывает порт проверки на время попытки.
type Brandmauer interface {
	// Otkryt пускает входящие на tcp-порт; zakryt снимает правила проверки.
	Otkryt(ip net.IP, port int) (zakryt func() error, err error)
	// Ubrat снимает правила, оставшиеся от прерванной попытки.
	Ubrat() error
}

// Opcii это то, что тесты подменяют. Нули значат как на сервере.
type Opcii struct {
	PortHTTP    int
	PortTLS     int
	Brandmauer  Brandmauer
	KatalogACME string // каталог ACME вместо Let's Encrypt
	HTTPKlient  *http.Client
	Chasy       func() time.Time
}

// Sostoyanie это выпуск глазами клиента: поле sert_ip_sam ответа sostoyanie.
type Sostoyanie struct {
	IP      string `json:"ip"`
	Staging bool   `json:"staging,omitempty"`
	// Est: настоящий сертификат на этот адрес лежит на диске.
	Est    bool   `json:"est"`
	Ot     int64  `json:"ot,omitempty"`
	Do     int64  `json:"do,omitempty"`
	Sposob string `json:"sposob,omitempty"` // каким путём получен текущий
	Idyot  bool   `json:"idyot,omitempty"`  // попытка идёт сейчас
	// Popytka: когда была последняя попытка, удачная или нет.
	Popytka int64 `json:"popytka,omitempty"`
	// Otkaz: чем кончилась последняя попытка, если неудачей; OtkazS - с
	// какого момента отказы идут подряд.
	Otkaz  string `json:"otkaz,omitempty"`
	OtkazS int64  `json:"otkaz_s,omitempty"`
	// Sleduyushchaya: не раньше какого момента будет следующая попытка.
	Sleduyushchaya int64 `json:"sleduyushchaya,omitempty"`
	// Trevoga: сертификата нет и выпуск уже отказывал, или до конца срока
	// меньше суток, или продлить пора, а не выходит дольше шести часов.
	Trevoga bool `json:"trevoga,omitempty"`
}

// uchet живёт на диске: перезапуск программы не должен обнулять счёт
// попыток, иначе падающая по кругу служба выбрала бы лимит Let's Encrypt.
type uchet struct {
	Popytki []int64 `json:"popytki,omitempty"` // попытки последнего часа
	Sposob  string  `json:"sposob,omitempty"`
	Otkaz   string  `json:"otkaz,omitempty"`
	OtkazS  int64   `json:"otkaz_s,omitempty"`
}

// Vypusk ведёт свой сертификат сервера.
type Vypusk struct {
	katalog      string // ключ аккаунта и учёт попыток
	katalogSerta string // fullchain.pem и privkey.pem, на них ссылаются входы ядра
	zhurnal      *slog.Logger
	obnovleno    func() // зовётся после смены сертификата, без замка выпуска
	o            Opcii

	budit chan struct{}

	mu         sync.Mutex
	nastroyka  *Nastroyka
	tekushchiy *tls.Certificate // настоящий, не заглушка; nil - его нет
	uchet      uchet
	idyot      bool
	gotovo     chan struct{} // закрывается, когда идущая попытка кончилась
	pauza      int           // сколько держателей Zanyat не дают начать попытку
	otlozheno  bool          // попытка не началась из-за паузы
}

// Novyy читает с диска то, что выпущено раньше. Выпуск выключен, пока
// Nastroit не задал адрес.
func Novyy(katalog, katalogSerta string, zhurnal *slog.Logger, obnovleno func(), o Opcii) *Vypusk {
	if o.PortHTTP == 0 {
		o.PortHTTP = 80
	}
	if o.PortTLS == 0 {
		o.PortTLS = 443
	}
	if o.Brandmauer == nil {
		o.Brandmauer = Iptables{}
	}
	if o.Chasy == nil {
		o.Chasy = time.Now
	}
	v := &Vypusk{
		katalog: katalog, katalogSerta: katalogSerta, zhurnal: zhurnal, obnovleno: obnovleno, o: o,
		budit: make(chan struct{}, 1),
	}
	t, err := prochitatSert(katalogSerta)
	switch {
	case err == nil && !zaglushka(t.Leaf):
		v.tekushchiy = t
	case err != nil && !errors.Is(err, os.ErrNotExist):
		zhurnal.Warn("свой сертификат с диска не читается, будет выпущен заново", "oshibka", err)
	}
	if b, err := os.ReadFile(filepath.Join(katalog, imyaUcheta)); err == nil {
		if err := json.Unmarshal(b, &v.uchet); err != nil {
			zhurnal.Warn("учёт попыток выпуска не читается, начат заново", "oshibka", err)
			v.uchet = uchet{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		zhurnal.Warn("учёт попыток выпуска не читается, начат заново", "oshibka", err)
	}
	return v
}

// Nastroit задаёт, на какой адрес выпускать; nil выключает выпуск (пакет
// центра везёт сертификат сам). kopiya это сертификат, сохранённый клиентом
// с прошлой установки: он ложится на диск, если годен для адреса и живёт
// дольше лежащего. Новая настройка будит выпуск сразу.
func (v *Vypusk) Nastroit(n *Nastroyka, kopiya *Pem) error {
	if n != nil {
		norm := Nastroyka{IP: net.ParseIP(n.IP).String(), Staging: n.Staging}
		n = &norm
	}
	v.mu.Lock()
	prezhnyaya := v.nastroyka
	v.nastroyka = n
	var zamena bool
	var err error
	if n != nil && kopiya != nil {
		zamena, err = v.prinyatKopiyu(*n, *kopiya)
	}
	v.mu.Unlock()
	if zamena && v.obnovleno != nil {
		v.obnovleno()
	}
	if n != nil && (prezhnyaya == nil || *prezhnyaya != *n) {
		v.razbudit()
	}
	return err
}

func (v *Vypusk) prinyatKopiyu(n Nastroyka, k Pem) (bool, error) {
	para, err := tls.X509KeyPair([]byte(k.Cert), []byte(k.Klyuch))
	if err != nil {
		return false, fmt.Errorf("копия сертификата негодна: %w", err)
	}
	seychas := v.o.Chasy()
	switch {
	case zaglushka(para.Leaf), !dlyaIP(para.Leaf, n.IP):
		return false, fmt.Errorf("копия сертификата не на %s", n.IP)
	case !seychas.Before(para.Leaf.NotAfter):
		return false, nil // истекла: выпуск сделает новый
	case v.tekushchiy != nil && dlyaIP(v.tekushchiy.Leaf, n.IP) && !para.Leaf.NotAfter.After(v.tekushchiy.Leaf.NotAfter):
		return false, nil // на диске не хуже
	}
	if err := v.zapisat(Pem{Cert: k.Cert, Klyuch: k.Klyuch}); err != nil {
		return false, err
	}
	v.tekushchiy = &para
	v.uchet.Sposob = SposobKopiya
	v.sohranitUchet()
	return true, nil
}

// Zaglushka кладёт самоподписанный сертификат на адрес, если годной пары
// файлов нет вовсе. TLS-входы ссылаются на файл, и без него ядро не
// поднялось бы целиком, вместе с reality, которому сертификат не нужен.
// Настоящий заменит заглушку первой удачной попыткой.
func (v *Vypusk) Zaglushka(ip string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, err := prochitatSert(v.katalogSerta); err == nil {
		return nil
	}
	adres := net.ParseIP(ip)
	if adres == nil {
		return fmt.Errorf("заглушка: %q не IP", ip)
	}
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	seychas := v.o.Chasy()
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(seychas.UnixNano()),
		Subject:      pkix.Name{CommonName: cnZaglushki},
		IPAddresses:  []net.IP{adres},
		NotBefore:    seychas.Add(-time.Hour),
		NotAfter:     seychas.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		return err
	}
	kDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		return err
	}
	return v.zapisat(Pem{
		Cert:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		Klyuch: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kDer})),
	})
}

// Tekushchiy это настоящий сертификат на настроенный адрес; nil - его нет,
// выпуск выключен или на диске заглушка.
func (v *Vypusk) Tekushchiy() *tls.Certificate {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.nastroyka == nil || v.tekushchiy == nil || !dlyaIP(v.tekushchiy.Leaf, v.nastroyka.IP) {
		return nil
	}
	return v.tekushchiy
}

// Zanyat не даёт начаться новой попытке и дожидается идущей: ядро меняется и
// может занять 443/tcp, на котором стоит слушатель проверки. osvobodit
// зовётся обязательно; попытка, которой не дали начаться, начнётся сразу
// после.
func (v *Vypusk) Zanyat(ctx context.Context) (osvobodit func(), err error) {
	v.mu.Lock()
	v.pauza++
	idyot, gotovo := v.idyot, v.gotovo
	v.mu.Unlock()
	osvobodit = sync.OnceFunc(func() {
		v.mu.Lock()
		v.pauza--
		budit := v.pauza == 0 && v.otlozheno
		if budit {
			v.otlozheno = false
		}
		v.mu.Unlock()
		if budit {
			v.razbudit()
		}
	})
	if !idyot {
		return osvobodit, nil
	}
	select {
	case <-gotovo:
		return osvobodit, nil
	case <-ctx.Done():
		osvobodit()
		return nil, errors.New("идёт выпуск сертификата, повтори через минуту")
	}
}

// Seychas будит выпуск вне круга. Лимит попыток при этом в силе.
func (v *Vypusk) Seychas() { v.razbudit() }

func (v *Vypusk) razbudit() {
	select {
	case v.budit <- struct{}{}:
	default:
	}
}

// Rabotat смотрит раз в krug и по просьбе, не пора ли выпускать, пока не
// отменён ctx.
func (v *Vypusk) Rabotat(ctx context.Context) {
	if err := v.o.Brandmauer.Ubrat(); err != nil {
		v.zhurnal.Warn("правила проверки от прошлой попытки не снялись", "oshibka", err)
	}
	t := time.NewTicker(krug)
	defer t.Stop()
	for {
		v.krug(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-v.budit:
		}
	}
}

func (v *Vypusk) krug(ctx context.Context) {
	v.mu.Lock()
	seychas := v.o.Chasy()
	n := v.nastroyka
	if n == nil || v.idyot || !v.pora(*n, seychas) {
		v.mu.Unlock()
		return
	}
	if v.pauza > 0 {
		v.otlozheno = true
		v.mu.Unlock()
		return
	}
	if !v.mozhno(seychas) {
		v.mu.Unlock()
		return
	}
	nastroyka, pervym := *n, v.uchet.Sposob
	v.idyot, v.gotovo = true, make(chan struct{})
	v.uchet.Popytki = append(v.uchet.Popytki, seychas.Unix())
	v.sohranitUchet()
	v.mu.Unlock()

	v.zhurnal.Info("выпуск своего сертификата", "ip", nastroyka.IP, "staging", nastroyka.Staging)
	vypushchen, sposob, err := v.vypustit(ctx, nastroyka, pervym)

	v.mu.Lock()
	zamena := false
	switch {
	case err != nil:
		v.uchet.Otkaz = err.Error()
		if v.uchet.OtkazS == 0 {
			v.uchet.OtkazS = v.o.Chasy().Unix()
		}
		v.zhurnal.Error("свой сертификат не выпущен", "ip", nastroyka.IP, "oshibka", err)
	case v.nastroyka == nil || *v.nastroyka != nastroyka:
		// Пока шла попытка, пакет сменил адрес: выпущенное уже не нужно.
		v.razbudit()
	default:
		para, errP := tls.X509KeyPair([]byte(vypushchen.Cert), []byte(vypushchen.Klyuch))
		if errP == nil {
			errP = v.zapisat(vypushchen)
		}
		if errP != nil {
			v.uchet.Otkaz = "выпущен, но не сохранён: " + errP.Error()
			if v.uchet.OtkazS == 0 {
				v.uchet.OtkazS = v.o.Chasy().Unix()
			}
			v.zhurnal.Error("свой сертификат выпущен, но не сохранён", "oshibka", errP)
			break
		}
		v.tekushchiy, zamena = &para, true
		v.uchet.Sposob, v.uchet.Otkaz, v.uchet.OtkazS = sposob, "", 0
		v.zhurnal.Info("свой сертификат выпущен", "ip", nastroyka.IP, "sposob", sposob,
			"do", para.Leaf.NotAfter.UTC().Format(time.DateTime))
	}
	v.sohranitUchet()
	v.idyot = false
	close(v.gotovo)
	v.mu.Unlock()
	if zamena && v.obnovleno != nil {
		v.obnovleno()
	}
}

// pora: сертификата на адрес нет, он из другого каталога (staging против
// настоящего) или прошла половина срока.
func (v *Vypusk) pora(n Nastroyka, seychas time.Time) bool {
	t := v.tekushchiy
	if t == nil || !dlyaIP(t.Leaf, n.IP) || staging(t.Leaf) != n.Staging {
		return true
	}
	srok := t.Leaf.NotAfter.Sub(t.Leaf.NotBefore)
	return t.Leaf.NotAfter.Sub(seychas) < srok/2
}

// mozhno: за последний час попыток меньше popytokVOkne. Старые попытки
// выпадают из учёта здесь же.
func (v *Vypusk) mozhno(seychas time.Time) bool {
	granica := seychas.Add(-oknoPopytok).Unix()
	v.uchet.Popytki = slices.DeleteFunc(v.uchet.Popytki, func(p int64) bool { return p <= granica })
	return len(v.uchet.Popytki) < popytokVOkne
}

// Sostoyanie отдаёт выпуск глазами клиента; nil - выпуск выключен.
func (v *Vypusk) Sostoyanie() *Sostoyanie {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.nastroyka == nil {
		return nil
	}
	n, seychas := *v.nastroyka, v.o.Chasy()
	s := &Sostoyanie{IP: n.IP, Staging: n.Staging, Idyot: v.idyot, Sposob: v.uchet.Sposob,
		Otkaz: v.uchet.Otkaz, OtkazS: v.uchet.OtkazS}
	if k := len(v.uchet.Popytki); k > 0 {
		s.Popytka = v.uchet.Popytki[k-1]
	}
	pora := v.pora(n, seychas)
	if t := v.tekushchiy; t != nil && dlyaIP(t.Leaf, n.IP) {
		s.Est, s.Ot, s.Do = true, t.Leaf.NotBefore.Unix(), t.Leaf.NotAfter.Unix()
		ostalos := t.Leaf.NotAfter.Sub(seychas)
		s.Trevoga = ostalos < zapasSroka ||
			pora && s.OtkazS != 0 && seychas.Sub(time.Unix(s.OtkazS, 0)) >= trevogaNeVyhodit
		if !pora {
			s.Sleduyushchaya = t.Leaf.NotAfter.Add(-t.Leaf.NotAfter.Sub(t.Leaf.NotBefore) / 2).Unix()
		}
	} else {
		s.Trevoga = s.OtkazS != 0
	}
	if pora {
		s.Sleduyushchaya = seychas.Unix()
		if !v.mozhno(seychas) {
			s.Sleduyushchaya = time.Unix(v.uchet.Popytki[0], 0).Add(oknoPopytok).Unix()
		}
	}
	return s
}

// vypustit это одна попытка: заказ, проверка адреса, выдача. Способ,
// отвергнутый Let's Encrypt, в этой попытке больше не берётся, и заказ
// повторяется оставшимися.
func (v *Vypusk) vypustit(ctx context.Context, n Nastroyka, pervym string) (Pem, string, error) {
	ctx, otmena := context.WithTimeout(ctx, srokPopytki)
	defer otmena()
	k, err := v.klient(ctx, n.Staging)
	if err != nil {
		return Pem{}, "", err
	}
	sposoby := []string{SposobHTTP, SposobTLS}
	if pervym == SposobTLS {
		sposoby = []string{SposobTLS, SposobHTTP}
	}
	var oshibki []error
	for len(sposoby) > 0 {
		vypushchen, sposob, otvergnut, err := v.zakaz(ctx, k, n.IP, sposoby)
		if err == nil {
			if sposob == "" {
				sposob = pervym // Let's Encrypt помнил прежнюю проверку адреса
			}
			return vypushchen, sposob, nil
		}
		oshibki = append(oshibki, err)
		if otvergnut == "" || ctx.Err() != nil {
			break
		}
		sposoby = slices.DeleteFunc(sposoby, func(s string) bool { return s == otvergnut })
	}
	return Pem{}, "", errors.Join(oshibki...)
}

// zakaz это один заказ. sposob - чем доказано владение (пусто, если Let's
// Encrypt помнил прежнюю проверку); otvergnut - способ, который Let's Encrypt
// проверял и отверг.
func (v *Vypusk) zakaz(ctx context.Context, k *acme.Client, ip string, sposoby []string) (vypushchen Pem, sposob, otvergnut string, err error) {
	z, err := k.AuthorizeOrder(ctx, acme.IPIDs(ip), acme.WithOrderIssuanceProfile(profil))
	if err != nil {
		return Pem{}, "", "", fmt.Errorf("заказ: %w", err)
	}
	for _, u := range z.AuthzURLs {
		a, err := k.GetAuthorization(ctx, u)
		if err != nil {
			return Pem{}, "", "", fmt.Errorf("авторизация: %w", err)
		}
		if a.Status == acme.StatusValid {
			continue
		}
		if sposob, otvergnut, err = v.dokazat(ctx, k, a, sposoby); err != nil {
			return Pem{}, "", otvergnut, err
		}
	}
	z, err = k.WaitOrder(ctx, z.URI)
	if err != nil {
		return Pem{}, "", "", fmt.Errorf("заказ не готов: %w", err)
	}
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Pem{}, "", "", err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{IPAddresses: []net.IP{net.ParseIP(ip)}}, klyuch)
	if err != nil {
		return Pem{}, "", "", err
	}
	cepochka, _, err := k.CreateOrderCert(ctx, z.FinalizeURL, csr, true)
	if err != nil {
		return Pem{}, "", "", fmt.Errorf("выдача: %w", err)
	}
	var cert strings.Builder
	for _, der := range cepochka {
		cert.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	kDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		return Pem{}, "", "", err
	}
	return Pem{Cert: cert.String(), Klyuch: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kDer}))}, sposob, "", nil
}

// dokazat проходит авторизацию первым способом, чей слушатель поднялся.
func (v *Vypusk) dokazat(ctx context.Context, k *acme.Client, a *acme.Authorization, sposoby []string) (sposob, otvergnut string, err error) {
	ip := a.Identifier.Value
	var neVyshlo []error
	for _, s := range sposoby {
		var ch *acme.Challenge
		for _, c := range a.Challenges {
			if c.Type == s {
				ch = c
			}
		}
		if ch == nil {
			neVyshlo = append(neVyshlo, fmt.Errorf("%s: Let's Encrypt его не предложил", s))
			continue
		}
		zakryt, err := v.podnyat(k, ch, ip)
		if err != nil {
			neVyshlo = append(neVyshlo, fmt.Errorf("%s: %w", s, err))
			continue
		}
		_, err = k.Accept(ctx, ch)
		if err == nil {
			_, err = k.WaitAuthorization(ctx, a.URI)
		}
		zakryt()
		if err != nil {
			return "", s, fmt.Errorf("%s: Let's Encrypt не достучался: %w", s, err)
		}
		return s, "", nil
	}
	return "", "", fmt.Errorf("ни один способ проверки не поднялся: %w", errors.Join(neVyshlo...))
}

// podnyat поднимает слушатель проверки и открывает его порт. zakryt гасит
// слушатель и снимает правило.
func (v *Vypusk) podnyat(k *acme.Client, ch *acme.Challenge, ip string) (zakryt func(), err error) {
	port := v.o.PortHTTP
	if ch.Type == SposobTLS {
		port = v.o.PortTLS
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("%d/tcp не слушается: %w", port, err)
	}
	var gasit func()
	switch ch.Type {
	case SposobHTTP:
		otvet, err := k.HTTP01ChallengeResponse(ch.Token)
		if err != nil {
			ln.Close()
			return nil, err
		}
		put := k.HTTP01ChallengePath(ch.Token)
		srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != put {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, otvet)
		})}
		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				v.zhurnal.Warn("слушатель проверки на 80 упал", "oshibka", err)
			}
		}()
		gasit = func() { srv.Close() }
	case SposobTLS:
		sert, err := k.TLSALPN01ChallengeCert(ch.Token, ip)
		if err != nil {
			ln.Close()
			return nil, err
		}
		tln := tls.NewListener(ln, &tls.Config{NextProtos: []string{alpnACME}, Certificates: []tls.Certificate{sert}})
		go func() {
			for {
				c, err := tln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer c.Close()
					// Рукопожатие и есть ответ на проверку; сбой одного
					// подключения ничего не решает, решает Let's Encrypt.
					_ = c.SetDeadline(time.Now().Add(10 * time.Second))
					_ = c.(*tls.Conn).Handshake()
				}()
			}
		}()
		gasit = func() { tln.Close() }
	default:
		ln.Close()
		return nil, fmt.Errorf("способ %s программа не умеет", ch.Type)
	}
	snyat, err := v.o.Brandmauer.Otkryt(net.ParseIP(ip), port)
	if err != nil {
		// Брандмауэра может и не быть вовсе; не вышло открыть - пробуем как
		// есть, ответ Let's Encrypt покажет.
		v.zhurnal.Warn("порт проверки не открыт в брандмауэре", "port", port, "oshibka", err)
		snyat = func() error { return nil }
	}
	return func() {
		gasit()
		if err := snyat(); err != nil {
			v.zhurnal.Error("правило проверки не снялось", "port", port, "oshibka", err)
		}
	}, nil
}

func (v *Vypusk) klient(ctx context.Context, staging bool) (*acme.Client, error) {
	adres, imya := katalogLE, "akkaunt.pem"
	switch {
	case v.o.KatalogACME != "":
		adres, imya = v.o.KatalogACME, "akkaunt-svoy.pem"
	case staging:
		adres, imya = katalogLEStaging, "akkaunt-staging.pem"
	}
	klyuch, err := klyuchAkkaunta(filepath.Join(v.katalog, imya))
	if err != nil {
		return nil, fmt.Errorf("ключ аккаунта: %w", err)
	}
	k := &acme.Client{Key: klyuch, DirectoryURL: adres, HTTPClient: v.o.HTTPKlient, UserAgent: "vpn-uzel"}
	if _, err := k.Register(ctx, &acme.Account{}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("аккаунт Let's Encrypt: %w", err)
	}
	return k, nil
}

func klyuchAkkaunta(put string) (*ecdsa.PrivateKey, error) {
	b, err := os.ReadFile(put)
	if err == nil {
		blok, _ := pem.Decode(b)
		if blok == nil {
			return nil, errors.New("ключ аккаунта не PEM")
		}
		return x509.ParseECPrivateKey(blok.Bytes)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(put), 0o700); err != nil {
		return nil, err
	}
	return k, zapisatAtomarno(put, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

// zapisat кладёт пару в каталог сертификата, ключ раньше цепочки: ядро
// следит за файлами, и пока ключ уже новый, а цепочка старая, оно разок
// напишет, что пара не сходится, и останется на прежней.
func (v *Vypusk) zapisat(p Pem) error {
	if err := os.MkdirAll(v.katalogSerta, 0o700); err != nil {
		return fmt.Errorf("каталог сертификата: %w", err)
	}
	if err := zapisatAtomarno(filepath.Join(v.katalogSerta, imyaKlyucha), []byte(p.Klyuch)); err != nil {
		return fmt.Errorf("ключ сертификата не лёг на диск: %w", err)
	}
	if err := zapisatAtomarno(filepath.Join(v.katalogSerta, imyaSerta), []byte(p.Cert)); err != nil {
		return fmt.Errorf("цепочка не легла на диск: %w", err)
	}
	return nil
}

func (v *Vypusk) sohranitUchet() {
	b, err := json.Marshal(v.uchet)
	if err == nil {
		if err = os.MkdirAll(v.katalog, 0o700); err == nil {
			err = zapisatAtomarno(filepath.Join(v.katalog, imyaUcheta), b)
		}
	}
	if err != nil {
		v.zhurnal.Warn("учёт попыток выпуска не сохранился", "oshibka", err)
	}
}

func prochitatSert(katalog string) (*tls.Certificate, error) {
	t, err := tls.LoadX509KeyPair(filepath.Join(katalog, imyaSerta), filepath.Join(katalog, imyaKlyucha))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func zaglushka(c *x509.Certificate) bool {
	return c.Subject.CommonName == cnZaglushki && c.Issuer.CommonName == cnZaglushki
}

func dlyaIP(c *x509.Certificate, ip string) bool {
	adres := net.ParseIP(ip)
	return adres != nil && slices.ContainsFunc(c.IPAddresses, adres.Equal)
}

// staging: сертификат выдан тестовым каталогом Let's Encrypt.
func staging(c *x509.Certificate) bool {
	return strings.Contains(c.Issuer.CommonName, "STAGING")
}

func zapisatAtomarno(put string, dannye []byte) error {
	vremennyy := put + ".novyy"
	f, err := os.OpenFile(vremennyy, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(dannye)
	if err == nil {
		err = f.Sync()
	}
	if errZ := f.Close(); err == nil {
		err = errZ
	}
	if err != nil {
		_ = os.Remove(vremennyy) // недописанный файл не нужен, ошибка удаления ничего не меняет
		return err
	}
	return os.Rename(vremennyy, put)
}
