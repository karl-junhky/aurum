package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/proby"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Константы целей больше нет: сервер берётся из списка (задача 3.8). Пока она
// была, критерий «сервер добавляется ссылкой» был недостижим физически, потому
// что разобранному protokol.Server было некуда лечь.
// pauzaPoslePaniki это отступ перед единственным перезапуском долгого фонового
// цикла, упавшего паникой (fon.SPovtorom).
const pauzaPoslePaniki = 5 * time.Second

const (
// Ядро одно, и его имя с путём живут в tunnel.go рядом с подъёмом туннеля.
// Прежде здесь стояли имя и путь конфига ВТОРОГО ядра (Xray), потому что волне 1
// нужен был процесс, ОТДАЮЩИЙ SOCKS. Второго ядра больше нет (П5).
)

// How long connect waits for the first successful probe before giving up. Longer
// than the 4 second threshold on purpose: the threshold is what we promise, this
// is when we stop hoping.
// A var, not a const, and only so the test can shorten it. A test that honestly
// waits fifteen seconds is a test that gets commented out in a month.
var zhdatPodyoma = 15 * time.Second

// popytokPodyoma это число стартов ядра за один connect (см. Connect).
var popytokPodyoma = 2

// Значения по умолчанию для расписания наблюдения. Они КОПИРУЮТСЯ в поля
// службы, а не читаются из горутин напрямую.
//
// Раньше это были var, которые тест правил у себя и возвращал в defer. Горутина
// наблюдателя при этом переживала свой тест и читала их, пока следующий писал:
// детектор гонок назвал это гонкой в тестах, но общая изменяемая переменная,
// которую читает фоновая горутина, это дефект устройства, а не теста.
const periodNablyudeniyaPoUmolchaniyu = yadra.PeriodProby

// Сколько ждать выбора у группы, которая на подъёме его ещё не назвала, и с
// каким шагом переспрашивать. Замерено 06.09.2026 на живом ядре 1.14.0-rc.5:
// через 1.23 с после подъёма now у группы avto пуст, через 5.63 с назван.
// Двадцать секунд это запас втрое; шаг в полсекунды дёшев, потому что вопрос
// уходит на 127.0.0.1, а не наружу.
// Как часто спрашивать ЯДРО, кто несёт. Отдельно от периода пробы живости:
// вопрос уходит на 127.0.0.1 и стоит копейки, а проба идёт наружу и стоит
// трафика. Пока оба сидели на одном тике в тридцать секунд, поле несущего
// после переключения называло прежний сервер ровно этот период (замерено
// 06.09.2026 трижды: 26.8, 29.5 и 31.1 с).
const periodNesushchegoPoUmolchaniyu = 2 * time.Second

const srokDosprosaPoUmolchaniyu = 20 * time.Second
const shagDosprosaPoUmolchaniyu = 500 * time.Millisecond

// Сколько провалов подряд считаем смертью туннеля. Один это норма жизни на
// мобильной сети, и рвать по нему связь значит рвать её на ровном месте.
const provalovPodryadPoUmolchaniyu = 2

// Отступы между попытками вернуться после аварии. Растущие, а не постоянные:
// постоянный отступ на мёртвой сети это ddos самого себя и съеденная батарея.
// Последний повторяется, пока сеть не вернётся; потолка попыток нет намеренно,
// зато каждая попытка попадает в журнал: молчаливый бесконечный цикл хуже
// отказа. Var ради теста.
var otstupyPoUmolchaniyu = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second,
}

// После десятка неудач подряд при ЖИВОЙ сети сервер, похоже, лёг надолго, и
// стучаться к нему раз в полминуты сутками незачем (С8 аудита 1.6.1). Сеть
// спрашивается напрямую перед каждой длинной паузой: при мёртвой сети
// остаётся обычный отступ, иначе вернувшуюся сеть человек ждал бы пять минут.
const neudachDoRedkih = 10

const redkiyOtstupPoUmolchaniyu = 5 * time.Minute

// periodSetiVPauzePoUmolchaniyu: пауза восстановления раз в столько сверяет
// местный резолвер, и смена сети обрывает паузу (Б2 аудита 1.8.0). Вопрос идёт
// к таблице адаптеров, наружу не ходит, как и у наблюдателя туннеля.
const periodSetiVPauzePoUmolchaniyu = 2 * time.Second

