// Package proverka проверяет ключи своего VPN трафиком из сети человека:
// ключ годен, когда через него с адреса сервера доехали 2 МБ.
//
// «Ядро приняло ссылку» и «ключ везёт» это разные утверждения. Между ними
// неверный ключ reality, чужой пароль, закрытый порт и провайдер, который
// пропускает рукопожатие и первые килобайты, а дальше замораживает поток.
// Поэтому каждый ключ качает 2 МБ, а не меряет задержку, и адрес выхода
// сверяется с адресом сервера. Тем же путём подбор портов проверяет
// кандидатов.
//
// Ядро проверки своё, с конфигом вне набора, и ходит к серверу через
// физический адаптер мимо любых туннелей: иначе проверка мерила бы чужой
// VPN, а при поднятом своём туннеле рвалась бы его перезапуском.
//
// Испорченная копия каждого вида ключа проверяется тем же путём и обязана не
// пройти. Прошла - проверка живой ключ от мёртвого не отличает, и итогу
// верить нельзя.
package proverka

import (
	"cmp"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// razmer это сколько байт обязан провезти ключ.
const razmer = 2_000_000

const tegVhoda = "proverka-in"

// celZagruzki отдаёт тело заданного размера и называет адрес, с которого
// пришёл запрос, заголовком cf-meta-ip (проверено 01.11.2026): адрес и поток
// проверяются одним соединением.
var celZagruzki = "https://speed.cloudflare.com/__down?bytes=" + strconv.Itoa(razmer)

var (
	// Ключи идут по одному с паузой: залп рукопожатий замораживал адрес
	// сервера на 70-150 с (02.10.2026).
	pauza = 2 * time.Second
	// Ни байта дольше tishina значит, что поток встал. Общий срок отдельно:
	// 2 МБ за минуту это 270 Кбит/с, медленнее ключом не пользоваться.
	tishina     = 10 * time.Second
	srokKlyucha = time.Minute
	srokPodyoma = 15 * time.Second
)

// Vid это чем кончилась проверка ключа.
type Vid string

const (
	// Vezyot: адрес выхода сервера, 2 МБ доехали.
	Vezyot Vid = "vezyot"
	// NetOtveta: через ключ не пришло ответа. Закрытый порт, неверный ключ
	// и блок по дороге выглядят здесь одинаково, их различает детектор.
	NetOtveta Vid = "net-otveta"
	// Zamerz: ответ пошёл и встал.
	Zamerz Vid = "zamerz"
	// ChuzhoyVyhod: ключ везёт, но цель видит не адрес сервера.
	ChuzhoyVyhod Vid = "chuzhoy-vyhod"
	// Cel: ключ провёз запрос, но цель ответила не тем, и проверка не
	// закончена. Блокировки здесь нет.
	Cel Vid = "cel"
)

// Itog это проверка одного ключа.
type Itog struct {
	Server protokol.Server
	Vid    Vid
	// Vyhod это адрес, который назвала цель; пустой, если не назвала.
	Vyhod  netip.Addr
	Bayt   int64
	Vremya time.Duration
	// Otkaz говорит, что не так; пусто у Vezyot.
	Otkaz string
}

// ErrNegoden значит, что испорченная копия ключа прошла: проверка живой ключ
// от мёртвого не отличает.
var ErrNegoden = errors.New("проверка ключей негодна")

// Opcii это обстоятельства проверки.
type Opcii struct {
	// Adapter это имя физического адаптера (set.Adapter.Imya), через который
	// ядро ходит к серверу.
	Adapter string
	// Vyhody это все адреса сервера из осмотра, обоих семейств: с любого из
	// них он может выйти в интернет. Адрес самого ключа годен и без них. Для
	// ключа на IPv6 обязательны: при адресе IPv4 программа сервера выходит
	// наружу через него.
	Vyhody []netip.Addr
	// CelZagruzki это адрес, откуда качаются 2 МБ; ответ обязан назвать
	// адрес запроса заголовком cf-meta-ip или CelVyhoda спросят отдельно.
	// Пусто значит Cloudflare. Свою цель ставит петля приёмки.
	CelZagruzki string
	// CelVyhoda отвечает голым адресом запроса, как adres_proverki клиента.
	// Пусто значит set.AdresProverkiPoUmolchaniyu.
	CelVyhoda string
	// Korni это корни доверия к CelZagruzki; nil значит системные. Свой
	// корень ставит петля приёмки: её цель обязана быть https, потому что
	// vision с голым HTTP внутри изредка встаёт на ~163 КБ (петля 01.11,
	// 3 из 314 загрузок; с TLS внутри 0 из 310).
	Korni *x509.CertPool
	// PervyyNaVid: по каждому виду ключа хватает первого, что везёт,
	// остальные этого вида не проверяются. Так подбор портов идёт по
	// кандидатам.
	PervyyNaVid bool
	// Hod узнаёт о каждом ключе в той же горутине: перед проверкой итогом с
	// пустым Vid и затем настоящим итогом. Оборвалась проверка ошибкой
	// (отмена, упавшее ядро) - второго вызова нет, ошибку отдаёт Proverit.
	// Испорченные копии сюда не попадают.
	Hod func(Itog)
}

// Zapusk поднимает ядро на конфиге и отдаёт его остановку. Файл конфига,
// путь ядра и его сверка с установкой у службы свои, поэтому запуск снаружи.
type Zapusk func(ctx context.Context, konfig []byte) (ostanovit func(), err error)

// Proverit проверяет ключи по одному в их порядке, затем испорченные копии.
// Итог есть по каждому проверенному ключу, в порядке проверки; ошибка
// значит, что проверка не состоялась.
//
// Испорченная копия одна на вид ключа и стоит на том же входе, что первый
// ключ этого вида, который везёт, а если такого нет, то первый, получивший
// хоть какой-то ответ. Копия на порту без ответа ничего не доказывает, а
// вид, где ответа не получил никто, контроля не требует: верить там нечему.
func Proverit(ctx context.Context, klyuchi []protokol.Server, o Opcii, zapusk Zapusk) (itogi []Itog, err error) {
	defer zashchita("Proverit", &err)
	if o.Adapter == "" {
		return nil, errors.New("проверка ключей: не назван адаптер мимо туннелей")
	}
	if len(klyuchi) == 0 {
		return nil, errors.New("проверка ключей: ключей нет")
	}
	// Копия на каждый ключ: какой из них понадобится, видно только после
	// проверки, а исходящие ядра заданы до запуска.
	kontrol := make([]protokol.Server, len(klyuchi))
	idy := map[string]bool{}
	for i, k := range klyuchi {
		// Id даёт тег исходящего, по нему служба находит жалобу ядра.
		if k.Id == "" || idy[k.Id] {
			return nil, fmt.Errorf("проверка ключей: у ключа %q нет своего Id", k.Imya)
		}
		idy[k.Id] = true
		if a, err := netip.ParseAddr(k.Host); err == nil && a.Unmap().Is6() && len(o.Vyhody) == 0 {
			return nil, fmt.Errorf("проверка ключей: ключ %q на IPv6, а адреса выхода сервера не названы", k.Imya)
		}
		if kontrol[i], err = isportit(k); err != nil {
			return nil, err
		}
	}
	port, err := yadra.VydatPort()
	if err != nil {
		return nil, err
	}
	parol, err := sluchaynyy()
	if err != nil {
		return nil, err
	}
	konfig, err := sobrat(klyuchi, kontrol, o.Adapter, port, parol)
	if err != nil {
		return nil, err
	}
	ostanovit, err := zapusk(ctx, konfig)
	if err != nil {
		return nil, fmt.Errorf("ядро проверки не поднялось: %w", err)
	}
	if ostanovit == nil {
		return nil, errors.New("ядро проверки: запуск не вернул остановки")
	}
	defer ostanovit()
	p := progon{
		vhod: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), parol: parol,
		celZagruzki: cmp.Or(o.CelZagruzki, celZagruzki), celVyhoda: cmp.Or(o.CelVyhoda, set.AdresProverkiPoUmolchaniyu),
		korni: o.Korni,
	}
	if err := dozhdatsyaVhoda(ctx, p.vhod); err != nil {
		return nil, err
	}
	hod := func(it Itog) {
		if o.Hod != nil {
			o.Hod(it)
		}
	}
	vezyot := map[string]bool{} // вид уже везёт
	for i, k := range klyuchi {
		if o.PervyyNaVid && vezyot[k.Transport] {
			continue
		}
		if len(itogi) > 0 {
			if err := pomolchat(ctx); err != nil {
				return nil, err
			}
		}
		hod(Itog{Server: k})
		it, err := p.odin(ctx, login(i), k, vyhodyServera(o.Vyhody, k))
		if err != nil {
			return nil, err
		}
		itogi = append(itogi, it)
		hod(it)
		vezyot[k.Transport] = vezyot[k.Transport] || it.Vid == Vezyot
	}
	for _, s := range kogoKontrolirovat(itogi) {
		if err := pomolchat(ctx); err != nil {
			return nil, err
		}
		// Id у ключей свои, это проверено до запуска.
		i := slices.IndexFunc(klyuchi, func(k protokol.Server) bool { return k.Id == s.Id })
		it, err := p.odin(ctx, loginKontrolya(i), kontrol[i], nil)
		if err != nil {
			return nil, err
		}
		// Отменённый контроль выглядел бы мёртвым: отмена проверяется в odin
		// раньше вида.
		if it.Vid != NetOtveta {
			return nil, fmt.Errorf("%w: испорченная копия %s на порту %d получила ответ (%s)", ErrNegoden, s.Transport, s.Port, it.Vid)
		}
	}
	return itogi, nil
}

