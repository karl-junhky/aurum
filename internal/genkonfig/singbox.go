package genkonfig

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Теги. Строки живут в одном месте, потому что ссылка на несуществующий тег это
// ровно та ошибка, которую sing-box check не находит вовсе (проверено пятью
// прогонами: висячий final, outbound, detour, server и rule_set проходят с
// кодом 0 и без единого слова).
const (
	TegPryamo  = "direct"
	TegTunnel  = "tunnel"
	TegMestnyy = "mestnyy"

	// Таблица адресов серверов, найденных службой (02.10.2026). Ядро берёт
	// адрес сервера отсюда, а не у DNS провайдера, см. tablicaServerov.
	TegAdresaServerov = "adresa-serverov"

	// Тег входящего прокси. Отдельная константа, потому что на теги смотрит
	// проверка висячих ссылок, а строка в двух местах разъезжается молча.
	TegProksiVhod = "proksi-in"
)

// SingBox собирает конфиг sing-box целиком.
func SingBox(v Vhod) ([]byte, error) {
	if err := v.proverit(); err != nil {
		return nil, err
	}
	// Блокировка сети вне VPN снимает прямые правила с применения ОДНИМ местом,
	// здесь. Ниже их просто нет, поэтому ни сборка правил, ни DNS, ни наборы
	// не знают про этот режим ничего особенного, и новое правило, добавленное
	// потом, не придётся вспоминать чинить отдельно.
	if v.VesTrafik {
		v.Trafik = bezPryamyh(v.Trafik)
	}
	if err := sveritKandidatov(v); err != nil {
		return nil, err
	}
	kandidaty := v.kandidaty()
	ishodyashchie := make([]any, 0, len(kandidaty)+3)
	tegi := make([]string, 0, len(kandidaty))
	idy := make([]string, 0, len(kandidaty))
	vybrannyy := ""
	for _, s := range kandidaty {
		// Кандидат с транспортом, которого ядро не несёт, ПРОПУСКАЕТСЯ, а не
		// роняет сборку. Сборка идёт одним куском, и отказ здесь означал бы
		// клиент, который не поднимается вовсе из-за одной чужой записи в
		// списке. Так уже было 02.09.2026 с чужой подпиской, и наружу это
		// приезжало как «TUN-адаптер не появился».
		//
		// Выбранный человеком сервер под это правило НЕ попадает: его проверяет
		// proverit() выше и отказывает по имени транспорта. Молча увести трафик
		// на соседнюю запись значило бы показать одно, а сделать другое.
		if !Izvestnyy(s.Transport) {
			continue
		}
		teg := TegKandidata(s.Id)
		o, err := ishodyashchiy(v, s, teg)
		if err != nil {
			return nil, err
		}
		if _, est := adresaIzTablicy(v, s.Host); est {
			o["domain_resolver"] = TegAdresaServerov
		}
		ishodyashchie = append(ishodyashchie, o)
		tegi = append(tegi, teg)
		idy = append(idy, s.Id)
		if s.Id == v.Server.Id {
			vybrannyy = teg
		}
	}
	// Пустой список кандидатов это не «конфиг без серверов», а отказ. Селектор
	// без единого исходящего ядро принимает молча, туннель при этом не несёт
	// ничего, и сказать человеку было бы нечего.
	if len(tegi) == 0 {
		return nil, fmt.Errorf("%w: ни один сервер набора этому ядру не годится", ErrTransport)
	}
	ishodyashchie = append(ishodyashchie, gruppy(v, tegi, vybrannyy)...)
	// domain_resolver это не только «прямой трафик резолвится местным»: без
	// единой опции sing-box 1.14 считает прямой исходящий ПУСТЫМ и отвергает
	// detour на него («detour to an empty direct outbound makes no sense»),
	// то есть загрузка наборов через http_client молча не работала с первого
	// дня (замерено в госте 03.09.2026: набор ru ни разу не обновился).
	ishodyashchie = append(ishodyashchie, map[string]any{"type": "direct", "tag": TegPryamo, "domain_resolver": TegMestnyy})

	marshrut := map[string]any{
		"auto_detect_interface":   true,
		"default_domain_resolver": map[string]any{"server": TegMestnyy},
		// Селектор, а не ядро: с появлением нескольких кандидатов «ядра»
		// как единственного выхода больше нет.
		"final": trafikFinal(v),
		"rules": append(pravilaZamera(v, idy), pravila(v)...),
	}
	if r := razdelNaborov(v); r != nil {
		marshrut["rule_set"] = r
	}
	if v.ProcessTracker.Endpoint != "" {
		marshrut["process_family_endpoint"] = v.ProcessTracker.Endpoint
		marshrut["process_family_secret"] = v.ProcessTracker.Secret
	}
	eksperimentalnoe := map[string]any{
		"clash_api": map[string]any{
			"external_controller": fmt.Sprintf("%s:%d", v.ClashApi.Adres, v.ClashApi.Port),
			"secret":              v.ClashApi.Sekret,
		},
	}
	if kesh, est := keshFayl(v); est {
		eksperimentalnoe["cache_file"] = kesh
	}

	k := map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"dns": map[string]any{
			"servers":  dnsServery(v),
			"rules":    dnsPravila(v),
			"final":    trafikDNSFinal(v),
			"strategy": "ipv4_only",
		},
		"inbounds":     vhodyashchie(v, idy),
		"outbounds":    ishodyashchie,
		"route":        marshrut,
		"experimental": eksperimentalnoe,
	}

	if err := sveritTegi(k); err != nil {
		return nil, err
	}
	if err := sveritUsloviya(k); err != nil {
		return nil, err
	}
	return json.MarshalIndent(k, "", "  ")
}