type Sluzhba struct {
	processTracker    *nablyudatelPrilozheniy
	processTrackerErr error
	// muTracker держит пересоздание трекера одним: сборку конфига зовут и
	// подъём, и проверка кандидата, а два трекера это две сессии событий.
	muTracker       sync.Mutex
	novyyTracker    func() (*nablyudatelPrilozheniy, error)
	klyuchVersiy    []byte
	serveryYadra    []protokol.Server
	otpechatkiYadra map[string][32]byte
	mu              sync.Mutex
	// Поколения обновлений защищены muNabor вместе с записью их результатов.
	nomerObnovleniya    uint64
	obnovleniyaPodpisok map[string]uint64
	speed               *speedJob
	speedRunner         speedRun
	// zhurnalKomand пишет строку на каждую команду: имя, кто прислал, исход.
	// nil значит «молчим»: тесты и стенд создают службу без журнала. Тел в нём
	// нет НИКОГДА, там ключи (см. zhurnalKomand.go).
	zhurnalKomand *log.Logger
	// Статистика (6.1): подписчики по id соединения, отмена опроса, период и
	// снимок подставляются тестами. adresVyhoda заполняет checkExitIp (6.3).
	statPodp    map[uint64]bool
	statOtmena  context.CancelFunc
	periodStat  time.Duration
	snimokStat  func(ctx context.Context, adres, sekret string) (yadra.Snimok, error)
	adresVyhoda string
	// Задержка экрана (6.1): свой замер круга через локальный прокси, а не
	// число ядра. Меряется реже цифр, потому что это настоящий запрос в сеть,
	// и держится между замерами.
	periodOtklika time.Duration
	adresOtklika  string
	zamerOtklika  func(ctx context.Context, cel string, portProksi int) (time.Duration, error)
	otklik        time.Duration
	otklikEst     bool
	// Пинг списка серверов (01.10.2026): прибор и временное ядро без TUN, через
	// вход замеров которого он ходит.
	zamerPinga  func(ctx context.Context, cel string, proksi *url.URL) (time.Duration, error)
	yadroZamera func(ctx context.Context) (vremennoeYadro, error)
	// Жалобы ядра на выходы по пути конфига (02.10.2026): только из них
	// узнаётся, почему сервер не ответил. Шов ради тестов, настоящее ядро
	// пишет их само.
	zhalobaVyhoda func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool)
	// Когда группа vybor живого ядра последний раз сменила сервер без
	// перезапуска ядра: её жалобы до этого про прежний сервер (prichina_yadra.go).
	vyborSmenyon time.Time
	// Одно место на замер пинга, см. measureDelays.
	vorotaPinga chan struct{}
	// Проверки 6.3: эндпоинт и два шва для сети и брандмауэра.
	adresProverki string
	sprositVyhod  func(ctx context.Context, endpoint string, portProksi int) (string, error)
	ipv6Zaglushen func() (bool, error)
	// Обновление (6.5): запуск подменщика подставляется тестами.
	zapustitPodmenshchika func(prog, novaya string, podnyat bool) error
	// Каталог данных: исход обновления читается оттуда; тесты подставляют свой.
	dirDannyh    string
	dirProgrammy string
	// Снимок сервера (6.4): загрузчик подставляется тестами.
	zagruzitSnimok func(ctx context.Context, adres string) (set.Snimok, error)
	// Журнал соединений (6.2): файл, период опроса и снимок соединений
	// подставляются тестами.
	zhurnalSoed *sostoyanie.ZhurnalSoedineniy

	// Подробный журнал: сам файл, швы над системой и такт. Всё три ради теста
	// подменяемо, потому что настоящие источники это системные вызовы.
	zhurnalDiag       *diagnostika.Zhurnal
	istochnikiDiag    diagnostika.Istochniki
	periodDiagnostiki time.Duration
	// Снимок выгрузки диагностики, из которого режутся порции (О6).
	vygruzkaDiag snimokVygruzki
	// Накопленные счётчики ядра, чтобы писать в журнал прирост за такт.
	byloVverh        uint64
	byloVniz         uint64
	periodZhurnala   time.Duration
	soedineniyaYadra func(ctx context.Context, adres, sekret string) ([]yadra.Soedinenie, error)
	// muVybor держит ВЕСЬ цикл живого переключения: чтение текущего выбора,
	// PUT, проба, откат, запись набора. Отдельный от mu намеренно: под mu
	// отвечают Status и izvestit, и держать его через два похода в ядро значит
	// подвесить интерфейс на время переключения.
	//
	// ПРАВИЛО, без которого второй замок это взаимная блокировка: muVybor
	// берётся ПЕРВЫМ и никогда не берётся под mu.
	muVybor sync.Mutex
	sost    protokol.Sostoyanie
	// vybranId это то, что человек выбрал сам, и в авто он пуст по построению.
	// nesushchiyId это то, что реально несёт трафик. Ш7-5 научит службу
	// спрашивать второе у ядра, а не переписывать собственное намерение.
	vybranId       string
	nesushchiyId   string
	nesushchiyImya string
	// Подмена молчащего выбранного сервера группой авто в ручном режиме
	// (zapasnoy.go). zapasnoyVmesto это id выбранного, пусто значит подмены
	// нет; имя хранится рядом, потому что Status не ходит в хранилище.
	zapasnoyVmesto     string
	zapasnoyVmestoImya string
	zapasnoyUspehov    int
	// Считать режим только в sobratTun нельзя: Status отвечает с первого кадра,
	// задолго до первого подъёма, и на провод уехала бы пустая строка, то есть
	// третье значение у типа с двумя.
	rezhim protokol.Rezhim
	// Доступ к clash_api живущего ядра. Рождается в sobratTun на каждый старт и
	// нужен пробе ПРЯМО СЕЙЧАС, а не следующему запуску: в файл состояния он
	// пишется для восстановления после смерти службы, и читать оттуда данные
	// собственного живого подключения значило бы ходить кругом через диск.
	portClash int
	// Порт нашего входа mixed. Ноль означает, что прокси мы не поднимали:
	// сторож при этом считает чужим любой включённый системный прокси, как и до
	// появления входа.
	portProksiNash int
	sekretClash    string
	podnyatS       *time.Time
	oshib          *protokol.Oshibka
	otmena         context.CancelFunc
	// pokolenieP это счётчик поколений подъёма, и он существует потому, что
	// отменять подъём контекстом НЕ РАБОТАЕТ.
	//
	// Цикл проб смотрел ctx команды, то есть контекст СОЕДИНЕНИЯ: его не
	// отменяет ни одна команда, он живёт, пока живёт труба. Отменять было
	// нечего и вторым способом: s.otmena к этому моменту уже забрал disconnect,
	// а следующая попытка строила себе новый контекст от Background и поднимала
	// туннель заново. Человеку отвечали «выключено», и через пятнадцать секунд
	// туннель поднимался сам.
	//
	// Счётчик решает это без контекстов вовсе: подъём запоминает своё поколение
	// на входе и на каждом шаге сверяет. Изменилось значит отключили, и подъём
	// выходит, не трогая состояние: его уже поставил тот, кто отключал.
	pokolenieP     int
	pravilaKonfiga string
	trafikKonfiga  protokol.MarshrutTrafika
	// Местный резолвер, УЕХАВШИЙ В КОНФИГ ядра (A4). Ядро не перечитывает
	// конфиг на ходу, поэтому единственный способ узнать, что сеть сменилась, -
	// сверить записанный адрес с текущим. Пустой значит «конфига нет».
	rezolverKonfiga netip.Addr
	// Когда последний раз переподнимались из-за смены сети, и не чаще чего это
	// делать. Поле, а не константа: тест не ждёт тридцати секунд.
	posledniyPerezapuskSeti  time.Time
	perezapuskSetiNeChashche time.Duration
	// Шов чтения местного резолвера. Настоящий спрашивает таблицу адаптеров
	// машины, а тесту смены сети нужно назвать ДРУГОЙ адрес, не трогая сеть
	// рабочей машины.
	mestnyyRezolver func(krome ...uint32) (netip.Addr, error)
	// Все DNS адаптеров и короткий вопрос одному из них (M9 аудита 1.8.0).
	// Настоящие читают адаптеры и ходят в сеть.
	mestnyeRezolvery func(krome ...uint32) ([]netip.Addr, error)
	sprositRezolver  func(context.Context, netip.Addr) error
	// Швы проверки слоёв (A3). Настоящие спрашивают систему и ходят в сеть, а
	// тесту нужно назвать состояние адаптеров и ответ резолвера, не трогая ни
	// сеть рабочей машины, ни её адаптеры.
	adaptery    func() ([]set.Adapter, error)
	probaImeni  func(context.Context, string) proby.Itog
	oshibkaIPv6 error
	// ostanovlena это конец жизни службы, и живёт он под тем же замком, что и
	// поколения подъёма, потому что стережёт то же самое: регистрацию горутин.
	//
	// sync.WaitGroup запрещает Add из нуля ОДНОВРЕМЕННО с Wait, и обе группы
	// службы это нарушали. Наблюдатель заводил восстановление (s.fon.Add) уже
	// после того, как Zavershit прошёл s.fon.Wait(); та горутина звала Connect,
	// тот доходил до s.nabl.Add(1), а Disconnect остановки в это время сидел на
	// s.nabl.Wait(). Детектор гонок называл это записью Zavershit против чтения
	// Connect.func4, и найти по такой подписи причину без счётчика в руках
	// нельзя.
	//
	// Флаг ставится ПОД ЗАМКОМ и раньше любого Wait, а обе точки регистрации
	// берут тот же замок. Поэтому всякая удавшаяся регистрация случилась строго
	// до флага, то есть строго до Wait, а всякая опоздавшая отвергнута.
	ostanovlena bool
	// nabl держит наблюдателя. Disconnect обязан ДОЖДАТЬСЯ его, а не только
	// отменить: отменённая горутина живёт ещё столько, сколько ей нужно
	// дойти до select, и всё это время она читает поля службы и общие
	// переменные. Детектор гонок нашёл это первым же прогоном.
	nabl sync.WaitGroup
	// Расписание наблюдения и восстановления живёт В СЛУЖБЕ, а не в пакете:
	// иначе тест, правящий его у себя, правит его и у чужой фоновой горутины.
	// snimok это ПОСЛЕДНЕЕ записанное состояние файла. Без него каждая запись
	// шла полным снимком и затирала предыдущую: подъём туннеля клал порт
	// clash_api, секрет и адаптер, а следующая запись, сидящая на УСПЕШНОМ
	// пути, стирала всё это обратно. Восстановление после нештатной смерти
	// службы оставалось без данных ровно тогда, когда они нужны.
	snimok sostoyanie.SostoyanieFayla

	period time.Duration
	// Срок и шаг доспроса несущего, см. srokDosprosaPoUmolchaniyu. Поля, а не
	// пакетные переменные: тест укорачивает их у СВОЕЙ службы, не задевая чужую.
	// Период опроса несущего, см. periodNesushchegoPoUmolchaniyu.
	periodNesushchego time.Duration
	srokDosprosa      time.Duration
	shagDosprosa      time.Duration
	provalov          int
	otstupy           []time.Duration
	redkiyOtstup      time.Duration
	// Период опроса системного прокси. Поле, а не пакетная переменная: см.
	// periodProksiPoUmolchaniyu.
	periodProksi time.Duration
	// Судья собранного конфига. Шов, потому что настоящий судья это отдельный
	// процесс ядра, а проверяется здесь НАШЕ поведение при его отказе.
	proveritKonfig func(putKonfiga string) error
	// Восстановление это ВТОРАЯ фоновая горутина, и до сегодня её не отменял и
	// не ждал никто. Она переживала и отключение, и остановку службы, а внутри
	// зовёт Connect, то есть умеет поднять туннель после того, как его опустили.
	//
	// Два уровня отмены, потому что причин две. fonOtmena это конец жизни
	// службы. otmenaVosst это решение человека отключиться: оно обязано
	// останавливать восстановление СРАЗУ, а не после текущего отступа, который
	// на последних попытках достигает минуты.
	fon         sync.WaitGroup
	fonCtx      context.Context
	fonOtmena   context.CancelFunc
	otmenaVosst context.CancelFunc
	// vosstIdyot: восстановление уже крутится (под mu). Второй запуск при нём
	// ничего не делает: прежде он затирал otmenaVosst первого, и «Отключить»
	// гасило только один из двух циклов.
	vosstIdyot bool
	// vosstPokaz: пока идёт восстановление, наружу уходит одно состояние
	// vosstanavlivaetsya (под mu). Круг otkaz, vyklyuchen и podnimaetsya
	// между попытками нужен самому циклу, а человеку он говорил «отказ» и
	// «выключено», пока служба пыталась сама (приёмка 1.9.0). oshibVosst это
	// причина последнего отказа внутри цикла, sledPopytka срок следующей
	// попытки на паузе.
	vosstPokaz  bool
	oshibVosst  *protokol.Oshibka
	sledPopytka *time.Time
	// muPerepodyom выстраивает переподъёмы в очередь (L1 аудита 1.8.0).
	// Порядок замков: muPerepodyom раньше muZaslon и mu.
	muPerepodyom sync.Mutex
	// proveritServer проверяет сервер ядром без TUN перед автоматическим
	// подъёмом (Б1 аудита 1.8.0). Шов: настоящий запускает sing-box.
	proveritServer func(ctx context.Context) error
	// periodSetiVPauze: как часто пауза восстановления сверяет сеть (Б2).
	periodSetiVPauze time.Duration

	podp   map[uint64]chan protokol.Kadr
	sledId uint64

	// Чужие программы, которые ломают VPN или могут мешать (pomehi.go, 1.10.0).
	// Последний ответ, его описание и текст неполной проверки живут под mu;
	// описания нужны, чтобы журнал получал только перемены. proveritPomehi это
	// шов: настоящий обходит процессы, драйверы и адаптеры машины. pomehiMu
	// держится на всю проверку, mu только на чтение и запись ответа.
	pomehiMu       sync.Mutex
	pomehiNaydeno  []pomehi.Nahodka
	pomehiOpis     string
	pomehiOshibka  string
	proveritPomehi func(pomehi.Svoi) ([]pomehi.Nahodka, error)
	periodPomeh    time.Duration

	// Сверка часов (chasy.go, 1.10.0): последнее расхождение и когда оно
	// измерено, под mu. Нулевое время значит «ни разу не сверены». chasyPopytka
	// это начало последней сверки, удачной или нет. zameritChasy это шов:
	// настоящий ходит в сеть.
	chasyRashozhdenie time.Duration
	chasyZamereno     time.Time
	chasyPopytka      time.Time
	zameritChasy      func(context.Context) (time.Duration, error)
	periodChasov      time.Duration

	// Seams. Not for elegance: without them the only way to test a state machine
	// is to own a hypervisor, and a state machine nobody tests is where the app
	// learns to lie about being connected.
	storozhit func(ctx context.Context, imya, konfig string, sob func(protokol.Sostoyanie)) error
	// Замер идёт у ТОГО ядра, которое ведёт трафик, по ТОМУ исходящему, которым
	// он пойдёт. Прежняя проба ходила в локальный SOCKS второго ядра, то есть
	// мимо туннеля, и на сервере, не несущем ничего, отвечала успехом.
	zamerit       func(ctx context.Context, adres, sekret, teg string) (time.Duration, error)
	zameritRezerv func(ctx context.Context, adres, sekret, teg string) (time.Duration, error)
	// peremeritAvto просит группу авто перемерить серверы перед разрывом (С6
	// аудита 1.6.1). Первое значение: был ли перемер, то есть стоит ли «авто».
	peremeritAvto func(ctx context.Context, adres, sekret string) (bool, error)
	// Несущего называет ЯДРО, а не наше намерение. Два имени групп параметрами:
	// пакет yadra не знает про генератор конфига и знать не должен.
	nesyot func(ctx context.Context, adres, sekret, gruppa, tegAvto string) (string, error)
	// Швы живого переключения. Оба сетевые, и оба ОБЯЗАНЫ быть заглушены в
	// фикстуре: она кладёт zapomnitKlash(52715, ...), и незаглушенный шов уходит
	// стучаться на этот порт по-настоящему.
	postavitVybor func(ctx context.Context, adres, sekret, gruppa, teg string) error
	vyborGruppy   func(ctx context.Context, adres, sekret, gruppa string) (string, error)
	zhdatKlash    func(ctx context.Context, adres, sekret string) error
	zapisat       func(sostoyanie.SostoyanieFayla) error
	// Волна 2. Шов на ВЕСЬ подъём туннеля, а не на ожидание адаптера внутри
	// него. Разница практическая: подъём ещё и пишет конфиг в каталог данных и
	// выдаёт порт, и тест с узким швом молча гадил бы в живой ProgramData.
	podnyatTunnel func(ctx context.Context) (set.Adapter, error)

	// Проверка обновлений (план «шесть удобств» §5): адрес каталога с
	// versiya.json и архивами, загрузчик (подставляется тестами), находка.
	adresObnovleniy     string
	skachatFayl         func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error)
	obnovlenie          *protokol.ObnovlenieOtvet
	obnovlenieProvereno *time.Time
	// Причина последнего неудавшегося захода. Держится до первой удачи: пока
	// проверка не прошла, человеку нужна причина, а не пустота.
	obnovlenieOtkaz string
	// Правило IPv6 живёт ровно столько же, сколько туннель, и НЕ зависит от
	// режима. Шов нужен не для красоты: без него тест опускания идёт заводить
	// настоящее правило в брандмауэре машины разработчика.
	glushitIPv6     func() error
	vernutIPv6      func() error
	vklyuchitVes    func(set.Razreshyonnoe, bool) error
	prochitatProksi func() ([]set.ProksiCheloveka, error)
	vyklyuchitVes   func() error
	// Узкая пересборка ОДНОГО правила, списка адресов серверов. Отдельный шов
	// от vklyuchitVes, потому что зовётся ровно там, где тот бессилен: ядро
	// умерло, адреса TUN нет, набор целиком не собрать.
	suzitServery func(set.Adresa) error
	// Переписывание правила DNS под замком без туннеля (L4 аудита 1.8.0) и
	// резолвер, под которым оно стоит сейчас. Пустой значит «не знаем».
	// dnsPodZamkom под muZaslon.
	perezavestiDns func(netip.Addr) error
	dnsPodZamkom   netip.Addr

	// killSwitch это НЕ производная от состояния брандмауэра: система может
	// быть заперта чужим правилом, а мы про это ничего не знаем. Здесь только
	// то, что включили мы сами.
	killSwitch    bool
	zaslonAktiven bool
	muZaslon      sync.Mutex
	// otpechatokZamka это отпечаток списка, под который замок собран
	// последним удачным vklyuchitVes (Д1 аудита 1.8.0). Пустой значит «не
	// знаем, что стоит». Под muZaslon.
	otpechatokZamka string
	// pravilaOtstali это ошибка, поставленная подъёмом, когда пересборка
	// правил после него не прошла (L3 аудита 1.8.0). nil значит «догнали».
	// Под mu.
	pravilaOtstali *protokol.Oshibka
	// estOtkat отвечает, лежит ли на диске файл отката замка. Шов: настоящий
	// читает ProgramData, а тесту нужно назвать расхождение с памятью (L2
	// аудита 1.8.0), не трогая живой брандмауэр.
	estOtkat func() bool

	// upalaSTunnelem: файл состояния при старте говорил podnyat, то есть прошлая
	// служба умерла, не опустив туннель (Н2 аудита 1.6.1). Штатное опускание
	// пишет vyklyuchen, так что podnyat в файле при старте бывает только после
	// аварии: убитый процесс, паника, пропавшее питание.
	upalaSTunnelem bool
	// podnyatPosleObnovleniya: служба стартовала после самообновления, при
	// котором туннель был поднят (Н10 аудита 1.6.1).
	podnyatPosleObnovleniya bool

	// muFayl держит порядок записей файла состояния: снимок и его запись идут
	// под одним замком (Н8 аудита 1.6.1). Под одним s.mu снимок снимался, а
	// писался уже без замка, и более старый снимок ложился на диск поверх
	// нового. Порядок замков: muFayl, потом mu, никогда наоборот.
	muFayl sync.Mutex

	// vneocherednaya будит наблюдателя туннеля на пробу вне очереди: после
	// сна туннель проверяется сразу, а не на следующем тике (Н11 аудита 1.6.1).
	vneocherednaya chan struct{}

	// rezhimNeProchitan: хранилище не прочиталось при создании службы, и режим
	// в статусе стоит умолчанием, а не из набора.
	rezhimNeProchitan bool
	// svoiSerty это сертификаты своих серверов из набора для Status(): под
	// mu, идёт за хранилищем, как rezhim.
	svoiSerty []sertSvoego

	// pauzaPaniki: отступ перед повтором наблюдателя после паники. Полем, а не
	// константой на месте вызова, ради теста второй паники (Г6).
	pauzaPaniki time.Duration

	// tolchokVosst обрывает паузу восстановления после сна машины: без
	// туннеля vneocherednaya забирать некому (Б2 аудита 1.8.0). С 16.10.2026
	// и по действию человека (popytatsyaSeychas); строка называет причину для
	// журнала.
	tolchokVosst chan string

	// Адаптер туннеля, известен только после подъёма. На его индексе держится
	// поиск канала ПОД туннелем, на алиасе порядок правил задачи 2.5.
	tun set.Adapter

	// Хранилище секретов. Шов, потому что настоящее пишет в ProgramData, и тест
	// без шва гадил бы в живой каталог машины разработчика.
	sekretyChitat func() ([]byte, error)
	sekretyPisat  func([]byte) error

	// Набор серверов. Шов, потому что настоящий ходит в хранилище секретов, а
	// тесту нужен список без DPAPI и без ProgramData.
	nabor func() (Nabor, error)

	// Запись конфига второго ядра. Шов, потому что настоящая пишет в живой
	// C:\ProgramData, а тест не должен трогать машину, на которой идёт.

	// Загрузка подписки. Шов, потому что настоящая ходит в сеть, а тест обязан
	// уметь показать и истекшую подписку, и недоступную, не поднимая сервера.
	//
	// Здесь СТРАТЕГИЯ целиком: бюджет времени, выбор пути и запасной заход
	// (A8). Тесты, которым нужен только ответ подписки, подменяют её и не
	// думают про дороги.
	zagruzitPodpisku func(ctx context.Context, adres string) (ssylki.Razbor, error)

	// Одна дорога загрузки: пустой proksi это прямой путь, непустой - через
	// локальный вход ядра. Отдельным швом от предыдущего, чтобы саму стратегию
	// можно было проверить настоящую, подменив под ней только сеть.
	zagruzitCherez func(ctx context.Context, adres, proksi string, popytok int) (ssylki.Razbor, error)

	// Адреса для правила петли И для разрешающих правил брандмауэра. ОДИН
	// источник на оба списка: два независимых сборщика неизбежно разошлись бы,
	// и разошлись бы тихо. Адрес в петле, но не в брандмауэре, даёт неработающий
	// туннель в запертом режиме; адрес в брандмауэре, но не в петле, даёт петлю.
	//
	// Шов ещё и потому, что настоящий сборщик ходит в системный резолвер, а тест
	// не должен зависеть от чужого DNS.
	sobratAdresa func() (set.Adresa, error)
	// Сам сборщик по спискам. Шов, чтобы тест мог увидеть, ЧТО ему ушло
	// (адреса наборов), не трогая резолвер машины.
	sobratAdresaSet func(servery []protokol.Server, podpiska string, zagruzki ...string) (set.Adresa, error)

	// Наборы rule_set. Желаемые: шов, потому что настоящие лежат в ProgramData.
	// Загрузка: шов, потому что настоящая ходит в сеть.
	naboryZhelaemye func() []genkonfig.NaborPravil
	skachatNabor    func(ctx context.Context, adres string) ([]byte, error)

	// Список рекламы. Швы: настоящие ходят в сеть, в ядро и в dnsapi.dll.
	skachatSpisok  func(ctx context.Context, adres string) ([]byte, error)
	sobratSpisok   func(ctx context.Context, tekst, vyhod string) error
	sbrositKeshDNS func() error
	zhdatReklamu   func(ctx context.Context, d time.Duration) (tolchok, zhiv bool)
	reklamaTolchok chan struct{} // буфер 1
	// muReklama стережёт ПАРУ «reklama.srs + reklama.json»: подмену, запись
	// меты, сверку sha256 и выкладку встроенного. Сеть и convert идут БЕЗ
	// замка: их минуты не должны держать подъём туннеля.
	muReklama sync.Mutex
	// Под s.mu: что лежит в файле и несёт ли его конфиг живого ядра. Имена не
	// reklama, чтобы не затенять пакет.
	sostReklamy     *protokol.ReklamaSostoyanie
	reklamaVKonfige bool

	// podRezhim поднят только на время перезапуска, затеянного включением
	// режима «весь трафик». Живёт под тем же замком, что и killSwitch.
	podRezhim bool

	// Свой сервер (svoy_vps.go): одно задание на службу под muVps. muVps не
	// держится через izvestit и не берётся под mu. Швы: настоящие ходят на
	// сервер, в сеть машины и за программой сервера. vpsStaging берёт
	// тестовый выпуск Let's Encrypt (AFFORY_LE_STAGING), istochnikUzla это
	// файл программы сервера для стенда (AFFORY_UZEL), vshityyUzel - какую
	// программу ждёт эта сборка.
	muVps sync.Mutex
	vps   *zadanieVps
	// vpsZaporDo: до этого срока задания своего сервера не заводятся, отказ
	// несёт vpsZaporTekst. Запирают установка обновления и «забыть»,
	// vpsZaporNomer отличает один запор от следующего.
	vpsZaporDo         time.Time
	vpsZaporTekst      string
	vpsZaporNomer      uint64
	vpsStaging         bool
	vpsOsmotret        func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error)
	vpsUstanovit       func(context.Context, upravlenie.Ustanovka) (upravlenie.Itog, error)
	vpsSnesti          func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.SnosItog, error)
	vpsVzyat           func(context.Context, upravlenie.Vzyatie) (upravlenie.Itog, error)
	vpsProverit        func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error)
	vpsOtpustit        func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.OtpuskItog, error)
	vpsVyklyuchitParol func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error)
	vpsVklyuchitParol  func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error)
	vpsPodobratPorty   func(context.Context, upravlenie.Podbor) (upravlenie.Itog, error)
	programmaUzla      func(context.Context) (io.ReadSeekCloser, error)
	istochnikUzla      string
	vshityyUzel        func() (vypuskUzla, error)
	fizicheskiyAdapter func() (set.Adapter, error)

	seychas   func() time.Time
	zhdat     func(ctx context.Context, d time.Duration) bool
	prochitat func() (sostoyanie.SostoyanieFayla, error)
}

