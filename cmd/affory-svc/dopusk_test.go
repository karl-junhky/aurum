package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// resheniyeOPravah is the DECISION, not a mirror of the code: the test compares
// it against komandyDlyaAdmina, so a silent change on either side fails here.
// Решение владельца 01.10.2026: администратор остался только у installUpdate,
// доводы в комментарии к komandyDlyaAdmina.
//
// Имена НЕ переписываются руками: их даёт vetkiDispetchera(t) разбором
// диспетчера. Прежний granicaDopuska был вторым экземпляром imenaKomand(), и
// сходились они ровно потому, что врали одинаково: setAutostart и
// setConnectOnStart отсутствовали в обоих.
var resheniyeOPravah = map[string]bool{
	"hello": false, "status": false, "subscribeStats": false,
	"setJournal": false, "setDiagnostics": false, "clearJournal": false,
	"checkUpdate": false, "getServerHealth": false,
	"checkExitIp": false, "checkLeaks": false,
	// measureDelays спрашивает у ядра и у сети то, что и так видно на экране:
	// жив ли узел и сколько до него. Ни машины, ни секретов он не трогает, а
	// админом здесь становится то, чей радиус поражения вся машина.
	"measureDelays": false,
	// measureBandwidth качает через НАШ же прокси на мишень, которую назвал сам
	// человек. Ни машины, ни секретов не трогает; трафик тратит его собственный
	// и по его же команде.
	"measureBandwidth": false,
	"startSpeedTest":   false, "speedTestStatus": false, "cancelSpeedTest": false,
	"connect": false, "disconnect": false, "setServer": false,
	"setRouteMode": false, "listServers": false, "listRules": false,
	// Область автовыбора это тот же класс, что и выбор сервера: собственный
	// набор на собственной машине. Худшее, что даёт чужая программа с этим
	// доступом, это сузить автомат до одного узла, тогда как выбрать этот узел
	// напрямую она могла и прежде командой setServer.
	"setAutoMember": false,
	"setAutostart":  false, "setConnectOnStart": false,
	// Полоса канала это число про домашний интернет, а не про права на машине:
	// UAC на её правку означал бы запрос прав ради поля ввода. Худшее, что даёт
	// чужая программа с этим доступом, это испорченный замер собственной
	// скорости, тогда как setKillSwitch выше меняет маршрут всего трафика.
	"setBandwidth": false,
	// Разовый снимок локального ядра не меняет маршруты и не включает журнал.
	"listConnections":    false,
	"inspectApplication": false,
	// Проверка слоёв сети (A3) НИЧЕГО не меняет: спрашивает адаптеры, свой же
	// clash_api и шлёт свои запросы наружу. Требовать под неё права значило бы
	// требовать их ровно тогда, когда человек уже в беде и ищет причину.
	"checkNetwork": false,
	// Выгрузка диагностики (О6 аудита 1.6.1) отдаёт журналы собственной машины
	// с вычищенными ключами и адресами подписок. Под администратором её не
	// получил бы именно тот, ради кого она сделана: без прав журналов не видно.
	"exportDiagnostics": false,
	"addServer":         false, "removeServer": false,
	// Пачка это те же addServer подряд, решение то же.
	"addServers": false,
	// Ключи VPN не тот секрет, ради которого спрашивать UAC (01.10.2026), а
	// файл выгрузки пишет само окно под правами человека.
	"exportServers":   false,
	"setSubscription": false, "refreshSubscription": false, "setRules": false,
	// Подписки это тот же класс, что и setSubscription: адрес панели, а не
	// маршрут всей машины. UAC ради переключения между своими панелями
	// приучал бы жать «да» на запрос прав каждый день.
	"listSubscriptions": false, "addSubscription": false,
	"removeSubscription": false, "setActiveSubscription": false,
	"setKillSwitch": false, "exportProfile": false, "importProfile": false,
	// Качает только последний выпуск с GitHub и сверяет его с суммой выпуска.
	"downloadUpdate": false,
	// Ставит архив с диска, сверяя с .sha256 рядом: подложить можно оба файла,
	// и без подтверждения любая программа ставила бы свою сборку с правами
	// SYSTEM.
	"installUpdate": true,
	// Свой VPN (§3.6 плана «Свой VPN в клиенте», 01.11.2026) без админа, как
	// и весь собственный набор. Цена одна: любой процесс сеанса может
	// заставить службу SYSTEM открыть SSH на любой адрес мимо туннеля и
	// защиты (канал INTERACTIVE, internal/kanal/server.go), то есть узнать
	// настоящий IP машины.
	"svoyVpsOsmotret": false, "svoyVpsUstanovit": false,
	"svoyVpsHod": false, "svoyVpsOtmenit": false, "svoyVpsSpisok": false,
	"svoyVpsSnesti": false, "svoyVpsVzyat": false, "svoyVpsZabyt": false, "svoyVpsProverit": false,
	"svoyVpsOtpustit": false, "svoyVpsParol": false, "svoyVpsPrinyatKlyuchHosta": false,
}