// vhodyashchie: TUN всегда, кроме конфига проверки сервера, локальный прокси по
// требованию.
//
// mixed, а не отдельные http и socks: один слушатель закрывает оба протокола,
// а два порта это два способа промахнуться настройкой. Слушает СТРОГО петлю:
// на 0.0.0.0 это открытый прокси для всей подсети, то есть чужой трафик под
// нашим адресом.
func vhodyashchie(v Vhod, idy []string) []any {
	vh := []any{}
	// Вход для замеров сеть машины не трогает, поэтому живёт и без TUN. Стоит
	// последним: туннель остаётся первым входом, как был.
	zamer, estZamer := vhodZamera(v, idy)
	if v.BezTun {
		// Проверка сервера меряет исходящий через clash API, входы ей не нужны.
		// Поднятый здесь TUN отнял бы у человека сеть ровно на ту проверку,
		// ради которой его не поднимают.
		if estZamer {
			vh = append(vh, zamer)
		}
		return vh
	}
	vh = append(vh,
		map[string]any{
			"type": "tun", "tag": "tun-in",
			"address":    []string{v.adresTun()},
			"auto_route": true, "strict_route": true,
			// mixed это умолчание самого sing-box: системный TCP плюс gvisor
			// для UDP. Здесь до 05.09.2026 стоял gvisor, без записанной
			// причины, и замер показал цену: 35.2 Мбит против 340.0 на том же
			// канале, пять кругов вперемежку. Причём не из-за процессора,
			// gvisor тратил 10% против 79% у mixed, то есть просто не тянул.
			"stack": "mixed", "dns_mode": "hijack",
			// MTU 1500, обычный размер кадра Ethernet.
			//
			// Без него ядро берёт своё умолчание, и интерфейс поднимается с
			// MTU 65535 (прочитано с живого туннеля 20.09.2026). Windows тогда
			// отдаёт в туннель блоки до 64 КБ, а режет их уже sing-box у себя.
			// Пока он жуёт такой кусок видео, голосовой пакет ждёт за ним:
			// очередь возникает не от объёма трафика, а от РАЗМЕРА КВАНТА.
			//
			// Замер 20.09.2026, голос одновременно с трансляцией 8 Мбит, по
			// четыре круга на клетку:
			//   trojan  65535: 59, 60, 79, 97 мс   1500: 60, 60, 64, 64
			//   anytls  65535: 70, 89, 96, 129     1500: 60, 60, 82, 88
			// У UDP-ключей (tuic) разницы нет, им нечего было терять. Оговорка:
			// у anytls залипания остаются и на 1500, то есть MTU это половина
			// причины, а не вся.
			//
			// Не путать с 1280, которое стояло здесь полдня 19.09.2026 под
			// гипотезу о потерях hysteria2 от размера датаграммы. Ту гипотезу
			// опроверг прогон по семи размерам: потери 0-6.5% размазаны по всем
			// размерам, теряется кусками по 50-300 пакетов подряд, и это провалы
			// канала, а не фрагментация. То число вдобавок было выведено из
			// замера, который TUN не задействует вовсе: прибор ходит через
			// прокси клиента.
			"mtu": 1500,
		},
	)
	if v.PortProksi > 0 {
		vh = append(vh, map[string]any{
			"type": "mixed", "tag": TegProksiVhod,
			"listen": "127.0.0.1", "listen_port": v.PortProksi,
		})
	}
	if estZamer {
		vh = append(vh, zamer)
	}
	return vh
}

func (v Vhod) adresTun() string {
	if v.AdresTun != "" {
		return v.AdresTun
	}
	return "172.19.0.1/30"
}

