// Package blok отвечает, почему свой сервер не виден из сети человека
// (§3.5 плана).
//
// Тишина одного порта ничего не доказывает: так же молчит порт, закрытый
// брандмауэром сервера, а чужой живой адрес молчит на 22. Поэтому вердикт
// складывается из нескольких проб:
//   - своя сеть: TLS до зарубежных сайтов, а при их молчании до российских.
//     Молчат зарубежные, отвечают российские - режим белых списков: там
//     рукопожатие TCP проходит с любым адресом, а приветствие TLS к
//     неразрешённому имени теряется, поэтому проба именно TLS;
//   - стук напрямую через физический адаптер в порт SSH, 80, 443 и порты
//     ключей, на портах TLS ещё и приветствие TLS, на SSH ответ сервера;
//   - что видит сервер: ловушка программы сервера считает попытки на порт до
//     брандмауэра, её спрашивают по SSH через туннель;
//   - вход SSH через туннель: служба уже знает, ответил ли так сервер. Через
//     туннель судит только ответ сервера: соединение ядро туннеля принимает
//     у себя даже на закрытом порту.
//
// Отрицательный вердикт выносится только после повторов через 2 и 10
// минут: бывают закрытия на 10 минут для одного адреса клиента. Сам детектор
// не должен заморозить адрес: к своему серверу пробы идут по одной, не чаще
// раза в секунду.
package blok

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/obhoddns"
)

var (
	// Цели проверки своей сети, «имя:порт».
	zarubezhnye = []string{"github.com:443", "google.com:443"}
	rossiyskie  = []string{"ya.ru:443", "vk.ru:443", "gosuslugi.ru:443"}

	srokStuka  = 5 * time.Second // соединение TCP
	srokOtveta = 5 * time.Second // ответ после соединения
	pauzaStuka = time.Second     // к своему серверу не чаще раза в секунду
	// povtory отсчитываются от конца первого круга.
	povtory = []time.Duration{2 * time.Minute, 10 * time.Minute}
	// Ловушка слушает sekundLovushki с той минуты, как встала, а встаёт она
	// не сразу: просьба идёт по SSH через туннель, бывает и новым входом.
	// Поэтому стук идёт по расписанию через всё окно: через zhdatLovushku и
	// дальше каждые shagStuka, stukovLovushki раз. Ловушка, вставшая до
	// последнего стука, его увидит.
	sekundLovushki    = 15
	zhdatLovushku     = 2 * time.Second
	shagStuka         = 3 * time.Second
	stukovLovushki    = 4
	srokStukaLovushki = 2 * time.Second
	// zapasLovushki это срок ответа ловушки сверх её секунд: SSH через
	// туннель.
	zapasLovushki = 20 * time.Second

	// srokPoiska это срок одного способа поиска имени цели.
	srokPoiska = 4 * time.Second
	iskat      = net.DefaultResolver.LookupNetIP
	sprositDoH = obhoddns.SprositCherez
	// korni это корни доверия к целям своей сети и маскам; nil - системные.
	korni *x509.CertPool
)

// Dial открывает соединение, как ssh.Put.Dial.
type Dial func(ctx context.Context, network, adres string) (net.Conn, error)

// Ishod это чем кончился стук в TCP-порт напрямую.
type Ishod string

const (
	Otkryt  Ishod = "otkryt"  // рукопожатие TCP прошло
	Otkaz   Ishod = "otkaz"   // сброс: никто не слушает, или сброс подделан по дороге
	Tishina Ishod = "tishina" // ни ответа, ни сброса до срока
	// Nedostupen: «адрес или сеть недоступны». Своя сеть к этому времени
	// уже ответила тем же путём, значит, адрес отверг кто-то по дороге.
	Nedostupen Ishod = "nedostupen"
	Sboy       Ishod = "sboy" // своя сторона: пропал адаптер, нет буфера
)

// Otvet это что пришло после рукопожатия TCP.
type Otvet string

