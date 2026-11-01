package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Пять команд, снятых с заглушки задачей 3.8.
//
// Общее правило у всех: НИ ОДНА не отдаёт наружу ключи и адрес подписки. Канал
// пускает INTERACTIVE осознанно, чтобы интерфейс не требовал администратора на
// каждый запуск, и это же значит, что всё, уехавшее в канал, доступно любой
// интерактивной сессии на машине.

func (s *Sluzhba) listServers(k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	versii, err := s.versiiServerov(n.Servery)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, "не удалось определить версии серверов")
	}
	spisok := make([]protokol.Server, 0, len(n.Servery))
	for _, srv := range n.Servery {
		e := dlyaEkrana(srv)
		// Пометка ставится ЗДЕСЬ, а не в dlyaEkrana: та знает только сервер, а
		// участие в автовыборе это свойство набора.
		e.VneAvto = !n.UchastvuetVAvto(srv.Id)
		spisok = append(spisok, e)
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"servery": spisok,
		"versii":  versii,
		// n.Vybran как есть, а не VybrannyyServer().Id: последний при пустом
		// выборе отдаёт Servery[0], то есть экран называл бы выбранным первый
		// по списку. В авто это станет штатным случаем, а не краем.
		"vybran": n.Vybran,
		// Не сам адрес, а только признак и узел. Адрес подписки это пропуск, и
		// показывать на экране его целиком незачем.
		"podpiska_zadana": n.AdresAktivnoy() != "",
		"podpiska_uzel":   uzelPodpiski(n.AdresAktivnoy()),
		// Возраст последнего обновления показывается рядом с узлом (задача
		// 4.9). Статус этого поля не отдаёт, а брать его больше неоткуда.
		"podpiska_obnovlena": s.podpiskaObnovlena(),
	})
}

func (s *Sluzhba) podpiskaObnovlena() *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snimok.PodpiskaObnovlena
}

func uzelPodpiski(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (s *Sluzhba) addServer(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Ssylka string `json:"ssylka"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	srv, err := ssylki.Razobrat(strings.TrimSpace(telo.Ssylka))
	if err != nil {
		return otkaz(k.Id, k.Imya, kodRazbora(err), prichinaRazbora(err))
	}

	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery, srv = ssylki.DobavitProfil(n.Servery, srv)
		return nil
	}); err != nil {
		return otkazIz(k, kodSohraneniya(err), err)
	}
	return otvet(k.Id, k.Imya, map[string]any{"server": dlyaEkrana(srv)})
}

func (s *Sluzhba) removeServer(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	// Поиск идёт ВНУТРИ правки, а не до неё: между «нашли» и «записали» сервер
	// успевает исчезнуть чужой командой, и снаружи замка это тот же TOCTOU,
	// против которого написана pravitNabor.
	ostalos := 0
	if err := s.pravitNabor(func(n *Nabor) error {
		spisok := make([]protokol.Server, 0, len(n.Servery))
		nashli := false
		for _, srv := range n.Servery {
			if srv.Id == telo.Id {
				nashli = true
				continue
			}
			spisok = append(spisok, srv)
		}
		if !nashli {
			return fmt.Errorf("%w: %s", ErrServerNeNayd, telo.Id)
		}
		n.Servery = spisok
		if n.Vybran == telo.Id {
			// Режим считается по Vybran, и чистка Vybran его бы сменила. Человек был
			// в ручном, раз выбор стоял; удаление сервера это не решение про режим.
			// Пишем ЗДЕСЬ, а не при каждой записи набора: раз проставленное поле
			// побеждает умолчание навсегда, и бланкетная запись сделала бы будущую
			// смену умолчания недоставляемой.
			if n.Rezhim == "" {
				n.Rezhim = protokol.RezhimRuchnoy
			}
			n.Vybran = ""
		}
		ostalos = len(spisok)
		return nil
	}); err != nil {
		return otkazIz(k, kodSohraneniya(err), err)
	}
	return otvet(k.Id, k.Imya, map[string]any{"ostalos": ostalos})
}

func (s *Sluzhba) setSubscription(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Adres string `json:"adres"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	adres := strings.TrimSpace(telo.Adres)
	if adres != "" {
		u, err := url.Parse(adres)
		if err != nil || u.Host == "" {
			// Текст ошибки строим сами: err.Error() у url.Parse содержит САМ
			// адрес, а он секрет класса ключа.
			return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, "адрес подписки не разобран")
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed,
				"поддерживаются только http и https")
		}
	}
	serverov := 0
	if err := s.pravitNabor(func(n *Nabor) error {
		n.ZadatAktivnuyu(adres)
		serverov = len(n.Servery)
		return nil
	}); err != nil {
		return otkazIz(k, kodSohraneniya(err), err)
	}
	if adres == "" {
		return otvet(k.Id, k.Imya, map[string]any{"zadana": false, "serverov": serverov})
	}
	// Наполнить список СРАЗУ. Отдельная команда refreshSubscription про себя
	// знает разработчик, а человек задаёт подписку и жмёт подключить, получая
	// «серверов нет». Адрес при этом уже сохранён выше: временно молчащая
	// панель не имеет права стереть верный ввод.
	r, posle, err := s.obnovitPodpisku(ctx)
	if err != nil {
		return otkazPodpiski(k, r, err)
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"zadana": true, "serverov": posle, "otkazy": r.Otkazy,
	})
}