func NovayaSluzhba() *Sluzhba {
	s := &Sluzhba{
		sost:      protokol.SostVyklyuchen,
		podp:      map[uint64]chan protokol.Kadr{},
		statPodp:  map[uint64]bool{},
		storozhit: yadra.Storozhit,
		zapisat:   sostoyanie.Zapisat,
		// На одну просьбу: две подряд значат ровно то же, что одна.
		vneocherednaya: make(chan struct{}, 1),
		tolchokVosst:   make(chan string, 1),
		vorotaPinga:    make(chan struct{}, 1),
	}
	s.period = periodNablyudeniyaPoUmolchaniyu
	s.periodNesushchego = periodNesushchegoPoUmolchaniyu
	s.srokDosprosa = srokDosprosaPoUmolchaniyu
	s.shagDosprosa = shagDosprosaPoUmolchaniyu
	s.periodStat = periodStatPoUmolchaniyu
	s.snimokStat = yadra.Statistika
	s.periodOtklika = periodOtklikaPoUmolchaniyu
	s.adresOtklika = set.CelOtklikaPoUmolchaniyu
	s.zamerOtklika = set.Otklik
	s.zamerPinga = func(ctx context.Context, cel string, proksi *url.URL) (time.Duration, error) {
		return set.LuchshiyOtklikCherez(ctx, cel, proksi, krugovPinga)
	}
	s.yadroZamera = s.podnyatYadroZamera
	s.zhalobaVyhoda = yadra.ZhalobaVyhoda
	s.adresProverki = set.AdresProverkiPoUmolchaniyu
	s.sprositVyhod = set.AdresVyhoda
	s.ipv6Zaglushen = set.IPv6Zakryt
	s.zapustitPodmenshchika = zapustitPodmenshchika
	s.dirDannyh = sostoyanie.KatalogDannyh()
	s.dirProgrammy = sostoyanie.KatalogProgrammy()
	s.zagruzitSnimok = set.ZagruzitSnimok
	s.adresObnovleniy = AdresObnovleniyPoUmolchaniyu
	// Не голая skachatPoSeti: при поднятом туннеле выпуск берётся через него,
	// см. skachatVypusk. Прямой поход остаётся откатом.
	s.skachatFayl = s.skachatVypusk
	s.zhurnalSoed = sostoyanie.NovyyZhurnalSoedineniy()
	s.periodDiagnostiki = periodDiagnostikiPoUmolchaniyu
	s.proveritPomehi = pomehi.NovyySkaner().Proverit
	s.periodPomeh = periodPomehPoUmolchaniyu
	s.zameritChasy = func(ctx context.Context) (time.Duration, error) {
		return set.RashozhdenieChasov(ctx, set.CeliChasov)
	}
	s.periodChasov = periodChasovPoUmolchaniyu
	// Файл открывается ВСЕГДА, а пишется только при включённой настройке: иначе
	// первая же секунда после включения уходила бы на открытие файла, а
	// интересна как раз она.
	if f, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), diagnostika.ImyaZhurnala); err == nil {
		s.zhurnalDiag = diagnostika.NovyyZhurnal(f)
		// Отпечаток сборки в каждой строке операции: журнал живой машины без
		// него не отвечает на вопрос, та ли это версия, в которой дефект
		// чинили (A7).
		s.zhurnalDiag.ZadatSborku(diagnostika.Sborka(versiyaProgrammy))
	} else {
		log.Printf("подробный журнал не открыт: %v", err)
	}
	s.istochnikiDiag = s.nastoyashchieIstochniki()
	s.periodZhurnala = periodZhurnalaPoUmolchaniyu
	s.soedineniyaYadra = yadra.Soedineniya
	s.provalov = provalovPodryadPoUmolchaniyu
	s.perezapuskSetiNeChashche = perezapusSetiNeChashche
	s.mestnyyRezolver = set.LokalnyyResolverKrome
	s.mestnyeRezolvery = set.LokalnyeResolveryKrome
	s.sprositRezolver = set.SprositResolver
	s.adaptery = set.Adaptery
	s.probaImeni = proby.Imya
	s.otstupy = otstupyPoUmolchaniyu
	s.redkiyOtstup = redkiyOtstupPoUmolchaniyu
	s.periodSetiVPauze = periodSetiVPauzePoUmolchaniyu
	s.proveritServer = s.proveritServerBezTun
	s.novyyTracker = novyyNablyudatelPrilozheniy
	s.pauzaPaniki = pauzaPoslePaniki
	s.periodProksi = periodProksiPoUmolchaniyu
	s.proveritKonfig = func(put string) error { return yadra.Proverit(imyaYadraTun, put) }
	s.vpsStaging = os.Getenv("AFFORY_LE_STAGING") == "1"
	s.vpsOsmotret = upravlenie.Osmotret
	s.vpsUstanovit = upravlenie.Ustanovit
	s.vpsSnesti = upravlenie.Snesti
	s.vpsVzyat = upravlenie.Vzyat
	s.vpsProverit = upravlenie.ProveritServer
	s.vpsOtpustit = upravlenie.Otpustit
	s.vpsVyklyuchitParol = upravlenie.VyklyuchitParol
	s.vpsVklyuchitParol = upravlenie.VklyuchitParol
	s.vpsPodobratPorty = upravlenie.PodobratPorty
	s.programmaUzla = s.programmaSVypuska
	s.istochnikUzla = os.Getenv(peremennayaUzla)
	s.vshityyUzel = prochitatVshityyUzel
	s.fizicheskiyAdapter = ssh.FizicheskiyAdapter
	s.fonCtx, s.fonOtmena = context.WithCancel(context.Background())
	s.speedRunner = runSpeed
	s.glushitIPv6 = set.ZakrytIPv6
	s.vernutIPv6 = set.OtkrytIPv6
	s.vklyuchitVes = set.VklyuchitVesTrafik
	s.prochitatProksi = set.ProksiLyudey
	s.vyklyuchitVes = set.VyklyuchitVesTrafik
	s.estOtkat = func() bool { _, err := set.ProchitatOtkat(); return err == nil }
	s.suzitServery = set.PerezavestiRazreshyonnyeServery
	s.perezavestiDns = set.PerezavestiPravilaDns
	// Значение метода берётся после создания: раньше её просто не у чего взять.
	s.podnyatTunnel = s.podnyatTunSistemno
	hr := hranenie.Novyy()
	s.sekretyChitat = hr.Zagruzit
	s.sekretyPisat = hr.Sohranit
	s.sobratAdresa = s.adresaKandidatov
	s.sobratAdresaSet = set.SobratAdresa
	s.naboryZhelaemye = naboryIzUmolchaniy
	s.skachatNabor = skachatNaborPoSeti
	s.skachatSpisok = s.skachatSpisokReklamy
	s.sobratSpisok = s.sobratSpisokYadrom
	s.sbrositKeshDNS = sbrositKeshDNSWindows
	s.reklamaTolchok = make(chan struct{}, 1)
	s.zhdatReklamu = func(ctx context.Context, d time.Duration) (bool, bool) {
		return zhdatIliTolchok(ctx, d, s.reklamaTolchok)
	}
	s.nabor = s.naborIzHranilishcha
	// Читаем набор ПРИ СТАРТЕ, а не ждём первой записи. Поведение считает
	// rezhimNabora по набору из хранилища, и одного умолчания в конструкторе
	// мало: после перезапуска с сохранённым Vybran провод сказал бы «авто», а
	// конфиг собрался бы ручным. Отказ чтения (хранилище заперто, DPAPI не
	// отдал) это не повод падать: набор всё равно недоступен, серверов ноль, и
	// умолчание тут честнее пустоты.
	s.rezhim = rezhimPoUmolchaniyu
	if n, err := s.naborIzHranilishcha(); err == nil {
		s.rezhim, s.svoiSerty = rezhimNabora(n), sertySvoih(n, s.estKlyuchUpravleniya)
	} else {
		// Дочитает DochitatKhranilishche, когда служба запустит фон (M4).
		s.rezhimNeProchitan = true
	}
	// Прямой загрузчик один на службу: транспорт держит пул соединений, и
	// заводить его заново на каждое обновление незачем. Загрузчик через туннель
	// собирается на месте: порт локального входа живёт ровно столько, сколько
	// поднят туннель, и меняется между подъёмами.
	zagr := ssylki.NovyyZagruzchik()
	zagr.Ustroystvo = ustroystvoMashiny()
	s.zagruzitCherez = func(ctx context.Context, adres, proksi string, popytok int) (ssylki.Razbor, error) {
		if proksi == "" {
			return zagr.ZagruzitSPovtorami(ctx, adres, popytok)
		}
		cherez, err := ssylki.NovyyZagruzchikCherez(proksi)
		if err != nil {
			return ssylki.Razbor{}, err
		}
		cherez.Ustroystvo = zagr.Ustroystvo
		// Транспорт свой на каждый вызов: без закрытия его соединения к
		// локальному входу висели до срока простоя (L8 аудита 1.8.0).
		defer cherez.Klient.CloseIdleConnections()
		return cherez.ZagruzitSPovtorami(ctx, adres, popytok)
	}
	s.zagruzitPodpisku = s.zagruzitPodpiskuStrategiey
	s.seychas = time.Now
	s.zhdat = zhdatPoChasam
	s.prochitat = sostoyanie.Prochitat
	s.zagruzitNastroyki()
	s.zamerit = yadra.Zaderzhka
	s.zameritRezerv = yadra.ZaderzhkaRezerv
	s.peremeritAvto = func(ctx context.Context, adres, sekret string) (bool, error) {
		return yadra.PeremeritAvto(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto)
	}
	s.nesyot = yadra.Nesyot
	s.postavitVybor = yadra.PostavitVybor
	s.vyborGruppy = yadra.VyborGruppy
	s.zhdatKlash = yadra.ZhdatKlash
	return s
}

func (s *Sluzhba) Podpisatsya() (uint64, <-chan protokol.Kadr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sledId++
	id := s.sledId
	// Buffered: a subscriber that stopped reading is a UI that is gone, and the
	// service must not block on its ghost.
	c := make(chan protokol.Kadr, 32)
	s.podp[id] = c
	return id, c
}

func (s *Sluzhba) Otpisatsya(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, est := s.podp[id]; est {
		delete(s.podp, id)
		close(c)
	}
	// Смерть соединения это отписка от статистики: интерфейс, убитый
	// диспетчером задач, сам отписаться не успевает.
	delete(s.statPodp, id)
	s.pogasitOprosStat()
}

func (s *Sluzhba) izvestit(imya string, telo any) {
	syroe, err := json.Marshal(telo)
	if err != nil {
		return
	}
	k := protokol.Kadr{Tip: "sobytie", Imya: imya, Telo: syroe}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.podp {
		select {
		case c <- k:
		default:
		}
	}
}

// The service owns the state. The UI never computes it from parts, because two
// places computing the same truth is how they start disagreeing.
func (s *Sluzhba) postavit(n protokol.Sostoyanie, oshib *protokol.Oshibka) {
	// Заслон здесь, а не в Status: Status зовётся на каждый опрос окна, и
	// журнал получил бы одну и ту же строку сотни раз.
	//
	// Текст правится НА МЕСТЕ, а не новым объектом: наблюдатель защиты
	// снимает свою ошибку, узнавая её по указателю (s.oshib == oshibka), и
	// подмена объекта оставила бы её висеть навсегда.
	if oshib != nil {
		oshib.Tekst = oshibkaDlyaOkna("состояние", oshib.Kod, oshib.Tekst).Tekst
	}
	s.mu.Lock()
	if s.sost != n {
		s.cancelSpeedLocked("подключение изменилось, запусти замер заново")
	}
	s.sost = n
	s.oshib = oshib
	if s.vosstPokaz && oshib != nil && n != protokol.SostPodnyat {
		s.oshibVosst = oshib
	}
	if n == protokol.SostPodnyat && s.podnyatS == nil {
		t := time.Now()
		s.podnyatS = &t
	}
	if n == protokol.SostVyklyuchen {
		s.podnyatS = nil
	}
	s.mu.Unlock()
	s.izvestit("state", s.Status())
	// Адрес выхода по подъёму, не по таймеру (§5).
	if n == protokol.SostPodnyat {
		fon.Zapustit("обновлении адреса выхода", s.obnovitAdresVyhoda)
	}
}

