// Пакет set спрашивает у Windows то, что нельзя знать заранее: какой адаптер
// сейчас несёт трафик, какой у него шлюз и какой резолвер.
//
// Everything here is asked of the OS at tunnel-raise time and nothing is
// remembered between runs. A laptop moves between networks, a docked machine
// changes adapters mid-session, DHCP renews at 3am. Каждое из этих событий
// делает вчерашний ответ ложью, а ложь про адрес резолвера уезжает сразу в три
// места: в конфиг ядра, в правило петли и в список разрешённого.
package set

import (
	"cmp"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sostoyanieVverh = windows.IfOperStatusUp
	tipPetli        = windows.IF_TYPE_SOFTWARE_LOOPBACK
)

// ErrNetAdaptera отдаётся, когда ни один адаптер не годится в кандидаты.
var ErrNetAdaptera = errors.New("нет адаптера с маршрутом по умолчанию")

// ErrNetResolvera отдаётся отдельно от ErrNetAdaptera: адаптер может нести
// маршрут по умолчанию и при этом не объявлять ни одного DNS-сервера. Молча
// вернуть нулевой адрес значит положить "invalid Addr" в конфиг ядра, а ядро
// откажется стартовать с сообщением совсем про другое.
//
// Отдаётся, только когда резолвера нет НИ У ОДНОГО кандидата: один такой
// адаптер это обычное дело, и он больше никого не заслоняет.
var ErrNetResolvera = errors.New("ни один адаптер с выходом в сеть не объявил DNS-сервер")

// Adapter это ровно то, что нужно волне 2, и ничего сверх. Полный
// IpAdapterAddresses сюда не тащится намеренно: он неудобен в тестах и половина
// его полей никогда не будет прочитана.
type Adapter struct {
	Indeks     uint32
	Imya       string
	Opisanie   string
	Tip        uint32
	Sostoyanie uint32
	Metrika    uint32
	// Adresa это СВОИ адреса адаптера. Нужны правилу брандмауэра: у netsh нет
	// привязки к адаптеру по имени вовсе, и разрешить исходящий через туннель
	// можно только через localip с адресом этого туннеля.
	Adresa    []netip.Addr
	Shlyuzy   []netip.Addr
	Resolvery []netip.Addr
	// Umolchanie значит «в таблице маршрутов есть 0.0.0.0/0 через этот
	// адаптер». Шлюз у адаптера для этого НЕ достаточен: чужой VPN его тоже
	// объявляет, наружу при этом не ведя (см. marshruty.go).
	Umolchanie bool
	// MetrikaMarshruta складывается с метрикой интерфейса при выборе, ровно как
	// это делает Windows.
	MetrikaMarshruta uint32
}

// Шов. Тесты подменяют перечисление и проверяют выбор, не имея сети вовсе.
var perechislit = perechislitSistemnye

// Adaptery отдаёт снимок адаптеров системы.
func Adaptery() ([]Adapter, error) { return perechislit() }