// Ручное обновление должно успеть ответить до минутного таймаута канала.
const srokRuchnogoObnovleniya = 25 * time.Second

func (s *Sluzhba) refreshSubscription(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	ctx, cancel := context.WithTimeout(ctx, srokRuchnogoObnovleniya)
	defer cancel()
	// За всеми подписками сразу: «Обновить» означает «сходи за свежим», и
	// запасная тут ничем не хуже активной. Наверх поднимается только отказ
	// активной, отказы запасных лежат в их строках.
	var telo struct {
		Id string `json:"id"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
		}
	}
	var r ssylki.Razbor
	var serverov int
	var err error
	if telo.Id != "" {
		r, serverov, err = s.obnovitPodpiskuPoId(ctx, telo.Id)
	} else {
		r, serverov, err = s.obnovitVsePodpiski(ctx)
	}
	if err != nil {
		return otkazPodpiski(k, r, err)
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"serverov": serverov,
		"otkazy":   r.Otkazy,
	})
}

// otkazPodpiski переводит отказ обновления в код §9.1. ОДИН перевод на обе
// команды: две копии разошлись бы, и один и тот же отказ назывался бы человеку
// по-разному в зависимости от того, откуда он пришёл.
func otkazPodpiski(k protokol.Kadr, r ssylki.Razbor, err error) protokol.Kadr {
	var nab oshibkaNabora
	var sohr oshibkaSohraneniya
	switch {
	case errors.Is(err, errObnovlenieZameneno):
		return otvet(k.Id, k.Imya, map[string]any{"obnovlenie_zameneno": true})
	case errors.As(err, &nab):
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	case errors.Is(err, errPodpiskaNeZadana):
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, "подписка не задана")
	case errors.Is(err, ssylki.ErrPodpiskaIstekla):
		// Текст панели дословно и без нашей формулировки поверх: в нём и
		// причина, и, у некоторых панелей, код оплаты.
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionExpired, textyUvedomleniy(r))
	case errors.Is(err, ssylki.ErrPodpiskaUstroystvo):
		return otkaz(k.Id, k.Imya, protokol.KodPodpiskaUstroystvo, tekstOtkazaPodpiski(err))
	case errors.Is(err, ssylki.ErrNeSsylki), errors.Is(err, ssylki.ErrPodpiskaVelika):
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, tekstOtkazaPodpiski(err))
	case errors.Is(err, ssylki.ErrPodpiskaPusta):
		// Прежний список ОСТАЁТСЯ. Одна опечатка в публикации не должна
		// оставлять запертую машину без единого адреса.
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, tekstOtkazaPodpiski(err))
	case errors.As(err, &sohr):
		return otkazIz(k, kodSohraneniya(err), err)
	}
	// Шаг, на котором сорвалась загрузка, меняет совет: не успевший ответ и
	// отказ панели по праву доступа лечатся по-разному, а до 22.09.2026 оба
	// приезжали как «подписка недоступна, обнови подписку».
	switch shagZagruzki(err) {
	case sboi.Srok:
		return otkaz(k.Id, k.Imya, protokol.KodPodpiskaSrok, tekstOtkazaPodpiski(err))
	case sboi.Dostup:
		return otkaz(k.Id, k.Imya, protokol.KodPodpiskaDostup, tekstOtkazaPodpiski(err))
	}
	// Остальные шаги (dns, tcp, tls) ведут в одну сторону - проверить сеть и
	// повторить, - поэтому код общий, а сам шаг назван в тексте отказа.
	return otkaz(k.Id, k.Imya, protokol.KodSubscriptionUnreach, tekstOtkazaPodpiski(err))
}

func textyUvedomleniy(r ssylki.Razbor) string {
	t := make([]string, 0, len(r.Uvedomleniya))
	for _, u := range r.Uvedomleniya {
		t = append(t, u.Tekst)
	}
	return strings.Join(t, "; ")
}

// kodRazbora переводит ошибку разбора ссылки в код §9.1.
func kodRazbora(err error) string {
	if errors.Is(err, ssylki.ErrUvedomleniePodpiski) {
		return protokol.KodSubscriptionExpired
	}
	return protokol.KodSubscriptionMalformed
}

// prichinaRazbora отдаёт текст без самой ссылки.
//
// В ссылке лежат uuid и ключи, а err.Error() у url.Parse кладёт туда весь адрес
// целиком. Ошибка разбора уехала бы в журнал вместе с ключами.
func prichinaRazbora(err error) string {
	switch {
	case errors.Is(err, ssylki.ErrUvedomleniePodpiski):
		return err.Error() // здесь как раз ТЕКСТ ПАНЕЛИ, ключей в нём нет
	case errors.Is(err, ssylki.ErrTransportNePodderzhan):
		return "транспорт не поддерживается"
	default:
		return "ссылка не разобрана"
	}
}

// serverPoId отвечает, есть ли такой сервер в наборе, и отдаёт его.
//
// Отдельной функцией, потому что тот же поиск делают оба пути выбора, и две
// копии разошлись бы на первом же изменении правил поиска.
func (s *Sluzhba) serverPoId(id string) (protokol.Server, error) {
	n, err := s.nabor()
	if err != nil {
		return protokol.Server{}, err
	}
	return serverIzNabora(n, id)
}

// serverIzNabora это тот же поиск по УЖЕ прочитанному набору.
//
// Отдельно, потому что под pravitNabor набор читать заново нельзя: там он уже
// в руках, а второе чтение это тот самый TOCTOU, ради которого заведён замок.
func serverIzNabora(n Nabor, id string) (protokol.Server, error) {
	for _, srv := range n.Servery {
		if srv.Id == id {
			return srv, nil
		}
	}
	return protokol.Server{}, fmt.Errorf("%w: %s", ErrServerNeNayd, id)
}

// zapomnitVybor помнит выбор и В ЯДРО НЕ ХОДИТ.
//
// Её зовёт connect, где ядра ещё нет. Замерено на живом ядре, что бывает без
// этого разделения: connect --server de при поднятом туннеле уводит весь трафик
// на другой сервер и СЛЕДОМ отвечает all-servers-down.
func (s *Sluzhba) zapomnitVybor(id string) error {
	// Проверка существования сервера ВНУТРИ правки: снаружи замка она читает
	// один набор, а пишется другой, и выбор уезжает на сервер, которого в
	// записываемом наборе уже нет.
	return s.pravitNabor(func(n *Nabor) error {
		if _, err := serverIzNabora(*n, id); err != nil {
			return err
		}
		n.Vybran = id
		// Выбор сервера руками ОЗНАЧАЕТ ручной режим. Без этой строки после Ш7-8
		// человек в авто жмёт на страну, команда проходит, Vybran записан, а
		// селектор по-прежнему стоит на avto: rezhimNabora предпочитает непустое
		// поле режима.
		n.Rezhim = protokol.RezhimRuchnoy
		return nil
	})
}

// setRouteMode выбирает режим маршрута: авто или ручной.
//
// На ЖИВОМ ядре режим переключается В ЯДРЕ (задача И1), тем же PUT /proxies,
// каким переключает сервер setServer. Прежняя редакция писала только набор и
// отвечала «применится со следующего подъёма»: человек, нажавший «авто» при
// поднятом туннеле, обязан был порвать себе все соединения ради переключения,
// которое ядро делает на лету.
//
// Признак «нужен переподъём» отдаётся ОТСЮДА, а не считается диспетчером.
// Диспетчер считал его по живому ядру, то есть отвечал «да» ровно в том случае,
// когда переподъём как раз не нужен.
func (s *Sluzhba) setRouteMode(ctx context.Context, r protokol.Rezhim) (trebuetPodyoma bool, err error) {
	// Отказ ДО записи. Команда пришла по протоколу, значит и отказ по протоколу:
	// нового кода тут не нужно, нужен нетронутый набор.
	if r != protokol.RezhimAvto && r != protokol.RezhimRuchnoy {
		return false, fmt.Errorf("%w: %q", ErrChuzhoyRezhim, r)
	}
	// Замок на ВЕСЬ цикл, как в setServer и по той же причине: между чтением
	// набора и его записью стоит сетевой вызов в ядро, то есть окно шире, чем у
	// любой команды, которой хватает muNabor.
	s.muVybor.Lock()
	defer s.muVybor.Unlock()
	// Режим выбран человеком: подмена молчащего сервера, заведённая без него,
	// кончается вместе с прежним режимом. Отказ команды её не трогает: ядро
	// осталось там, где было.
	defer func() {
		if err == nil {
			s.snyatZapasnoy()
		}
	}()

	// Ручной режим без выбранного сервера это не противоречие: VybrannyyServer
	s.cancelSpeed("Режим выбора сервера изменяется. Замер остановлен.")
	// отдаёт первый по списку, и человек, вернувшийся в ручной, выбирает страну
	// следующим действием. Чистить Vybran при переходе в авто тоже нельзя:
	// вернувшись в ручной, человек ожидает свой прежний выбор, а не первый по
	// списку.
	zapisat := func() error {
		return s.pravitNabor(func(n *Nabor) error {
			n.Rezhim = r
			return nil
		})
	}

	adres, sekret := s.dostupKKlash()
	if adres == "" {
		// Ядра нет: применять не к чему. Режим доедет следующим подъёмом,
		// который собирает конфиг заново, и переподнимать ради этого нечего.
		return false, zapisat()
	}

	teg, err := s.tegRezhima(r)
	if err != nil {
		return false, err
	}
	// PUT ПЕРЕД записью набора. Обратный порядок оставлял бы записанный режим,
	// которого в ядре нет: экран говорит «авто», а трафик идёт по ручному
	// выбору, и заметить это человеку нечем.
	if err := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, teg); err != nil {
		// Ядра НЕТ на связи это не отказ ядра, и разница здесь решающая.
		//
		// Порядок «PUT перед записью» защищает от расхождения между экраном и
		// живым трафиком. Расходиться не с чем, когда ядра нет вовсе: трафика
		// через него не идёт никакого, а следующий подъём соберёт конфиг
		// заново и возьмёт режим из набора. Ведём себя ровно как при пустом
		// адресе выше.
		//
		// Без этой ветки человек, у которого сервер лёг, заперт: авария уже
		// опустила ядро, нажатие «Автоматически» отвечает отказом, набор
		// остаётся ручным, и восстановление продолжает поднимать ровно тот
		// сервер, который лёг. Замерено в госте 13.09.2026, пять минут подряд.
		if yadra.YadroNeSlushaet(err) {
			return false, zapisat()
		}
		return false, fmt.Errorf("%w: %w", ErrRezhimNeDoehalVYadro, err)
	}
	if err := zapisat(); err != nil {
		return false, err
	}
	// Режим живёт в ПАМЯТИ ядра: горячей перезагрузки конфига нет, и сторож,
	// перезапустив ядро, поднял бы его по файлу с прежним маршрутом молча.
	//
	// Отказом команды это быть не может: режим уже применён к живому трафику, и
	// отказ увёл бы экран обратно на маршрут, которого в ядре больше нет.
	// Молчать нельзя тем более, поэтому отказ уезжает в СОСТОЯНИЕ, ровно как в
	// setServer.
	if err := s.perepisatVyborVKonfige(teg); err != nil {
		log.Printf("режим не переписан в конфиге, перезапуск ядра его отменит: %v", err)
		s.postavit(s.vnutriSost(), &protokol.Oshibka{
			Kod:   protokol.KodPereklyuchenieNeDoehalo,
			Tekst: "режим выбран, но не сохранён: после перезапуска VPN вернётся прежний",
		})
	}
	return false, nil
}

var ErrChuzhoyRezhim = errors.New("режим маршрута не опознан")

// ErrRezhimNeDoehalVYadro отделяет отказ ЯДРА от отказа записи набора.
//
// Без него отказ PUT уезжал в kodSohraneniya и приезжал человеку хвостовым
// secrets-unreadable: «секреты не читаются» вместо «ядро не приняло
// переключение». Совет при этом менялся на противоположный.
var ErrRezhimNeDoehalVYadro = errors.New("ядро не приняло режим маршрута")

// kodSmenyMarshruta различает две стороны отказа setRouteMode: ядро и
// хранилище. Имя не kodRezhima: та занята режимом «весь трафик» в
// killswitch.go, и две разные причины под одним именем однажды сошлись бы.
func kodSmenyMarshruta(err error) string {
	if errors.Is(err, ErrRezhimNeDoehalVYadro) {
		return kodPereklyucheniya(err)
	}
	return kodSohraneniya(err)
}

// tegRezhima переводит режим в тег селектора ТЕМ ЖЕ способом, каким его считает
// сборка конфига (genkonfig.gruppy): в авто это группа, в ручном тег выбранного
// сервера. Разъедься эти два места, ядро и файл говорили бы разное.
func (s *Sluzhba) tegRezhima(r protokol.Rezhim) (string, error) {
	if r == protokol.RezhimAvto {
		return genkonfig.TegAvto, nil
	}
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

// setServer переключает ЖИВОЙ туннель на другой сервер, не перезапуская ядро.
func (s *Sluzhba) setServer(ctx context.Context, id string) error {
	if _, err := s.serverPoId(id); err != nil {
		return err
	}
	// Замок на ВЕСЬ цикл чтения-правки-записи набора. Команды идут каждая своей
	// горутиной, а здесь между чтением и записью стоят два сетевых вызова, то
	// есть окно шире, чем у любой существующей команды. Замок ОТДЕЛЬНЫЙ, не
	// s.mu: под s.mu отвечает Status, и держать его через поход в ядро значит
	// подвесить весь интерфейс.
	s.muVybor.Lock()
	defer s.muVybor.Unlock()

	s.cancelSpeed("Сервер изменяется. Замер остановлен.")
	adres, sekret := s.dostupKKlash()
	if adres == "" {
		// Ядра нет: переключать нечего, выбор просто запоминается. Отказ здесь
		// означал бы, что выбрать сервер заранее нельзя.
		return s.zapomnitBezYadra(id)
	}

	// Прежний выбор берётся ОДНОЭТАЖНО и ДО всякой правки: в авто он равен тегу
	// вложенной группы, и вернуться надо именно в авто, а не в тот сервер,
	// который она выбрала.
	//
	// Отказ здесь ПРЕРЫВАЕТ переключение, не дойдя до PUT. Не зная текущего
	// выбора, откатывать некуда, а переключиться и не суметь вернуться значит
	// оставить человека на мёртвом кандидате со словами «выбери другой».
	prezhniy, err := s.vyborGruppy(ctx, adres, sekret, genkonfig.TegSelector)
	if err != nil {
		// Ядра нет на связи: та же ветка, что и при пустом адресе выше, и по
		// той же причине. «Сервер лёг, выберу другой» это первое, что делает
		// человек, и попадает он ровно в это окно: авария опустила ядро
		// секундой раньше. Отказать здесь значит отказать там, где нужнее.
		if yadra.YadroNeSlushaet(err) {
			return s.zapomnitBezYadra(id)
		}
		return fmt.Errorf("текущий выбор ядра не прочитан, переключаться вслепую нельзя: %w", err)
	}

	// Считаем ДО pravitSost: замыкание выполняется под s.mu, и вызов метода
	// службы внутри него это тупик. Правило, а не совпадение.
	nach := s.seychas()
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
		f.SelectorNach, f.SelectorGotov = &nach, nil
	})

	teg := genkonfig.TegKandidata(id)
	// По часам машины, а не s.seychas: с ними сверяется время строк ядра.
	smena := time.Now()
	if err := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, teg); err != nil {
		// Ядро могло исчезнуть и между чтением выбора и этим PUT: окно узкое,
		// но оно то же самое.
		if yadra.YadroNeSlushaet(err) {
			return s.zapomnitBezYadra(id)
		}
		return err
	}
	// Жалобы ядра на группу до этой секунды говорят о прежнем сервере.
	s.otmetitSmenuVybora(smena)
	// 204 говорит «команда принята», а не «трафик пошёл туда». Судит проба.
	if _, err := s.zameritYadro(ctx, adres, sekret, tegDlyaZamera(), smena); err != nil {
		// Человек был подключён и работал. Неудачный выбор не имеет права
		// оставить его без сети: возвращаем прежний и говорим, что случилось.
		vozvrat := time.Now()
		if e := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, prezhniy); e != nil {
			// Возврат ТОЖЕ не прошёл. Ядро осталось на кандидате, который не
			// несёт, и называть это «новый не несёт, подключение цело» значит
			// соврать: целого подключения больше нет.
			//
			// Состояние переводится в ne-neset, потому что восстановление
			// продолжается именно из него и из otkaz (vosstanavlivat). Ядро
			// здесь НЕ опускается намеренно: опустив его, мы отменили бы
			// контекст наблюдателя, а он единственный, кто заводит
			// восстановление. Его же проба сейчас проваливается, значит он
			// доведёт дело до опускания и подъёма заново сам.
			log.Printf("откат выбора на %s не прошёл: %v", prezhniy, e)
			s.postavit(protokol.SostNeNeset, &protokol.Oshibka{
				Kod:   protokol.KodTunnelNeNeset,
				Tekst: "переключение не прошло, и возврат на прежний сервер тоже: VPN не несёт трафик",
			})
			return fmt.Errorf("%w: проба %v, возврат %v", ErrOtkatNeUdalsya, err, e)
		}
		s.otmetitSmenuVybora(vozvrat)
		// Проба нового сервера падает за доли секунды, и строки трафика через
		// него у ядра почти никогда нет: причину называет ядро замера. Уже
		// после возврата, чтобы человек ждал её на рабочем подключении.
		err = s.utochnitPrichinu(ctx, err, id)
		// Оба %w: вызывающему нужны и обещание «подключение цело», и причина.
		return fmt.Errorf("%w: %w", ErrNovyyVyborNeNesyot, err)
	}

	// Отметка СРАЗУ после удачной пробы. Дальше идут пересборка запрета и два
	// GET у ядра: поставив отметку после них, прибор мерил бы запрет.
	gotov := s.seychas()
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.SelectorGotov = &gotov })
	// Человек выбрал сам, и выбор понёс: подмена молчащего сервера кончилась.
	s.snyatZapasnoy()

	// Запись набора ПОСЛЕ удачной пробы. Обратный порядок оставлял бы после
	// неудачного переключения записанный выбор, которого в ядре нет.
	//
	// Отставшие правила брандмауэра здесь НЕ отказ: трафик уже переключён, а
	// отказ пришёл бы уже после удачных PUT и пробы. Поступаем ровно как
	// Connect: говорим вслух и продолжаем.
	if err := s.zapomnitVybor(id); err != nil {
		if !errors.Is(err, errPravilaOtstali) {
			return err
		}
		log.Printf("правила брандмауэра отстали от переключения: %v", err)
		s.izvestit("state", s.StatusS(&protokol.Oshibka{
			Kod: protokol.KodFirewallFailed, Tekst: tekstIz("подключение", err)}))
	}

	// Состояние могло смениться, пока мы ходили в ядро: человек нажал
	// «отключить», наблюдатель опустил туннель по трём провалам подряд. Ставить
	// несущего поверх выключенного значит показать экран, которого нет.
	//
	// Критерий тот же, что на входе: ядро либо держит наш список, либо нет.
	if adres, _ := s.dostupKKlash(); adres == "" {
		return nil
	}
	// Выбор это поле ЖИВОГО подключения, а не настройка между сессиями:
	// Otklyuchit снимает его вместе с несущим. Поэтому пишется ЗДЕСЬ, после
	// того же заслона, а не в sohranitIPeresobrat рядом с режимом: та зовётся и
	// без туннеля, и выбор уехал бы на экран, где состояние vyklyuchen.
	s.mu.Lock()
	s.vybranId = id
	s.mu.Unlock()

	// Спрашиваем ЯДРО, а не переписываем своё же намерение.
	if t, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err != nil {
		log.Printf("несущий после переключения не спрошен: %v", err)
	} else if nid, ok := genkonfig.IdIzTega(t); ok {
		s.zapomnitNesushchego(nid)
	} else {
		log.Printf("ядро назвало несущим %q, а это не тег кандидата", t)
	}
	s.izvestit("state", s.Status())
	// Выбор живёт в ПАМЯТИ ядра: горячей перезагрузки конфига нет, и перезапуск
	// ядра сторожем вернул бы трафик на сервер из файла молча.
	if err := s.perepisatVyborVKonfige(teg); err != nil {
		// НЕ только в журнал. Отказом команды это быть не может: трафик уже
		// идёт на новый сервер, и отказ увёл бы экран обратно на тот, которого
		// в ядре больше нет. Но молчать нельзя тем более: перезапуск ядра
		// сторожем вернёт трафик на сервер из файла, а экран останется на
		// новом, и расхождение будет нечем заметить.
		//
		// Отказ уезжает в СОСТОЯНИЕ, а значит и в событие state, и в ответ
		// команды: ответом setServer служит s.Status().
		log.Printf("выбор не переписан в конфиге, перезапуск ядра его отменит: %v", err)
		s.postavit(s.vnutriSost(), &protokol.Oshibka{
			Kod:   protokol.KodPereklyuchenieNeDoehalo,
			Tekst: "сервер выбран, но не сохранён: после перезапуска VPN вернётся прежний",
		})
	}
	return nil
}

// zapomnitBezYadra запоминает выбор, когда ядра нет. Если восстановление в эту
// минуту ждёт на паузе, выбранный сервер пробуется сразу (жалоба 15.10.2026):
// другой сервер выбирают ровно потому, что прежний не отвечает, и ждать ради
// этого конца пятиминутной паузы незачем.
//
// Отставшие правила брандмауэра попытке не помеха: выбор записан, и отказ
// уходит человеку тем же ответом, что и прежде.
func (s *Sluzhba) zapomnitBezYadra(id string) error {
	err := s.zapomnitVybor(id)
	if err == nil || errors.Is(err, errPravilaOtstali) {
		// Новый выбор человека отменяет подмену прежнего: следующий подъём
		// пробует то, что он выбрал.
		s.snyatZapasnoy()
		if s.popytatsyaSeychas("выбран другой сервер") {
			log.Printf("выбран сервер %s во время паузы восстановления: пробую сразу", id)
		}
	}
	return err
}

// putKonfigaVybora это шов над путём к конфигу ядра.
//
// Заведён по той же причине, что и normalizovatPut в komandy_pravil.go:
// настоящая функция ходит в C:\ProgramData живой машины. Без шва каждый
// зелёный прогон setServer переписывал поле default у УСТАНОВЛЕННОГО клиента,
// подставляя ему тег тестового сервера. Замерено 03.09.2026: после прогона
// ворот в конфиге машины стояло "default": "srv-de", а такого исходящего в нём
// нет, то есть следующий старт ядра уже не поднялся бы.
var putKonfigaVybora = putKonfigaTun

// perepisatVyborVKonfige правит поле default у селектора в ФАЙЛЕ конфига.
//
// Отказ переписки не роняет переключение: трафик уже идёт куда надо, а
// испорченный конфиг чинится следующим подъёмом, который собирает файл заново.
func (s *Sluzhba) perepisatVyborVKonfige(teg string) error {
	put := putKonfigaVybora()
	telo, err := os.ReadFile(put)
	if err != nil {
		return fmt.Errorf("конфиг не прочитан: %w", err)
	}
	var k map[string]any
	if err := json.Unmarshal(telo, &k); err != nil {
		return fmt.Errorf("конфиг не разобран: %w", err)
	}
	spisok, _ := k["outbounds"].([]any)
	for _, v := range spisok {
		o, ok := v.(map[string]any)
		if !ok || o["tag"] != genkonfig.TegSelector {
			continue
		}
		o["default"] = teg
		novoe, err := json.MarshalIndent(k, "", "  ")
		if err != nil {
			return fmt.Errorf("конфиг не собран обратно: %w", err)
		}
		// 0600 тем же режимом, что и первичная запись: файл несёт секрет
		// clash_api и адрес домашнего резолвера.
		if err := os.WriteFile(put, novoe, 0o600); err != nil {
			return fmt.Errorf("конфиг не записан: %w", err)
		}
		return nil
	}
	return fmt.Errorf("селектора %s в конфиге нет", genkonfig.TegSelector)
}

// kodPereklyucheniya различает причины, и у каждой своё действие человеку.
//
// Хвост уходит в kodSohraneniya НАМЕРЕННО: отказ записи набора это один и тот
// же сбой для всех команд, и называть его по-разному в зависимости от того,
// какая команда писала, значит завести ту самую путаницу, против которой
// написан otkazPodpiski.
func kodPereklyucheniya(err error) string {
	switch {
	case errors.Is(err, yadra.ErrTegaNetVYadre):
		return protokol.KodNuzhenPodyom
	// Раньше ErrNovyyVyborNeNesyot: ядро назвало причину отказа нового
	// сервера, и она точнее «не отвечает». Подключение при этом цело: в
	// ErrOtkatNeUdalsya проба обёрнута через %v, и туда эта ветка не попадает.
	case estOtkazZashchity(err):
		return protokol.KodZashchitaServera
	// Раньше ErrNovyyVyborNeNesyot: «этот сервер не отвечает, выбери другой».
	// На отвергнутом рукопожатии это ложный совет, другой сервер той же
	// подписки отвергнут ровно так же.
	case errors.Is(err, yadra.ErrServerOtvergKlyuchi):
		return protokol.KodServerAuthFailed
	// А вот здесь другой сервер как раз осмыслен: этот не ответил в срок, и
	// отказа от него не приходило.
	case errors.Is(err, yadra.ErrProbaNeUspela):
		return protokol.KodProbaNeUspela
	case errors.Is(err, ErrNovyyVyborNeNesyot):
		return protokol.KodNovyyNeNesyot
	// Отдельно от предыдущего: тот обещает целое подключение, этот его уже не
	// обещает. Код тот же, что служба поставила себе в состояние.
	case errors.Is(err, ErrOtkatNeUdalsya):
		return protokol.KodTunnelNeNeset
	case errors.Is(err, ErrServerNeNayd):
		return protokol.KodSelectedServerGone
	case errors.Is(err, ErrKandidatZanyat), errors.Is(err, errPravilaOtstali):
		return kodSohraneniya(err)
	default:
		return protokol.KodPereklyuchenieNeDoehalo
	}
}

// muNabor это ОДИН замок на весь набор.
//
// The set is a single blob in one DPAPI store, and every command runs on its
// own goroutine (obsluzhivanie.go). Read, edit, write without a lock is a lost
// update: addServer plus removeServer, addServer plus the subscription tick
// that needs no human at all. Measured before the lock existed: sixteen runs
// out of twenty lost one of the two edits.
//
// Пакетная переменная, а не поле службы, по двум причинам. Настоящий набор это
// один блоб на машину и одна служба в процессе, то есть замок на процесс это
// честная модель ресурса. И граница полос: поле пришлось бы заводить в
// komandy.go, который принадлежит соседней полосе (см. отчёт полосы В).
var muNabor sync.Mutex

// pravitNabor это ЕДИНСТВЕННАЯ дверь к правке набора.
//
// Восемь путей записи (addServer, removeServer, setSubscription,
// obnovitPodpisku вместе с расписанием, zapomnitVybor, setRouteMode, setRules и
// importProfilya) отличались только тем, ЧТО правят. Всё остальное у них
// одинаково: замок, чтение, заслон, запись, поле режима, пересборка правил.
// Восемь копий этого порядка означают, что девятая однажды забудет половину, и
// заметить это будет нечем.
//
// Замок держится на ВЕСЬ цикл, включая пересборку правил: правила строятся по
// списку, и собрать их по набору, который уже переписан соседом, значит
// разрешить брандмауэру адреса, которых в наборе нет.
//
// Замок НЕ реентрантный: под izmenit нельзя звать ничего, что само заходит в
// pravitNabor. Проверено грепом по всем восьми путям.
func (s *Sluzhba) pravitNabor(izmenit func(*Nabor) error) error {
	muNabor.Lock()
	defer muNabor.Unlock()

	n, err := s.nabor()
	if err != nil {
		return err
	}
	// Копия СПИСКА, а не только заголовка структуры: izmenit имеет право
	// заменить срез целиком, и заслону было бы не с чем сравнивать.
	staryy := Nabor{Servery: append([]protokol.Server(nil), n.Servery...), Podpiski: append([]ZapisPodpiski(nil), n.Podpiski...)}
	// Состав подписок ДО правки. Сравнение идёт по идентификаторам, а строка в
	// журнал пишется только при расхождении: набор пишут восемь путей и
	// расписание, и строка на каждую запись утонула бы в своём же шуме.
	sostavDo := sostavPodpisok(n)
	opisanieDo := opisatPodpiski(n)
	if err := izmenit(&n); err != nil {
		return err
	}
	if err := s.spisokNeTeryaetZhivyh(staryy, n); err != nil {
		return err
	}
	if err := s.zapisatNabor(n); err != nil {
		return err
	}
	for _, old := range staryy.Podpiski {
		if current := n.zapisPodpiski(old.Id); current == nil || current.Adres != old.Adres {
			delete(s.obnovleniyaPodpisok, old.Id)
		}
	}
	if !sostavySovpadayut(sostavDo, sostavPodpisok(n)) {
		log.Printf("состав подписок изменён: было [%s], стало [%s]; %s",
			opisanieDo, opisatPodpiski(n), sledSekretov())
	}
	return s.posleZapisiNabora(n)
}

// zamenitNaborBlobom это дверь ИМПОРТА профиля, восьмой путь записи.
//
// Отдельная от pravitNabor по двум причинам, и обе записаны решениями раньше.
// Импорт пишет расшифрованный блоб КАК ЕСТЬ: разобрать его в Nabor и собрать
// обратно значило бы терять поля профиля, выгруженного будущей версией
// (см. servery.go про проверку режима на чтении). И заслон
// spisokNeTeryaetZhivyh сюда не годится: он означал бы «нельзя импортировать
// профиль, пока подключён», то есть запрет вместо починки.
//
// Замок ТОТ ЖЕ. Восьмой путь мимо замка обессмыслил бы остальные семь.
func (s *Sluzhba) zamenitNaborBlobom(telo []byte) error {
	muNabor.Lock()
	defer muNabor.Unlock()
	var imported Nabor
	if err := json.Unmarshal(telo, &imported); err != nil {
		return fmt.Errorf("профиль не содержит корректный набор: %w", err)
	}
	if imported.Pravila.Trafik != nil {
		if _, err := proveritTrafik(*imported.Pravila.Trafik, *imported.Pravila.Trafik); err != nil {
			return err
		}
		// Прямые маршруты в профиле защите не мешают: под ней они не
		// применяются, а после выключения возвращаются в дело.
	}

	if err := s.sekretyPisat(telo); err != nil {
		return err
	}
	clear(s.obnovleniyaPodpisok)
	n, err := s.nabor()
	if err != nil {
		return err
	}
	return s.posleZapisiNabora(n)
}

// posleZapisiNabora это то, что обязано случиться после ЛЮБОЙ записи набора.
// Под muNabor.
func (s *Sluzhba) posleZapisiNabora(n Nabor) error {
	// Поле службы идёт за хранилищем.
	r, serty := rezhimNabora(n), sertySvoih(n, s.estKlyuchUpravleniya)
	s.mu.Lock()
	s.rezhim, s.rezhimNeProchitan, s.svoiSerty = r, false, serty
	s.mu.Unlock()
	// Отказ пересборки НЕ откатывает запись: список уже верен, а правила лишь
	// отстали. Откат вернул бы человеку старый список и оставил бы его в
	// уверенности, что команда не сработала вовсе.
	if err := s.PeresobratRazresheniya(); err != nil {
		// Оба %w: вызывающему нужен и признак «список всё же сохранён», и
		// причина, по которой правила отстали.
		return fmt.Errorf("%w: список сохранён, но правила брандмауэра отстали: %w",
			errPravilaOtstali, err)
	}
	return nil
}

var errPravilaOtstali = errors.New("правила не пересобраны")

// spisokNeTeryaetZhivyh допускает очистку каталога, когда адреса ядра
// защищены отдельным снимком. Без снимка сохраняет прежний запрет: иначе
// пересборка правил могла бы закрыть адрес работающего сервера.
func (s *Sluzhba) spisokNeTeryaetZhivyh(staryy, novyy Nabor) error {
	if adres, _ := s.dostupKKlash(); adres == "" {
		return nil
	}
	s.mu.Lock()
	estSnimok := s.serveryYadra != nil
	s.mu.Unlock()
	if estSnimok {
		return nil
	}
	servery := s.serveryDlyaRazresheniy(novyy.Servery)
	est := make(map[string]bool, len(servery))
	for _, srv := range servery {
		est[srv.Id] = true
	}
	for _, srv := range staryy.Servery {
		if !est[srv.Id] {
			return fmt.Errorf("%w: %s", ErrKandidatZanyat, srv.Id)
		}
	}
	return nil
}

var ErrKandidatZanyat = errors.New("сервер держит живое ядро: сначала отключись")

// ErrNovyyVyborNeNesyot: ядро команду приняло, но проба через новый выбор не
// прошла, И ВОЗВРАТ НА ПРЕЖНИЙ УДАЛСЯ. Только в этом случае подключение цело,
// и только его человеку можно называть «выбери другой сервер».
var ErrNovyyVyborNeNesyot = errors.New("новый сервер не несёт трафик")

// ErrOtkatNeUdalsya: проба через новый выбор не прошла И возврат на прежний
// тоже. Ядро осталось на кандидате, который не несёт, целого подключения нет,
// и служба переведена в ne-neset, откуда её поднимает восстановление.
var ErrOtkatNeUdalsya = errors.New("возврат на прежний сервер не прошёл, VPN не несёт трафик")

func kodSohraneniya(err error) string {
	// Раньше errPravilaOtstali, а не после: выключенный профиль это ПРИЧИНА,
	// по которой правила отстали, и человеку с ней делать нечего, кроме как
	// включить брандмауэр. Прежний firewall-failed звал повторять команду,
	// которая при выключенном профиле не сработает никогда.
	if errors.Is(err, set.ErrBrandmauerVyklyuchen) {
		return protokol.KodFirewallDisabled
	}
	if errors.Is(err, errPravilaOtstali) {
		return protokol.KodFirewallFailed
	}
	if errors.Is(err, ErrKandidatZanyat) {
		return protokol.KodKandidatZanyat
	}
	// Поиск сервера уехал ВНУТРЬ правки набора (pravitNabor), поэтому «такого
	// сервера нет» приходит сюда наравне с отказом записи. Код тот же, что и
	// прежде отдавал removeServer напрямую.
	if errors.Is(err, ErrServerNeNayd) {
		return protokol.KodSelectedServerGone
	}
	return protokol.KodSecretsUnreadable
}

// navyazatVybor ставит ядру тег текущего режима сразу после его старта.
//
// Ядро не обязано слушаться поля default: при включённом cache_file оно
// восстанавливает ПРОШЛЫЙ выбор (store_selected включён по умолчанию, см.
// подъём в komandy.go). Единственный способ сказать ядру правду это clash_api,
// и говорить надо на каждом подъёме, а не только при смене.
//
// Тег берётся тем же tegRezhima, что и у смены режима: два способа посчитать
// одно и то же разошлись бы молча, и разошлись бы именно на подъёме, где
// проверить некому.
// Замок тот же, что у смены сервера и смены режима: между чтением набора и
// вызовом в ядро стоит сеть. Без него смена сервера, пришедшая ровно во время
// подъёма, поставила бы ядру свой тег, а подъём следом вернул бы прежний, и
// человек получил бы отменённый выбор без единого признака.
func (s *Sluzhba) navyazatVybor(ctx context.Context, adres, sekret string) error {
	if adres == "" {
		return nil
	}
	s.muVybor.Lock()
	defer s.muVybor.Unlock()
	n, err := s.nabor()
	if err != nil {
		return err
	}
	// Режим подъёма, а не набора: подменённый выбранный сервер не должен
	// вернуться в ядро на подъёме, который восстановление затеяло ради подмены.
	teg, err := s.tegRezhima(s.rezhimPodyoma(n))
	if err != nil {
		return err
	}
	return s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, teg)
}