// Status это то, что служба ПОКАЗЫВАЕТ. Решать по нему нельзя: во время
// восстановления там vosstanavlivaetsya вместо внутреннего круга, и postavit
// с состоянием, взятым отсюда, записал бы маску внутрь. Решения берут
// состояние из vnutri.
func (s *Sluzhba) Status() protokol.StatusOtvet {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Копия, а не указатель на поле: ответ уходит в канал и живёт дольше замка.
	var rk *protokol.ReklamaSostoyanie
	if s.sostReklamy != nil {
		k := *s.sostReklamy
		k.Deystvuet = s.reklamaVKonfige && s.sost == protokol.SostPodnyat
		rk = &k
	}
	sost, oshib := s.sost, s.oshib
	var sled *time.Time
	if s.vosstPokaz && vKrugeVosstanovleniya(sost) {
		sost = protokol.SostVosstanavl
		// На паузе и в начале попытки своей ошибки у состояния нет, а причина
		// последнего отказа нужна человеку всё время цикла.
		if oshib == nil {
			oshib = s.oshibVosst
		}
		if s.sledPopytka != nil {
			t := *s.sledPopytka
			sled = &t
		}
	}
	// Подмена показывается, только пока туннель поднят и несёт ДРУГОЙ сервер:
	// группа авто могла снова выбрать выбранный, и тогда «не отвечает» было бы
	// неправдой.
	vremennoVmesto := ""
	if s.zapasnoyVmesto != "" && sost == protokol.SostPodnyat && s.nesushchiyId != s.zapasnoyVmesto {
		vremennoVmesto = s.zapasnoyVmestoImya
	}
	return protokol.StatusOtvet{
		Reklama:             rk,
		VremennoVmesto:      vremennoVmesto,
		Sostoyanie:          sost,
		SledPopytka:         sled,
		TrafikPoUmolchaniyu: s.trafikKonfiga,
		VybranId:            s.vybranId,
		NesushchiyId:        s.nesushchiyId,
		NesushchiyImya:      s.nesushchiyImya,
		RezhimMarshruta:     s.rezhim,
		VersiyaSluzh:        fmt.Sprintf("%d", protokol.Versiya),
		PodnyatS:            s.podnyatS,
		KillSwitch:          s.killSwitch,
		PortProksi:          s.portProksiNash,
		// Read from the registry every time, never cached: the Run key is the
		// only truth, and a cached flag would disagree with a key edited by hand.
		Avtozapusk:           avtozapuskVklyuchen(),
		PodklyuchatPriStarte: s.snimok.PodklyuchatPriStarte,
		Zhurnal:              s.snimok.Zhurnal,
		Diagnostika:          s.snimok.Diagnostika,
		PolosaVverh:          s.snimok.PolosaVverh,
		PolosaVniz:           s.snimok.PolosaVniz,
		Oshib:                oshib,
		VersiyaProgrammy:     versiyaDlyaEkrana(),
		Obnovlenie:           s.obnovlenie,
		ObnovlenieProvereno:  s.obnovlenieProvereno,
		ObnovlenieOtkaz:      s.obnovlenieOtkaz,
		Pomehi:               s.pomehiDlyaStatusa(),
		ChasyRashozhdenie:    s.chasyDlyaStatusa(),
		SvoiTrevogi:          s.svoiTrevogiDlyaStatusa(),
	}
}

// vKrugeVosstanovleniya: состояния, через которые цикл восстановления ходит
// между попытками. podnyat в их число не входит, это конец цикла.
func vKrugeVosstanovleniya(s protokol.Sostoyanie) bool {
	switch s {
	case protokol.SostVyklyuchen, protokol.SostPodnimaetsya, protokol.SostOtkaz, protokol.SostNeNeset:
		return true
	}
	return false
}

// vnutri отдаёт состояние и ошибку, по которым служба РЕШАЕТ, без маски
// восстановления.
func (s *Sluzhba) vnutri() (protokol.Sostoyanie, *protokol.Oshibka) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sost, s.oshib
}

func (s *Sluzhba) vnutriSost() protokol.Sostoyanie {
	sost, _ := s.vnutri()
	return sost
}

func (s *Sluzhba) vnutriOshib() *protokol.Oshibka {
	_, oshib := s.vnutri()
	return oshib
}

// pokazVosst включает и снимает vosstanavlivaetsya снаружи. Смена уходит
// событием сразу: иначе она дошла бы до окна и трея только следующим
// опросом, через пять секунд.
func (s *Sluzhba) pokazVosst(vkl bool) {
	s.mu.Lock()
	bylo := s.vosstPokaz
	s.vosstPokaz = vkl
	if !vkl {
		s.oshibVosst, s.sledPopytka = nil, nil
	}
	s.mu.Unlock()
	if bylo != vkl {
		s.izvestit("state", s.Status())
	}
}

// zagruzitNastroyki читает с диска то, что является НАСТРОЙКОЙ, а не
// состоянием: без этого первая же запись snimok затёрла бы флаг нулём.
// Отказ чтения это первый запуск или запертый каталог, умолчание честнее.
func (s *Sluzhba) zagruzitNastroyki() {
	f, err := s.prochitat()
	if err != nil {
		return
	}
	s.mu.Lock()
	s.snimok.PodklyuchatPriStarte = f.PodklyuchatPriStarte
	s.snimok.KillSwitch, s.killSwitch = f.KillSwitch, f.KillSwitch
	s.snimok.Zhurnal = f.Zhurnal
	s.snimok.Diagnostika = f.Diagnostika
	s.snimok.PolosaVverh, s.snimok.PolosaVniz = f.PolosaVverh, f.PolosaVniz
	if f.AdresProverki != "" {
		s.adresProverki = f.AdresProverki
	}
	s.upalaSTunnelem = f.Sostoyanie == protokol.SostPodnyat
	s.mu.Unlock()
}

// SetConnectOnStart запоминает второе решение (§9.2, задача 4.8).
//
// Отдаёт отказ записи наружу: флаг живёт ТОЛЬКО в файле, и незаписанный флаг
// это настройка, которой не существует. Отвечать успехом на неё значит обещать
// подъём при старте, которого не будет.
func (s *Sluzhba) SetConnectOnStart(vkl bool) error {
	return s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodklyuchatPriStarte = vkl })
}

// SetPolosa запоминает полосу канала человека в мегабитах.
//
// Пара нулей это законное «снять объявление»: без полосы hysteria2 идёт на BBR,
// и это рабочее состояние, а не поломка. Всё остальное проверяется ЗДЕСЬ, а не
// в генераторе: там отказ означает несобранный конфиг всего туннеля, то есть
// человек увидит «туннель не поднялся» вместо «в поле полосы ерунда».
//
// Потолок стоит намеренно. Brutal гонит трафик РОВНО с объявленной скоростью,
// поэтому опечатка в поле это не косметика: замер 05.09.2026 на полке 20 Мбит
// при объявленных 440 дал восемь провалов и p95 3214 мс против нуля и 92 мс у
// честного объявления.
const PotolokPolosy = 10000

func (s *Sluzhba) SetPolosa(vverh, vniz int) error {
	switch {
	case vverh == 0 && vniz == 0:
	case vverh <= 0 || vniz <= 0:
		return fmt.Errorf("полоса задаётся парой положительных чисел либо снимается парой нулей: вверх %d, вниз %d", vverh, vniz)
	case vverh > PotolokPolosy || vniz > PotolokPolosy:
		return fmt.Errorf("полоса больше %d Мбит: вверх %d, вниз %d", PotolokPolosy, vverh, vniz)
	}
	return s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
		f.PolosaVverh, f.PolosaVniz = vverh, vniz
	})
}

// PodklyuchitPriStarte поднимает туннель при старте службы, если человек это
// выбрал. Выключенный флаг означает ровно ничего: туннель, поднявшийся сам
// без просьбы, это самый неприятный сюрприз у VPN.
//
// Одна попытка, дальше восстановление (H1в и M1 аудита 1.8.0). Прежде здесь
// крутились шесть попыток по десять секунд, и у цикла было два изъяна. Он
// сдавался через минуту на незапертой машине: Wi-Fi после входа, PPPoE или
// долгий логин, и VPN не поднимался до перезагрузки. И его не гасило
// «Отключить»: оно отменяет только восстановление, а цикл через десять секунд
// поднимал туннель снова. Восстановление умеет и то, и другое: у него растущий
// отступ без потолка попыток, и его отменяет «Отключить».
func (s *Sluzhba) PodklyuchitPriStarte(ctx context.Context) {
	s.mu.Lock()
	vkl := s.snimok.PodklyuchatPriStarte
	// Замок ПОДРАЗУМЕВАЕТ туннель, и это не удобство, а единственный выход.
	// Запертая машина без туннеля не имеет связи вовсе: правила пускают наружу
	// только ядро, а его нет. Ф1 от 05.09.2026, замерено в госте: после
	// убийства службы SCM вернул её, замок помнился, флаг был выключен, и
	// машина осталась без сети навсегда.
	//
	// Самовольный подъём на НЕЗАПЕРТОЙ машине по-прежнему запрещён: там он
	// сюрприз, а здесь возврат отнятого.
	zaperta := s.zaslonAktiven
	// Туннель, который опустила авария или обновление, а не человек,
	// возвращается так же, как при замке: человек его не выключал.
	avariya, obnovlenie := s.upalaSTunnelem, s.podnyatPosleObnovleniya
	s.mu.Unlock()
	if !vkl && !zaperta && !avariya && !obnovlenie {
		return
	}
	switch {
	case vkl || zaperta:
	case obnovlenie:
		log.Printf("до обновления туннель был поднят: поднимаю его обратно")
	case avariya:
		log.Printf("прошлая служба завершилась, не опустив туннель: поднимаю его обратно")
	}
	err := s.connect(ctx, nil, true)
	if err == nil {
		log.Printf("туннель поднят при старте")
		return
	}
	// Отмена это «Отключить» или остановка службы: возвращать туннель некуда.
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		log.Printf("подъём при старте отменён: %v", err)
		return
	}
	log.Printf("подъём при старте не удался, ухожу в восстановление: %v", err)
	s.zapustitVosstanovlenie()
}

// ProbaPosleSna просит наблюдателя проверить туннель вне очереди, а
// восстановление, если оно ждёт в паузе, попробовать сразу. Просьбу, которую
// некому забрать, следующий подъём перекроет сам.
func (s *Sluzhba) ProbaPosleSna() {
	log.Printf("машина проснулась: проверяю туннель вне очереди")
	select {
	case s.vneocherednaya <- struct{}{}:
	default:
	}
	select {
	case s.tolchokVosst <- "машина проснулась":
	default:
	}
}

// PomnitPodnyatPosleObnovleniya отмечает старт после самообновления, при
// котором туннель был поднят.
func (s *Sluzhba) PomnitPodnyatPosleObnovleniya() {
	s.mu.Lock()
	s.podnyatPosleObnovleniya = true
	s.mu.Unlock()
}

// StatusS отдаёт статус с РАЗОВОЙ ошибкой вместо запомненной.
//
// Нужна там, где сбой надо показать человеку, но не запоминать: postavit
// переписал бы s.sost и затёр чужую ошибку, а Status() положить разовую ошибку
// нечем. Замок берётся и отпускается внутри Status, поэтому вызывать её из-под
// s.mu так же нельзя.
func (s *Sluzhba) StatusS(oshib *protokol.Oshibka) protokol.StatusOtvet {
	st := s.Status()
	if oshib != nil {
		st.Oshib = oshibkaDlyaOkna("состояние", oshib.Kod, oshib.Tekst)
	}
	return st
}

// tegDlyaZamera отвечает, ЧТО именно мы спрашиваем у ядра про здоровье пути.
//
// Группу, а не кандидата. Тег кандидата берётся один раз при подъёме и живёт
// всё подключение, а выбор внутри группы может смениться без нас. Замер по
// группе уходит через её текущий выбор: замерено на живом ядре, задержка
// приходит числом и падает в 503, когда выбранный кандидат мёртв.
//
// Функция, а не константа на месте вызова: так подмена аргумента видна мутацией.
func tegDlyaZamera() string { return genkonfig.TegSelector }

// Connect это подъём по команде человека. Автоматические подъёмы (старт,
// восстановление, смена сети) идут через connect с avto=true.
func (s *Sluzhba) Connect(ctx context.Context) error {
	return s.connect(ctx, nil, false)
}