// pravila это весь смысл пакета. Порядок менялся дважды, и оба раза check
// пропускал ошибку молча:
//
//	редакция 1: hijack-dns выше обходов  -> DNS службы уезжал в несуществующий
//	                                        туннель, холодный старт мёртв;
//	редакция 5: ВСЕ обходы выше hijack   -> ip_is_private съедал запросы к
//	                                        домашнему резолверу, DNS мимо
//	                                        туннеля всегда и открытым текстом.
//
// Верный порядок: наши процессы и адреса кандидатов выше, hijack-dns следом,
// ip_is_private ниже всех.
func pravila(v Vhod) []any {
	p := []any{
		map[string]any{"action": "sniff"},
		// Только для TUN-входа. Через mixed-вход sing-box тоже узнаёт процесс,
		// и без ограничения наш же affory-svc.exe, постучавшийся в прокси,
		// чтобы выйти ЧЕРЕЗ туннель (checkExitIp), уезжал напрямую. Найдено
		// живьём 03.09.2026: проверка утечек обвинила туннель, а виновато
		// было это правило.
		map[string]any{"inbound": []string{"tun-in"}, "process_path": v.PutiProtsessov, "outbound": TegPryamo},
	}
	if pp, est := praviloPetli(v); est {
		p = append(p, pp)
	}
	p = append(p,
		map[string]any{"ip_cidr": []string{"224.0.0.0/4", "255.255.255.255/32"}, "outbound": TegPryamo},
		map[string]any{
			"type": "logical", "mode": "or",
			"rules": []any{
				map[string]any{"protocol": "dns"},
				map[string]any{"port": 53},
			},
			"action": "hijack-dns",
		},
	)
	// Блокировка рекламы: НИЖЕ петлевых (наши процессы, адреса серверов,
	// multicast) и hijack-dns, ВЫШЕ правила proksi-in и любого обхода. Не
	// обход, поэтому остаётся и в режиме «весь трафик».
	if pr, est := praviloReklamy(v); est {
		p = append(p, pr)
	}
	// В режиме "весь трафик" отменяются УДОБНЫЕ исключения, а не петлевые.
	if v.Trafik != nil {
		// Explicit proxy traffic requests the VPN even in selective mode.
		if v.PortProksi > 0 {
			p = append(p, map[string]any{"inbound": []string{TegProksiVhod}, "outbound": TegSelector})
		}
		// Частные сети выше правил приложений (L5 аудита 1.8.0): приложение,
		// направленное в VPN, теряло сети за шлюзом (NAS, принтер соседней
		// подсети). Вход прокси остаётся выше: явный заход в прокси значит
		// «через VPN», и через него стенд ходит к мишени внутри сервера.
		if !v.VesTrafik {
			p = append(p, map[string]any{"ip_is_private": true, "outbound": TegPryamo})
		}
		p = append(p, trafikPravila(v, false)...)
		if pn, est := praviloNaborov(v); est {
			p = append(p, pn)
		}
		return p
	}
	// Петлевые выше по списку и остаются всегда: без них туннель съедает сам
	// себя, и это отказ другого класса, чем неработающий принтер.
	if !v.VesTrafik {
		p = append(p, map[string]any{"ip_is_private": true, "outbound": TegPryamo})
	}
	// Исключения по процессам и домены из наборов: тоже удобные исключения,
	// оба помощника сами молчат в режиме «весь трафик».
	if pp, est := praviloProtsessov(v); est {
		p = append(p, pp)
	}
	if pd, est := praviloDomenov(v); est {
		p = append(p, pd)
	}
	if pn, est := praviloNaborov(v); est {
		p = append(p, pn)
	}
	return p
}

// praviloDomenov: домен и его поддомены мимо туннеля. domain_suffix, а не
// domain: человек пишет «example.org» и ждёт, что cdn.example.org пойдёт туда
// же. Правило совпадает по имени из sniff (первое правило списка), поэтому
// стоит после него и ниже hijack-dns, как всякое удобное исключение.
func praviloDomenov(v Vhod) (map[string]any, bool) {
	if len(v.Domeny) == 0 || v.VesTrafik {
		return nil, false
	}
	return map[string]any{"domain_suffix": v.Domeny, "outbound": TegPryamo}, true
}

// praviloProtsessov это исключения человека, ОТДЕЛЬНО от правила по нашим
// процессам выше hijack-dns: то держит петлю и стоит всегда, это удобство и
// стоит ниже. process_path, а не process_name: имя ловит любой steam.exe на
// машине, включая чужой, а подмена одноимённого процесса это ровно тот случай,
// ради которого правило и ставится.
func praviloProtsessov(v Vhod) (map[string]any, bool) {
	if len(v.Protsessy) == 0 || v.VesTrafik {
		return nil, false
	}
	return map[string]any{"process_path": v.Protsessy, "outbound": TegPryamo}, true
}

// dnsServery это резолверы ядра.
//
// Резолвер туннеля намеренно публичный: запрос к нему уходит внутрь туннеля и
// потому не выдаёт нас домашнему провайдеру. Местный нужен только для имён,
// которых в интернете нет.
//
// Через туннель DNS идёт по https (С7 аудита 1.6.1): замером в госте udp на
// hy2 терял запросы и дольше всех открывал страницу, https по p90 лучше tcp на
// обоих проверенных протоколах.
func dnsServery(v Vhod) []any {
	servery := []any{
		map[string]any{"type": "https", "tag": TegTunnel, "server": "1.1.1.1", "detour": TegSelector},
		map[string]any{"type": "udp", "tag": TegMestnyy, "server": v.Resolver.String()},
	}
	if t, est := tablicaServerov(v); est {
		servery = append(servery, t)
	}
	return servery
}

