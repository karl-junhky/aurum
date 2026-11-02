package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// A service with every seam replaced: no cores, no sockets, no ProgramData.
// What is left is the state machine, which is the part that can lie.
// podstavnaya берёт t не для красоты, а чтобы уборку нельзя было забыть.
//
// Забыли её ровно один раз, в TestKillSwitchSobiraetSpisokIzSistemy, и этого
// хватило: горутина наблюдателя пережила свой тест, следующий тест писал общую
// переменную, а она её читала. Детектор гонок нашёл это первым же прогоном за
// весь проект. Уборка на каждом вызывающем это сорок семь мест, где её можно
// не написать; уборка в конструкторе это ноль таких мест.
func podstavnaya(t *testing.T, zamerOtvet error) *Sluzhba {
	t.Helper()
	s := NovayaSluzhba()
	// Закрытие журнала заводится ДО Zavershit: уборка идёт в обратном порядке,
	// и файл отпускается, когда фоновые писатели уже остановлены.
	zakrytZhurnalDiag(t, s)
	t.Cleanup(s.Zavershit)
	s.dirDannyh = t.TempDir()
	s.dirProgrammy = t.TempDir()
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { return nil }
	s.vyklyuchitVes = func() error { return nil }
	// Настоящий шов читает файл отката живой машины разработчика.
	s.estOtkat = func() bool { return false }
	// Настоящая проверка сервера запускает sing-box.
	s.proveritServer = func(context.Context) error { return nil }
	s.suzitServery = func(set.Adresa) error { return nil }
	s.perezavestiDns = func(netip.Addr) error { return nil }
	s.naboryZhelaemye = func() []genkonfig.NaborPravil { return nil }
	s.skachatNabor = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("network disabled in service fixture")
	}
	s.skachatSpisok = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("network disabled in service fixture")
	}
	s.sobratSpisok = func(context.Context, string, string) error {
		return errors.New("core disabled in service fixture")
	}
	// Настоящий сброс чистит кэш DNS всей машины: тестам это ни к чему.
	s.sbrositKeshDNS = func() error { return nil }
	// Настоящий поиск помех читает процессы и адаптеры машины разработчика, а
	// на ней бывает запущен и Happ, и чужой VPN.
	s.proveritPomehi = func(pomehi.Svoi) ([]pomehi.Nahodka, error) { return nil, nil }
	// Настоящая сверка часов ходит в сеть. Нулевое расхождение: часы верны.
	s.zameritChasy = func(context.Context) (time.Duration, error) { return 0, nil }
	// Настоящая проба резолвера шлёт вопрос в сеть. Список пуст, значит
	// выбирать не из чего и резолвер конфига это mestnyyRezolver.
	s.mestnyeRezolvery = func(...uint32) ([]netip.Addr, error) { return nil, nil }
	s.sprositRezolver = func(context.Context, netip.Addr) error { return nil }
	s.zapisat = func(sostoyanie.SostoyanieFayla) error { return nil }
	// Конструктор уже прочитал НАСТОЯЩИЙ файл настроек машины: 03.09.2026
	// тест «журнал выключен по умолчанию» покраснел на хосте, где журнал
	// включили в окне. Подставная служба читает пустоту и перечитывает.
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) { return sostoyanie.SostoyanieFayla{}, nil }
	s.zagruzitNastroyki()
	// Судьи конфига в фикстуре НЕТ. Настоящий зовёт sing-box.exe и сверяет его
	// с файлом отпечатков, которого во временном каталоге теста не бывает; до
	// A6 это ни на что не влияло, потому что подъём здесь заглушен целиком и до
	// проверки не доходил, а проверка кандидата зовётся уже из перезапуска.
	// Тесты, которым судья нужен, ставят свой сами (см. kandidat_test.go).
	s.proveritKonfig = nil
	// Свой сервер: настоящие швы читают адаптеры машины, ходят по SSH и за
	// программой сервера. Тесты своего сервера ставят свои.
	s.fizicheskiyAdapter = func() (set.Adapter, error) { return set.Adapter{Indeks: 7, Imya: "Ethernet"}, nil }
	s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
		return upravlenie.Otchet{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsUstanovit = func(context.Context, upravlenie.Ustanovka) (upravlenie.Itog, error) {
		return upravlenie.Itog{}, errors.New("ssh disabled in service fixture")
	}
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return nil, errors.New("server program disabled in service fixture")
	}
	s.vpsSnesti = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.SnosItog, error) {
		return upravlenie.SnosItog{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsVzyat = func(context.Context, upravlenie.Vzyatie) (upravlenie.Itog, error) {
		return upravlenie.Itog{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsOtpustit = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.OtpuskItog, error) {
		return upravlenie.OtpuskItog{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsVyklyuchitParol = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		return upravlenie.ParolItog{}, errors.New("ssh disabled in service fixture")
	}
	s.vpsVklyuchitParol = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		return upravlenie.ParolItog{}, errors.New("ssh disabled in service fixture")
	}
	// Настоящий замер пинга поднимает sing-box. Тесты пинга ставят своё ядро.
	s.yadroZamera = func(context.Context) (vremennoeYadro, error) {
		return vremennoeYadro{}, errors.New("core disabled in service fixture")
	}
	s.storozhit = func(ctx context.Context, imya, konfig string, sob func(protokol.Sostoyanie)) error {
		<-ctx.Done()
		return ctx.Err()
	}
	// Замер через clash_api заменил SOCKS-пробу целиком (задача П3). Параметр
	// фикстуры отвечает теперь за него: тесты, которые проверяли «туннель не
	// несёт», проверяют ровно то же самое, но по тому пути, которым идёт трафик.
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		return 42 * time.Millisecond, zamerOtvet
	}
	s.zameritRezerv = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("резервная проба недоступна в фикстуре")
	}
	// Фикстура стоит не в «авто»: перемерять нечего, и сетевой шов не уходит
	// стучаться на порт подставного ядра.
	s.peremeritAvto = func(context.Context, string, string) (bool, error) { return false, nil }
	// Подъём дёргает адрес выхода в фоне: в тестах в сеть не ходим.
	s.sprositVyhod = func(ctx context.Context, endpoint string, port int) (string, error) { return "192.0.2.10", nil }
	s.ipv6Zaglushen = func() (bool, error) { return true, nil }
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error { return nil }
	// Отвечает так же, как ответило бы ядро в ручном режиме: тегом того сервера,
	// который сейчас выбран. Постоянный тег здесь НЕЛЬЗЯ: на нём краснеет
	// TestServerBerotsyaIzSpiskaANeIzKonstanty, который судит несущего на наборе
	// без выбора. Заглушка, отвечающая мимо набора, это фикстура, диктующая
	// продукту ответ.
	//
	// Заглушать ОБЯЗАТЕЛЬНО: фикстура кладёт zapomnitKlash(52715, ...), и
	// незаглушенный сетевой шов уйдёт стучаться на этот порт по-настоящему.
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		n, err := s.nabor()
		if err != nil {
			return "", err
		}
		srv, err := n.VybrannyyServer()
		if err != nil {
			return "", err
		}
		return genkonfig.TegKandidata(srv.Id), nil
	}
	s.postavitVybor = func(context.Context, string, string, string, string) error { return nil }
	s.vyborGruppy = func(context.Context, string, string, string) (string, error) {
		// Отвечает как ядро в ручном режиме: прежний выбор это выбранный сервер.
		n, err := s.nabor()
		if err != nil {
			return "", err
		}
		srv, err := n.VybrannyyServer()
		if err != nil {
			return "", err
		}
		return genkonfig.TegKandidata(srv.Id), nil
	}
	// Волна 2 добавила второй шаг подъёма. Без заглушки тест уходил бы поднимать
	// настоящий sing-box, писать конфиг в живой ProgramData и двадцать секунд
	// ждать адаптер, которого в тестовой машине не будет никогда.
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		// Настоящий подъём запоминает доступ к clash_api до старта ядра; без
		// этого замер спрашивал бы пустой адрес и тест не заметил бы разрыва.
		s.zapomnitKlash(52715, "sekret-stenda")
		// Адрес адаптера обязателен: к нему привязывается правило разрешения.
		return set.Adapter{
			Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
		}, nil
	}
	s.glushitIPv6 = func() error { return nil }
	s.vernutIPv6 = func() error { return nil }
	// Список серверов. До задачи 3.8 сервер был константой, и фикстуре не нужно
	// было о нём знать; теперь служба берёт его из хранилища, и подставлять надо
	// именно набор, а не адрес.
	// Под замком: с 6.3 подъём читает набор из фоновой горутины (адрес
	// несущего для текста проверки), и детектор гонок поймал фикстуру, а не
	// продукт.
	var muNabor sync.Mutex
	naborProby := Nabor{Servery: []protokol.Server{serverProby()}}
	s.nabor = func() (Nabor, error) { muNabor.Lock(); defer muNabor.Unlock(); return naborProby, nil }
	s.sekretyChitat = func() ([]byte, error) { muNabor.Lock(); defer muNabor.Unlock(); return json.Marshal(naborProby) }
	// Запись ЗАМЕЩАЕТ набор, а не сливается с прежним. Unmarshal в живую
	// структуру оставлял на месте всякое поле с omitempty, которое запись
	// очищала: Vybran, Podpiska, Rezhim. Настоящее хранилище так не умеет,
	// naborIzHranilishcha разбирает в свежий var n Nabor. Фикстура тем самым
	// прятала целый класс: «поле снято» проходило зелёным при неснятом поле.
	s.sekretyPisat = func(b []byte) error {
		muNabor.Lock()
		defer muNabor.Unlock()
		var n Nabor
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		naborProby = n
		return nil
	}
	s.sobratAdresa = func() (set.Adresa, error) {
		return adresaIz(netip.MustParseAddr("192.0.2.225")), nil
	}
	// Конфиг второго ядра в тесте не пишем: настоящая запись ходит в
	// C:\ProgramData живой машины.
	// Загрузка подписки. Без заглушки тест уходит в НАСТОЯЩУЮ сеть: первый
	// прогон стоил 25 секунд ожидания и мог бы дать любой ответ, включая
	// зелёный по неверной причине.
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		switch {
		case strings.Contains(adres, "/pusto"):
			return ssylki.Razbor{}, ssylki.ErrPodpiskaPusta
		case strings.Contains(adres, "/nedostupno"):
			return ssylki.Razbor{}, ssylki.ErrPodpiskaNedostupna
		// Шаги, на которых срывается загрузка, ведут человека в разные
		// стороны: не успевший ответ лечится повтором, отказ панели по праву
		// доступа - исправлением ссылки (A7).
		case strings.Contains(adres, "/molchit"):
			return ssylki.Razbor{}, ssylki.OtkazSVidom(sboi.Srok,
				fmt.Errorf("%w: ответа нет", ssylki.ErrPodpiskaNedostupna))
		case strings.Contains(adres, "/chuzhaya"):
			return ssylki.Razbor{}, ssylki.OtkazSVidom(sboi.Dostup,
				fmt.Errorf("%w: код ответа 403", ssylki.ErrPodpiskaNedostupna))
		case strings.Contains(adres, "/zhivaya"):
			return ssylki.Razbor{Servery: []protokol.Server{vtoroyServer()}}, nil
		default:
			return ssylki.Razbor{Uvedomleniya: []ssylki.Uvedomlenie{
				{Stroka: 1, Tekst: "Подписка закончилась"},
			}}, ssylki.ErrPodpiskaIstekla
		}
	}
	return s
}