// connect поднимает туннель. avto отличает подъём, который служба затеяла сама:
// ему перед TUN положена проверка сервера (Б1 аудита 1.8.0).
func (s *Sluzhba) connect(ctx context.Context, expected *int, avto bool) (itogErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	// «Подключить», пока идёт восстановление, отдаётся циклу, а не идёт рядом с
	// ним (жалоба 15.10.2026). Рядом оно поднимало туннель под отсчётом «снова
	// через 5 мин», а цикл, проснувшись посреди такого подъёма, видел чужое
	// podnimaetsya и прекращался: неудачная попытка руками оставляла машину
	// без восстановления вовсе.
	if expected == nil && !avto && s.vosstanovlenieIdyot() {
		if s.popytatsyaSeychas("нажато «Подключить»") {
			log.Printf("подключение во время паузы восстановления: пробую сразу")
		}
		return nil
	}
	s.mu.Lock()
	sost := s.sost
	s.mu.Unlock()

	// Отказ это НЕ занятое состояние, это провалившаяся попытка, и повторную
	// обязаны принять. Замерено на стенде 01.09.2026: импорт профиля с неверным
	// паролем, connect, честный отказ «серверов нет», затем импорт с верным
	// паролем и connect, который ответил «туннель уже в состоянии otkaz».
	// Серверы к тому моменту были на месте. Лечилось только перезапуском службы,
	// то есть человек, у которого один раз не поднялось, оставался с кнопкой,
	// которая больше ничего не делает.
	//
	// Уборка перед повторной попыткой это ВТОРОЙ барьер, а не необходимость, и
	// названо это так по факту проверки: каждая ветка отказа, наступающая после
	// запуска ядер, уже зовёт Disconnect сама, а ранние ветки отказывают до
	// того, как появляется что убирать. Мутационный прогон это подтвердил, сняв
	// вызов без единого красного теста. Барьер оставлен потому, что веток отказа
	// шесть, растут они по одной, и достаточно седьмой забыть про уборку, чтобы
	// повторный подъём пошёл поверх живого сторожа и второй пары ядер.
	// ne-neset здесь наравне с otkaz, и по той же причине: это провалившаяся
	// попытка, а не занятое состояние. Без него автовосстановление невозможно
	// физически: наблюдатель ставит именно ne-neset.
	if expected == nil && (sost == protokol.SostOtkaz || sost == protokol.SostNeNeset) {
		s.Disconnect()
	}

	// Проверка и переход ПОД ОДНИМ замком. Прежде состояние читалось, замок
	// отпускался, и только потом начиналась работа: человек жал «подключить»
	// ровно тогда, когда срабатывало автовосстановление, обе горутины видели
	// vyklyuchen и обе шли поднимать. Два ядра, два TUN-адаптера, два набора
	// правил брандмауэра, и вдобавок второй вызывающий затирал s.otmena
	// первого, после чего первый наблюдатель становился неубиваемым.
	//
	// Само по себе это старый код, который никто не правил. Дефектным он стал в
	// тот день, когда у Connect появился второй вызывающий.
	s.mu.Lock()
	// A command names the state the human wants, and podnyat/podnimaetsya are
	if s.ostanovlena || ctx.Err() != nil || (expected != nil && s.pokolenieP != *expected) {
		s.mu.Unlock()
		return errPodyomOtmenyon
	}
	// that state already reached. Answering with an error there is not honesty,
	// it is a red banner over a working tunnel: the dispatcher had no code to
	// put on such an error, so it fell through to all-servers-down and blamed
	// servers that were carrying traffic at that very moment.
	//
	// The other busy states stay errors on purpose. otkaz and ne-neset are
	// handled above by Disconnect and fall through to a real second attempt,
	// and vosstanavlivaetsya means another goroutine is already lifting: two
	// lifts at once are two cores, two adapters and two rule sets.
	if s.sost == protokol.SostPodnyat || s.sost == protokol.SostPodnimaetsya {
		s.mu.Unlock()
		return nil
	}
	if s.sost != protokol.SostVyklyuchen {
		zanyato := s.sost
		s.mu.Unlock()
		return fmt.Errorf("VPN уже в состоянии %s", zanyato)
	}
	s.sost = protokol.SostPodnimaetsya
	s.cancelSpeedLocked("началось подключение к VPN, запусти замер после него")
	// Поколение берётся ЗДЕСЬ, под тем же замком, что и переход в podnimaetsya.
	// Это точка, с которой отключение нас уже видит: раньше её отключать было
	// нечего, позже осталось бы окно, в котором disconnect уже прошёл, а мы про
	// это ещё не знаем.
	moyo := s.pokolenieP
	s.mu.Unlock()

	// Сервер берётся из списка ДО всего остального: без него подниматься некуда,
	// и узнать это лучше до того, как выставлено состояние «поднимается».
	n, err := s.nabor()
	if err != nil {
		log.Printf("подключение: набор серверов не прочитан: %v", err)
		s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
			Kod: protokol.KodSecretsUnreadable, Tekst: tekstIz("подключение", err)})
		return err
	}
	srv, err := n.VybrannyyServer()
	if err != nil {
		s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
			Kod: protokol.KodSelectedServerGone, Tekst: "серверов нет: добавь сервер ссылкой или подпиской"})
		return err
	}

	nach := time.Now()
	// podnimaetsya is not decoration: the UI draws a different screen for it, and
	// jumping straight to podnyat makes the app look frozen for four seconds.
	s.postavit(protokol.SostPodnimaetsya, nil)
	// Помехи заново при каждом подъёме: перехватчик, запущенный минуту назад,
	// фоновая проверка назвала бы только через полминуты.
	s.obnovitPomehi()

	// Контекст операции подъёма (A7). Одним defer, а не записью на каждой
	// ветке: веток выхода ниже десять, и одиннадцатая однажды забыла бы
	// записать себя. Отсюда, а не с начала функции: выше нет ни сервера, ни
	// поколения, то есть нечего называть операцией.
	//
	// Идентификатор сервера идёт в журнал отпечатком: две строки одного
	// сервера сходятся между собой и не сходятся ни с чем снаружи.
	shagPodyoma := "podgotovka"
	defer func() {
		itog, shag := "ok", ""
		switch {
		case itogErr == nil:
		case errors.Is(itogErr, errPodyomOtmenyon), errors.Is(itogErr, context.Canceled):
			// Отмена это НЕ отказ. Записать её кодом отказа значит потом
			// считать нажатие «отключить» аварией подъёма.
			itog, shag = "otmena", string(sboi.Otmena)
		default:
			// Тот же ответ, что уйдёт человеку: код берётся у
			// kodPodklyucheniya, а не собирается здесь заново. Иначе журнал и
			// экран однажды назвали бы один отказ по-разному.
			itog, shag = kodPodklyucheniya(itogErr, s.vnutriOshib()), shagPodyoma
		}
		_ = s.zhurnalDiag.SobytieOperatsii(diagnostika.Operatsiya{
			Vid:        "podklyuchenie",
			Pokolenie:  uint64(moyo),
			Dlitelnost: time.Since(nach),
			Itog:       itog,
			Shag:       shag,
			Uzel:       diagnostika.Obezlichit(srv.Id),
		})
	}()

	if err := s.proveritServerDoTun(ctx, avto, moyo); err != nil {
		shagPodyoma = "proverka-servera"
		return err
	}

	s.mu.Lock()
	// Только ВЫБОР, и только из n.Vybran как есть. srv это VybrannyyServer(), а
	// он при пустом Vybran отдаёт Servery[0]: для подъёма это верно, человеку с
	// одним сервером не надо сначала его «выбирать», но записать это же в поле
	// выбора значило бы назвать сервер, которого человек не выбирал.
	//
	// Несущего здесь НЕ пишем. Ш7-1 писала его отсюда, до всякого опроса ядра, и
	// при отказе опроса поле показывало бы Servery[0] с той же уверенностью, что
	// и при живом ядре. Несущего называет ядро, ниже, после удачной пробы.
	s.vybranId = n.Vybran
	s.mu.Unlock()

	// Отказ записи файла НЕ роняет подъём: файл нужен восстановлению после
	// нештатной смерти службы, а туннель человеку нужен сейчас. Но молчать о
	// нём нельзя, иначе восстановление однажды не найдёт данных и объяснить это
	// будет нечем.
	if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
		f.Sostoyanie, f.ConnectNach = protokol.SostPodnimaetsya, &nach
	}); err != nil {
		log.Printf("файл состояния не записан на начале подъёма: %v", err)
	}

	// Две попытки, а не одна. 03.09.2026 дважды за день connect отвечал
	// all-servers-down через 15 с, а через минуты та же запись поднималась за
	// секунду; причина не поймана. Человек видит это как «не работает».
	// Вторая попытка это перезапуск ядра, не ещё одна проба того же: если
	// беда в ядре или в первом рукопожатии, свежий старт её лечит, а если в
	// сети, вторые 15 с ничего не стоят. Причина последней пробы теперь
	// пишется и в журнал, и в текст отказа: прежде она терялась.
	var poslednyaya error
	for popytka := 1; popytka <= popytokPodyoma; popytka++ {
		// Состояние службы в начале попытки прежде не перечитывалось НИ РАЗУ, и
		// это половина дефекта: вторая попытка строила себе новый контекст от
		// Background и поднимала туннель, которого уже никто не просил.
		if s.podyomOtmenyon(moyo) {
			return errPodyomOtmenyon
		}
		vnutr, otmena := context.WithCancel(context.Background())
		s.mu.Lock()
		s.otmena = otmena
		s.mu.Unlock()

		// Слежение за чужим прокси живёт столько же, сколько подключение: вне его
		// предупреждать не о чем, туннеля всё равно нет.
		fon.Zapustit("слежении за прокси", func() { s.slediZaProksi(vnutr) })

		// Туннель поднимается ДО замера, и это обратный порядок к прежнему.
		//
		// Прежний порядок (проба, потом TUN) защищал от состояния ne-neset: туннель,
		// заворачивающий трафик в ядро, которое никуда не дозвонилось. Защита была
		// настоящей, но куплена ценой, которую заметили только на чужих серверах:
		// пробовать ДО подъёма умеет лишь тот путь, что идёт мимо туннеля, а он
		// меряет не то, чем трафик пойдёт. Теперь от ne-neset защищает не порядок, а
		// откат: замер идёт по живому туннелю, и его провал туннель опускает.
		shagPodyoma = "tunnel"
		a, err := s.podnyatTunnel(vnutr)
		if err != nil {
			// Отмена это НЕ отказ подъёма, и спрашиваем мы об этом ДО того, как
			// выдумывать диагноз. Полоса Б убрала выдуманный диагноз из ОТВЕТА
			// команды connect и до состояния не дошла: ответ был честный, а
			// экран после него всё равно показывал «TUN-адаптер не появился».
			// Человек нажал «отключить» и получил за своё же действие красный
			// экран. Замерено раундом 3 04.09.2026, судья отмены.
			//
			// Уборка та же, что и у отказа: подъём адаптера не удался, значит
			// за собой могло остаться недоделанное, и Disconnect убирает это
			// целиком. Разница ровно в одном: состояние не трогаем, его уже
			// поставил тот, кто отменял.
			if s.podyomOtmenyon(moyo) {
				log.Printf("подключение отменено, пока поднимался VPN: %v", err)
				s.Disconnect()
				return errPodyomOtmenyon
			}
			log.Printf("подключение: туннель не поднялся: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: kodPodyomaTunnelya(err), Tekst: tekstIz("подключение", err)})
			return err
		}
		// Отключение, успевшее ПОКА поднимался туннель. Проверка стоит до
		// записи файла и до glushitIPv6 намеренно: opustitYadro отключавшего уже
		// прошёл, его vernutIPv6 отработал, и правило, заведённое следующей
		// строкой, осталось бы висеть без туннеля до следующего подъёма.
		// Замерено тестом: без этой проверки правил оставалось два.
		//
		// Убираем за собой САМИ: адаптер и ядро наши, отключавший про них не
		// знал. Состояние при этом не трогаем, его уже поставил он.
		if s.podyomOtmenyon(moyo) {
			s.otmenit()
			s.opustitYadro()
			return errPodyomOtmenyon
		}
		// Доступ к clash_api и адаптер пишутся СРАЗУ. Именно они нужны службе,
		// поднявшейся после нештатной смерти: без порта не у кого спросить, жив ли
		// туннель, без индекса адаптера нечего убирать за собой.
		s.mu.Lock()
		portKlash, sekretKlash := s.portClash, s.sekretClash
		s.mu.Unlock()
		if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
			f.Sostoyanie = protokol.SostPodnyat
			f.PortClash, f.SekretClash = portKlash, sekretKlash
			f.AdapterTun, f.IndeksTun = a.Imya, a.Indeks
		}); err != nil {
			log.Printf("доступ к ядру и адаптер не записаны в файл состояния: %v", err)
		}
		s.mu.Lock()
		s.tun = a
		s.mu.Unlock()

		// Порядок из задачи 2.5: адаптер есть -> правило IPv6 -> (дальше
		// разрешающие правила и политика). Раньше адаптера ставить нечего, позже уже
		// поздно: промежуток между поднятым туннелем и глушением v6 это окно утечки.
		shagPodyoma = "brandmauer"
		if err := s.glushitIPv6(); err != nil {
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodFirewallFailed, Tekst: tekstIz("подключение", err)})
			return err
		}

		adres, sekret := s.dostupKKlash()
		// Ядро отвечает не мгновенно, и спросить его сразу после старта значит
		// измерить пустоту.
		shagPodyoma = "yadro"
		if err := s.zhdatKlash(vnutr, adres, sekret); err != nil {
			// Не KodTunCreateFailed: адаптер к этому моменту создан, молчит
			// ядро. Прежний код отправлял человека чинить исправный туннель.
			log.Printf("подключение: ядро не отвечает по управляющему порту: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodYadroNeOtvechaet, Tekst: tekstIz("подключение", err)})
			return err
		}

		// Выбор НАВЯЗЫВАЕТСЯ ядру, а не оставляется на поле default в конфиге.
		//
		// Ядро запоминает выбранный исходящий: store_selected у sing-box
		// «включено по умолчанию, когда задан cache_file.enabled», а он у нас
		// задан. При старте ядро восстанавливает ПРОШЛЫЙ выбор и перебивает им
		// default, то есть выбор человека доезжает до конфига и проигрывает
		// кэшу. Найдено живым прогоном 04.09.2026: на чистом наборе из четырёх
		// серверов дважды подряд просили один, а несущим выходил кэшированный,
		// при верном vybran_id на экране.
		//
		// В авто перебивается ровно так же и хуже: кэш держит КОНКРЕТНЫЙ
		// сервер, и автоматический режим молча выродился бы в один навсегда
		// выбранный.
		shagPodyoma = "vybor"
		if err := s.navyazatVybor(vnutr, adres, sekret); err != nil {
			// Отказ закрытый: поднятый туннель через ЧУЖОЙ сервер хуже
			// неподнятого. Человек выбрал страну, и отдать ему другую молча
			// значит соврать ровно в том, ради чего программу ставят.
			log.Printf("подключение: выбор не доехал до ядра: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodPereklyuchenieNeDoehalo, Tekst: tekstIz("подключение", err)})
			return err
		}

		teg := tegDlyaZamera()
		srok := time.Now().Add(zhdatPodyoma)
		// Номер пробы и её длительность пишутся КАЖДЫЙ раз. Две пробы за 15 с
		// и полсотни это разные диагнозы: первое значит, что ядро думает над
		// каждой, второе, что контекст уже отменён и они возвращаются мгновенно.
		// Различить их можно только по числу строк, поэтому строк не жалеем.
		shagPodyoma = "proba"
		nomer := 0
		for time.Now().Before(srok) {
			// Отмена замечается на БЛИЖАЙШЕЙ итерации, а не через пятнадцать
			// секунд. Выход не ставит состояние: его поставил disconnect, и
			// написать поверх него свой отказ значило бы вернуть на экран
			// «поднимается» после того, как человеку ответили «выключено».
			if s.podyomOtmenyon(moyo) {
				s.otmenit()
				s.opustitYadro()
				return errPodyomOtmenyon
			}
			nomer++
			nachProby := time.Now()
			// Жалобы ядра с начала подъёма: ядро стартовало в нём же, и
			// чужих среди них нет.
			d, err := s.zameritYadro(vnutr, adres, sekret, teg, nach)
			if err != nil {
				log.Printf("подключение: проба %d за %v (с начала подъёма %v): %v",
					nomer, time.Since(nachProby).Round(time.Millisecond),
					time.Since(nach).Round(time.Millisecond), err)
			}
			if err == nil {
				// Наблюдатель регистрируется ПЕРВЫМ делом на удачном пути, до
				// единой записи о подъёме.
				//
				// Порядок не косметический. Регистрация это ещё и место, где
				// проверяется, не отключили ли нас и не остановлена ли служба, а
				// объявить podnyat и только потом это заметить значит оставить
				// экран с поднятым туннелем поверх чужого vyklyuchen.
				//
				// С этой строки и до запуска горутины ниже НЕТ ни одного
				// возврата, и это условие договора zavestiNablyudatelya: уйти
				// отсюда, не дойдя до defer s.nabl.Done(), значит подвесить
				// Disconnect навсегда.
				//
				// Обратная сторона тоже работает на нас: раз регистрация прошла,
				// отключающийся ждёт нас на s.nabl.Wait(), и весь остаток этого
				// блока случается ДО его уборки, а не наперегонки с ней.
				if !s.zavestiNablyudatelya(moyo) {
					s.otmenit()
					s.opustitYadro()
					return errPodyomOtmenyon
				}
				pervaya := time.Now()
				// Именно podnyat, а не podnimaetsya: следующей строкой это и
				// объявляется наружу, а файл до сих пор оставался с промежуточным
				// состоянием навсегда.
				if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
					f.Sostoyanie, f.ConnectNach = protokol.SostPodnyat, &nach
					f.ProbaPervaya = &pervaya
					// Оба поля пишутся ЗДЕСЬ, а не в Status(): Status только читает,
					// файл наполняет pravitSost. Без этой пары поля появились бы в
					// структуре без единого писателя, то есть повтор находки 52.
					// srv и n локальные, замка им не нужно: замыкание и так
					// выполняется под s.mu (pravitSost, :520).
					f.NesushchiyId, f.VybranId = srv.Id, n.Vybran
				}); err != nil {
					log.Printf("файл состояния не записан на удачном подъёме: %v", err)
				}
				// Спрашиваем ЯДРО, кто несёт, а не переписываем своё намерение.
				// Отказ опроса НЕ роняет подъём: туннель уже несёт, а неизвестный
				// несущий это пустое поле и строка в журнале. Ронять рабочее
				// подключение из-за неотвеченного вопроса значило бы поменять его на
				// отказ ради надписи на экране.
				//
				// IdIzTega отдаёт ДВА значения, и второе не декорация: тег, не
				// начинающийся с префикса кандидата, это имя группы, а не сервер.
				// «Ещё не выбрала» это НЕ отказ, и сдаваться на нём нельзя: поле
				// осталось бы пустым до следующего опроса наблюдателя, то есть на
				// тридцать секунд. Замеренная цена ожидания около шести секунд.
				dosprosNuzhen := false
				if nteg, err := s.nesyot(vnutr, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err != nil {
					if errors.Is(err, yadra.ErrNeVybrala) {
						dosprosNuzhen = true
					} else {
						log.Printf("несущий не спрошен у ядра: %v", err)
					}
				} else if id, ok := genkonfig.IdIzTega(nteg); ok {
					s.zapomnitNesushchego(id)
				} else {
					log.Printf("ядро назвало несущим %q, а это не тег кандидата", nteg)
				}
				log.Printf("исходящий %s отвечает за %v", teg, d)
				s.postavit(protokol.SostPodnyat, nil)
				// Правила пересобираются на КАЖДЫЙ подъём при включённом режиме.
				// Прежде это делалось только на изменение списка серверов, поэтому
				// после автовосстановления или ручного disconnect/connect правило
				// продолжало разрешать localip СТАРОГО адаптера. Работало лишь
				// потому, что sing-tun обычно выдаёт тот же 172.19.0.1.
				//
				// Отказ пересборки НЕ роняет подъём: туннель уже несёт, а запертая
				// машина с отставшим правилом чинится следующим подъёмом. Но
				// молчать нельзя.
				//
				// Кроме одного случая: подъём, затеянный САМИМ включением режима.
				// Там запирание делает SetKillSwitch, и оно обязано доложить об
				// отказе человеку, а не спрятать его в журнал. Два вызова подряд
				// трогали бы брандмауэр дважды одним и тем же.
				s.mu.Lock()
				podRezhim := s.podRezhim
				s.mu.Unlock()
				s.mu.Lock()
				s.pravilaOtstali = nil
				s.mu.Unlock()
				if err := s.peresobratEsliNado(podRezhim); err != nil {
					log.Printf("правила брандмауэра отстали от подъёма: %v", err)
					// Наблюдатель повторяет пересборку (L3 аудита 1.8.0):
					// «чинится следующим подъёмом» значило держать машину
					// запертой при статусе «подключено» до случайного
					// переподъёма.
					oshibka := &protokol.Oshibka{Kod: protokol.KodFirewallFailed, Tekst: tekstIz("подключение", err)}
					s.mu.Lock()
					s.pravilaOtstali = oshibka
					s.mu.Unlock()
					s.postavit(protokol.SostPodnyat, oshibka)
				}
				// Наблюдатель живёт столько же, сколько подключение. До него подъём
				// проверялся ровно один раз, и туннель, умерший через минуту, до
				// перезагрузки продолжал числиться поднятым.
				//
				// Доспрос ОТДЕЛЬНОЙ горутиной, а не первой фазой наблюдателя:
				// иначе ядро, не назвавшее выбор никогда, отодвинуло бы первую
				// проверку живости туннеля на срок доспроса. Регистрация своя, и
				// отказ в ней означает лишь, что нас уже отключают.
				if dosprosNuzhen && s.zavestiNablyudatelya(moyo) {
					fon.Zapustit("доспросе несущего", func() {
						defer s.nabl.Done()
						s.dosprositNesushchego(vnutr, adres, sekret)
					})
				}
				fon.Zapustit("наблюдателе туннеля", func() {
					defer s.nabl.Done()
					sdalsya := fon.SPovtorom(vnutr, "наблюдатель туннеля", s.pauzaPaniki, func() {
						s.nablyudat(vnutr, adres, sekret, teg)
					})
					if sdalsya {
						s.bezNablyudatelya(vnutr, moyo)
					}
				})
				return nil
			}
			poslednyaya = err
			select {
			case <-ctx.Done():
				log.Printf("подключение: команда отменена на пробе %d: %v", nomer, ctx.Err())
				s.Disconnect()
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		// Строка пишется на каждую попытку, а не только на неудачную
		// промежуточную: прежде исход ПОСЛЕДНЕЙ попытки не попадал в журнал
		// никогда, а именно она и объявляет отказ.
		log.Printf("подключение: попытка подъёма %d из %d, ядро не понесло за %s, проб %d, последняя: %v",
			popytka, popytokPodyoma, zhdatPodyoma, nomer, poslednyaya)
		if popytka < popytokPodyoma {
			s.otmenit()
			s.opustitYadro()
		}
	}

	// В ручном режиме проба шла через один сервер, и безымянный 503 можно
	// доспросить у ядра замера (utochnitPrichinu). Туннель ещё стоит: опустив
	// его раньше, служба показала бы «выключено» между попыткой и отказом.
	if s.rezhimPodyoma(n) == protokol.RezhimRuchnoy {
		poslednyaya = s.utochnitPrichinu(ctx, poslednyaya, srv.Id)
		// Человек мог отключить VPN, пока шёл доспрос: его «выключено» главнее.
		if s.podyomOtmenyon(moyo) {
			s.otmenit()
			s.opustitYadro()
			return errPodyomOtmenyon
		}
	}

	// Up but carrying nothing is its own state, and it is the dangerous one.
	//
	// Вердикт пишется в журнал СО ВСЕЙ обстановкой. Разовый отказ 03.09.2026
	// разбирать было нечем именно потому, что здесь молчали: состояние живёт
	// в памяти до следующей команды, а в журнале команд пишется только код.
	//
	// Строка пишется ПОСЛЕ опускания: сверка часов ниже ходит в сеть напрямую,
	// а поднятый туннель, который не несёт, съел бы и её.
	s.Disconnect()
	// Человеческой фразой, а не сводкой замера: «не понёс трафик (2 попытки
	// по 15s)» читалось как строка журнала, случайно попавшая на экран
	// (владелец, 23.09.2026). Число попыток и срок остались - они говорят,
	// что программа не сдалась после первой неудачи, - но словами.
	// Причина понятной частью: полный текст пробы записан в журнал ниже.
	tekst := fmt.Sprintf("VPN включился, но данные через него не пошли. Попыток: %d, по %d секунд каждая. Причина: %s",
		popytokPodyoma, int(zhdatPodyoma.Seconds()), sboi.DlyaCheloveka(poslednyaya))
	// Антивирус с проверкой трафика и запущенный чужой VPN-клиент называются
	// здесь, а не на главном экране (решение владельца 21.10.2026): у
	// большинства они работают рядом с VPN без всяких бед.
	if p := s.podskazkaPomeh(); p != "" {
		tekst += ". " + p
	}
	// Код по ПРИЧИНЕ последней пробы, а не all-servers-down насмерть: ответ
	// команды выбирает kodPodklyucheniya, а состояние живёт на Главной до
	// следующей команды, и разойтись им нельзя. Сбитые часы главнее причины
	// пробы: сервер с ними отказывает при любом ключе (1.10.0).
	var kod string
	kod, tekst, poslednyaya = s.sUchetomChasov(kodNepodnyavshegosya(poslednyaya), tekst, poslednyaya)
	// Код тот же, что уйдёт в состояние. До 1.10.0 строка печатала
	// all-servers-down всегда, и журнал спорил с экраном.
	log.Printf("подключение: отказ %s, сервер %s (%s, %s), попыток %d по %s, последняя проба: %v",
		kod, srv.Id, srv.Imya, srv.Transport, popytokPodyoma, zhdatPodyoma, poslednyaya)
	s.postavit(protokol.SostNeNeset, &protokol.Oshibka{Kod: kod, Tekst: tekst})
	return fmt.Errorf("VPN не понёс трафик за %d попытки по %s: %w", popytokPodyoma, zhdatPodyoma, poslednyaya)
}

// kodPodyomaTunnelya различает пропавший драйвер и не созданный адаптер.
//
// Оба приходят одним таймаутом ожидания, и до этой ветки человек с невставшим
// драйвером читал «не удалось создать адаптер», то есть шёл повторять там, где
// повторять нечего. Признак ставит ядро, разбор в otkazOzhidaniyaAdaptera.
//
// Молчащий резолвер тоже не адаптер (жалоба 02.10.2026): подъём падает на
// сборе адресов серверов, до создания адаптера, и чинить надо DNS.
//
// Пропавшее или подменённое ядро тоже не адаптер (1.10.0): чаще всего его
// уносит антивирус, и повтор тут не поможет никогда.
func kodPodyomaTunnelya(err error) string {
	switch {
	case errors.Is(err, hranenie.ErrFaylPropal), errors.Is(err, hranenie.ErrFaylIzmenen):
		return protokol.KodYadroPropalo
	case errors.Is(err, yadra.ErrDrayverNeVstal):
		return protokol.KodWintunMissing
	case errors.Is(err, ErrRezolverMolchit):
		return protokol.KodDnsResolveFailed
	}
	return protokol.KodTunCreateFailed
}

// kodNepodnyavshegosya называет причину последней пробы.
//
// Отдельной функцией, а не веткой на месте: тот же вопрос задаёт
// kodPodklyucheniya для ответа команды, и разные ответы на один вопрос это
// экран, который спорит сам с собой.
func kodNepodnyavshegosya(poslednyaya error) string {
	switch {
	case estOtkazZashchity(poslednyaya):
		return protokol.KodZashchitaServera
	case errors.Is(poslednyaya, yadra.ErrServerOtvergKlyuchi):
		return protokol.KodServerAuthFailed
	case errors.Is(poslednyaya, yadra.ErrProbaNeUspela):
		return protokol.KodProbaNeUspela
	}
	return protokol.KodAllServersDown
}

// Disconnect опускает туннель по решению ЧЕЛОВЕКА или по отказу подъёма.
//
// Три шага раздельно, и порядок важен: отменить, ДОЖДАТЬСЯ наблюдателя, убрать.
// Без ожидания уборка идёт наперегонки с горутиной, которая ещё спрашивает
// clash_api и ещё имеет право объявить аварию.
//
// Сам наблюдатель зовёт otmenit и opustit ПОРОЗНЬ, а не Disconnect: ждать
// здесь он стал бы ждать самого себя.
func (s *Sluzhba) Disconnect() {
	// otmenitPodyom, а не otmenit: идущий подъём обязан выйти, а не досидеть до
	// своего срока и поднять туннель поверх отключения.
	s.otmenitPodyom()
	s.nabl.Wait()
	s.opustit()
}

// Zavershit гасит ВСЁ фоновое и опускает туннель. Зовётся на остановке службы
// и в уборке тестов.
//
// Без него горутина восстановления переживала процесс: тест заканчивался,
// следующий писал общие переменные, а она их читала. Детектор гонок называл это
// гонкой в тестах, хотя болезнь была в продукте.
// pravitSost меняет ЧАСТЬ файла состояния, оставляя остальное на месте.
//
// Полные снимки на каждой записи и были дефектом: четыре независимых
// вызывающих, каждый знает про свои три поля и обнуляет чужие семь.
// peresobratEsliNado пропускает пересборку, когда подъём затеян самим
// включением режима: там запирание делает SetKillSwitch и оно обязано доложить
// об отказе человеку.
func (s *Sluzhba) peresobratEsliNado(podRezhim bool) error {
	if podRezhim {
		return nil
	}
	return s.PeresobratRazresheniya()
}

// pravitSost ОТДАЁТ отказ записи, а не глотает его.
//
// Прежде тут стояло _ = s.zapisat(f), и один вызывающий на этом врал человеку:
// setConnectOnStart отвечала успехом при непрошедшей записи, экран рисовал
// галочку, а после перезагрузки туннель не поднимался, потому что флага в файле
// не было. Остальные вызывающие сидят на пути подъёма, им ронять подключение
// из-за файла не за что, и они пишут причину в журнал.
func (s *Sluzhba) pravitSost(pravka func(*sostoyanie.SostoyanieFayla)) error {
	s.muFayl.Lock()
	defer s.muFayl.Unlock()
	s.mu.Lock()
	pravka(&s.snimok)
	f := s.snimok
	s.mu.Unlock()
	return s.zapisat(f)
}

// sbrositSost стирает файл целиком. Здесь полная перезапись ВЕРНА: туннеля
// больше нет, и оставленные порт с адаптером это данные о ядре, которого не
// существует. Устаревший индекс адаптера опаснее пустого.
func (s *Sluzhba) sbrositSost() error {
	s.muFayl.Lock()
	defer s.muFayl.Unlock()
	s.mu.Lock()
	// Отметка обновления подписки к туннелю отношения не имеет и переживает
	// сброс. У неё появился второй читатель, расписание, и без этой строки
	// каждое отключение стоило бы лишнего похода за подпиской.
	podpiska := s.snimok.PodpiskaObnovlena
	priStarte := s.snimok.PodklyuchatPriStarte
	zhurnal := s.snimok.Zhurnal
	// Полоса канала это настройка машины, а не след поднятого туннеля: канал от
	// отключения не меняется. Без этой пары каждое отключение молча снимало бы
	// объявление, и hysteria2 возвращался бы на BBR без единого слова.
	polosaVverh, polosaVniz := s.snimok.PolosaVverh, s.snimok.PolosaVniz
	s.snimok = sostoyanie.SostoyanieFayla{
		Sostoyanie:           protokol.SostVyklyuchen,
		PodpiskaObnovlena:    podpiska,
		PodklyuchatPriStarte: priStarte,
		KillSwitch:           s.killSwitch,
		Zhurnal:              zhurnal,
		PolosaVverh:          polosaVverh,
		PolosaVniz:           polosaVniz,
	}
	f := s.snimok
	s.mu.Unlock()
	return s.zapisat(f)
}

func (s *Sluzhba) Zavershit() {
	// Флаг ПЕРВЫМ, до всякого Wait: с этой строки ни одна горутина больше не
	// регистрируется, и оба ожидания ниже досчитываются до нуля навсегда, а не
	// до следующего опоздавшего Add.
	//
	// Идущий подъём отменяется ДО ожидания фоновых горутин (Г3 аудита 1.8.0):
	// поколение меняется и его контекст гасится. Прежде менялось одно
	// поколение, а подъём из восстановления досиживал пробу до своего срока,
	// и остановка службы ждала его до 35 секунд.
	s.mu.Lock()
	s.ostanovlena = true
	s.mu.Unlock()
	s.otmenitPodyom()
	s.fonOtmena()
	s.fon.Wait()
	s.Disconnect()
	s.muTracker.Lock()
	s.mu.Lock()
	tracker := s.processTracker
	s.processTracker = nil
	s.mu.Unlock()
	s.muTracker.Unlock()
	if tracker != nil {
		if err := tracker.Close(); err != nil {
			log.Printf("остановка наблюдения за приложениями: %v", err)
		}
	}
	s.osvoboditSet()
}

func (s *Sluzhba) otmenit() {
	s.mu.Lock()
	otmena := s.otmena
	s.otmena = nil
	s.mu.Unlock()
	if otmena != nil {
		otmena()
	}
}

// otmenitPodyom это otmenit плюс смена поколения: отмена, которую цикл проб
// обязан ЗАМЕТИТЬ.
//
// Отдельный метод, а не строка внутри otmenit, потому что вызывающих у otmenit
// два рода. Disconnect отменяет чужой подъём и хочет, чтобы тот вышел. Сам
// подъём между своими двумя попытками отменяет СЕБЯ и обязан продолжить: смена
// поколения там означала бы, что вторая попытка не начнётся никогда.
//
// Счётчик и отмена контекста меняются ПОД ОДНИМ замком. Порознь они дали бы
// окно, в котором подъём видит прежнее поколение и живой контекст, то есть
// ровно то, чего эта пара и должна не допускать.
func (s *Sluzhba) otmenitPodyom() {
	s.mu.Lock()
	s.cancelSpeedLocked("VPN отключается, замер остановлен")
	s.pokolenieP++
	otmena := s.otmena
	s.otmena = nil
	s.mu.Unlock()
	if otmena != nil {
		otmena()
	}
}

// zavestiNablyudatelya регистрирует наблюдателя подъёма и отвечает, можно ли
// его пускать.
//
// Проверка и s.nabl.Add(1) ПОД ОДНИМ замком с отменой и остановкой. Порознь
// они и давали гонку: подъём видел живую службу, отпускал замок, и Add уходил
// уже наперегонки с Wait.
//
// Вызывающий, получивший true, ОБЯЗАН дойти до горутины с defer s.nabl.Done():
// возврат между регистрацией и запуском подвесит Disconnect навсегда.
func (s *Sluzhba) zavestiNablyudatelya(moyo int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ostanovlena || s.pokolenieP != moyo {
		return false
	}
	s.nabl.Add(1)
	return true
}

// zavestiFonovuyu регистрирует фоновую горутину (восстановление) по тем же
// правилам. Отдельная группа, потому что Zavershit ждёт её раньше туннеля.
func (s *Sluzhba) zavestiFonovuyu() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ostanovlena {
		return false
	}
	s.fon.Add(1)
	return true
}