// Команда без прав администратора обязана получить отказ, а не выполниться.
//
// Угроза не «другой человек за машиной», а «другая программа от тебя». Windows
// даёт админу раздельный токен, поэтому браузер, игра и скрипт из npm
// postinstall сидят в INTERACTIVE, куда канал пускает намеренно. До этой
// проверки такой процесс мог без единого запроса прав добавить свой сервер и
// увести весь трафик машины.
func TestKomandyDlyaAdminaOtvergayutsyaBezPrav(t *testing.T) {
	// Контекст БЕЗ допуска: отсутствие сведений трактуется как «не админ».
	ctx := context.Background()
	// Имена из разбора, флаг из решения: список admin-команд нигде не написан
	// руками третий раз.
	for _, imya := range vetkiDispetchera(t) {
		if !resheniyeOPravah[imya] {
			continue
		}
		t.Run(imya, func(t *testing.T) {
			s := podstavnaya(t, nil)
			o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: imya, Telo: json.RawMessage(`{}`)})
			if o.Oshib == nil {
				t.Fatalf("команда %s выполнена без прав администратора", imya)
			}
			if o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
				t.Fatalf("команда %s отказала кодом %s, ожидался %s",
					imya, o.Oshib.Kod, protokol.KodTrebuetsyaAdmin)
			}
		})
	}
}

// Обратная сторона: ежедневные команды НЕ должны требовать прав. Иначе UAC
// вылезает на каждое подключение, и программа превращается в то, что отключают.
func TestEzhednevnyeKomandyRabotayutBezPrav(t *testing.T) {
	for _, imya := range []string{"hello", "status", "listServers"} {
		t.Run(imya, func(t *testing.T) {
			s := podstavnaya(t, nil)
			o := s.Obrabotat(context.Background(), protokol.Kadr{Id: 1, Imya: imya})
			if o.Oshib != nil && o.Oshib.Kod == protokol.KodTrebuetsyaAdmin {
				t.Fatalf("команда %s потребовала админа: UAC на каждый запуск", imya)
			}
		})
	}
}

// Ворота против забытой команды: новая ветка диспетчера обязана попасть в
// решение о правах, а не унаследовать его по умолчанию.
func TestKazhdayaKomandaImeetResheniyeOPravah(t *testing.T) {
	for _, v := range vetkiDispetchera(t) {
		nuzhen, prinyato := resheniyeOPravah[v]
		if !prinyato {
			t.Errorf("для ветки %q решение о правах не принято", v)
			continue
		}
		if komandyDlyaAdmina[v] != nuzhen {
			t.Errorf("ветка %q: решено админ=%v, в коде админ=%v", v, nuzhen, komandyDlyaAdmina[v])
		}
	}
	for imya := range komandyDlyaAdmina {
		if _, prinyato := resheniyeOPravah[imya]; !prinyato {
			t.Errorf("komandyDlyaAdmina держит %q, а решения о нём нет", imya)
		}
	}
}