// serverProby это тот же сервер, что раньше был вбит константой celProby.
func serverProby() protokol.Server {
	return protokol.Server{
		Id: "nl", Imya: "Нидерланды", Transport: "ws",
		Host: "192.0.2.225", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Put: "/ws",
	}
}

func sostoyaniyaIzSobytiy(t *testing.T, c <-chan protokol.Kadr) []protokol.Sostoyanie {
	t.Helper()
	var out []protokol.Sostoyanie
	for {
		select {
		case k, ok := <-c:
			if !ok {
				return out
			}
			if k.Imya != "state" {
				continue
			}
			var st protokol.StatusOtvet
			if err := json.Unmarshal(k.Telo, &st); err != nil {
				t.Fatalf("событие state не разбирается: %v", err)
			}
			out = append(out, st.Sostoyanie)
		default:
			return out
		}
	}
}

func TestConnectMenyaetSostoyanie(t *testing.T) {
	// vyklyuchen -> podnimaetsya -> podnyat, and never straight to podnyat: the
	// UI draws a different screen for each, and skipping the middle one makes
	// the app look frozen for four seconds.
	s := podstavnaya(t, nil)
	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)

	if s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("до connect состояние %s", s.Status().Sostoyanie)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect не удался: %v", err)
	}
	if s.Status().Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("после connect состояние %s", s.Status().Sostoyanie)
	}

	shag := sostoyaniyaIzSobytiy(t, sob)
	if len(shag) < 2 {
		t.Fatalf("событий state пришло %d: %v", len(shag), shag)
	}
	if shag[0] != protokol.SostPodnimaetsya {
		t.Fatalf("первым пришло %s, а должно podnimaetsya: %v", shag[0], shag)
	}
	if shag[len(shag)-1] != protokol.SostPodnyat {
		t.Fatalf("последним пришло %s: %v", shag[len(shag)-1], shag)
	}
	if s.Status().PodnyatS == nil {
		t.Fatal("отметка подъёма не проставлена, мерить порог нечем")
	}
}