const (
	OtvetEst     Otvet = "est"     // сервер ответил своим: приветствие SSH или TLS
	OtvetObryv   Otvet = "obryv"   // соединение сброшено после приветствия
	OtvetTishina Otvet = "tishina" // ни байта до срока
	OtvetChuzhoy Otvet = "chuzhoy" // ответил не тот протокол
)

// PortTCP это порт для стука. TLS это имя для приветствия TLS (маска
// reality); пусто - спрашивается только TCP.
type PortTCP struct {
	Port int
	TLS  string
}

// Stuk это стук в один TCP-порт.
type Stuk struct {
	Port  int
	Ishod Ishod
	// Otvet пуст, если порт не открылся или протокол не спрашивали.
	Otvet Otvet
	// Podrobno идёт в журнал: текст ошибки сети, без секретов.
	Podrobno string
}

// Tunnel это что показал вход SSH через туннель.
type Tunnel string

const (
	TunnelNeProveren Tunnel = ""        // туннеля нет или вход им не пробовали
	TunnelOtvetil    Tunnel = "otvetil" // сервер отдал приветствие SSH
	TunnelMolchit    Tunnel = "molchit" // и через туннель сервер не ответил
)

// Set это своя сеть человека.
type Set string

const (
	SetZhivaya     Set = "zhivaya"
	SetBelyeSpiski Set = "belye-spiski"
	SetMertva      Set = "mertva"
)

// Vid это вердикт.
type Vid string

const (
	// NeSostoyalas: своя сеть не отвечает, о сервере сказать нечего.
	NeSostoyalas Vid = "ne-sostoyalas"
	// BelyeSpiski: в сети человека режим белых списков, свой VPS не пройдёт
	// ничем, и сервер в этом не виноват.
	BelyeSpiski Vid = "belye-spiski"
	// Dostupen: напрямую сервер отвечает.
	Dostupen Vid = "dostupen"
	// Glushat: TCP доходит, а приветствие TLS обрывают или глушат по дороге.
	Glushat Vid = "glushat"
	// Podmena: на адрес отвечает не сервер (не тот протокол).
	Podmena Vid = "podmena"
	// NeDohodit: сервер жив (через туннель ответил), напрямую не доходит.
	NeDohodit Vid = "ne-dohodit"
	// OtvetNeVozvrashchaetsya: стук дошёл до сервера, ответа нет. Брандмауэр
	// сервера или хостера, или ответ режут на обратном пути.
	OtvetNeVozvrashchaetsya Vid = "otvet-ne-vozvrashchaetsya"
	// ServerMolchit: молчит и напрямую, и через туннель: выключен или адрес
	// неверный.
	ServerMolchit Vid = "server-molchit"
	// MolchitNapryamuyu: напрямую молчит, а через туннель не проверить.
	MolchitNapryamuyu Vid = "molchit-napryamuyu"
)

// otricatelnyy: такой вердикт выносится только после повторов.
func (v Vid) otricatelnyy() bool {
	return v != NeSostoyalas && v != BelyeSpiski && v != Dostupen
}

// Opcii это что проверять и чем.
type Opcii struct {
	Adres netip.Addr
	// PortSsh это порт, на который входят по SSH: на нём ждётся приветствие.
	PortSsh int
	// TCP это порты для стука кроме SSH: 80, 443 и порты ключей.
	TCP []PortTCP
	// UDP это порты hy2. Есть они - ловушка смотрит, доходят ли датаграммы
	// до сервера вообще. Стук идёт в случайный свободный порт, а не в порт
	// hy2: там ловушка сосчитала бы и датаграммы других клиентов сервера.
	// Годен ли сам порт hy2, судит рукопожатие ключа (proverka).
	UDP []int
	// Pryamo это соединение через физический адаптер мимо туннеля
	// (ssh.PutPryamo).
	Pryamo Dial
	Tunnel Tunnel
	// Lovushka спрашивает программу сервера (server.Lovit через туннель);
	// nil - программы на сервере ещё нет.
	Lovushka func(ctx context.Context, proto string, port, sekund int) (int64, error)
	// Hod узнаёт о конце каждого круга, в той же горутине.
	Hod func(Krug)
}

