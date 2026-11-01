package protokol

import "time"

// Сроки ответа РАЗНЫЕ у разных команд, и это не тонкая настройка.
//
// Замерено на стенде 01.09.2026: единый срок в пять секунд отваливал `connect`,
// `setKillSwitch` и `refreshSubscription`, причём все три КОМАНДЫ ПРИ ЭТОМ
// СРАБАТЫВАЛИ. Проверено фактом: после отказа «служба не ответила на connect за
// 5s» состояние было `podnyat`, оба ядра жили, внешний адрес был адресом
// сервера. Человеку сообщали о провале выполненной работы, и лечить он пошёл бы
// то, что не сломано.
//
// Задирать общий срок нельзя с другого конца: `status` это чтение поля под
// мьютексом, и минута ожидания на нём означает интерфейс, зависший на минуту
// вместо честного «служба не отвечает».
const (
	// SrokBystroy для команд, которые только читают состояние.
	SrokBystroy = 5 * time.Second
	// SrokDolgoy для команд, которые ходят наружу: поднимают TUN, запускают
	// ядра, зовут netsh пачками, тянут подписку по сети, считают Argon2.
	SrokDolgoy = 60 * time.Second
	// srokObnovleniya для двух команд, которые тянут и ставят архив сборки.
	// Минута им мала: srokZagruzki сам по себе три минуты.
	srokObnovleniya = 4 * time.Minute
	// srokHranilishcha для команд, вся работа которых это одно чтение из
	// зашифрованного хранилища. Минута была бы честной, но чрезмерной: минута
	// ожидания на списке серверов это интерфейс, висящий минуту.
	srokHranilishcha = 30 * time.Second
	// srokZamera для команд, которые МЕРЯЮТ. Их длительность заказывает сам
	// человек, и она заведомо больше минуты: замер полосы идёт в два
	// направления подряд, каждое до yadra.SrokMax (30 с).
	//
	// Найдено живой пробой в госте 07.09.2026. Обе команды замера отвечали
	// «служба не ответила за 5s», работая при этом честно, то есть человек
	// платил трафиком и получал отказ. Тесты службы зовут Obrabotat напрямую,
	// минуя канал, поэтому не видели этого с 05.09.
	srokZamera = 90 * time.Second
	// srokPodyoma для подключения и смены сервера. Минуты им мало, и это
	// замерено в госте 16.10.2026: при молчащих серверах ручной подъём шёл
	// 34 с по TCP и 43 с по hy2, потому что QUIC отказывает за четыре секунды и
	// в окно пробы влезает лишняя. На машине медленнее стенда это переходило
	// за минуту, и окно рисовало «не получилось выполнить действие», пока
	// служба ещё поднимала (жалоба 15.10.2026).
	//
	// Срок взят не от замера, а от суммы ограниченных ожиданий подъёма с
	// запасом: её держит srok_podyoma_test.go в службе.
	srokPodyoma = 3 * time.Minute
	// srokPodpiski для команд, которые тянут подписку по сети. У службы на
	// это свой общий бюджет в 90 с (budzhetPodpiski), после него ещё запись
	// набора и пересборка правил. Окно, ждущее минуту, рвало работающую
	// команду (L14 аудита 1.8.0).
	srokPodpiski = 2 * time.Minute
)