func TestConnectNeNesyotEtoNeUspeh(t *testing.T) {
	// The tunnel is up and carries nothing. From the socket it looks identical to
	// success, and reporting it as success is the single worst thing this program
	// could do.
	s := podstavnaya(t, errors.New("туннель молчит"))
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	err := s.Connect(context.Background())
	if err == nil {
		t.Fatal("молчащий туннель признан поднятым")
	}
	if s.Status().Sostoyanie != protokol.SostNeNeset {
		t.Fatalf("состояние %s, ожидалось ne-neset", s.Status().Sostoyanie)
	}
}

func TestDisconnectVozvrashchaetVyklyuchen(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	s.Disconnect()
	if s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("после disconnect состояние %s", s.Status().Sostoyanie)
	}
	if s.Status().PodnyatS != nil {
		t.Fatal("отметка подъёма пережила disconnect")
	}
}

func TestPovtornyyConnectNePodnimaetVtoroyRaz(t *testing.T) {
	// Two cores on one port is the bug that looks like a working tunnel until it
	// does not.
	//
	// Судится ПОДЪЁМ, а не ошибка. С Б1 повторный connect на поднятом туннеле
	// отвечает успехом: команда называет желаемое состояние, а оно достигнуто.
	// Ошибка была плохим сторожем ещё и потому, что она молчала о главном: тест
	// прошёл бы и в тот день, когда второй вызов поднял бы второе ядро и
	// отказал по какой-то своей причине.
	s := podstavnaya(t, nil)
	var podyomov atomic.Int32
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podyomov.Add(1)
		return prezhniy(ctx)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("повторный connect на поднятом туннеле отказал: %v", err)
	}
	if n := podyomov.Load(); n != 1 {
		t.Fatalf("туннель поднимался %d раз: два ядра на одном порту", n)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние после повторного connect %v, ожидали podnyat", got)
	}
}

func TestNeizvestnayaKomandaNePadaet(t *testing.T) {
	// A command nobody wrote must answer, not hang and not panic. The half that
	// panics here is the half holding the firewall rules.
	s := podstavnaya(t, nil)
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 7, Imya: "sdelayHorosho"})
	if o.Id != 7 {
		t.Fatalf("ответ не на тот кадр: %+v", o)
	}
	if o.Oshib == nil {
		t.Fatal("неизвестная команда принята")
	}
	if o.Oshib.Kod != protokol.KodProtocolMismatch {
		t.Fatalf("не тот код: %s", o.Oshib.Kod)
	}
}

func TestHelloOtdayotVersiyu(t *testing.T) {
	s := podstavnaya(t, nil)
	// Тело обязательно с 01.09.2026: сверка версий стала двусторонней, и кадр
	// без версии теперь отказ, а не приветствие по умолчанию.
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "hello",
		Telo: json.RawMessage(fmt.Sprintf(`{"protocol":%d}`, protokol.Versiya))})
	if o.Oshib != nil {
		t.Fatalf("hello отказал: %v", o.Oshib)
	}
	var telo struct {
		Protocol int `json:"protocol"`
	}
	if err := json.Unmarshal(o.Telo, &telo); err != nil {
		t.Fatalf("тело hello не разбирается: %v", err)
	}
	if telo.Protocol != protokol.Versiya {
		t.Fatalf("версия %d вместо %d", telo.Protocol, protokol.Versiya)
	}
}

func TestConnectSNesushchestvuyushchimServeromOtkazyvaet(t *testing.T) {
	// Заглушка снята задачей 3.8, и теперь флаг обязан РАБОТАТЬ. Проверяется
	// самый опасный случай: несуществующий идентификатор. Молча подняться на
	// первом попавшемся сервере значило бы подменить то, что человек выбрал, и
	// узнал бы он об этом по чужой стране в браузере.
	s := podstavnaya(t, nil)
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "connect", Telo: json.RawMessage(`{"server":"takogo-net"}`)})
	if o.Oshib == nil {
		t.Fatal("подключение к несуществующему серверу принято")
	}
	if o.Oshib.Kod != protokol.KodSelectedServerGone {
		t.Fatalf("код %s, ожидался selected-server-gone", o.Oshib.Kod)
	}
	if s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatal("служба ушла поднимать туннель, хотя сервер не найден")
	}
}

func TestDisconnectPerepisyvaetFaylSostoyaniya(t *testing.T) {
	// The file outlives the process, so what it says after Disconnect is what the
	// next start believes. Leaving "podnyat" with the index of an adapter that no
	// longer exists is worse than leaving nothing: a freed interface index gets
	// reused, and the firewall would then exempt somebody else's adapter.
	s := podstavnaya(t, nil)
	var poslednyaya sostoyanie.SostoyanieFayla
	pisali := 0
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error {
		poslednyaya = f
		pisali++
		return nil
	}
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{
			Indeks: 10, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
		}, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	do := pisali
	s.Disconnect()
	if pisali == do {
		t.Fatal("файл состояния после Disconnect не переписан: там остался podnyat")
	}
	if poslednyaya.Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("в файле %q, а туннель опущен", poslednyaya.Sostoyanie)
	}
	if poslednyaya.IndeksTun != 0 || poslednyaya.AdapterTun != "" {
		t.Fatalf("в файле остался адаптер %q (%d): устаревший индекс опаснее пустого",
			poslednyaya.AdapterTun, poslednyaya.IndeksTun)
	}
}

func TestKazhdayaDolgayaKomandaSushchestvuet(t *testing.T) {
	// Опечатка в ключе карты долгих сроков не ломает ничего видимого: команда
	// молча получает обратно пять секунд, и дефект, из-за которого connect
	// отвечал отказом на сработавшем подъёме, возвращается без единого
	// признака.
	//
	// Прежняя проверка сверялась со списком izvestny, написанным руками, и
	// потому наказывала за починку: внесение команды в dolgie роняло тест,
	// пока имя не допишут в третьем месте. Имена теперь берутся разбором
	// диспетчера, и рукописного списка не остаётся вовсе.
	vetki := map[string]bool{}
	for _, v := range vetkiDispetchera(t) {
		vetki[v] = true
	}
	for _, imya := range protokol.DolgieKomandy() {
		if !vetki[imya] {
			t.Errorf("в dolgie объявлена команда %q, а ветки такой нет", imya)
		}
	}
}