// Krug это конец одного круга проб.
type Krug struct {
	Nomer int
	Vid   Vid
	// Posledniy: кругов больше не будет, вердикт окончательный.
	Posledniy bool
	// Sleduyushchiy это через сколько следующий круг. Бывает 0 и у не
	// последнего: круг шёл дольше паузы до повтора.
	Sleduyushchiy time.Duration
	// Povtory это когда по плану начнутся оставшиеся круги: окно называет
	// человеку время, а не отсчёт, который без событий не идёт.
	Povtory []time.Time
}

// Itog это вердикт и то, из чего он сложен (последний круг).
type Itog struct {
	Vid   Vid
	Set   Set
	Stuki []Stuk
	// Doshlo это сколько попыток стука TCP в случайный порт увидел сервер;
	// -1 - счёт ничего не значит: ловушку не спрашивали, она отказала, стук
	// не ушёл с компьютера или ловушка ответила раньше него.
	Doshlo int64
	// DoshloUDP это то же для датаграмм; ловушку спрашивают, когда у сервера
	// есть порты hy2.
	DoshloUDP int64
	// OtkazLovushki это для журнала, почему счёт -1, когда ловушку
	// спрашивали: её отказ или беда своего стука.
	OtkazLovushki string
	// Krugov это сколько прошло кругов. Dostupen после нескольких кругов
	// значит, что адрес закрывали ненадолго.
	Krugov int
}

// Proverit выносит вердикт. Ошибка значит отмену ctx или негодные опции;
// всё остальное, включая мёртвую сеть, это вердикт.
func Proverit(ctx context.Context, o Opcii) (itog Itog, err error) {
	defer zashchita("Proverit", &err)
	switch {
	case !o.Adres.IsValid():
		return Itog{}, errors.New("детектор: нет адреса сервера")
	case o.PortSsh < 1 || o.PortSsh > 65535:
		return Itog{}, fmt.Errorf("детектор: порт SSH %d", o.PortSsh)
	case o.Pryamo == nil:
		return Itog{}, errors.New("детектор: нет прямого пути")
	}
	for _, p := range o.TCP {
		if p.Port < 1 || p.Port > 65535 {
			return Itog{}, fmt.Errorf("детектор: порт %d", p.Port)
		}
	}
	for _, p := range o.UDP {
		if p < 1 || p > 65535 {
			return Itog{}, fmt.Errorf("детектор: порт UDP %d", p)
		}
	}
	var konecPervogo time.Time
	for nomer := 1; ; nomer++ {
		if itog, err = krug(ctx, o); err != nil {
			return Itog{}, err
		}
		itog.Krugov = nomer
		if nomer == 1 {
			konecPervogo = time.Now()
		}
		posledniy := !itog.Vid.otricatelnyy() || nomer > len(povtory)
		var dalshe time.Duration
		var plan []time.Time
		if !posledniy {
			dalshe = max(time.Until(konecPervogo.Add(povtory[nomer-1])), 0)
			for _, d := range povtory[nomer-1:] {
				plan = append(plan, konecPervogo.Add(d))
			}
		}
		if o.Hod != nil {
			o.Hod(Krug{Nomer: nomer, Vid: itog.Vid, Posledniy: posledniy, Sleduyushchiy: dalshe, Povtory: plan})
		}
		if posledniy {
			return itog, nil
		}
		if err := podozhdat(ctx, dalshe); err != nil {
			return Itog{}, err
		}
	}
}