// tablicaServerov отдаёт ядру адреса серверов, которые уже нашла служба
// (жалоба 02.10.2026). Прежде ядро спрашивало имя сервера у местного
// резолвера, то есть у DNS провайдера, и провайдер, не отдающий имя, отрезал
// человека от VPN, даже когда служба нашла адрес в обход.
//
// Заодно ядро звонит ровно на те адреса, что стоят в правиле петли и в
// брандмауэре: прежде оно резолвило имя второй раз, и ответ с другим адресом
// мог увести трафик к серверу в туннель, который на этом трафике и держится.
//
// Только IPv4: DNS ядра работает в режиме ipv4_only, и адрес IPv6 из таблицы
// ему всё равно не годится.
func tablicaServerov(v Vhod) (map[string]any, bool) {
	tablica := map[string]any{}
	for _, s := range v.kandidaty() {
		if adresa, est := adresaIzTablicy(v, s.Host); est {
			tablica[s.Host] = adresa
		}
	}
	if len(tablica) == 0 {
		return nil, false
	}
	return map[string]any{"type": "hosts", "tag": TegAdresaServerov, "predefined": tablica}, true
}

// adresaIzTablicy отвечает, есть ли у сервера строка в таблице: имя, а не
// литерал, и хоть один адрес IPv4 от службы.
func adresaIzTablicy(v Vhod, host string) ([]string, bool) {
	if _, err := netip.ParseAddr(host); err == nil {
		return nil, false
	}
	var adresa []string
	for _, a := range v.AdresaServerov[host] {
		if a.Unmap().Is4() {
			adresa = append(adresa, a.Unmap().String())
		}
	}
	return adresa, len(adresa) > 0
}

// praviloPetli ведёт мимо туннеля адреса серверов, но только на их портах.
//
// До С2 аудита 1.6.1 правило брало все адреса кандидатов на любом порту, а в
// них входили подписка и raw.githubusercontent.com: сайт на том же хостинге или
// на GitHub Pages шёл мимо туннеля. Службе и загрузкам ядра правило не нужно,
// их ведут правило процессов и detour наборов. У sing-box условия одного
// правила складываются через «и», поэтому у каждого сервера своё подправило.
func praviloPetli(v Vhod) (map[string]any, bool) {
	var pod []any
	for _, s := range v.kandidaty() {
		adresa, est := v.AdresaServerov[s.Host]
		if !est {
			a, err := netip.ParseAddr(s.Host)
			if err != nil {
				// Имя без адресов не разрешилось и у службы. Ядро к такому
				// серверу не пойдёт вовсе, петле неоткуда взяться.
				continue
			}
			adresa = []netip.Addr{a.Unmap()}
		}
		if len(adresa) == 0 {
			continue
		}
		r := map[string]any{"ip_cidr": setiKandidatov(adresa)}
		if s.Porty != "" {
			r["port_range"] = portyHy2(s.Porty)
		} else {
			r["port"] = []int{s.Port}
		}
		pod = append(pod, r)
	}
	if len(pod) == 0 {
		return nil, false
	}
	return map[string]any{"type": "logical", "mode": "or", "rules": pod, "outbound": TegPryamo}, true
}

func setiKandidatov(a []netip.Addr) []string {
	s := make([]string, 0, len(a))
	for _, k := range a {
		bit := 32
		if k.Is6() {
			bit = 128
		}
		s = append(s, fmt.Sprintf("%s/%d", k.String(), bit))
	}
	return s
}

// Ishodyashchiy это исходящий одного сервера, тот же, что в боевом конфиге,
// без полосы канала человека. Им ядро проверки своего VPN проверяет ключи до
// записи в набор: проверяется ровно то, что потом повезёт трафик.
func Ishodyashchiy(s protokol.Server, teg string) (map[string]any, error) {
	return ishodyashchiy(Vhod{}, s, teg)
}