func TestPovtornyyConnectPosleOtkazaPrinimaetsya(t *testing.T) {
	// Замерено на стенде 01.09.2026: после неудачного подъёма служба залипала в
	// состоянии otkaz и больше не принимала connect. Человек, у которого один
	// раз не поднялось, добавлял сервер, жал «Подключить» и получал «туннель уже
	// в состоянии otkaz» до перезапуска службы. Кнопка переставала работать
	// навсегда, и причина этого нигде не была видна.
	s := podstavnaya(t, nil)
	// Первый заход валится на пустом списке, это штатный отказ.
	s.nabor = func() (Nabor, error) { return Nabor{}, nil }
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("подъём без серверов прошёл")
	}
	if s.Status().Sostoyanie != protokol.SostOtkaz {
		t.Fatalf("состояние %s, ожидался otkaz", s.Status().Sostoyanie)
	}

	// Серверы появились. Повторная попытка обязана быть принята.
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{serverProby()}}, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("повторный подъём отвергнут: %v", err)
	}
	if s.Status().Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("состояние %s, ожидался podnyat", s.Status().Sostoyanie)
	}
	// Ошибка прошлой попытки обязана уйти: статус с чужой ошибкой поверх
	// поднятого туннеля это тот же обман, только наоборот.
	if s.Status().Oshib != nil {
		t.Fatalf("ошибка прошлой попытки осталась: %v", s.Status().Oshib)
	}
	s.Disconnect()
}

// ctxAdmina это контекст, какой канал даёт админской сессии.
//
// Нужен потому, что граница прав переехала в диспетчер: раньше тесты звали
// Obrabotat с голым Background и это проходило, потому что проверки не было.
// Голый Background теперь означает «не админ», и это ВЕРНО: отсутствие сведений
// о допуске толкуется в пользу отказа.
func ctxAdmina() context.Context {
	return kanal.SDopuskom(context.Background(), kanal.Dopusk{Admin: true})
}

func TestPosleOtklyucheniyaNesushchegoNet(t *testing.T) {
	// Иначе экран называет сервер ядра, которого больше нет, а после Ш7-6 это
	// поле по договору значит «то, что ответило ядро».
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Otklyuchit()
	if st := s.Status(); st.NesushchiyId != "" || st.VybranId != "" {
		t.Fatalf("после отключения несущий %q, выбранный %q", st.NesushchiyId, st.VybranId)
	}
}

func TestBezVyboraPoleVyboraPustoe(t *testing.T) {
	// VybrannyyServer при пустом Vybran возвращает Servery[0], и это ВЕРНО для
	// подъёма: человеку с одним сервером не надо сначала «выбирать» его. Но
	// поле «что человек выбрал» обязано остаться пустым, иначе оно называет
	// сервер, которого человек не выбирал. В авто это станет штатным случаем.
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if st.VybranId != "" {
		t.Errorf("выбранный %q, а человек не выбирал ничего", st.VybranId)
	}
	if st.NesushchiyId != "nl" {
		t.Errorf("несущий %q, а поднят был nl", st.NesushchiyId)
	}
}

// Несущий берётся у ЯДРА, а не переписывается из нашего же намерения. Иначе
// поле подтверждает само себя: мы решили поднять nl, мы же и написали, что
// несёт nl, а ядро об этом не спрашивали ни разу.
func TestNesushchiyBerotsyaUYadra(t *testing.T) {
	s := podstavnaya(t, nil)
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return genkonfig.TegKandidata("de"), nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().NesushchiyId; got != "de" {
		t.Fatalf("несущий %q, а ядро отвечает de", got)
	}
}

// Отказ опроса обязан оставить поле ПУСТЫМ, а не значением из намерения. Иначе
// при мёртвом clash_api экран называет сервер, который мы собирались поднять, и
// делает это с той же уверенностью, что и при живом ядре.
func TestPriOtkazeOprosaNesushchiyPustoy(t *testing.T) {
	s := podstavnaya(t, nil)
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return "", errors.New("clash_api не отвечает")
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().NesushchiyId; got != "" {
		t.Fatalf("несущий %q, а ядро на вопрос не ответило", got)
	}
}

// Задача 4.10. Экран над отложенной командой обязан показывать номер волны из
// ответа службы, а не из своей строки (контракт fallback, правило 2). Читать
// его пробой самой команды нельзя: setJournal и setRules это запись. Поэтому
// hello называет отложенные команды с номерами волн, и экран узнаёт их один раз.
func TestHelloNazyvaetOtlozhennyeKomandy(t *testing.T) {
	s := podstavnaya(t, nil)
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "hello",
		Telo: json.RawMessage(`{"protocol":` + strconv.Itoa(protokol.Versiya) + `}`)})
	if o.Oshib != nil {
		t.Fatalf("hello отказал: %v", o.Oshib)
	}
	var telo struct {
		Pozzhe map[string]int `json:"pozzhe"`
	}
	if err := json.Unmarshal(o.Telo, &telo); err != nil {
		t.Fatal(err)
	}
	if len(telo.Pozzhe) != len(pozzhe) {
		t.Fatalf("в hello %d отложенных команд, в диспетчере %d", len(telo.Pozzhe), len(pozzhe))
	}
	for imya, volna := range pozzhe {
		if telo.Pozzhe[imya] != volna {
			t.Fatalf("%s: в hello волна %d, в диспетчере %d", imya, telo.Pozzhe[imya], volna)
		}
	}
	if _, est := telo.Pozzhe["status"]; est {
		t.Fatal("реализованная команда названа отложенной")
	}
}

// Задача 4.11. Порт локального прокси показывается на экране: с 01.09.2026
// рядом с TUN поднимается mixed на 127.0.0.1:10809, занятый порт молча
// отключает прокси, и без поля в статусе человек об этом не узнаёт никак.
func TestStatusNazyvaetPortProksi(t *testing.T) {
	s := podstavnaya(t, nil)
	if s.Status().PortProksi != 0 {
		t.Fatalf("до подъёма порт прокси %d, ожидался ноль (не поднят)", s.Status().PortProksi)
	}
	s.mu.Lock()
	s.portProksiNash = 10809
	s.mu.Unlock()
	if got := s.Status().PortProksi; got != 10809 {
		t.Fatalf("порт прокси в статусе %d, ожидался 10809", got)
	}
}