// podyomOtmenyon отвечает, сменилось ли поколение с начала этого подъёма.
func (s *Sluzhba) podyomOtmenyon(moyo int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pokolenieP != moyo
}

// errPodyomOtmenyon оборачивает context.Canceled НАМЕРЕННО: отмена подъёма это
// отмена, и граница протокола уже умеет отвечать на неё состоянием, а не
// выдуманным кодом отказа (см. kodPodklyucheniya в dispetcher.go).
var errPodyomOtmenyon = fmt.Errorf("подъём отменён отключением: %w", context.Canceled)

// Otklyuchit это отключение по решению ЧЕЛОВЕКА, и оно единственное гасит
// восстановление.
//
// Отделено от Disconnect намеренно. Disconnect зовут и внутренние ветки отказа
// самого Connect, в том числе тот Connect, который запустило восстановление.
// Гасить восстановление оттуда значило бы прекращать его первой же неудачной
// попыткой, а вся его суть в повторах: замерено на стенде, одна попытка при
// ещё не поднявшейся сети проваливается всегда.
func (s *Sluzhba) Otklyuchit() {
	s.mu.Lock()
	otmenaV := s.otmenaVosst
	s.otmenaVosst = nil
	// Показ восстановления снимается БЕЗ события: Disconnect ниже сам объявит
	// vyklyuchen, а событие отсюда показало бы на миг otkaz, в котором цикл
	// стоял на паузе.
	s.vosstPokaz, s.oshibVosst, s.sledPopytka = false, nil, nil
	s.mu.Unlock()
	if otmenaV != nil {
		otmenaV()
	}
	// Человек отключился сам: следующее подключение начинается с его выбора, а
	// не с подмены, заведённой без него.
	if s.snyatZapasnoy() {
		log.Printf("подмена выбранного сервера снята: VPN отключён")
	}
	s.Disconnect()
	s.osvoboditSet()
}