// ishodyashchiy строит исходящий под транспорт КОНКРЕТНОГО сервера.
//
// Тег передаётся снаружи, а не берётся из константы: один тег на всех означал
// бы, что два кандидата схлопнутся в селекторе в один, причём молча.
func ishodyashchiy(v Vhod, s protokol.Server, teg string) (map[string]any, error) {
	// Имя TLS: sni, затем заголовок Host, затем адрес. Ws+tls за CDN часто
	// записан адресом и host= без sni, и с адресом фронт отвечал сертификатом
	// не того сайта (С1 аудита 1.6.1). IP в последней ступени ничего не меняет:
	// пустое имя ядро и само заменяет адресом (common/tls/std_client.go).
	sni := putIli(s.Sni, putIli(s.HostZagolovka, s.Host))
	// У reality имя это маска, заголовок Host к ней отношения не имеет.
	sniReality := putIli(s.Sni, s.Host)
	// ALPN и заголовок Host приезжают ИЗ ССЫЛКИ: разбор их вынимал с самого
	// начала, а сюда они не доезжали.
	//
	// Отпечаток TLS не из ссылки, а по двум правилам из замеров: reality ходит
	// только с chrome, обычный TLS по умолчанию с firefox (см. otpechatokTLS).
	// `sing-box check` негодный отпечаток принимает молча, и отказ выглядит как
	// «сервер не отвечает», поэтому правила меняются замером, а не догадкой.

	switch s.Transport {
	case "reality-tcp":
		o := vless(s, teg)
		o["tls"] = tlsReality(s, sniReality)
		dobavitAlpn(o, s)
		return o, nil

	case "tls-tcp":
		// vless поверх голого TCP под обычным TLS, с Vision или без (П1 аудита
		// 1.6.1). До 1.7.0 такая ссылка отвергалась разбором.
		o := vless(s, teg)
		o["tls"] = tlsSPinom(s, sni)
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		return o, nil

	case "reality-grpc":
		// Тот же reality, что выше, плюс транспорт grpc. Flow здесь нет и быть
		// не может: vision живёт только на голом TCP (см. flowVozmozhen).
		o := vless(s, teg)
		o["tls"] = tlsReality(s, sniReality)
		dobavitAlpn(o, s)
		o["transport"] = map[string]any{"type": "grpc", "service_name": putIli(s.Put, "gun")}
		return o, nil

	case "ws":
		o := vless(s, teg)
		// Открытый ws это не экзотика: в живом сборнике таких шесть из 145, на
		// портах 80 и 2200. См. protokol.Server.BezTLS.
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
		dobavitHost(tr, s)
		o["transport"] = tr
		return o, nil

	case "httpupgrade":
		// Тот же смысл, что ws, но без веб-сокетов: одно рукопожатие Upgrade,
		// дальше голый поток. Ядро несёт его без отдельного тега сборки.
		o := vless(s, teg)
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		tr := map[string]any{"type": "httpupgrade", "path": putIli(s.Put, "/")}
		// У httpupgrade ядро ждёт host отдельным полем, а не только в headers.
		if s.HostZagolovka != "" {
			tr["host"] = s.HostZagolovka
		}
		dobavitHost(tr, s)
		o["transport"] = tr
		return o, nil

	case "trojan", "trojan-ws":
		// TLS у trojan есть всегда: протокол только им и прикрыт. Поэтому
		// здесь нет ветки BezTLS, в отличие от ws и grpc у vless.
		o := map[string]any{
			"type": "trojan", "tag": teg,
			"server": s.Host, "server_port": s.Port, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
		}
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		if s.Transport == "trojan-ws" {
			tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
			dobavitHost(tr, s)
			o["transport"] = tr
		}
		return o, nil

	case "vmess", "vmess-ws":
		// security ставится ЗДЕСЬ, а не разбором: ссылка вправе его не
		// называть, а ядру нужно значение. auto это то, что подставляют все
		// клиенты, и оно же единственное безопасное умолчание.
		o := map[string]any{
			"type": "vmess", "tag": teg,
			"server": s.Host, "server_port": s.Port, "uuid": s.Uuid,
			"security": putIli(s.Shifr, "auto"),
		}
		// alter_id пишется только ненулевым: ноль это современный режим, и
		// явный ноль в конфиге ядро понимает так же, но читателя путает.
		if s.AlterId > 0 {
			o["alter_id"] = s.AlterId
		}
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
			dobavitAlpn(o, s)
		}
		if s.Transport == "vmess-ws" {
			tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
			dobavitHost(tr, s)
			o["transport"] = tr
		}
		return o, nil

	case "grpc":
		o := vless(s, teg)
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		o["transport"] = map[string]any{"type": "grpc", "service_name": putIli(s.Put, "gun")}
		return o, nil

	case "hy2":
		o := map[string]any{
			"type": "hysteria2", "tag": teg,
			"server": s.Host, "password": s.Parol,
			// Пин публичного ключа: проверка ПОВЕРХ цепочки, не вместо неё.
			"tls": tlsSPinom(s, sni),
		}
		if s.Porty != "" {
			// server_ports и server_port взаимно исключают друг друга у ядра.
			// Ссылка пишет «от-до», ядро ждёт «от:до».
			o["server_ports"] = portyHy2(s.Porty)
			// Интервал прыжка задаётся ВИЛКОЙ, а не числом. Умолчание ядра
			// это ровно 30 с, то есть прыжок раз в полминуты как по часам;
			// наблюдателю такой ритм сам по себе примета. hop_interval_max
			// появился в 1.14.0, и ядро у нас пиновано на 1.14.1, так что
			// поле не требует оговорок о совместимости.
			//
			// В ссылку это не выносится: грамматика панелей Hysteria 2 знает
			// mport и не знает интервала, а выдумывать свой параметр значит
			// сделать наши ссылки нечитаемыми для чужих клиентов.
			o["hop_interval"] = "30s"
			o["hop_interval_max"] = "60s"
		} else {
			o["server_port"] = s.Port
		}
		if s.Obfs != "" {
			o["obfs"] = map[string]any{"type": s.Obfs, "password": s.ObfsParol}
		}
		// Объявление полосы это переключатель Brutal, а не ограничитель:
		// отдельного флага у протокола нет. Пустые поля означают BBR, поэтому
		// неизмеренная полоса НЕ пишется, а не пишется нулём.
		//
		// Источников два, и настройка сильнее ссылки. Ссылку пишет держатель
		// сервера, и его число это оценка со стороны; настройку ставит человек
		// про свой домашний канал, а Brutal управляется именно им.
		if vverh, vniz := polosaDlya(v, s); vverh > 0 && vniz > 0 {
			o["up_mbps"] = vverh
			o["down_mbps"] = vniz
		}
		dobavitAlpn(o, s)
		return o, nil

	case "anytls":
		// TLS есть всегда: имя протокола это и есть его обёртка, ветки BezTLS
		// здесь нет по той же причине, что у trojan. Uuid не пишется вовсе:
		// у anytls его не существует, и пустое поле ядро отвергло бы.
		o := map[string]any{
			"type": "anytls", "tag": teg,
			"server": s.Host, "server_port": s.Port, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
			// Три тёплых сессии вместо нуля (умолчание ядра).
			//
			// AnyTLS для того и сделан, чтобы не заводить TLS-соединение на
			// каждый поток, но при min_idle_session=0 проверка раз в 30 секунд
			// закрывает ВСЕ простаивающие, и следующая вкладка снова платит
			// рукопожатием. Держать три штуки дешевле и тише: долгоживущее
			// соединение выглядит обычно, пачка новых рукопожатий нет.
			"min_idle_session": 3,
		}
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		return o, nil

	case "tuic":
		o := map[string]any{
			"type": "tuic", "tag": teg,
			"server": s.Host, "server_port": s.Port,
			"uuid": s.Uuid, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
		}
		// Оба поля пишутся ТОЛЬКО когда их назвала ссылка. Умолчание у ядра своё
		// (`cubic` и `native`), и подставлять вместо него своё значит менять
		// протокол под тем же именем. Пустая строка при этом не безобидна: ядро
		// отвергает неизвестное значение и роняет весь конфиг.
		if s.Peregruzka != "" {
			o["congestion_control"] = s.Peregruzka
		}
		if s.RezhimUDP != "" {
			o["udp_relay_mode"] = s.RezhimUDP
		}
		// utls тут НЕ ставится, и это проверено живым подъёмом 21.09.2026:
		// ядро отвечает «unsupported usage for uTLS» на каждое соединение, а
		// `sing-box check` такой конфиг принимает молча и с кодом 0. Наружу это
		// приехало бы как «tuic перестал работать без причины».
		dobavitAlpn(o, s)
		return o, nil

	case "ss":
		return map[string]any{
			"type": "shadowsocks", "tag": teg,
			"server": s.Host, "server_port": s.Port,
			"method": s.Metod, "password": s.Parol,
		}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrTransport, s.Transport)
}