// Вторая попытка подъёма (план «шесть удобств» §1). Первый старт ядра не
// несёт, второй несёт: итог podnyat, ядро стартовало дважды.
func TestConnectPerezapuskaetYadroOdinRazPeredOtkazom(t *testing.T) {
	s := podstavnaya(t, nil)
	staryy := zhdatPodyoma
	zhdatPodyoma = 600 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()
	startov := 0
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		startov++
		return prezhniy(ctx)
	}
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if startov < 2 {
			return 0, errors.New("исходящий не отвечает (код 504)")
		}
		return 42 * time.Millisecond, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect со второй попытки должен пройти: %v", err)
	}
	if startov != 2 {
		t.Fatalf("ядро стартовало %d раз, ждали 2", startov)
	}
	if st := s.Status(); st.Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("состояние %s, ждали podnyat", st.Sostoyanie)
	}
}

func TestConnectOtkazNesyotPrichinuPosledneyProby(t *testing.T) {
	s := podstavnaya(t, errors.New("исходящий не отвечает (код 504)"))
	staryy := zhdatPodyoma
	zhdatPodyoma = 400 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()
	startov := 0
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) { startov++; return prezhniy(ctx) }
	err := s.Connect(context.Background())
	if err == nil {
		t.Fatal("молчащий туннель признан поднятым")
	}
	if startov != popytokPodyoma {
		t.Fatalf("ядро стартовало %d раз, ждали %d", startov, popytokPodyoma)
	}
	st := s.Status()
	if st.Sostoyanie != protokol.SostNeNeset || st.Oshib == nil || !strings.Contains(st.Oshib.Tekst, "код 504") {
		t.Fatalf("отказ без причины пробы: %s %+v", st.Sostoyanie, st.Oshib)
	}
}

// Смена несущего в авто-режиме происходит без единой команды человека, и до
// сих пор служба молчала о ней: экран узнавал при следующем опросе, трей не
// узнавал никогда. Теперь смена это событие state с именем несущего (план
// «шесть удобств» §4).
func TestSmenaNesushchegoShlyotSostoyanieSImenem(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Второй сервер добавляется ссылкой, как это делает человек: несущий
	// ищется по НАБОРУ, а не по фикстуре.
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatalf("сервер не добавился: %v", o.Oshib)
	}
	var vtoroy protokol.Server
	for _, srv := range spisokServerov(t, s) {
		if srv.Imya == "Германия" {
			vtoroy = srv
		}
	}
	if vtoroy.Id == "" {
		t.Fatal("добавленный сервер не нашёлся в списке")
	}
	_, sob := s.Podpisatsya()
	s.zapomnitNesushchego(vtoroy.Id)
	select {
	case k := <-sob:
		var st protokol.StatusOtvet
		if k.Imya != "state" || json.Unmarshal(k.Telo, &st) != nil {
			t.Fatalf("пришло не состояние: %+v", k)
		}
		if st.NesushchiyId != vtoroy.Id || st.NesushchiyImya == "" {
			t.Fatalf("в состоянии несущий %q с именем %q", st.NesushchiyId, st.NesushchiyImya)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("смена несущего не породила события")
	}
	// Повтор того же несущего событием не является: иначе наблюдатель раз в
	// период будил бы трей одним и тем же.
	s.zapomnitNesushchego(vtoroy.Id)
	select {
	case k := <-sob:
		t.Fatalf("тот же несущий породил событие %s", k.Imya)
	case <-time.After(300 * time.Millisecond):
	}
}

func spisokServerov(t *testing.T, s *Sluzhba) []protokol.Server {
	t.Helper()
	o := vypolnit(t, s, "listServers", nil)
	if o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	var otvet struct {
		Servery []protokol.Server `json:"servery"`
	}
	if err := json.Unmarshal(o.Telo, &otvet); err != nil {
		t.Fatal(err)
	}
	return otvet.Servery
}

// Б1. Повторное нажатие «подключить» на живом туннеле.
//
// The command names a desired state. Answering with an error when that state is
// already reached turns a no-op into a red banner, and the banner used to say
// "все серверы недоступны" on a tunnel that was carrying traffic just fine:
// postavit(SostPodnyat, nil) zeroes s.oshib, so the dispatcher fell through to
// its default code.
func TestConnectNaPodnyatomTunneleNeOtkazyvaet(t *testing.T) {
	s := podstavnaya(t, nil)
	s.postavit(protokol.SostPodnyat, nil)

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect на поднятом туннеле обязан отвечать успехом, а ответил: %v", err)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Errorf("состояние после повторного connect: %v, ожидали podnyat", got)
	}
}

func TestConnectNaPodnimayushchemsyaNeOtkazyvaet(t *testing.T) {
	s := podstavnaya(t, nil)
	s.postavit(protokol.SostPodnimaetsya, nil)

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect во время подъёма обязан отвечать успехом, а ответил: %v", err)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnimaetsya {
		t.Errorf("состояние после connect во время подъёма: %v, ожидали podnimaetsya", got)
	}
}

// zhdatSostoyanie опрашивает состояние до срока. Опрос, а не подписка: события
// приходят и на промежуточные переходы, и тесту нужно «дожили до», а не «прошли
// через».
func zhdatSostoyanie(t *testing.T, s *Sluzhba, hochu protokol.Sostoyanie, srok time.Duration) {
	t.Helper()
	do := time.Now().Add(srok)
	for time.Now().Before(do) {
		if s.Status().Sostoyanie == hochu {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("состояние %v не наступило за %s, сейчас %v", hochu, srok, s.Status().Sostoyanie)
}

// Б1. Пустой набор это не «все серверы недоступны», это «серверов нет».
//
// Тест характеризующий: код на этом пути уже верен (Connect кладёт
// selected-server-gone в состояние). Он сторожит его от возврата к умолчанию.
func TestKodOtkazaConnectNeVryotProServery(t *testing.T) {
	s := podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) { return Nabor{}, nil }
	s.postavit(protokol.SostVyklyuchen, nil)

	otvet := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "connect"})
	if otvet.Oshib == nil {
		t.Fatal("connect без серверов обязан отказать")
	}
	if otvet.Oshib.Kod == protokol.KodAllServersDown {
		t.Errorf("пустой набор нельзя объяснять недоступностью серверов")
	}
	if otvet.Oshib.Kod != protokol.KodSelectedServerGone {
		t.Errorf("код на пустом наборе %q, ожидали %q", otvet.Oshib.Kod, protokol.KodSelectedServerGone)
	}
}