// kandidaty отдаёт адаптеры, которые могут нести трафик наружу, лучший первым.
//
// Правило простое и намеренно тупое: адаптер должен быть поднят, не быть петлёй,
// нести маршрут по умолчанию и объявлять шлюз, а порядок задаёт сумма метрики
// маршрута и метрики интерфейса. Это те же величины, что печатают Get-NetRoute и
// Get-NetIPInterface, и та же сумма, по которой Windows разводит два умолчания.
//
// Шлюз остаётся в условии вместе с маршрутом: без него Shlyuz читал бы пустой
// список. Один маршрут без шлюза наружу всё равно не ведёт.
//
// Функция чистая: весь разговор с системой остался в perechislit. Поэтому её
// поведение проверяется списком структур, а не сетью, и правило можно менять,
// не поднимая ничего.
func kandidaty(spisok []Adapter, krome ...uint32) []Adapter {
	propustit := make(map[uint32]bool, len(krome))
	for _, i := range krome {
		propustit[i] = true
	}

	godnye := make([]Adapter, 0, len(spisok))
	for _, a := range spisok {
		switch {
		case propustit[a.Indeks], a.Sostoyanie != sostoyanieVverh,
			a.Tip == tipPetli, !a.Umolchanie, len(a.Shlyuzy) == 0:
			continue
		}
		godnye = append(godnye, a)
	}
	// Равные метрики разводятся индексом, иначе порядок перечисления системой
	// становится частью поведения, а он не обещан никем.
	slices.SortStableFunc(godnye, func(a, b Adapter) int {
		if c := cmp.Compare(vesAdaptera(a), vesAdaptera(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.Indeks, b.Indeks)
	})
	return godnye
}

// vesAdaptera считает в uint64: обе метрики бывают по 4 миллиарда у адаптера,
// который система считает негодным, и их сумма в uint32 переполнилась бы в ноль,
// то есть негодный адаптер стал бы лучшим.
func vesAdaptera(a Adapter) uint64 {
	return uint64(a.MetrikaMarshruta) + uint64(a.Metrika)
}

// Kandidaty отдаёт снимок kandidaty: свой VPN ищет среди них физический
// адаптер для SSH мимо туннеля.
func Kandidaty() ([]Adapter, error) {
	spisok, err := perechislit()
	if err != nil {
		return nil, err
	}
	return kandidaty(spisok), nil
}

func vybrat(spisok []Adapter, krome ...uint32) (Adapter, error) {
	g := kandidaty(spisok, krome...)
	if len(g) == 0 {
		return Adapter{}, ErrNetAdaptera
	}
	return g[0], nil
}

// AktivnyyAdapter отдаёт имя адаптера, несущего трафик наружу ПРЯМО СЕЙЧАС.
//
// Оговорка, которую надо знать: при поднятом туннеле это будет туннель, и это
// правильный ответ на заданный вопрос. Когда нужен физический канал под
// туннелем, зовётся вариант Krome с индексом TUN-адаптера, который волна 2.3
// узнаёт при его подъёме.
func AktivnyyAdapter() (string, error) { return AktivnyyAdapterKrome() }

func AktivnyyAdapterKrome(krome ...uint32) (string, error) {
	spisok, err := perechislit()
	if err != nil {
		return "", err
	}
	a, err := vybrat(spisok, krome...)
	if err != nil {
		return "", err
	}
	return a.Imya, nil
}

func Shlyuz() (netip.Addr, error) { return ShlyuzKrome() }

func ShlyuzKrome(krome ...uint32) (netip.Addr, error) {
	a, err := aktivnyy(krome...)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Shlyuzy[0], nil
}

func LokalnyyResolver() (netip.Addr, error) { return LokalnyyResolverKrome() }

// LokalnyyResolverKrome берёт резолвер ПЕРВОГО кандидата, который его объявил,
// а не первого кандидата вообще.
//
// Адаптер может нести маршрут по умолчанию и не назвать ни одного DNS-сервера.
// Прежде это был отказ, и отказ уезжал в подъём туннеля целиком: человек с
// чужим VPN в системе не мог подключиться ни разу, а сообщение называло ему имя
// чужого адаптера. Резолвер соседнего канала это рабочий ответ, а отказ - нет.
func LokalnyyResolverKrome(krome ...uint32) (netip.Addr, error) {
	vse, err := LokalnyeResolveryKrome(krome...)
	if err != nil {
		return netip.Addr{}, err
	}
	return vse[0], nil
}

// LokalnyeResolveryKrome отдаёт DNS-серверы всех кандидатов по порядку:
// сначала лучшего адаптера, потом следующих, без повторов. Первый в списке
// тот же, что у LokalnyyResolverKrome.
//
// Нужен целиком, а не первым (M9 аудита 1.8.0): у адаптера бывает несколько
// DNS, и если первый молчит, Windows сама уходит на второй, а ядро с одним
// первым оставалось без имён и туннель не поднимался.
func LokalnyeResolveryKrome(krome ...uint32) ([]netip.Addr, error) {
	spisok, err := perechislit()
	if err != nil {
		return nil, err
	}
	g := kandidaty(spisok, krome...)
	if len(g) == 0 {
		return nil, ErrNetAdaptera
	}
	var vse []netip.Addr
	for _, a := range g {
		for _, r := range a.Resolvery {
			if !slices.Contains(vse, r) {
				vse = append(vse, r)
			}
		}
	}
	if len(vse) > 0 {
		return vse, nil
	}
	imena := make([]string, 0, len(g))
	for _, a := range g {
		imena = append(imena, a.Imya)
	}
	return nil, fmt.Errorf("%w: %s", ErrNetResolvera, strings.Join(imena, ", "))
}

// pometitUmolchanie отмечает адаптер по таблице маршрутов, а при её отказе по
// прежнему правилу «есть шлюз значит есть выход».
//
// Запасной путь оставлен намеренно: без него отказ таблицы означал бы ноль
// кандидатов и полную неработоспособность продукта, то есть починка одной
// жалобы завела бы жалобу пострашнее.
func pometitUmolchanie(a *Adapter, umolchaniya map[uint32]uint32, errTablitsy error) {
	if errTablitsy != nil {
		a.Umolchanie = len(a.Shlyuzy) > 0
		return
	}
	m, est := umolchaniya[a.Indeks]
	a.Umolchanie, a.MetrikaMarshruta = est, m
}

func aktivnyy(krome ...uint32) (Adapter, error) {
	spisok, err := perechislit()
	if err != nil {
		return Adapter{}, err
	}
	return vybrat(spisok, krome...)
}

// perechislitSistemnye спрашивает GetAdaptersAddresses, а не реестр: в реестре
// лежит то, что настроено, а в API то, что действует. Разница видна ровно
// тогда, когда она дорога: DHCP выдал другой резолвер, а запись осталась.
func perechislitSistemnye() ([]Adapter, error) {
	// Дружелюбное имя НЕ пропускается: именно оно потом уезжает в netsh и в
	// сообщения человеку, а Description отличается и годится только для опознания.
	const flagi = windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST |
		windows.GAA_FLAG_INCLUDE_GATEWAYS

	// The size is asked for, not guessed, and the loop exists because adapters
	// can appear between the two calls. Three tries is not superstition: it is
	// one for the answer, one for the race, one for the second race.
	razmer := uint32(15 * 1024)
	var syroy []byte
	for popytka := 0; popytka < 3; popytka++ {
		syroy = make([]byte, razmer)
		err := windows.GetAdaptersAddresses(windows.AF_INET,
			flagi, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&syroy[0])), &razmer)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil, fmt.Errorf("GetAdaptersAddresses: %w", err)
		}
		if popytka == 2 {
			return nil, fmt.Errorf("GetAdaptersAddresses: буфер растёт быстрее, чем мы его просим")
		}
	}

	// Маршруты спрашиваются ОДИН раз на перечисление, а не по адаптеру: между
	// двумя вызовами таблица меняется, и половина адаптеров получила бы вчерашний
	// ответ. Отказ таблицы не рушит перечисление: продукт откатывается к прежнему
	// правилу «есть шлюз значит есть выход», и это хуже, но работает.
	umolchaniya, errMarshrutov := marshrutyUmolchaniya()

	var itog []Adapter
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&syroy[0])); p != nil; p = p.Next {
		a := Adapter{
			Indeks:     p.IfIndex,
			Imya:       windows.UTF16PtrToString(p.FriendlyName),
			Opisanie:   windows.UTF16PtrToString(p.Description),
			Tip:        p.IfType,
			Sostoyanie: p.OperStatus,
			Metrika:    p.Ipv4Metric,
		}
		for u := p.FirstUnicastAddress; u != nil; u = u.Next {
			if adr, ok := netip.AddrFromSlice(u.Address.IP()); ok {
				a.Adresa = append(a.Adresa, adr.Unmap())
			}
		}
		for g := p.FirstGatewayAddress; g != nil; g = g.Next {
			if adr, ok := netip.AddrFromSlice(g.Address.IP()); ok {
				a.Shlyuzy = append(a.Shlyuzy, adr.Unmap())
			}
		}
		for d := p.FirstDnsServerAddress; d != nil; d = d.Next {
			adr, ok := netip.AddrFromSlice(d.Address.IP())
			if !ok {
				continue
			}
			a.Resolvery = append(a.Resolvery, adr.Unmap())
		}
		pometitUmolchanie(&a, umolchaniya, errMarshrutov)
		itog = append(itog, a)
	}
	return itog, nil
}