// portyHy2 переводит mport ссылки («20000-21000,443») в server_ports ядра
// («20000:21000», «443:443»). Пробелы вокруг запятых панели тоже ставят.
//
// Одиночный порт обязан стать диапазоном из самого себя. Числом ядро его не
// принимает: «bad port range: 443», и падает ВЕСЬ конфиг, а вместе с ним и
// исправные серверы рядом. Найдено 03.09.2026 прогоном инварианта 8 настоящим
// ядром: обычный go test без ядра эти профили пропускает и печатает ok.
func portyHy2(mport string) []string {
	var itog []string
	for _, ch := range strings.Split(mport, ",") {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		ch = strings.Replace(ch, "-", ":", 1)
		if !strings.Contains(ch, ":") {
			ch += ":" + ch
		}
		itog = append(itog, ch)
	}
	return itog
}

func vless(s protokol.Server, teg string) map[string]any {
	o := map[string]any{
		"type": "vless", "tag": teg,
		"server": s.Host, "server_port": s.Port, "uuid": s.Uuid,
	}
	// Flow только там, где он существует. `xtls-rprx-vision` работает поверх
	// ГОЛОГО TCP под TLS или REALITY, и больше нигде: «XTLS only supports TLS
	// and REALITY directly» в исходниках Xray. Любой транспорт поверх (ws, grpc,
	// httpupgrade, xhttp) его исключает.
	//
	// Чужая ссылка с такой парой не отвергается: сервер с flow над ws невозможен
	// в принципе, значит это ошибка панели, а не негодный сервер. Игнорируем,
	// как игнорируем insecure. Отвергать было бы хуже: сервер рабочий.
	//
	// Проверить это ядром нельзя: `sing-box check` пару принимает молча, конфиг
	// стартует, трафика нет.
	if s.Flow != "" && flowVozmozhen(s.Transport) {
		o["flow"] = s.Flow
	}
	return o
}