// Б1. Отменённый connect не сваливает отмену на серверы.
//
// The probe loop returns ctx.Err(), Disconnect zeroes s.oshib on the way out,
// and the dispatcher used to read an empty state and answer all-servers-down
// with the text "context canceled". The human closed the window or pressed
// disconnect, and the app blamed the servers for it.
func TestOtmenyonnyyConnectNeVinitServery(t *testing.T) {
	s := podstavnaya(t, errors.New("проба не прошла"))
	ctx, otmena := context.WithCancel(context.Background())
	gotovo := make(chan protokol.Kadr, 1)
	go func() { gotovo <- s.Obrabotat(ctx, protokol.Kadr{Imya: "connect"}) }()

	zhdatSostoyanie(t, s, protokol.SostPodnimaetsya, 3*time.Second)
	otmena()

	select {
	case o := <-gotovo:
		if o.Oshib != nil {
			t.Fatalf("отменённый connect ответил отказом %q: %s", o.Oshib.Kod, o.Oshib.Tekst)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("отменённый connect не ответил за 10 с")
	}
}

// Б2. The connect loop used to watch the CONNECTION context, which no command
// can cancel. A disconnect answered "off" and the tunnel came up 15 seconds
// later: cancelling s.otmena did nothing, because disconnect had already taken
// it, and attempt 2 built a brand new context from Background and lifted again.
func TestDisconnectPrekrashchaetIdushchiyConnect(t *testing.T) {
	// Короче настоящих 15 с: иначе КРАСНЫЙ прогон оставляет за собой горутину
	// подъёма на полминуты, и она переживает свой тест.
	staryy := zhdatPodyoma
	zhdatPodyoma = 5 * time.Second
	defer func() { zhdatPodyoma = staryy }()

	// Проба заведомо не проходит: цикл обязан крутиться, пока его не прервут.
	s := podstavnaya(t, errors.New("проба не прошла"))

	gotovo := make(chan error, 1)
	go func() { gotovo <- s.Connect(context.Background()) }()

	zhdatSostoyanie(t, s, protokol.SostPodnimaetsya, 3*time.Second)
	s.Otklyuchit()

	select {
	case <-gotovo:
	case <-time.After(3 * time.Second):
		t.Fatal("connect не вышел через 3 с после disconnect: цикл проб не заметил отмены")
	}

	// Ещё столько же: за это время старый цикл успевал начать вторую попытку.
	time.Sleep(2 * time.Second)
	if got := s.Status().Sostoyanie; got != protokol.SostVyklyuchen {
		t.Errorf("через 2 с после disconnect состояние %v, ожидали vyklyuchen", got)
	}
}

// Б2, шаг 5. Гипотеза разбора: отключение, успевшее между подъёмом туннеля и
// глушением IPv6, оставляет правило IPv6 без туннеля. vernutIPv6 уже отработал
// в opustitYadro, а glushitIPv6 заводит правило после него, и снять его больше
// некому до следующего подъёма.
func TestOtmenaPosleTunnelyaNeZavoditPraviloIPv6(t *testing.T) {
	staryy := zhdatPodyoma
	zhdatPodyoma = 400 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	s := podstavnaya(t, errors.New("проба не прошла"))
	vPodyome := make(chan struct{})
	pustit := make(chan struct{})
	// Once, потому что до починки попыток подъёма ДВЕ, и второй заход закрыл бы
	// уже закрытый канал: тест падал бы паникой вместо своей находки.
	var raz sync.Once
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		raz.Do(func() {
			close(vPodyome)
			// Отключение приходит ровно сюда: туннель ещё не поднят, но подъём
			// уже идёт и вот-вот вернёт живой адаптер.
			<-pustit
		})
		return prezhniy(ctx)
	}
	var glushili atomic.Int32
	s.glushitIPv6 = func() error { glushili.Add(1); return nil }

	gotovo := make(chan error, 1)
	go func() { gotovo <- s.Connect(context.Background()) }()

	<-vPodyome
	s.Otklyuchit()
	close(pustit)

	select {
	case <-gotovo:
	case <-time.After(20 * time.Second):
		t.Fatal("connect не вернулся после отключения")
	}
	if n := glushili.Load(); n != 0 {
		t.Errorf("правило IPv6 заведено %d раз после отключения: снять его больше некому", n)
	}
}

// Б3. Команда, ответившая отказом, не оставляет следов на диске.
//
// zapomnitVybor писала выбор ДО подъёма, и через sohranitIPeresobrat: набор на
// диск, режим в ручной, правила брандмауэра пересобраны. Отказ подъёма ничего
// из этого не откатывал, и человек, у которого не поднялось, оставался с чужим
// выбранным сервером и сменённым режимом.
func TestConnectSServeromNePishetVyborPriOtkaze(t *testing.T) {
	staryy := zhdatPodyoma
	zhdatPodyoma = 300 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	// Проба не проходит: подъём заведомо провалится.
	s := podstavnaya(t, errors.New("проба не прошла"))
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Rezhim:  protokol.RezhimAvto,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}

	otvet := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "connect", Telo: []byte(`{"server":"de"}`)})
	if otvet.Oshib == nil {
		t.Fatal("connect с непроходящей пробой обязан отказать")
	}

	posle, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if posle.Vybran != "" {
		t.Errorf("отказавший connect записал выбранный сервер %q", posle.Vybran)
	}
	if posle.Rezhim != protokol.RezhimAvto {
		t.Errorf("отказавший connect сменил режим на %q", posle.Rezhim)
	}
}

// Приёмка Б, 03.09.2026: откат, которому нечего откатывать, молчит.
//
// Написан после мутации, которая осталась ЗЕЛЁНОЙ. Откат идёт через pravitNabor
// и выходит из правки сигнальной ошибкой errOtkatNeNuzhen, когда выбор и режим
// уже те самые; сигнал глушится и наружу не уезжает. Снятие этого глушителя не
// уронило ни одного теста из 233, то есть ветка не проверялась ничем.
//
// Судить приходится по журналу: наружу отказ отката не уезжает по построению
// (у команды уже есть свой отказ, и он про подъём), а единственный след ложной
// тревоги это строка «выбор сервера не откачен». Человек, читающий журнал после
// неудачного подъёма, шёл бы искать испорченный набор, которого нет.
func TestOtkatBezIzmeneniyNeZhaluetsyaVZhurnal(t *testing.T) {
	staryy := zhdatPodyoma
	zhdatPodyoma = 300 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	var zhurnal bytes.Buffer
	bylVyvod, bylFlagi := log.Writer(), log.Flags()
	log.SetOutput(&zhurnal)
	log.SetFlags(0)
	defer func() { log.SetOutput(bylVyvod); log.SetFlags(bylFlagi) }()

	s := podstavnaya(t, errors.New("проба не прошла"))
	// Тот же сервер и тот же режим, что назовёт команда: откатывать будет
	// НЕЧЕГО. Это и есть проверяемая ветка.
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Vybran:  "de",
		Rezhim:  protokol.RezhimRuchnoy,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}

	otvet := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "connect", Telo: []byte(`{"server":"de"}`)})
	if otvet.Oshib == nil {
		t.Fatal("connect с непроходящей пробой обязан отказать")
	}

	if strings.Contains(zhurnal.String(), "не откачен") {
		t.Errorf("откат, которому нечего откатывать, пожаловался в журнал: %q", zhurnal.String())
	}

	// Набор при этом обязан остаться прежним, а не обнулиться заодно с жалобой.
	posle, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if posle.Vybran != "de" || posle.Rezhim != protokol.RezhimRuchnoy {
		t.Errorf("набор изменился: выбран %q, режим %q", posle.Vybran, posle.Rezhim)
	}
}