// kogoKontrolirovat выбирает по каждому виду ключ, чья испорченная копия
// пойдёт в контроль: первый, что везёт, иначе первый с каким-нибудь ответом.
// Вид без единого ответа не выбирается. Порядок видов - порядок итогов.
func kogoKontrolirovat(itogi []Itog) []protokol.Server {
	var vidy []string
	vybor := map[string]Itog{}
	for _, it := range itogi {
		vid := it.Server.Transport
		if !slices.Contains(vidy, vid) {
			vidy = append(vidy, vid)
		}
		bylo, est := vybor[vid]
		if est && bylo.Vid == Vezyot {
			continue
		}
		if it.Vid == Vezyot || !est && it.Vid != NetOtveta {
			vybor[vid] = it
		}
	}
	var k []protokol.Server
	for _, vid := range vidy {
		if it, est := vybor[vid]; est {
			k = append(k, it.Server)
		}
	}
	return k
}

// zapasKlyucha это сколько ключ идёт сверх срока потока: стук во вход ядра
// и отдельный запрос адреса выхода (до 15 с у set.AdresVyhodaCherez).
const zapasKlyucha = 20 * time.Second

// Predel это самое долгое, сколько идёт проверка klyuchey ключей vidov
// видов вместе с контролем: подбор портов ставит по нему срок пробного
// пакета.
func Predel(klyuchey, vidov int) time.Duration {
	// Контроль один на вид, и видов не больше, чем ключей.
	return srokPodyoma + time.Duration(klyuchey+min(vidov, klyuchey))*(pauza+srokKlyucha+zapasKlyucha)
}