// flowVozmozhen отвечает, бывает ли у транспорта XTLS-flow.
//
// Список от РАЗРЕШЁННОГО, а не от запрещённого: новый транспорт по умолчанию
// оказывается без flow, и это верная сторона для ошибки. Обратный список молча
// раздал бы flow всему, что добавят потом.
func flowVozmozhen(transport string) bool {
	return transport == "reality-tcp" || transport == "tls-tcp"
}

// tlsReality это TLS-блок reality. У reality без utls рукопожатие не
// состоится вовсе, поэтому отпечаток входит в блок всегда.
func tlsReality(s protokol.Server, sni string) map[string]any {
	return map[string]any{
		"enabled": true, "server_name": sni,
		"reality": map[string]any{
			"enabled": true, "public_key": s.PublicKey, "short_id": s.ShortId,
		},
		"utls": map[string]any{"enabled": true, "fingerprint": otpechatokRealityDlya(s)},
	}
}

// otpechatokRealityDlya: своему серверу firefox, с которым его ключ проверен
// при установке (proverka собирает исходящий этой же функцией), остальным
// chrome.
func otpechatokRealityDlya(s protokol.Server) string {
	if s.Svoy {
		return otpechatokSvoegoReality
	}
	return otpechatokReality
}

const otpechatokSvoegoReality = "firefox"

// У reality отпечаток всегда chrome, что бы ни просила ссылка.
//
// Xray с 26.9.8 (сентябрь 2026) отвергает приветствие reality без ключа
// X25519MLKEM768, а из пресетов utls в ядре его несёт только chrome: firefox
// там это Firefox 120 с X25519 и P-256. Официальный клиент Xray с fp=firefox
// к такому серверу подключается (у него Firefox 148), то есть чужая ссылка с
// firefox у нас молча перестала бы работать. Своему серверу sing-box ключ не
// нужен, но отличить его по ссылке нельзя.
const otpechatokReality = "chrome"

// Отпечатки обычного TLS, проверенные живым подъёмом. Пополнять ТОЛЬКО
// прогоном на стенде: негодный отпечаток не отвергается ядром, а тихо не
// несёт трафик.
var proverennyeOtpechatki = map[string]bool{"chrome": true, "firefox": true}

// otpechatokTLS это отпечаток обычного TLS: из ссылки, если он проверен,
// иначе firefox.
//
// firefox по умолчанию с 02.10.2026. Приветствие chrome в ядре весит 1757
// байт и едет двумя TCP-сегментами, и залп из шести соединений разом
// замораживал адрес сервера на 70-150 с: путь резал второй сегмент или ответ
// сервера (запись пакетов на сервере). firefox весит 663 байта, идёт одним
// сегментом, и тот же залп проходил. Имя в рукопожатии на это не влияло.
func otpechatokTLS(fp string) string {
	if proverennyeOtpechatki[fp] {
		return fp
	}
	return "firefox"
}

// dobavitAlpn кладёт список ТОЛЬКО когда он задан. Пустой список ALPN это не
// то же самое, что его отсутствие: сервер будет выбирать по нему и не найдёт
// ничего.
func dobavitAlpn(o map[string]any, s protokol.Server) {
	if s.Alpn == "" {
		return
	}
	spisok := make([]string, 0, 2)
	for _, ch := range strings.Split(s.Alpn, ",") {
		if ch = strings.TrimSpace(ch); ch != "" {
			spisok = append(spisok, ch)
		}
	}
	if len(spisok) == 0 {
		return
	}
	tls, est := o["tls"].(map[string]any)
	if !est {
		return
	}
	tls["alpn"] = spisok
}

// dobavitHost нужен фронту: без заголовка Host он вернёт чужую страницу вместо
// туннеля, и выглядеть это будет как молчащий сервер.
func dobavitHost(tr map[string]any, s protokol.Server) {
	if s.HostZagolovka == "" {
		return
	}
	tr["headers"] = map[string]any{"Host": s.HostZagolovka}
}