// Б3, вторая половина: откат не имеет права сломать то, ради чего запись стоит
// ДО подъёма. connect --server поднимает ИМЕННО названный сервер, а не тот, что
// был выбран прежде.
func TestConnectSServeromPodnimaetNazvannyy(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Rezhim:  protokol.RezhimAvto,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	var podnyali []string
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		n, err := s.nabor()
		if err != nil {
			return set.Adapter{}, err
		}
		srv, err := n.VybrannyyServer()
		if err != nil {
			return set.Adapter{}, err
		}
		podnyali = append(podnyali, srv.Id)
		s.zapomnitKlash(52715, "sekret-stenda")
		return set.Adapter{Indeks: 10, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}, nil
	}

	otvet := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "connect", Telo: []byte(`{"server":"de"}`)})
	if otvet.Oshib != nil {
		t.Fatalf("connect отказал: %s %s", otvet.Oshib.Kod, otvet.Oshib.Tekst)
	}
	if len(podnyali) != 1 || podnyali[0] != "de" {
		t.Fatalf("подняли %v, а просили de: молчаливая подмена сервера", podnyali)
	}
	if n, _ := s.nabor(); n.Vybran != "de" {
		t.Errorf("удачный connect не записал выбор: %q", n.Vybran)
	}
}

// Б5. Переподъём требуется по ЖИВОМУ ядру, а не по названию состояния.
//
// Состояние otkaz означает, что ядра НЕТ: любая ветка отказа зовёт Disconnect,
// opustit обнуляет порт clash_api и сам ставит vyklyuchen, а otkaz ложится уже
// после него. Признак «сменится со следующего подъёма» при этом отвечал «да», и
// человек шёл переподключать то, чего не поднято: экран просил его починить
// исправное, а режим уже был применён к следующему подъёму по построению.
func TestSetRouteModeVSostoyaniiOtkazNeTrebuetPerepodyoma(t *testing.T) {
	s := podstavnaya(t, nil)
	s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
		Kod: protokol.KodAllServersDown, Tekst: "серверы не ответили"})

	o := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setRouteMode", Telo: []byte(`{"rezhim":"avto"}`)})
	if trebuet(t, o) {
		t.Error("в состоянии otkaz ядра нет, переподъём требовать не за что")
	}
}

// Обратная половина. Прежде здесь стояло «на живом ядре признак обязан
// остаться»: решение переменилось задачей И1 в пользу
// приёмки. Живое ядро переключает режим само, и требовать переподъёма значит
// рвать человеку все соединения ради того, что делается одним PUT.
//
// Тест ОСТАЁТСЯ, а не снимается: без него голое true проходит насквозь, ровно
// как прежде проходило голое false.
func TestSetRouteModeNaZhivomYadreNeTrebuetPerepodyoma(t *testing.T) {
	// Шов конфига ОБЯЗАТЕЛЕН: setRouteMode доводит переключение до переписи
	// файла, и без шва она правит sing-box.json установленного на машине
	// клиента. Та же грабля, из-за которой шов заведён для setServer.
	konfigVyboraDlyaTesta(t, konfigProby)
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	o := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setRouteMode", Telo: []byte(`{"rezhim":"ruchnoy"}`)})
	if trebuet(t, o) {
		t.Error("ядро живо, режим переключён в нём же, а человека всё равно шлют переподнимать")
	}
}

// Остановленная служба не заводит горутин.
//
// Гонка, найденная 03.09.2026 тремя полосами независимо, подпись:
// запись (*Sluzhba).Zavershit-fm против чтения (*Sluzhba).Connect.func4 на
// счётчике s.nabl. Это не «Done против Wait», это Add из нуля ОДНОВРЕМЕННО с
// Wait: sync.WaitGroup такого не допускает по договору.
//
// Как получалось. Наблюдатель заводит восстановление через s.fon.Add(1), и
// заводить он его мог ПОСЛЕ того, как Zavershit уже прошёл s.fon.Wait(). Такая
// горутина зовёт Connect на остановленной службе, тот доходит до s.nabl.Add(1),
// а Disconnect остановки в это же время сидит на s.nabl.Wait().
//
// Класс из таблицы плана: код стал дефектным без единой правки в нём, потому
// что у поля появился второй писатель.
func TestPosleZavershitPodyomNeNachinaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.Zavershit()

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("служба остановлена, а connect поднял туннель и завёл наблюдателя, которого больше никто не ждёт")
	}
	if got := s.Status().Sostoyanie; got == protokol.SostPodnyat {
		t.Errorf("состояние %v после подъёма на остановленной службе", got)
	}
}

// Вторая половина той же гонки: наблюдатель не заводит восстановление после
// остановки. Проверяется на самой точке регистрации, потому что попасть в окно
// прогоном получается примерно раз из трёх, и такой судья ничего не судит.
func TestOstanovlennayaSluzhbaNeRegistriruetFonovyh(t *testing.T) {
	s := podstavnaya(t, nil)
	s.Zavershit()

	if s.zavestiFonovuyu() {
		s.fon.Done()
		t.Error("остановленная служба зарегистрировала фоновую горутину: Zavershit её уже не дождётся")
	}
	// Поколение берётся ТЕКУЩЕЕ, а не нулевое: с чужим поколением отказ придёт
	// от проверки отмены, и флаг остановки останется непроверенным. Мутация
	// «убрать ostanovlena из регистрации» на нуле проходила зелёной.
	s.mu.Lock()
	tekushchee := s.pokolenieP
	s.mu.Unlock()
	if s.zavestiNablyudatelya(tekushchee) {
		s.nabl.Done()
		t.Error("остановленная служба зарегистрировала наблюдателя: Disconnect его уже не дождётся")
	}
}