// Допуск администратора проходит насквозь: иначе предыдущие тесты были бы
// зелёными и на службе, которая отказывает всем подряд.
func TestSAdminskimDopuskomKomandaProhodit(t *testing.T) {
	s := podstavnaya(t, nil)
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{Admin: true})
	o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: "installUpdate", Telo: json.RawMessage(`{}`)})
	if o.Oshib != nil && o.Oshib.Kod == protokol.KodTrebuetsyaAdmin {
		t.Fatal("админу отказано: проверка прав отвергает всех и ничего не доказывает")
	}
}

// Сверка версий обязана быть ДВУСТОРОННЕЙ.
//
// Клиент присылает свою версию в теле hello и проверяет ответ службы. Служба
// свою версию объявляет, а чужую не читала вовсе. Старый интерфейс против новой
// службы получал зелёное приветствие и падал позже, на поле, которого не знает,
// то есть ровно в том месте, где сверка версий должна была его остановить.
func TestHelloOtvergaetChuzhuyuVersiyu(t *testing.T) {
	s := podstavnaya(t, nil)

	o := s.Obrabotat(context.Background(), protokol.Kadr{
		Id: 1, Imya: "hello", Telo: json.RawMessage(`{"protocol":99}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodProtocolMismatch {
		t.Fatalf("чужая версия принята: %+v", o.Oshib)
	}

	// КОНТРОЛЬ: своя версия обязана проходить, иначе проверка отвергает всех и
	// канал не открывается вовсе.
	o = s.Obrabotat(context.Background(), protokol.Kadr{
		Id: 2, Imya: "hello",
		Telo: json.RawMessage(fmt.Sprintf(`{"protocol":%d}`, protokol.Versiya)),
	})
	if o.Oshib != nil {
		t.Fatalf("своя версия отвергнута: %+v", o.Oshib)
	}
}

// Импорт профиля обязан пересобрать правила брандмауэра.
//
// sohranitIPeresobrat объявлена в коде ЕДИНСТВЕННЫМ путём изменения списка
// серверов, и четыре команды идут через неё. Импорт профиля писал секреты
// напрямую, а он меняет список целиком. При включённом режиме «весь трафик»
// импортированные серверы оказывались заблокированы собственным kill-switch:
// человек видит их на экране и не может подключиться ни к одному.
// Импорт заменяет список ЦЕЛИКОМ и пишет блоб мимо sohranitIPeresobrat, то есть
// мимо заслона. Заслон здесь и не годится: он означал бы «нельзя импортировать
// профиль, пока подключён». Импорт меняет всю картину сразу, и нести трафик по
// старой картине не значит ничего, поэтому туннель опускается.
//
// Утверждения ДВА, и первое унаследовано от прежнего теста: правила обязаны
// пересобраться ДО опускания, иначе машина остаётся запертой под старые адреса
// и подключиться после импорта нельзя вовсе.
func TestImportProfilyaPeresobiraetPravilaIOpuskaetTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	peresobrali := false
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { peresobrali = true; return nil }
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()
	// Нужен НАСТОЯЩИЙ подъём, а не выставленное руками состояние: опускание
	// смотрит на адрес clash_api, который выставляет только Connect.
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	telo := []byte(`{"servery":[{"id":"a","host":"203.0.113.4","port":443,"transport":"reality-tcp"}]}`)
	blob, err := hranenie.Eksport("parol-dlinnyy-dostatochno", telo)
	if err != nil {
		t.Fatal(err)
	}
	o := vypolnit(t, s, "importProfile", map[string]string{
		"parol": "parol-dlinnyy-dostatochno", "profil": vBase64(blob)})
	if o.Oshib != nil {
		t.Fatalf("импорт отказал: %+v", o.Oshib)
	}
	if !peresobrali {
		t.Fatal("правила не пересобраны ДО опускания: машина заперта под старые адреса, подключиться нельзя")
	}
	if st := s.Status(); st.Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("состояние %q: ядро продолжает нести трафик через сервер, которого в наборе больше нет", st.Sostoyanie)
	}
}