// dobavitUtls ставит браузерное рукопожатие ВСЕГДА, где есть TLS.
//
// До 21.09.2026 отпечаток брался только из ссылки, а стоял он лишь у reality.
// Значит trojan, anytls и httpupgrade ходили с родным рукопожатием Go, и вот
// что снято в тот день с живого пользователя на входе trojan (tcpdump на
// сервере, разбор ClientHello):
//
//	расширения: ec_point_formats, renegotiation_info, extended_master_secret,
//	            signed_certificate_timestamp, status_request, supported_groups,
//	            signature_algorithms, signature_algorithms_cert,
//	            supported_versions, key_share
//
// Ни server_name, ни ALPN, ни одного GREASE. Это подпись crypto/tls, а не
// браузера: Chrome кладёт GREASE в четыре места и ALPN всегда. По такому
// ClientHello соединение отличается от браузерного первым же пакетом.
//
// Заодно чинится ALPN: пресеты chrome и firefox в utls перезаписывают NextProtos на
// h2/http1.1 (u_tls_extensions.go, writeToUConn), то есть расширение появляется
// само. Сервер его не выбирает, и это безопасно: checkALPN в crypto/tls при
// пустом ответе сервера возвращает nil для всего, кроме QUIC.
//
// К QUIC (hy2, tuic) не относится: там ядро требует STDConfig, которого у
// utls-конфига нет, и рукопожатие строит quic-go.
// tlsSPinom это блок TLS для ВСЕХ протоколов, кроме reality. Пин добавляется
// только когда он есть, и он ПОВЕРХ цепочки, а не вместо неё: insecure остаётся
// выключенным всегда.
//
// Вынесено 05.09.2026, когда третий протокол подряд начал собирать один и тот же
// блок руками. Три копии разошлись бы на первом же изменении, причём молча:
// забытый пин это не отказ ядра, а потерянная проверка подлинности. В тот же
// день это и случилось: ws, grpc, httpupgrade, xhttp, trojan и vmess собирали
// блок своими литералами, и пин до ядра не доезжал ни у одного из шести.
//
// reality сюда НЕ переведён намеренно. Сертификат там подставной и живёт одно
// рукопожатие, сверять его с записанным отпечатком нечему: объявленный пин не
// совпал бы никогда, то есть лишний параметр в ссылке ломал бы рабочий сервер.
func tlsSPinom(s protokol.Server, sni string) map[string]any {
	tls := map[string]any{"enabled": true, "server_name": sni}
	if s.Pin != "" {
		tls["certificate_public_key_sha256"] = []string{s.Pin}
	}
	return tls
}

func dobavitUtls(o map[string]any, s protokol.Server) {
	tls, est := o["tls"].(map[string]any)
	if !est {
		return
	}
	tls["utls"] = map[string]any{"enabled": true, "fingerprint": otpechatokTLS(s.Fp)}
}

func putIli(v, poumolchaniyu string) string {
	if v == "" {
		return poumolchaniyu
	}
	return v
}

// dnsPravila: имена, которых в интернете нет, местным резолвером всегда; а
// домены, идущие МИМО туннеля (исключения человека и наборы), тоже местным,
// иначе российский сайт получает ответ из Нидерландов и голландский узел CDN,
// и прямой путь оказывается медленнее, чем без VPN. Вторая пара правил это
// удобные исключения: в режиме «весь трафик» их нет, как и самих обходов.
//
// Блок рекламы стоит сразу под локальными именами и до выхода в режиме «весь
// трафик»: выше набора ru (an.yandex.ru, mc.yandex.ru, ad.mail.ru входят в
// category-ru и иначе ушли бы местному резолверу) и выше доменных правил
// Trafik (adeventtracker.spotify.com под суффиксом spotify.com).
func dnsPravila(v Vhod) []any {
	p := []any{
		map[string]any{"domain_suffix": []string{".local", ".lan", ".home.arpa"}, "server": TegMestnyy},
		map[string]any{"domain_regex": []string{"^[^.]+$"}, "server": TegMestnyy},
	}
	p = append(p, dnsPravilaReklamy(v)...)
	if v.VesTrafik {
		return p
	}
	if v.Trafik != nil {
		p = append(p, trafikPravila(v, true)...)
	} else if len(v.Domeny) > 0 {
		p = append(p, map[string]any{"domain_suffix": v.Domeny, "server": TegMestnyy})
	}
	if len(v.Nabory) > 0 {
		tegi := make([]string, 0, len(v.Nabory))
		for _, n := range v.Nabory {
			tegi = append(tegi, n.Teg)
		}
		p = append(p, map[string]any{"rule_set": tegi, "server": TegMestnyy})
	}
	return p
}

// polosaDlya выбирает, чью полосу объявлять серверу hysteria2.
//
// Настройка человека сильнее ссылки, и это не вкусовщина. Brutal шлёт с
// объявленной скоростью, не глядя на потери, поэтому число обязано описывать
// канал ТОГО, кто шлёт, то есть домашний канал человека. Ссылка знает лишь то,
// что вписал держатель сервера.
//
// Пара проверяется у каждого источника отдельно: смешивать «вверх из настройки,
// вниз из ссылки» нельзя, получилось бы объявление, которого не делал никто.
func polosaDlya(v Vhod, s protokol.Server) (int, int) {
	if v.PolosaVverh > 0 && v.PolosaVniz > 0 {
		return v.PolosaVverh, v.PolosaVniz
	}
	if s.PolosaVverh > 0 && s.PolosaVniz > 0 {
		return s.PolosaVverh, s.PolosaVniz
	}
	return 0, 0
}
