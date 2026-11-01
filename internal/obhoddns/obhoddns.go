// Package obhoddns спрашивает адрес имени у публичного DNS через HTTPS, когда
// DNS системы его не нашёл.
//
// Повод, жалоба 02.10.2026: провайдер не отдавал по DNS имя, на котором
// висят все серверы подписки человека. Клиент не мог ни
// подключиться, ни обновить подписку, и окно звало чинить адаптер.
//
// Обход только запасной: сначала всегда спрашивается DNS системы. Он знает
// имена локальной сети и корпоративного DNS, а публичный о них не знает, и
// лишний раз отдавать имена своих серверов чужому DNS тоже незачем.
package obhoddns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
)

// Adresa это DNS через HTTPS по IP, а не по имени: имя самого DNS-сервиса
// пришлось бы спрашивать у того же DNS провайдера. Сертификаты Google и
// Cloudflare выписаны и на эти IP, поэтому TLS проверяется обычным порядком.
// Проверено 02.10.2026: Quad9 на IP отвечал 505 по HTTP/1.1, Яндекс на IP не
// отвечал вовсе.
var Adresa = []string{
	"https://8.8.8.8/dns-query",
	"https://1.1.1.1/dns-query",
	"https://8.8.4.4/dns-query",
	"https://1.0.0.1/dns-query",
}

// Taymaut это срок на весь обход одного имени. Все адреса спрашиваются разом,
// поэтому медленный или закрытый у провайдера сервис не съедает срок
// соседних.
var Taymaut = 4 * time.Second

// potolokOtveta ограничивает тело ответа: ответ DNS на одно имя укладывается в
// пару килобайт, и мегабайт от сервиса это не ответ.
const potolokOtveta = 64 << 10

// ErrNetImeni значит, что публичный DNS ответил честным «такого имени нет».
var ErrNetImeni = errors.New("публичный DNS ответил, что такого имени нет")

var klient = klientCherez((&net.Dialer{Timeout: 3 * time.Second}).DialContext)

