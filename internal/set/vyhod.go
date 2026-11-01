package set

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Умолчание эндпоинта checkExitIp (§5 спеки: адрес выхода узнаётся внешним
// запросом). Владелец умолчания не выбирал; принято 03.09.2026 без него в
// пользу того же адреса, которым стенд пользуется с волны 2: отвечает голым
// адресом в теле, без JSON и без ключей. Переопределяется полем
// adres_proverki в файле состояния.
const AdresProverkiPoUmolchaniyu = "https://api.ipify.org"

const srokProverkiVyhoda = 15 * time.Second

// AdresVyhoda спрашивает у внешнего эндпоинта, каким адресом нас видят.
//
// portProksi это локальный mixed-вход sing-box. Собственные процессы службы по
// правилу петли идут МИМО туннеля, поэтому запрос напрямую из службы показал
// бы домашний адрес при поднятом туннеле и назвал бы это утечкой. Через прокси
// запрос идёт тем же путём, что и трафик приложений. Ноль означает
// «напрямую», и это тоже законный замер: контрольная половина сравнения.
func AdresVyhoda(ctx context.Context, endpoint string, portProksi int) (string, error) {
	var proksi *url.URL
	if portProksi > 0 {
		proksi = &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", portProksi)}
	}
	return AdresVyhodaCherez(ctx, endpoint, proksi)
}

// AdresVyhodaCherez это тот же вопрос через заданный прокси, nil значит
// напрямую. Проверка ключей своего VPN ходит через вход ядра проверки, и
// логин в адресе прокси называет ключ.
func AdresVyhodaCherez(ctx context.Context, endpoint string, proksi *url.URL) (string, error) {
	do, otm := context.WithTimeout(ctx, srokProverkiVyhoda)
	defer otm()
	z, err := http.NewRequestWithContext(do, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("запрос адреса выхода не собран: %w", err)
	}
	// DisableKeepAlives намеренно. Транспорт тут собирается на вызов, потому что
	// прокси у каждого вызова свой, а транспорт, собранный на вызов, уносит
	// соединение в свой пул простоя и хоронит его там навсегда (разбор у
	// klientKlash в internal/yadra). Держать пул нечему: адрес выхода
	// спрашивают на подъёме и при смене несущего, а не в цикле.
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	if proksi != nil {
		tr.Proxy = http.ProxyURL(proksi)
	}
	kl := &http.Client{Transport: tr}
	o, err := kl.Do(z)
	if err != nil {
		return "", fmt.Errorf("адрес выхода не получен: %w", err)
	}
	defer o.Body.Close()
	if o.StatusCode != http.StatusOK {
		return "", fmt.Errorf("эндпоинт адреса выхода ответил кодом %d", o.StatusCode)
	}
	telo, err := io.ReadAll(io.LimitReader(o.Body, 256))
	if err != nil {
		return "", fmt.Errorf("ответ эндпоинта не дочитан: %w", err)
	}
	// Тело обязано быть АДРЕСОМ. Страница «403 Forbidden» чужого прокси,
	// принятая за адрес, уехала бы на экран как адрес выхода.
	a, err := netip.ParseAddr(strings.TrimSpace(string(telo)))
	if err != nil {
		return "", fmt.Errorf("эндпоинт ответил не адресом: %q", strings.TrimSpace(string(telo)))
	}
	return a.String(), nil
}