// opustitYadro убирает то, что относится к ЯДРУ, не трогая состояние службы:
// между двумя попытками подъёма служба остаётся в podnimaetsya.
func (s *Sluzhba) opustitYadro() {
	// Правило IPv6 снимается вместе с туннелем, и его отказ не должен мешать
	// остальному опусканию: осиротевшее правило чинится проверкой при старте
	// службы, а незавершённое опускание чинить нечем.
	if s.vernutIPv6 != nil {
		err := s.vernutIPv6()
		s.mu.Lock()
		s.oshibkaIPv6 = err
		s.mu.Unlock()
		if err != nil {
			log.Printf("правило IPv6 не снято: %v", err)
		}
	}
	// Секрет живёт ровно столько, сколько поднят туннель: он новый на каждый
	// старт ядра, и оставленный после опускания это секрет от ядра, которого
	// больше нет. Забыть адаптер обязательно по той же причине. Устаревший индекс опаснее пустого: по нему
	// исключат из правил брандмауэра адаптер, которого уже нет, а то и чужой,
	// занявший освободившийся индекс.
	s.mu.Lock()
	s.tun = set.Adapter{}
	s.portClash, s.sekretClash = 0, ""
	s.serveryYadra = nil
	s.otpechatkiYadra = nil
	s.pravilaKonfiga, s.trafikKonfiga = "", ""
	s.reklamaVKonfige = false
	// Резолвер конфига забывается вместе с ним: без ядра сверять нечего, а
	// оставленный адрес заставил бы наблюдателя следующего подъёма считать
	// сменой сети то, что сменилось, пока туннель лежал.
	s.rezolverKonfiga = netip.Addr{}
	s.mu.Unlock()
}

func (s *Sluzhba) opustit() {
	s.opustitYadro()
	s.mu.Lock()
	// Несущего больше нет: ядро опущено. Оставить имя значило бы показывать
	// экран с сервером, который ничего не несёт. Выбор снимается вместе с ним:
	// он поле ЖИВОГО подключения, а не настройка, живущая между сессиями. До
	// Ш7-1 serverId не снимался нигде, и служба отдавала vyklyuchen вместе с
	// именем сервера.
	s.vybranId, s.nesushchiyId, s.nesushchiyImya = "", "", ""
	s.mu.Unlock()
	s.postavit(protokol.SostVyklyuchen, nil)
	s.mu.Lock()
	oshibkaIPv6 := s.oshibkaIPv6
	s.mu.Unlock()
	if oshibkaIPv6 != nil {
		s.postavit(protokol.SostVyklyuchen, &protokol.Oshibka{Kod: protokol.KodFirewallFailed, Tekst: "Не удалось восстановить IPv6: " + tekstIz("IPv6", oshibkaIPv6)})
	}
	// Файл переживает процесс, поэтому написанное в нём после опускания это то,
	// во что поверит следующий старт. Оставить там podnyat с индексом мёртвого
	// адаптера значит подсунуть следующему запуску ровно то, что абзацем выше
	// названо опаснее пустого: освободившийся индекс переиспользуется системой.
	if err := s.sbrositSost(); err != nil {
		log.Printf("файл состояния не переписан при опускании: %v", err)
	}
}

// zapomnitKlash запоминает доступ к clash_api поднявшегося ядра.
func (s *Sluzhba) zapomnitKlash(port int, sekret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.portClash, s.sekretClash = port, sekret
}

// dostupKKlash отдаёт адрес и секрет для пробы. Пустой адрес означает, что
// ядро не поднято: спрашивать нечего и некого.
// zapomnitNesushchego пишет ответ ядра под замком.
//
// Сеттер, а не голое присваивание: вызывающие живут ВНЕ s.mu (подъём после
// пробы и наблюдатель), и каждый брал бы замок сам. В Ш7-1 присваивание стояло
// внутри уже взятого замка, поэтому сеттера там не было: sync.Mutex
// нереентрантный.
func (s *Sluzhba) zapomnitNesushchego(id string) {
	// Тот же несущий значит «ничего не изменилось», и платить за это чтением
	// набора нельзя: опрос теперь идёт каждые две секунды, а набор лежит на
	// диске зашифрованным.
	s.mu.Lock()
	tot := s.nesushchiyId == id
	s.mu.Unlock()
	if tot {
		return
	}
	// Имя ищется ДО замка: набор читается с диска и расшифровывается.
	imya := ""
	s.mu.Lock()
	for _, srv := range s.serveryYadra {
		if srv.Id == id {
			imya = srv.Imya
			break
		}
	}
	s.mu.Unlock()
	if id != "" && imya == "" {
		if n, err := s.nabor(); err == nil {
			for _, srv := range n.Servery {
				if srv.Id == id {
					imya = srv.Imya
					break
				}
			}
		}
	}
	s.mu.Lock()
	smenilsya := s.nesushchiyId != id
	if smenilsya {
		s.cancelSpeedLocked("сервер изменился, запусти замер заново")
	}
	s.nesushchiyId, s.nesushchiyImya = id, imya
	s.mu.Unlock()
	if !smenilsya || id == "" {
		return
	}
	// Смена несущего меняет и адрес выхода (§5: при смене сервера).
	fon.Zapustit("обновлении адреса выхода", s.obnovitAdresVyhoda)
	// И это событие: в авто ядро переключает само, без команды человека, и
	// до сих пор экран узнавал при следующем опросе, а трей не узнавал никогда
	// (план «шесть удобств» §4). Тот же несущий событием не является.
	s.izvestit("state", s.Status())
}

func (s *Sluzhba) dostupKKlash() (adres, sekret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.portClash == 0 {
		return "", ""
	}
	// Адрес петлевой и совпадает с тем, что генератор кладёт в
	// external_controller. Спрашивать конфиг обратно с диска было бы вторым
	// источником правды о том, что мы сами же и записали.
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.portClash)), s.sekretClash
}

// dosprositNesushchego добивается ответа у группы, которая на подъёме ещё не
// назвала выбор.
//
// Группа avto это urltest, и до конца первой пробы задержки её now пуст. Это
// фаза, а не поломка, поэтому единственный вопрос при подъёме её не ловит:
// замерено 06.09.2026, вопрос уходит примерно на 1.2 с, ответ появляется на
// 5.6 с. Без доспроса поле несущего оставалось пустым до следующего опроса
// наблюдателя, и приёмка ловила это как провал продукта.
func (s *Sluzhba) dosprositNesushchego(ctx context.Context, adres, sekret string) {
	srok := time.Now().Add(s.srokDosprosa)
	for time.Now().Before(srok) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.shagDosprosa):
		}
		nteg, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto)
		if errors.Is(err, yadra.ErrNeVybrala) {
			continue
		}
		if err != nil {
			log.Printf("несущий не спрошен у ядра: %v", err)
			return
		}
		if id, ok := genkonfig.IdIzTega(nteg); ok {
			s.zapomnitNesushchego(id)
		} else {
			log.Printf("ядро назвало несущим %q, а это не тег кандидата", nteg)
		}
		return
	}
	// Молчание тут тоже ответ, и его надо назвать: пустое поле несущего при
	// живом туннеле иначе выглядит как наша забывчивость.
	log.Printf("группа не назвала выбор за %v: поле несущего осталось пустым", s.srokDosprosa)
}

// nablyudat спрашивает живой туннель, несёт ли он ещё трафик.
//
// Смысл ровно в том состоянии, ради которого в проекте заведено отдельное имя:
// поднятый туннель, не несущий ничего, со стороны сокета неотличим от рабочего.
// Заметить его может только тот, кто спрашивает регулярно.
// dognatPravila повторяет пересборку правил, упавшую на подъёме (L3 аудита
// 1.8.0). Удалась: снимает ту ошибку, что поставил подъём, и только её.
// Раз в период пробы, а не на каждом тике: пока фильтрация отказывает,
// частые повторы ничего не дадут.
func (s *Sluzhba) dognatPravila() {
	s.mu.Lock()
	oshibka := s.pravilaOtstali
	s.mu.Unlock()
	if oshibka == nil {
		return
	}
	if err := s.PeresobratRazresheniya(); err != nil {
		log.Printf("правила брандмауэра всё ещё отстают от подъёма: %v", err)
		return
	}
	log.Printf("правила брандмауэра догнали подъём")
	s.mu.Lock()
	s.pravilaOtstali = nil
	snyat := s.oshib == oshibka && s.sost == protokol.SostPodnyat
	if snyat {
		s.oshib = nil
	}
	s.mu.Unlock()
	if snyat {
		s.izvestit("state", s.Status())
	}
}