func login(i int) string          { return "klyuch" + strconv.Itoa(i+1) }
func loginKontrolya(i int) string { return "kontrol" + strconv.Itoa(i+1) }

// sobrat это конфиг ядра проверки: один вход на петле, логин называет ключ,
// исходящие привязаны к адаптеру. Чужой логин отбивается, а не уходит в
// первый исходящий.
func sobrat(klyuchi, kontrol []protokol.Server, adapter string, port int, parol string) ([]byte, error) {
	var polzovateli, ishodyashchie, pravila []any
	dobavit := func(login, teg string, s protokol.Server) error {
		// Проверяются только свои ключи, и исходящий собирается тем же
		// сборщиком, что в ядре клиента: reality своему серверу идёт с
		// firefox и здесь, и там, и проверка судит то приветствие, что
		// повезёт трафик. Chrome нужен чужим серверам на Xray, а частые
		// chrome-приветствия к одному адресу на подборе замораживают его в
		// части сетей (§3.3 плана).
		s.Svoy = true
		o, err := genkonfig.Ishodyashchiy(s, teg)
		if err != nil {
			return fmt.Errorf("проверка ключей: ключ %q: %w", s.Imya, err)
		}
		o["bind_interface"] = adapter
		ishodyashchie = append(ishodyashchie, o)
		polzovateli = append(polzovateli, map[string]any{"username": login, "password": parol})
		pravila = append(pravila, map[string]any{"inbound": []string{tegVhoda}, "auth_user": []string{login}, "outbound": teg})
		return nil
	}
	for i, k := range klyuchi {
		if err := dobavit(login(i), genkonfig.TegKandidata(k.Id), k); err != nil {
			return nil, err
		}
	}
	for i, k := range kontrol {
		if err := dobavit(loginKontrolya(i), "kontrol-"+strconv.Itoa(i+1), k); err != nil {
			return nil, err
		}
	}
	pravila = append(pravila, map[string]any{"action": "reject"})
	return json.Marshal(map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"inbounds": []any{map[string]any{
			"type": "mixed", "tag": tegVhoda, "listen": "127.0.0.1", "listen_port": port, "users": polzovateli,
		}},
		"outbounds": ishodyashchie,
		"route":     map[string]any{"rules": pravila},
	})
}