// krug это один круг проб.
func krug(ctx context.Context, o Opcii) (Itog, error) {
	itog := Itog{Doshlo: -1, DoshloUDP: -1}
	itog.Set = proveritSet(ctx, o)
	if err := ctx.Err(); err != nil {
		return Itog{}, err
	}
	if itog.Set != SetZhivaya {
		itog.Vid = vynesti(itog.Set, nil, o.Tunnel, -1)
		return itog, nil
	}
	porty := append([]PortTCP{{Port: o.PortSsh}}, o.TCP...)
	for i, p := range porty {
		if i > 0 {
			if err := podozhdat(ctx, pauzaStuka); err != nil {
				return Itog{}, err
			}
		}
		itog.Stuki = append(itog.Stuki, stuk(ctx, o, p, i == 0))
	}
	if err := ctx.Err(); err != nil {
		return Itog{}, err
	}
	// Дошёл ли стук, важно, только когда напрямую не открылось ничего.
	if o.Lovushka != nil && !slices.ContainsFunc(itog.Stuki, func(s Stuk) bool { return s.Ishod == Otkryt }) {
		zanyaty := []int{o.PortSsh}
		for _, p := range o.TCP {
			zanyaty = append(zanyaty, p.Port)
		}
		itog.Doshlo, itog.OtkazLovushki = lovit(ctx, o, "tcp", sluchaynyyPort(zanyaty))
	}
	if o.Lovushka != nil && len(o.UDP) > 0 {
		var otkaz string
		if itog.DoshloUDP, otkaz = lovit(ctx, o, "udp", sluchaynyyPort(o.UDP)); otkaz != "" && itog.OtkazLovushki == "" {
			itog.OtkazLovushki = otkaz
		}
	}
	if err := ctx.Err(); err != nil {
		return Itog{}, err
	}
	itog.Vid = vynesti(itog.Set, itog.Stuki, o.Tunnel, itog.Doshlo)
	return itog, nil
}

// vynesti складывает вердикт круга.
func vynesti(set Set, stuki []Stuk, tunnel Tunnel, doshlo int64) Vid {
	switch set {
	case SetMertva:
		return NeSostoyalas
	case SetBelyeSpiski:
		return BelyeSpiski
	}
	var otkryto, otvet, glushat, chuzhoy, sboy int
	for _, s := range stuki {
		switch s.Ishod {
		case Otkryt:
			otkryto++
		case Sboy:
			sboy++
		}
		switch s.Otvet {
		case OtvetEst:
			otvet++
		case OtvetChuzhoy:
			chuzhoy++
		case OtvetObryv, OtvetTishina:
			glushat++
		}
	}
	switch {
	case len(stuki) == 0 || sboy == len(stuki):
		return NeSostoyalas
	case glushat > 0:
		return Glushat
	case otvet > 0:
		return Dostupen
	case chuzhoy > 0:
		return Podmena
	case otkryto > 0:
		return Dostupen
	case doshlo > 0:
		return OtvetNeVozvrashchaetsya
	// Ловушка ответила нулём: она спрошена по SSH через туннель, значит,
	// сервер жив, что бы ни показал вход раньше.
	case tunnel == TunnelOtvetil || doshlo == 0:
		return NeDohodit
	case tunnel == TunnelMolchit:
		return ServerMolchit
	}
	return MolchitNapryamuyu
}

// stuk стучит в порт p. На порту SSH ждётся приветствие сервера, на порту с
// именем TLS - ответ на приветствие TLS. Молчание после соединения на SSH не
// судится: так же молчит sshd, который наказывает адрес за неудачные входы.
func stuk(ctx context.Context, o Opcii, p PortTCP, ssh bool) Stuk {
	s := Stuk{Port: p.Port}
	do, otm := context.WithTimeout(ctx, srokStuka)
	c, err := o.Pryamo(do, "tcp", net.JoinHostPort(o.Adres.String(), strconv.Itoa(p.Port)))
	otm()
	if err != nil {
		s.Ishod, s.Podrobno = ishodSoedineniya(err), err.Error()
		return s
	}
	defer c.Close()
	s.Ishod = Otkryt
	if err := c.SetDeadline(time.Now().Add(srokOtveta)); err != nil {
		s.Podrobno = err.Error()
		return s
	}
	switch {
	case ssh:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			s.Podrobno = err.Error()
			return s
		}
		s.Otvet = OtvetChuzhoy
		if string(b) == "SSH-" {
			s.Otvet = OtvetEst
		}
	case p.TLS != "":
		// На порту reality чужое приветствие уходит маске, и отвечает она
		// своим настоящим сертификатом. Чужой сертификат значит, что
		// ответил кто-то по дороге.
		t := tls.Client(c, &tls.Config{ServerName: p.TLS, RootCAs: korni})
		err := t.HandshakeContext(ctx)
		var trevoga *net.OpError
		var zapis tls.RecordHeaderError
		var sert *tls.CertificateVerificationError
		switch {
		// Тревога TLS от сервера это тоже ответ по-TLS: Go отдаёт её
		// ошибкой «remote error».
		case err == nil, errors.As(err, &trevoga) && trevoga.Op == "remote error":
			s.Otvet = OtvetEst
		case errors.As(err, &zapis), errors.As(err, &sert):
			s.Otvet = OtvetChuzhoy
		case istekSrok(err):
			s.Otvet = OtvetTishina
		default:
			s.Otvet = OtvetObryv
		}
		if err != nil {
			s.Podrobno = err.Error()
		}
	}
	return s
}