func (s *Sluzhba) nablyudat(ctx context.Context, adres, sekret, teg string) {
	podryad := 0
	// Тик идёт по МЕНЬШЕМУ из двух периодов, а проба живости отсчитывается
	// отдельно. Так частый вопрос о несущем не превращается в частый выход
	// наружу, а тест, укорачивающий period, по-прежнему получает частую пробу.
	shag := s.periodNesushchego
	if s.period < shag {
		shag = s.period
	}
	sledZamer := time.Now().Add(s.period)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(shag):
		case <-s.vneocherednaya:
			sledZamer = time.Time{}
		}
		// В авто ядро меняет выбор САМО, без единой нашей команды. Без
		// периодического опроса экран показывал бы выбор, сделанный при подъёме,
		// и расходился бы с действительностью тем сильнее, чем дольше сессия.
		if nteg, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err == nil {
			if id, ok := genkonfig.IdIzTega(nteg); ok {
				s.zapomnitNesushchego(id)
			}
		}
		// Смена сети проверяется на КАЖДОМ тике, а не раз в период пробы:
		// вопрос идёт к таблице адаптеров, наружу не ходит и стоит копейки, а
		// человек с поломанным разрешением имён ждать минуту не должен.
		if s.smotretSet(ctx) {
			return
		}
		if time.Now().Before(sledZamer) {
			continue
		}
		sledZamer = time.Now().Add(s.period)
		s.dognatPravila()
		if _, err := s.zameritYadro(ctx, adres, sekret, teg, s.zhalobyNePrezhe()); err != nil {
			// Отменённый контекст это НЕ авария, а нас самих опускают. Считать
			// его провалом значило бы поставить ne-neset поверх выключенного и
			// запустить восстановление, которое подключит туннель обратно.
			// Кнопка «отключить» переставала работать, и заметил это только
			// тест, доводящий замер до отказа ровно в момент Disconnect.
			if ctx.Err() != nil {
				return
			}
			// Нехватка ресурсов МАШИНЫ это не смерть туннеля.
			//
			// Замер ходит в clash_api на 127.0.0.1. Кончились эфемерные порты,
			// и локальный запрос не уходит никуда: туннель при этом жив и
			// несёт. Считать это аварией значит перезапустить ядро и порвать
			// человеку все соединения из-за того, что на машине торрент.
			//
			// Замерено по журналу живой машины 10.09.2026: 197 таких отказов и
			// 25 разрывов следом, раз в 98 минут. Утечка, съедавшая порты,
			// починена в этой же волне, но спусковой крючок от неё не зависит.
			//
			// Счётчик подряд при этом СБРАСЫВАЕТСЯ: иначе один отказ по портам
			// и один настоящий сложились бы в аварию, которой не было.
			if yadra.ResursyMashiny(err) {
				podryad = 0
				log.Printf("замер исходящего %s не прошёл из-за нехватки ресурсов машины,"+
					" туннель не трогаем: %v", teg, err)
				continue
			}
			podryad++
			log.Printf("замер исходящего %s не прошёл (%d подряд): %v", teg, podryad, err)
			if podryad < s.provalov {
				continue
			}
			// Недоступность gstatic не доказывает отказ туннеля. Перед разрывом
			// проверяем другую цель через тот же исходящий, не меняя сервер.
			_, rezervErr := s.zameritRezerv(ctx, adres, sekret, teg)
			if ctx.Err() != nil {
				return
			}
			if rezervErr == nil || yadra.ResursyMashiny(rezervErr) {
				podryad = 0
				if rezervErr == nil {
					log.Printf("резервная цель исходящего %s доступна, туннель сохраняем", teg)
				} else {
					log.Printf("резервная проба: нехватка ресурсов машины, туннель сохраняем: %v", rezervErr)
				}
				continue
			}
			log.Printf("резервная цель исходящего %s тоже недоступна: %v", teg, rezervErr)
			// В «авто» группа держится за мёртвый сервер до своей следующей
			// пробы, и рвать туннель раньше неё значит бросить живые серверы
			// рядом (С6 аудита 1.6.1). Перемер выбирает заново сразу, и рвём,
			// только если трафик не пошёл и после него.
			if s.pomoglaGruppa(ctx, adres, sekret, teg) {
				podryad = 0
				continue
			}
			// В ручном режиме перемерять нечего, и прежде здесь туннель
			// рвался, оставляя живые серверы рядом (жалоба 17.10.2026).
			if s.pereytiNaZapasnoy(ctx, adres, sekret, teg) {
				podryad = 0
				continue
			}
			if ctx.Err() != nil {
				return
			}
			// Причина выясняется ДО остановки, пока адаптер и конфиг ещё на
			// месте: после s.opustit() спрашивать уже нечего и не у чего (A3).
			prichina := s.prichinaRazryva(ctx)
			// Своей причины у сети нет, а ядро назвало, почему не
			// устанавливается защищённое соединение: её и говорим (02.10.2026
			// так выглядел сертификат сервера, истёкший посреди работы).
			if prichina == "" && estOtkazZashchity(err) {
				prichina = err.Error()
			}
			// Показ восстановления ДО опускания: иначе между ним и запуском
			// цикла наружу ушли бы vyklyuchen и ne-neset.
			s.pokazVosst(true)
			s.otmenit()
			s.opustit()
			tekst := "VPN перестал нести трафик"
			if prichina != "" {
				tekst += ": " + prichina
				log.Printf("причина разрыва: %s", prichina)
			}
			s.postavit(protokol.SostNeNeset, &protokol.Oshibka{
				Kod: protokol.KodTunnelNeNeset, Tekst: tekst})
			// Возвращаемся САМИ, и только после аварии. Осознанное отключение
			// человеком не переподключает никогда, иначе кнопка «отключить»
			// перестаёт работать: это и есть граница между обычным режимом и
			// постоянным, ради которой у чужих клиентов заведено два kill switch.
			s.zapustitVosstanovlenie()
			return
		}
		// Счётчик сбрасывается только успехом: два провала ПОДРЯД, а не два
		// провала за всё время работы.
		podryad = 0
		// Туннель несёт, и только на несущем есть смысл мерить, вернулся ли
		// подменённый выбранный сервер.
		s.proveritVozvrat(ctx, adres, sekret, teg)
	}
}

// pomoglaGruppa перемеряет группу авто и пробует исходящий ещё раз. Вне авто
// отвечает false сразу: перемерять нечего.
func (s *Sluzhba) pomoglaGruppa(ctx context.Context, adres, sekret, teg string) bool {
	nach := time.Now()
	bylo, err := s.peremeritAvto(ctx, adres, sekret)
	if err != nil {
		log.Printf("группа авто не перемерена: %v", err)
	}
	if !bylo || ctx.Err() != nil {
		return false
	}
	// Перемер мог выбрать другой сервер: жалобы до него про прежний.
	s.otmetitSmenuVybora(nach)
	if _, err := s.zameritYadro(ctx, adres, sekret, teg, nach); err != nil {
		log.Printf("и после перемера группы исходящий %s не несёт: %v", teg, err)
		return false
	}
	log.Printf("группа авто выбрала другой сервер, туннель сохраняем")
	return true
}

// bezNablyudatelya опускает туннель, наблюдатель которого упал паникой дважды
// (Г6 аудита 1.8.0), и отдаёт его восстановлению.
//
// Туннель без наблюдателя это туннель, чью смерть никто не заметит: экран
// говорит «подключено», а трафик стоит. Новый подъём заводит нового
// наблюдателя, и если паника была от состояния, его сбросит переподъём.
// Зовётся из горутины наблюдателя, до её s.nabl.Done(), поэтому Disconnect
// здесь нельзя: он ждал бы саму эту горутину.
func (s *Sluzhba) bezNablyudatelya(ctx context.Context, moyo int) {
	if ctx.Err() != nil || s.podyomOtmenyon(moyo) {
		return
	}
	log.Printf("наблюдатель туннеля сдался после двух паник: опускаю туннель и ухожу в восстановление")
	s.pokazVosst(true)
	s.otmenit()
	s.opustit()
	s.postavit(protokol.SostNeNeset, &protokol.Oshibka{
		Kod: protokol.KodTunnelNeNeset, Tekst: "Проверка VPN сломалась, переподключаюсь"})
	s.zapustitVosstanovlenie()
}

// zapustitVosstanovlenie заводит фоновое восстановление туннеля.
//
// Это единственная дверь, через которую туннель, опущенный не человеком,
// возвращается сам (А1 аудита 1.8.0): наблюдатель, старт, переподъём. Цикл
// один: пока он крутится, повторный вызов ничего не делает.
func (s *Sluzhba) zapustitVosstanovlenie() {
	vosstCtx, otmenaV := context.WithCancel(s.fonCtx)
	s.mu.Lock()
	if s.vosstIdyot {
		s.mu.Unlock()
		otmenaV()
		return
	}
	s.vosstIdyot = true
	s.otmenaVosst = otmenaV
	s.mu.Unlock()
	s.pokazVosst(true)
	// Пробуждение, случившееся до аварии, к этому циклу отношения не имеет.
	select {
	case <-s.tolchokVosst:
	default:
	}
	zakonchit := func() {
		s.mu.Lock()
		s.vosstIdyot = false
		s.mu.Unlock()
		otmenaV()
		s.pokazVosst(false)
	}
	// Не зарегистрировались значит службу останавливают, и возвращаться
	// некуда: восстановление подняло бы туннель уже после того, как его
	// опустили насовсем.
	if !s.zavestiFonovuyu() {
		zakonchit()
		return
	}
	fon.Zapustit("восстановлении", func() {
		defer s.fon.Done()
		defer zakonchit()
		fon.SPovtorom(vosstCtx, "восстановление", pauzaPoslePaniki, func() {
			s.vosstanavlivat(vosstCtx)
		})
	})
}

// vosstanovlenieIdyot отвечает, крутится ли цикл восстановления.
func (s *Sluzhba) vosstanovlenieIdyot() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vosstIdyot
}

// popytatsyaSeychas обрывает паузу восстановления по действию человека
// (жалоба 15.10.2026). После десятка неудач пауза растёт до пяти минут, и
// выбранный в неё другой сервер ждал её конца, а окно всё это время
// показывало «снова через 5 мин».
//
// Срок следующей попытки снимается здесь же, а не циклом: ответ команде
// собирается сразу после толчка, и с прежним сроком он показал бы ту же паузу.
//
// Отвечает, стояло ли восстановление на паузе. Попытке, которая идёт прямо
// сейчас, толкать нечего.
func (s *Sluzhba) popytatsyaSeychas(pochemu string) bool {
	s.mu.Lock()
	vPauze := s.vosstIdyot && s.sledPopytka != nil
	if vPauze {
		s.sledPopytka = nil
	}
	s.mu.Unlock()
	if !vPauze {
		return false
	}
	select {
	case s.tolchokVosst <- pochemu:
	default:
	}
	return true
}

// setZhiva спрашивает внешний адрес напрямую, тем же путём, что проверка
// выхода при выключенном VPN. Между попытками туннеля нет, и свой запрос
// служба в запертом режиме шлёт по правилу процессов.
func (s *Sluzhba) setZhiva(ctx context.Context) bool {
	ctx, otmena := context.WithTimeout(ctx, 10*time.Second)
	defer otmena()
	s.mu.Lock()
	endpoint := s.adresProverki
	s.mu.Unlock()
	if _, err := s.sprositVyhod(ctx, endpoint, 0); err != nil {
		log.Printf("сеть напрямую не отвечает, пауза восстановления остаётся короткой: %v", err)
		return false
	}
	return true
}

// zhdatVPauze ждёт отступ восстановления и отвечает, идти ли на попытку.
//
// Пауза обрывается раньше срока двумя событиями, после которых прежний отказ
// уже ничего не говорит о следующей попытке: машина проснулась или сменилась
// сеть (M10 аудита 1.8.0). Без этого после десятка неудач человек ждал до пяти
// минут, хотя сервер вернулся или он сам пересел на другую сеть. Смена сети
// видна по местному резолверу: другая сеть это другой шлюз и другой DNS, а
// короткие паузы её не ждут, им хватает своего срока.
func (s *Sluzhba) zhdatVPauze(ctx context.Context, otstup time.Duration) bool {
	taymer := time.NewTimer(otstup)
	defer taymer.Stop()

	var tik <-chan time.Time
	var bylo netip.Addr
	var errBylo error
	if s.periodSetiVPauze > 0 && otstup > s.periodSetiVPauze {
		t := time.NewTicker(s.periodSetiVPauze)
		defer t.Stop()
		tik = t.C
		bylo, errBylo = s.mestnyyRezolver()
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case <-taymer.C:
			return true
		case pochemu := <-s.tolchokVosst:
			log.Printf("пауза восстановления прервана: %s", pochemu)
			return true
		case <-tik:
			stalo, err := s.mestnyyRezolver()
			if err != nil || !stalo.IsValid() || (errBylo == nil && stalo == bylo) {
				continue
			}
			log.Printf("пауза восстановления прервана: сменилась сеть, резолвер %s", stalo)
			return true
		}
	}
}

// vosstanavlivat поднимает туннель обратно после АВАРИИ.
//
// Своим контекстом, а не контекстом подключения: тот уже отменён Disconnect'ом,
// и наследовать от него значило бы отменить восстановление в момент рождения.
func (s *Sluzhba) vosstanavlivat(ctx context.Context) {
	for popytka := 0; ; popytka++ {
		otstup := s.otstupy[min(popytka, len(s.otstupy)-1)]
		// Цикл идёт на новый круг только после неудачи, так что номер попытки
		// и есть число неудач подряд.
		if popytka >= neudachDoRedkih && s.setZhiva(ctx) {
			otstup = s.redkiyOtstup
			log.Printf("сеть жива, а туннель не встаёт %d раз подряд: следующая попытка через %s", popytka, otstup)
		}
		// Срок следующей попытки виден снаружи всю паузу, и событием сразу:
		// окно ведёт по нему отсчёт, а не угадывает.
		// Только пока показ включён и цикл не отменён: после «Отключить» это
		// событие показало бы на миг otkaz, в котором цикл стоял.
		sled := time.Now().Add(otstup)
		s.mu.Lock()
		obyavit := s.vosstPokaz && ctx.Err() == nil
		if obyavit {
			s.sledPopytka = &sled
		}
		s.mu.Unlock()
		if obyavit {
			s.izvestit("state", s.Status())
		}
		// Сон прерываемый. Голый Sleep означал, что отключение человеком
		// доходит до цикла только после текущего отступа, а он на последних
		// попытках достигает минуты: кнопка нажата, а туннель ещё поднимется.
		ok := s.zhdatVPauze(ctx, otstup)
		s.mu.Lock()
		s.sledPopytka = nil
		s.mu.Unlock()
		if !ok {
			return
		}

		// Человек мог отключиться руками, пока мы ждали. Его решение старше
		// нашего: проверяем состояние ПЕРЕД каждой попыткой.
		//
		// Продолжаем и из otkaz тоже, и это не мелочь: после провалившейся
		// попытки состояние именно otkaz, а не ne-neset. Замерено 01.09.2026 на
		// стенде: погасили адаптер, служба упала за 40.3 с, сделала одну
		// попытку, та честно провалилась (сети ещё нет), и цикл остановился на
		// собственной проверке. Сеть вернули, туннель не вернулся.
		s.mu.Lock()
		sost := s.sost
		s.mu.Unlock()
		if sost != protokol.SostNeNeset && sost != protokol.SostOtkaz {
			log.Printf("восстановление прекращено: состояние %s", sost)
			return
		}

		// Под замком без туннеля подъёму нужны имена серверов, а правило DNS
		// могло остаться под резолвером прежней сети (L4 аудита 1.8.0).
		if err := s.obnovitDnsPodZamkom(); err != nil {
			log.Printf("правило DNS под замком не переписано: %v", err)
		}
		log.Printf("восстановление, попытка %d", popytka+1)
		// Контекст восстановления, а не фоновый: «Отключить» и остановка
		// службы отменяют и попытку, идущую прямо сейчас, а не только следующую.
		if err := s.connect(ctx, nil, true); err != nil {
			log.Printf("восстановление не удалось: %v", err)
			continue
		}
		log.Printf("туннель восстановлен с попытки %d", popytka+1)
		return
	}
}