// dolgie перечисляет команды, за которыми стоит работа, а не чтение поля.
//
// Список ЗДЕСЬ, рядом с кодами и тайнами, а не в клиенте: команду добавляют в
// диспетчер и в протокол, и забыть третье место проще всего именно тогда, когда
// новая команда как раз и окажется долгой.
var dolgie = map[string]time.Duration{
	// Подъём TUN, запуск ядра, две попытки с пробой, доспрос причины.
	"connect":    srokPodyoma,
	"disconnect": SrokDolgoy,
	// PUT в живое ядро, проба, возможный откатный PUT, доспрос причины и
	// GET-запросы, каждый до 10 с. Без этой строки клиент оборвал бы её на
	// середине, и человек увидел бы отказ на СРАБОТАВШЕМ переключении.
	"setServer": srokPodyoma,
	// Пересобирает правила брандмауэра через sohranitIPeresobrat, то есть при
	// включённом режиме «весь трафик» уходит в netsh пачкой.
	"setRouteMode": SrokDolgoy,
	// Пачка вызовов netsh на включение и на выключение режима.
	"setKillSwitch": SrokDolgoy,
	// Поход в сеть за подпиской, плюс пересборка правил после слияния.
	"refreshSubscription": SrokDolgoy,
	// Меняют список серверов, а значит пересобирают правила брандмауэра.
	"addServer":       SrokDolgoy,
	"addServers":      SrokDolgoy,
	"removeServer":    SrokDolgoy,
	"setSubscription": srokPodpiski,
	// Обе ходят в сеть тем же путём, что и setSubscription: добавление первой
	// подписки и переключение активной тянут список сразу.
	"addSubscription":       srokPodpiski,
	"setActiveSubscription": srokPodpiski,
	// Пишут набор, а запись набора при включённом режиме «весь трафик»
	// пересобирает правила брандмауэра (M6 аудита 1.8.0).
	"setAutoMember":      SrokDolgoy,
	"removeSubscription": SrokDolgoy,
	// Argon2id с 64 МиБ памяти и тремя проходами. На слабой машине это секунды,
	// и они честные: дешёвый вывод ключа означал бы дешёвый подбор пароля.
	"exportProfile": SrokDolgoy,
	"importProfile": SrokDolgoy,
	// Читает набор, то есть хранилище секретов, как listServers.
	"exportServers": SrokDolgoy,

	// Below: seven commands found by reading the code on 03.09.2026. Every one
	// of them outlives SrokBystroy, and the client tore each of them off
	// mid-work while the service was doing exactly what it was asked.
	//
	// srokZagruzki in proverka_obnovleniy.go is three minutes and the archive
	// cap is 64 MiB. Four minutes is that plus a margin: the deadline has to
	// outlive the work, not match it.
	"downloadUpdate": srokObnovleniya,
	// sha256 over an archive up to predelArhiva, unpacking, taking the tunnel
	// down through a batch of netsh, starting the replacer, and on failure the
	// rollback repeats the install and waits SrokPodyoma again.
	"installUpdate": srokObnovleniya,
	// posledniyVypuskUporno делает до трёх заходов по srokPopytkiProverki (10 с)
	// с отступами 1 и 2 секунды, и каждый заход это два похода в сеть: описание
	// выпуска и .sha256 рядом с архивом. Худшие 33 секунды обязаны помещаться в
	// срок ответа с запасом, иначе повторы обернутся молчанием службы.
	"checkUpdate": SrokDolgoy,
	// Rebuilds the firewall rules through sohranitIPeresobrat, which is exactly
	// what put addServer and setRouteMode on this list.
	"setRules": SrokDolgoy,
	// srokProverkiVyhoda in internal/set/vyhod.go is 15 s.
	"checkExitIp": srokHranilishcha,
	// TWO such probes in a row (internal/set/utechki.go): through the tunnel
	// and directly, plus reading the set for the carrying server's address.
	"checkLeaks": SrokDolgoy,
	// Пять проб подряд, и худший случай у каждой свой: серия из десяти пакетов
	// UDP с шагом в сотню миллисекунд, два похода к серверам имён по три
	// секунды и замер через clash_api. Сама команда держит себя в тридцати
	// секундах, и срок ответа обязан быть БОЛЬШЕ её собственного лимита.
	//
	// Найдено живой пробой в госте 22.09.2026, в день, когда команда завелась:
	// на выключенном VPN она отвечала за 1.8 с и выглядела здоровой, а на
	// поднятом канал рвал её молча, и окно показало бы «служба не ответила» на
	// работающей проверке.
	"checkNetwork": SrokDolgoy,
	// srokSnimka in internal/set/snimok.go is 15 s, and the set is read first.
	"getServerHealth": SrokDolgoy,

	// Ниже команды, которые «просто читают», и это ровно та формулировка,
	// которая их сюда не пускала.
	//
	// Читают они не поле под мьютексом, а зашифрованное хранилище, и худший
	// случай там не миллисекунды. `PopytokRasshifrovki` равно пяти, паузы
	// удваиваются от секунды, то есть при негодном блобе `Zagruzit` спит
	// 1+2+4+8 = 15 секунд, потом ещё хоронит блоб. Обычный замер этого не
	// покажет: на здоровой машине обе отвечают мгновенно.
	//
	// Цена молчания известна заранее: человек с битым sekrety.dat получает
	// «служба не ответила» вместо secrets-unreadable и идёт чинить канал.
	"listServers": srokHranilishcha,
	"listRules":   srokHranilishcha,
	// Свой сервер (01.11.2026): список читает набор и по файлу ключа
	// управления на каждый сервер, установка читает набор до запуска
	// задания. Остальные команды своего сервера сразу отдают снимок.
	"svoyVpsSpisok":    srokHranilishcha,
	"svoyVpsUstanovit": srokHranilishcha,
	"svoyVpsSnesti":    srokHranilishcha,
	"svoyVpsVzyat":     srokHranilishcha,
	"svoyVpsProverit":  srokHranilishcha,
	"svoyVpsOtpustit":  srokHranilishcha,
	// Убирает ключи из набора, то есть пересобирает правила, как removeServer.
	"svoyVpsZabyt": SrokDolgoy,

	// Замеры. Длительность заказывает человек, и она перекрывает всё остальное
	// в этом списке: полоса меряется до 30 секунд в каждую сторону, пинг
	// поднимает временное ядро и обходит весь набор по четыре сервера разом,
	// до 8 секунд на сервер, и сам обрывается на 75-й секунде.
	"measureBandwidth": srokZamera,
	"measureDelays":    srokZamera,
}

// SrokOtveta отдаёт срок ожидания ответа для команды.
func SrokOtveta(imya string) time.Duration {
	if srok, est := dolgie[imya]; est {
		return srok
	}
	return SrokBystroy
}

// DolgieKomandy отдаёт имена долгих команд.
//
// Нужно проверке на стороне службы: опечатка в ключе карты не ломает ничего
// видимого, команда молча получает пять секунд обратно, и починенный дефект
// возвращается без единого признака. Сверять список с реальными командами
// отсюда нечем, реестра команд в протоколе нет, поэтому сверка живёт там, где
// список команд есть.
func DolgieKomandy() []string {
	out := make([]string, 0, len(dolgie))
	for imya := range dolgie {
		out = append(out, imya)
	}
	return out
}