func ishodSoedineniya(err error) Ishod {
	switch {
	case errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, syscall.ECONNREFUSED):
		return Otkaz
	case errors.Is(err, windows.WSAEHOSTUNREACH) || errors.Is(err, windows.WSAENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH):
		return Nedostupen
	case istekSrok(err):
		return Tishina
	}
	return Sboy
}

func istekSrok(err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout()
}

// proveritSet судит свою сеть: зарубежные цели это контроль, российские
// отличают белые списки от мёртвой сети.
func proveritSet(ctx context.Context, o Opcii) Set {
	switch {
	case otvechaet(ctx, o, zarubezhnye):
		return SetZhivaya
	case otvechaet(ctx, o, rossiyskie):
		return SetBelyeSpiski
	}
	return SetMertva
}

func otvechaet(ctx context.Context, o Opcii, celi []string) bool {
	for _, c := range celi {
		err := tlsDo(ctx, o, c)
		if err == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		log.Printf("детектор: %s не ответил напрямую: %v", c, err)
	}
	return false
}

// tlsDo это рукопожатие TLS с цепочкой до cel напрямую.
func tlsDo(ctx context.Context, o Opcii, cel string) error {
	imya, port, err := net.SplitHostPort(cel)
	if err != nil {
		return err
	}
	adresa, err := najti(ctx, o, imya)
	if err != nil {
		return err
	}
	do, otm := context.WithTimeout(ctx, srokStuka+srokOtveta)
	defer otm()
	c, err := o.Pryamo(do, "tcp", net.JoinHostPort(adresa[0].Unmap().String(), port))
	if err != nil {
		return err
	}
	defer c.Close()
	return tls.Client(c, &tls.Config{ServerName: imya, RootCAs: korni}).HandshakeContext(do)
}