// Отмена во время подъёма ТУННЕЛЯ не оставляет человека в отказе.
//
// Полоса Б убрала выдуманный диагноз из ОТВЕТА команды connect и не дошла до
// СОСТОЯНИЯ. Ветка «туннель не поднялся» (komandy.go, сразу после podnyatTunnel)
// не спрашивает, отменял ли кто подъём, и кладёт otkaz с кодом
// tun-create-failed поверх выключенного. Человек нажал «отключить», получил
// честный ответ, а следом красный экран с диагнозом «TUN-адаптер не появился».
//
// Замерено раундом 3 04.09.2026: судья отмены видел vyklyuchen ровно один раз,
// а потом четырнадцать замеров подряд otkaz.
func TestOtmenaVoVremyaPodyomaTunnelyaNeStavitOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	vPodyome := make(chan struct{})
	var raz sync.Once
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		raz.Do(func() { close(vPodyome) })
		<-ctx.Done()
		// Ровно как настоящий: ждал адаптера, дождался отмены.
		return set.Adapter{}, fmt.Errorf("tun0: %w", ctx.Err())
	}

	gotovo := make(chan error, 1)
	go func() { gotovo <- s.Connect(context.Background()) }()
	<-vPodyome
	s.Otklyuchit()
	<-gotovo

	st := s.Status()
	if st.Sostoyanie != protokol.SostVyklyuchen {
		t.Errorf("после отмены состояние %q (код %v), а человек нажал «отключить»",
			st.Sostoyanie, st.Oshib)
	}
	if st.Oshib != nil {
		t.Errorf("отмене выдуман диагноз %q: %s", st.Oshib.Kod, st.Oshib.Tekst)
	}
}

// Группа urltest не называет выбор, пока не прошла первая проба задержки.
// Замерено 06.09.2026 на живом ядре: через 1.23 с после подъёма now пуст, через
// 5.63 с назван. Служба спрашивала РОВНО ОДИН РАЗ и на пустоту сдавалась, а
// чинил поле только наблюдатель через тридцать секунд. Приёмка ловила это как
// провал: «nesushchiy_id пуст, ядро о несущем не спросили».
//
// Отличие от TestPriOtkazeOprosaNesushchiyPustoy рядом: там ядро НЕ ОТВЕТИЛО, и
// пустое поле честно. Здесь ядро ответило «ещё не выбрал», и правильный ответ
// это спросить ещё раз, а не выдать незнание за окончательное.
func TestGruppaNeVybralaZnachitSprositEshcho(t *testing.T) {
	s := podstavnaya(t, nil)
	var sprosheno int32
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		if atomic.AddInt32(&sprosheno, 1) <= 2 {
			return "", fmt.Errorf("%w: avto", yadra.ErrNeVybrala)
		}
		return genkonfig.TegKandidata("de"), nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if s.Status().NesushchiyId == "de" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("несущий %q, а ядро назвало de после двух «ещё не выбрал» (спрошено раз: %d)",
		s.Status().NesushchiyId, atomic.LoadInt32(&sprosheno))
}

// Доспрос не имеет права быть вечным: ядро, которое не назовёт выбор никогда,
// оставило бы за собой горутину на всё время подключения.
func TestDosprosNesushchegoNeVechen(t *testing.T) {
	s := podstavnaya(t, nil)
	// Срок укорачивается у СВОЕЙ службы: двадцать секунд умолчания превратили бы
	// проверку в двадцатисекундный тест.
	s.srokDosprosa = 600 * time.Millisecond
	s.shagDosprosa = 50 * time.Millisecond
	var sprosheno int32
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		atomic.AddInt32(&sprosheno, 1)
		return "", fmt.Errorf("%w: avto", yadra.ErrNeVybrala)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(s.srokDosprosa + 300*time.Millisecond)
	bylo := atomic.LoadInt32(&sprosheno)
	time.Sleep(300 * time.Millisecond)
	if stalo := atomic.LoadInt32(&sprosheno); stalo != bylo {
		t.Fatalf("доспрос продолжается после срока: было %d, стало %d", bylo, stalo)
	}
	if s.Status().NesushchiyId != "" {
		t.Fatalf("несущий %q, а ядро выбор так и не назвало", s.Status().NesushchiyId)
	}
}

// Поле несущего обязано следовать за ядром, а не за пробой живости.
//
// Наблюдатель делал оба дела на одном тике в тридцать секунд, поэтому после
// переключения поле до полуминуты называло ПРЕЖНИЙ сервер, пока трафик уже шёл
// через другой. Замерено 06.09.2026 три раза: 26.8, 29.5 и 31.1 с, то есть
// ровно период опроса, а не цена переключения.
//
// Вопрос о несущем уходит на 127.0.0.1 и стоит копейки, проба живости идёт
// наружу и стоит трафика. Значит и частота у них разная.
func TestNesushchiyObnovlyaetsyaChashcheProbyZhivosti(t *testing.T) {
	s := podstavnaya(t, nil)
	s.period = 30 * time.Second
	s.periodNesushchego = 20 * time.Millisecond
	var teg atomic.Value
	teg.Store(genkonfig.TegKandidata("nl"))
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return teg.Load().(string), nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().NesushchiyId; got != "nl" {
		t.Fatalf("несущий %q сразу после подъёма, а ядро отвечает nl", got)
	}
	teg.Store(genkonfig.TegKandidata("de"))
	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if s.Status().NesushchiyId == "de" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("несущий %q через три секунды, а ядро сменило выбор на de: поле ждёт пробы живости",
		s.Status().NesushchiyId)
}

// Частый опрос имеет цену, и платить её на каждом тике нельзя: имя несущего
// ищется в наборе, а набор лежит на диске зашифрованным. Пока несущий тот же,
// искать нечего.
func TestTotZheNesushchiyNeChitaetNabor(t *testing.T) {
	s := podstavnaya(t, nil)
	nastoyashchiy := s.nabor
	var chteniy atomic.Int32
	s.nabor = func() (Nabor, error) {
		chteniy.Add(1)
		return nastoyashchiy()
	}
	s.zapomnitNesushchego("nl")
	poslePervogo := chteniy.Load()
	if poslePervogo == 0 {
		t.Fatal("имя несущего взялось ниоткуда: набор не читался ни разу")
	}
	for i := 0; i < 5; i++ {
		s.zapomnitNesushchego("nl")
	}
	if stalo := chteniy.Load(); stalo != poslePervogo {
		t.Fatalf("набор прочитан %d раз на пяти повторах того же несущего (было %d)", stalo, poslePervogo)
	}
}