func klientCherez(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Client {
	return &http.Client{Transport: &http.Transport{
		// Прокси из окружения не берётся: обход заменяет DNS системы и идёт
		// прямо, как шёл бы он.
		Proxy:               nil,
		DialContext:         dial,
		TLSHandshakeTimeout: 3 * time.Second,
		// Обход случается редко, и держать открытыми четыре соединения к
		// чужим сервисам ради следующего раза незачем. HTTP/1.1 оба сервиса
		// принимают.
		DisableKeepAlives: true,
	}}
}

// Sprosit отдаёт адреса IPv4 имени по ответу первого из публичных DNS.
func Sprosit(ctx context.Context, host string) ([]netip.Addr, error) {
	return sprositVse(ctx, klient, Adresa, host)
}

// SprositCherez это Sprosit со своим путём соединения: детектор блокировки
// спрашивает мимо туннеля, через физический адаптер.
func SprositCherez(ctx context.Context, dial func(ctx context.Context, network, addr string) (net.Conn, error), host string) ([]netip.Addr, error) {
	return sprositVse(ctx, klientCherez(dial), Adresa, host)
}

func sprositVse(ctx context.Context, k *http.Client, adresa []string, host string) ([]netip.Addr, error) {
	ctx, otmena := context.WithTimeout(ctx, Taymaut)
	defer otmena()
	type itog struct {
		adresa []netip.Addr
		err    error
	}
	// Буфер на все ответы: опоздавшие пишут в него и выходят, даже когда
	// годный ответ уже забран и никто их не читает.
	otvety := make(chan itog, len(adresa))
	for _, a := range adresa {
		fon.Zapustit("обходе DNS через "+a, func() {
			r, err := sprosit(ctx, k, a, host)
			otvety <- itog{r, err}
		})
	}
	var oshibki []error
	for range adresa {
		it := <-otvety
		if it.err == nil {
			return it.adresa, nil
		}
		oshibki = append(oshibki, it.err)
	}
	// Честное «нет такого имени» важнее сбоев соседей: оно и есть ответ.
	for _, e := range oshibki {
		if errors.Is(e, ErrNetImeni) {
			return nil, e
		}
	}
	return nil, fmt.Errorf("публичный DNS через HTTPS не ответил: %w", oshibki[0])
}

// sprosit задаёт один вопрос по RFC 8484: тип A, сообщение DNS в теле POST.
func sprosit(ctx context.Context, k *http.Client, adres, host string) ([]netip.Addr, error) {
	imya, err := dnsmessage.NewName(strings.TrimSuffix(host, ".") + ".")
	if err != nil {
		return nil, fmt.Errorf("имя %q не годится для DNS: %w", host, err)
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{RecursionDesired: true})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(dnsmessage.Question{Name: imya, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	vopros, err := b.Finish()
	if err != nil {
		return nil, err
	}

	zapros, err := http.NewRequestWithContext(ctx, http.MethodPost, adres, bytes.NewReader(vopros))
	if err != nil {
		return nil, err
	}
	zapros.Header.Set("Content-Type", "application/dns-message")
	zapros.Header.Set("Accept", "application/dns-message")
	otvet, err := k.Do(zapros)
	if err != nil {
		return nil, err
	}
	defer otvet.Body.Close()
	if otvet.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s ответил кодом %d", adres, otvet.StatusCode)
	}
	telo, err := io.ReadAll(io.LimitReader(otvet.Body, potolokOtveta))
	if err != nil {
		return nil, fmt.Errorf("%s: ответ не дочитан: %w", adres, err)
	}
	return razobrat(telo, host)
}

// razobrat достаёт адреса A из ответа. CNAME по дороге не мешает: рекурсивный
// DNS кладёт в ответ всю цепочку вместе с конечными адресами.
func razobrat(telo []byte, host string) ([]netip.Addr, error) {
	var p dnsmessage.Parser
	h, err := p.Start(telo)
	if err != nil {
		return nil, fmt.Errorf("ответ DNS не разобран: %w", err)
	}
	switch h.RCode {
	case dnsmessage.RCodeSuccess:
	case dnsmessage.RCodeNameError:
		return nil, fmt.Errorf("%w: %s", ErrNetImeni, host)
	default:
		return nil, fmt.Errorf("публичный DNS отказал: код ответа %s", h.RCode)
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, fmt.Errorf("ответ DNS не разобран: %w", err)
	}
	var adresa []netip.Addr
	for {
		zag, err := p.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ответ DNS не разобран: %w", err)
		}
		if zag.Type != dnsmessage.TypeA || zag.Class != dnsmessage.ClassINET {
			if err := p.SkipAnswer(); err != nil {
				return nil, fmt.Errorf("ответ DNS не разобран: %w", err)
			}
			continue
		}
		a, err := p.AResource()
		if err != nil {
			return nil, fmt.Errorf("ответ DNS не разобран: %w", err)
		}
		adresa = append(adresa, netip.AddrFrom4(a.A))
	}
	if len(adresa) == 0 {
		return nil, fmt.Errorf("у имени %s нет адреса IPv4", host)
	}
	return adresa, nil
}

// Nabrat это DialContext для http.Transport: соединяется обычным порядком, а
// если упал именно поиск имени, берёт адрес у публичного DNS и соединяется с
// ним. Имя в TLS остаётся прежним: его берёт транспорт из адреса запроса.
//
// Исходная ошибка сохраняется через %w, когда не помог и обход: классификатор
// сбоев (internal/sboi) по ней называет шаг, на котором всё сломалось.
func Nabrat(ctx context.Context, network, addr string) (net.Conn, error) {
	return nabrat(ctx, &net.Dialer{}, Sprosit, network, addr)
}

func nabrat(ctx context.Context, d *net.Dialer, sprositObhodom func(context.Context, string) ([]netip.Addr, error), network, addr string) (net.Conn, error) {
	conn, err := d.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}
	var oshibkaDNS *net.DNSError
	if !errors.As(err, &oshibkaDNS) || ctx.Err() != nil {
		return nil, err
	}
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, err
	}
	adresa, e := sprositObhodom(ctx, host)
	if e != nil {
		log.Printf("имя %s не нашлось ни у DNS системы, ни у публичного DNS через HTTPS: %v", host, e)
		return nil, fmt.Errorf("%w (обход DNS: %v)", err, e)
	}
	log.Printf("имя %s не нашлось у DNS системы, адрес взят у публичного DNS через HTTPS", host)
	for _, a := range adresa {
		if conn, e = d.DialContext(ctx, network, net.JoinHostPort(a.String(), port)); e == nil {
			return conn, nil
		}
	}
	return nil, e
}