// najti ищет адреса IPv4 имени: системой, иначе у публичного DNS через
// HTTPS по Pryamo. Системный поиск при запущенном ядре идёт через туннель,
// а тот сам может быть мёртв. DNS адаптера мимо туннеля не спросить: порт 53
// на физическом адаптере закрывают фильтры защиты ядра (проверено
// 01.11.2026: UDP молчит, TCP отвергнут правами доступа), а без ядра его и
// так спрашивает система. В белых списках при мёртвом туннеле закрыт и
// публичный DNS: тогда сеть не судится, и проверка честно не состоялась.
func najti(ctx context.Context, o Opcii, imya string) ([]netip.Addr, error) {
	sposoby := []func(context.Context) ([]netip.Addr, error){
		func(ctx context.Context) ([]netip.Addr, error) { return iskat(ctx, "ip4", imya) },
		func(ctx context.Context) ([]netip.Addr, error) { return sprositDoH(ctx, o.Pryamo, imya) },
	}
	var oshibki []error
	for _, s := range sposoby {
		do, otm := context.WithTimeout(ctx, srokPoiska)
		adresa, err := s(do)
		otm()
		if err == nil && len(adresa) == 0 {
			err = fmt.Errorf("у %s нет адреса IPv4", imya)
		}
		if err == nil {
			return adresa, nil
		}
		oshibki = append(oshibki, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(oshibki...)
}

// lovit ставит ловушку на порт и стучит в него напрямую, пока она слушает.
// Отдаёт, сколько попыток дошло, или -1 и почему счёт ничего не значит.
func lovit(ctx context.Context, o Opcii, proto string, port int) (int64, string) {
	srok := time.Duration(sekundLovushki)*time.Second + zapasLovushki
	do, otm := context.WithTimeout(ctx, srok)
	defer otm()
	itog := make(chan otvetLovushki, 1)
	fon.Zapustit("svoyvps/blok.lovushka", func() {
		n, err := o.Lovushka(do, proto, port, sekundLovushki)
		itog <- otvetLovushki{n, err}
	})
	adres := net.JoinHostPort(o.Adres.String(), strconv.Itoa(port))
	nachalo := time.Now()
	ushlo := 0
	var mestnaya error
	for i := range stukovLovushki {
		t := time.NewTimer(time.Until(nachalo.Add(zhdatLovushku + time.Duration(i)*shagStuka)))
		select {
		case <-ctx.Done():
			t.Stop()
			return -1, ctx.Err().Error()
		case r := <-itog:
			// Ловушка ответила до конца стука, обычно отказом: дальше
			// стучать некому.
			t.Stop()
			return r.schet(proto, port, ushlo, mestnaya)
		case <-t.C:
		}
		if err := stuknut(ctx, o, proto, adres); err != nil {
			mestnaya = err
		} else {
			ushlo++
		}
	}
	select {
	case <-do.Done():
		if ctx.Err() != nil {
			return -1, ctx.Err().Error()
		}
		return -1, fmt.Sprintf("ловушка не ответила за %s", srok)
	case r := <-itog:
		return r.schet(proto, port, ushlo, mestnaya)
	}
}

type otvetLovushki struct {
	n   int64
	err error
}

// schet это счёт ловушки, если он что-то значит: ноль без единого ушедшего
// стука не говорит ни о чём.
func (r otvetLovushki) schet(proto string, port, ushlo int, mestnaya error) (int64, string) {
	switch {
	case r.err != nil:
		log.Printf("детектор: ловушка %s/%d отказала: %v", proto, port, r.err)
		return -1, r.err.Error()
	case ushlo > 0:
		return r.n, ""
	case mestnaya != nil:
		return -1, "стук не ушёл с компьютера: " + mestnaya.Error()
	}
	return -1, "ловушка ответила раньше стука"
}

// stuknut это одна попытка: соединение TCP или три датаграммы UDP. Дошла ли
// она, судит ловушка; ошибка значит, что стук не ушёл с компьютера.
func stuknut(ctx context.Context, o Opcii, proto, adres string) error {
	do, otm := context.WithTimeout(ctx, srokStukaLovushki)
	defer otm()
	c, err := o.Pryamo(do, proto, adres)
	if err != nil {
		// Сброс, тишина и «недоступен» приходят с той стороны: SYN ушёл.
		if ishodSoedineniya(err) == Sboy {
			return err
		}
		return nil
	}
	defer c.Close()
	if proto == "udp" {
		for i := range 3 {
			if _, err := c.Write([]byte("affory-stuk")); err != nil {
				if i == 0 {
					return err
				}
				return nil
			}
		}
	}
	return nil
}

// sluchaynyyPort это порт, который ничто не слушает и боты почти не
// трогают: стук в него ловушка отличит от чужого. Диапазон ниже эфемерных
// портов Linux (32768-60999): ловушка UDP считает любую датаграмму, и на
// эфемерном порту ей попались бы ответы на собственные потоки сервера.
func sluchaynyyPort(zanyaty []int) int {
	for {
		p := 20000 + rand.IntN(12768)
		if !slices.Contains(zanyaty, p) {
			return p
		}
	}
}

func podozhdat(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// zashchita ставится первой строкой каждого входа: пакет живёт в службе.
func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/blok."+gde, r)
		*err = fmt.Errorf("паника в blok.%s: %v", gde, r)
	}
}