// isportit даёт копию ключа на тот же вход с чужим секретом.
func isportit(s protokol.Server) (protokol.Server, error) {
	switch s.Transport {
	case "reality-tcp":
		k, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return s, fmt.Errorf("проверка ключей: ключ контроля: %w", err)
		}
		s.PublicKey = base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
	case "hy2":
		p, err := sluchaynyy()
		if err != nil {
			return s, err
		}
		s.Parol = p
	default:
		return s, fmt.Errorf("проверка ключей: контроля для %s нет, а без него итогу не верить", s.Transport)
	}
	s.Imya = "контроль " + s.Transport
	return s, nil
}

func sluchaynyy() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("проверка ключей: случайные байты: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// vyhodyServera это адреса, с которых цель вправе видеть ключ, в том виде,
// в каком odin сверяет адрес цели: IPv4 без записи IPv6.
func vyhodyServera(vyhody []netip.Addr, k protokol.Server) []netip.Addr {
	var v []netip.Addr
	if a, err := netip.ParseAddr(k.Host); err == nil {
		v = append(v, a.Unmap())
	}
	for _, a := range vyhody {
		v = append(v, a.Unmap())
	}
	return v
}

// postuchat открывает и закрывает соединение со входом ядра проверки.
func postuchat(ctx context.Context, adres string) error {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", adres)
	if err != nil {
		return err
	}
	return c.Close()
}

// vhodZhiv отличает мёртвый ключ от упавшего ядра проверки: без этого все
// ключи после падения ядра вышли бы «нет ответа», и детектор искал бы
// блокировку живого сервера. Порт, занятый за это время чужой программой,
// так не ловится.
func vhodZhiv(ctx context.Context, adres string) error {
	do, otm := context.WithTimeout(ctx, time.Second)
	defer otm()
	if err := postuchat(do, adres); err != nil {
		return fmt.Errorf("ядро проверки перестало принимать соединения: %w", err)
	}
	return nil
}

func dozhdatsyaVhoda(ctx context.Context, adres string) error {
	do, otm := context.WithTimeout(ctx, srokPodyoma)
	defer otm()
	for {
		err := postuchat(do, adres)
		if err == nil {
			return nil
		}
		t := time.NewTimer(100 * time.Millisecond)
		select {
		case <-do.Done():
			t.Stop()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("ядро проверки не открыло вход за %s: %w", srokPodyoma, err)
		case <-t.C:
		}
	}
}

func pomolchat(ctx context.Context) error {
	t := time.NewTimer(pauza)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type progon struct {
	vhod, parol            string
	celZagruzki, celVyhoda string
	korni                  *x509.CertPool
}

var errTishina = errors.New("поток молчит")

// odin проверяет один ключ. Ошибка значит отмену ctx или упавшее ядро
// проверки: всё остальное это итог ключа. Отмена проверяется здесь одним
// местом, после всех веток: оборванный отменой запрос иначе стал бы итогом
// «нет ответа» или «поток встал».
func (p progon) odin(ctx context.Context, login string, k protokol.Server, vyhody []netip.Addr) (it Itog, err error) {
	it.Server = k
	nachalo := time.Now()
	defer func() {
		it.Vremya = time.Since(nachalo)
		if ctx.Err() != nil {
			it, err = Itog{}, ctx.Err()
		}
	}()
	// SOCKS5, а не HTTP: на обычный http-запрос HTTP-прокси ядра при мёртвом
	// ключе сам отвечает кодом 502, и испорченная копия в петле 01.11 «получала
	// ответ». Через SOCKS5 запрос идёт туннелем, отвечает только цель.
	proksi := &url.URL{Scheme: "socks5", User: url.UserPassword(login, p.parol), Host: p.vhod}

	do, otm := context.WithTimeout(ctx, srokKlyucha)
	defer otm()
	tihiy, otmTishiny := context.WithCancelCause(do)
	defer otmTishiny(nil)
	storozh := time.AfterFunc(tishina, func() { otmTishiny(errTishina) })
	defer storozh.Stop()

	z, err := http.NewRequestWithContext(tihiy, http.MethodGet, p.celZagruzki, nil)
	if err != nil {
		return it, fmt.Errorf("проверка ключей: запрос не собран: %w", err)
	}
	// Транспорт на один запрос и без пула: соединение каждого ключа своё.
	kl := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proksi), DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: p.korni},
	}}
	o, err := kl.Do(z)
	if err != nil {
		// Человеку уходит наша часть текста, полный остаётся в журнале.
		log.Printf("проверка ключей: %s: нет ответа: %v", k.Imya, err)
		if err := vhodZhiv(ctx, p.vhod); err != nil {
			return it, err
		}
		it.Vid, it.Otkaz = NetOtveta, "нет ответа через ключ: "+sboi.DlyaCheloveka(err)
		return it, nil
	}
	defer o.Body.Close()
	if o.StatusCode != http.StatusOK {
		it.Vid, it.Otkaz = Cel, fmt.Sprintf("цель проверки ответила кодом %d", o.StatusCode)
		return it, nil
	}
	storozh.Reset(tishina)
	it.Bayt, err = io.Copy(io.Discard, sTishinoy{io.LimitReader(o.Body, razmer), storozh})
	if it.Bayt < razmer {
		if err != nil {
			log.Printf("проверка ключей: %s: поток оборвался на %d байтах: %v", k.Imya, it.Bayt, err)
		}
		if err := vhodZhiv(ctx, p.vhod); err != nil {
			return it, err
		}
		it.Vid = Zamerz
		doehalo := fmt.Sprintf("доехало %d КБ из %d КБ", it.Bayt/1000, razmer/1000)
		switch {
		case do.Err() != nil:
			// Поток шёл, но медленнее, чем годится для ключа.
			it.Otkaz = "поток не уложился в срок: " + doehalo
		case err != nil && tihiy.Err() == nil:
			it.Otkaz = "поток встал: " + doehalo + ": " + sboi.DlyaCheloveka(err)
		default:
			it.Otkaz = "поток встал: " + doehalo
		}
		return it, nil
	}
	vyhod := strings.TrimSpace(o.Header.Get("cf-meta-ip"))
	if vyhod == "" {
		// Второе соединение через тот же ключ: поток уже доказан, нужен
		// только адрес.
		if vyhod, err = set.AdresVyhodaCherez(ctx, p.celVyhoda, proksi); err != nil {
			log.Printf("проверка ключей: %s: адрес выхода не назван: %v", k.Imya, err)
			it.Vid, it.Otkaz = Cel, "адрес выхода не назван: "+sboi.DlyaCheloveka(err)
			return it, nil
		}
	}
	a, err := netip.ParseAddr(vyhod)
	if err != nil {
		it.Vid, it.Otkaz = Cel, fmt.Sprintf("цель назвала адрес выхода %q", vyhod)
		return it, nil
	}
	it.Vyhod = a.Unmap()
	if !slices.Contains(vyhody, it.Vyhod) {
		it.Vid, it.Otkaz = ChuzhoyVyhod, "выход с "+it.Vyhod.String()+", а не с адреса сервера"
		return it, nil
	}
	it.Vid = Vezyot
	return it, nil
}

// sTishinoy заводит сторожа тишины заново на каждом пришедшем куске.
type sTishinoy struct {
	r io.Reader
	t *time.Timer
}

func (s sTishinoy) Read(b []byte) (int, error) {
	n, err := s.r.Read(b)
	if n > 0 {
		s.t.Reset(tishina)
	}
	return n, err
}

func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/proverka."+gde, r)
		*err = fmt.Errorf("паника в proverka.%s: %v", gde, r)
	}
}
