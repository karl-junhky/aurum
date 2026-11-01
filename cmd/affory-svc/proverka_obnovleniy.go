package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Проверка обновлений (план «шесть удобств» §5).
//
// Источник один: последний выпуск этого репозитория на GitHub. Служба читает
// описание выпуска через API, находит в нём архив affory-X.Y.Z.zip и файл
// .sha256 рядом, сравнивает версию из тега со своей. Запрос идёт НАПРЯМУЮ,
// как снимок состояния: обновление нужно и тогда, когда туннель лежит.
// Без токена GitHub даёт 60 запросов в час с адреса; раз в сутки это ничто.
const AdresObnovleniyPoUmolchaniyu = "https://api.github.com/repos/zxczxczxczxczxczxc1111/affory/releases/latest"

const (
	predelOpisaniya  = 256 << 10
	predelHesha      = 4 << 10
	predelArhivaSeti = 64 << 20
	// Срок ОДНОЙ попытки, а не всей проверки. Прежние двадцать секунд были
	// сроком единственной, и повтором служил человек: замерено на живой машине
	// 19 и 20.09.2026, `checkUpdate` отказывал, а следующее нажатие через
	// пятнадцать секунд проходило, причём между ними не менялось ничего.
	//
	// Запрос за описанием выпуска идёт МИМО туннеля всегда: конфиг ядра уводит
	// affory-svc.exe в direct, иначе обновиться было бы нельзя при лежащем
	// туннеле. Значит канал до GitHub тут ровно такой, каким его даёт
	// провайдер, и одиночная попытка на нём это лотерея.
	srokPopytkiProverki = 10 * time.Second
	// Три попытки с отступами 1 и 2 секунды это худшие 33 секунды при сроке
	// ответа команды 60 (protokol.SrokDolgoy). Больше в срок не влезает, а
	// нужды в большем нет: канал либо отвечает, либо лежит дольше, чем человек
	// готов ждать у окна.
	popytokProverki   = 3
	srokZagruzki      = 3 * time.Minute
	pervayaProverka   = time.Minute
	periodProverki    = 24 * time.Hour
	otstupProverki    = time.Hour
	katalogObnovleniy = "obnovleniya"
)

// vypuskGitHub это та часть ответа releases/latest, которая здесь нужна.
type vypuskGitHub struct {
	Teg   string `json:"tag_name"`
	Fayly []struct {
		Imya   string `json:"name"`
		Razmer int64  `json:"size"`
		Adres  string `json:"browser_download_url"`
	} `json:"assets"`
}

// svedeniyaVypuska это то, что служба знает о последнем выпуске: версия,
// имя и адрес архива, его хеш из файла .sha256 рядом.
type svedeniyaVypuska struct {
	Versiya     string
	Arhiv       string
	AdresArhiva string
	Sha256      string
	Razmer      int64
}

var errNetVersii = errors.New("сборка без версии обновления не проверяет")

// oshibkaSeti помечает отказ, который имеет смысл повторить: поход наружу не
// состоялся.
//
// Разделение обязательно, иначе повторы стали бы вредом. Негодное описание
// выпуска (тег не вида X.Y.Z, нет архива по https, .sha256 не шестнадцатеричный)
// на второй заход придёт ровно таким же, а человек прождёт три срока вместо
// одного и получит тот же отказ.
type oshibkaSeti struct{ err error }

func (o oshibkaSeti) Error() string { return o.err.Error() }
func (o oshibkaSeti) Unwrap() error { return o.err }

// versiyaDlyaEkrana: dev это не версия, экран покажет пустоту, а не «dev».
func versiyaDlyaEkrana() string {
	if versiyaProgrammy == "dev" {
		return ""
	}
	return versiyaProgrammy
}

// novee сравнивает X.Y.Z по числам. Нечисловое считается не новее: лучше
// промолчать, чем предложить «обновиться» на строку.
func novee(kandidat, tekushchaya string) bool {
	a, okA := razobratVersiyu(kandidat)
	b, okB := razobratVersiyu(tekushchaya)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func razobratVersiyu(v string) ([3]int, bool) {
	var itog [3]int
	ch := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(ch) != 3 {
		return itog, false
	}
	for i, c := range ch {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 {
			return itog, false
		}
		itog[i] = n
	}
	return itog, true
}

// skachatPoSeti это настоящая загрузка с потолком: адреса файлов приходят из
// описания выпуска, и страница-заглушка на их месте не должна занять диск.
// hod зовётся по ходу чтения тела и может быть nil: описание выпуска и файл
// суммы весят килобайты, полосу по ним не рисуют.
func skachatPoSeti(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
	return skachatCherez(ctx, "", adres, predel, hod)
}

// skachatCherez это та же загрузка, но с выбором дороги: пустой proksi значит
// «наружу как есть», непустой отправляет запрос в локальный вход туннеля.
func skachatCherez(ctx context.Context, proksi, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
	z, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
	if err != nil {
		return nil, err
	}
	z.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	z.Header.Set("User-Agent", "affory/"+versiyaProgrammy)
	klient := &http.Client{}
	if proksi != "" {
		u, err := url.Parse("http://" + proksi)
		if err != nil {
			return nil, fmt.Errorf("адрес локального входа не разобран: %w", err)
		}
		// Своя копия транспорта, а не правка общего: DefaultTransport один на
		// процесс, и прокси в нём увёл бы в туннель заодно подписку и замеры.
		tr := &http.Transport{Proxy: http.ProxyURL(u)}
		// Транспорт на один вызов: его соединения закрываются вместе с ним
		// (L8 аудита 1.8.0).
		defer tr.CloseIdleConnections()
		klient = &http.Client{Transport: tr}
	}
	o, err := klient.Do(z)
	if err != nil {
		return nil, err
	}
	defer o.Body.Close()
	if o.StatusCode == http.StatusNotFound {
		// 404 у этого адреса означает не поломку, а «смотреть нечего»:
		// репозиторий закрыт либо выпусков в нём пока нет. Голое «код ответа
		// 404» человек читает как отказ программы и идёт чинить не то.
		return nil, fmt.Errorf("код ответа 404: список выпусков не отдан, репозиторий закрыт или выпусков нет")
	}
	if o.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("код ответа %d", o.StatusCode)
	}
	var telo io.Reader = io.LimitReader(o.Body, predel+1)
	if hod != nil {
		telo = &schetchikChteniya{iz: telo, vsego: o.ContentLength, hod: hod}
	}
	b, err := io.ReadAll(telo)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > predel {
		return nil, fmt.Errorf("ответ больше потолка %d байт", predel)
	}
	return b, nil
}

// otstupHoda между докладами о доле. Каждый прочитанный кусок это событие всем
// подписчикам и перерисовка окна; двадцать мегабайт дали бы их тысячи.
const otstupHoda = 250 * time.Millisecond

// schetchikChteniya считает прочитанное и докладывает не чаще отступа. Последний
// доклад приходит с концом тела, поэтому полоса доходит до края, а не замирает
// на девяноста восьми процентах.
type schetchikChteniya struct {
	iz       io.Reader
	vsego    int64
	bylo     int64
	posledny time.Time
	hod      func(bylo, vsego int64)
}

func (s *schetchikChteniya) Read(p []byte) (int, error) {
	n, err := s.iz.Read(p)
	s.bylo += int64(n)
	if err == io.EOF || time.Since(s.posledny) >= otstupHoda {
		s.posledny = time.Now()
		s.hod(s.bylo, s.vsego)
	}
	return n, err
}

// proksiCherezTunnel отдаёт адрес локального входа ядра, когда идти ЧЕРЕЗ СВОЙ
// ЖЕ туннель можно. Зовут её двое: проверка обновлений и загрузка подписки.
//
// Конфиг ядра уводит affory-svc.exe в direct, и это правильно: обновляться
// нужно и при лежащем туннеле, а замыкать службу на туннель, которого нет,
// значит не обновиться никогда. Побочный эффект вскрылся 20.09.2026 на живой
// машине: единственная программа, которая ходит к GitHub по каналу
// провайдера, это сам VPN-клиент. У соседнего скриншотера, чей трафик
// перехватывает TUN, то же обновление проходит всегда, а у нас github.com
// рвал соединение на файле .sha256.
//
// Поэтому при ПОДНЯТОМ туннеле запрос идёт в локальный вход (`proksi-in` в
// конфиге ядра маршрутизируется в `vybor`), то есть ровно тем путём, что у
// всех остальных программ машины. Порт ноль это законный случай: прокси
// надстройка, и туннель поднимают без него, когда 10809 занят чужим клиентом.
func (s *Sluzhba) proksiCherezTunnel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sost != protokol.SostPodnyat || s.portProksiNash <= 0 {
		return ""
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.portProksiNash))
}

// skachatVypusk это шов загрузки с откатом: сначала через туннель, если он
// поднят, при отказе напрямую.
//
// Откат обязателен. Туннель бывает поднят и нездоров, и «обновляться только
// через VPN» означало бы, что сломанный туннель нельзя починить обновлением.
// Половина срока каждому: походу через туннель нельзя съедать время прямого,
// иначе откат существует только на бумаге.
func (s *Sluzhba) skachatVypusk(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
	proksi := s.proksiCherezTunnel()
	if proksi == "" {
		return skachatPoSeti(ctx, adres, predel, hod)
	}
	do, otm := polovinaSroka(ctx)
	b, err := skachatCherez(do, proksi, adres, predel, hod)
	otm()
	if err == nil {
		return b, nil
	}
	// Отмена СВЕРХУ это не повод идти второй дорогой: человек закрыл окно или
	// служба гасится, и прямой поход только задержит уборку.
	if ctx.Err() != nil {
		return nil, err
	}
	log.Printf("выпуск через туннель не пришёл (%v), иду напрямую", err)
	return skachatPoSeti(ctx, adres, predel, hod)
}

// polovinaSroka делит остаток времени пополам. Без дедлайна сверху делить
// нечего, и первый же поход имеет право занять всё время.
func polovinaSroka(ctx context.Context) (context.Context, context.CancelFunc) {
	dl, est := ctx.Deadline()
	if !est {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, time.Until(dl)/2)
}

// posledniyVypusk читает описание последнего выпуска и файл .sha256 рядом с
// архивом. Ошибка любого шага это ошибка проверки, не «обновлений нет».
func (s *Sluzhba) posledniyVypusk(ctx context.Context) (*svedeniyaVypuska, error) {
	b, err := s.skachatFayl(ctx, s.adresObnovleniy, predelOpisaniya, nil)
	if err != nil {
		return nil, oshibkaSeti{fmt.Errorf("описание выпуска не получено: %w", err)}
	}
	var v vypuskGitHub
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("описание выпуска не разобрано: %w", err)
	}
	versiya := strings.TrimPrefix(strings.TrimSpace(v.Teg), "v")
	if _, ok := razobratVersiyu(versiya); !ok {
		return nil, fmt.Errorf("тег выпуска %q не вида X.Y.Z", v.Teg)
	}
	arhiv := "affory-" + versiya + ".zip"
	sv := &svedeniyaVypuska{Versiya: versiya, Arhiv: arhiv}
	var adresHesha string
	for _, f := range v.Fayly {
		switch f.Imya {
		case arhiv:
			sv.AdresArhiva, sv.Razmer = f.Adres, f.Razmer
		case arhiv + ".sha256":
			adresHesha = f.Adres
		}
	}
	// Адреса файлов пришли из описания, но качать по ним можно только https:
	// иначе подмена описания по дороге превратилась бы в загрузку с чужого узла.
	if !strings.HasPrefix(sv.AdresArhiva, "https://") || !strings.HasPrefix(adresHesha, "https://") {
		return nil, fmt.Errorf("в выпуске %s нет %s с .sha256 по https", versiya, arhiv)
	}
	h, err := s.skachatFayl(ctx, adresHesha, predelHesha, nil)
	if err != nil {
		return nil, oshibkaSeti{fmt.Errorf("%s.sha256 не получен: %w", arhiv, err)}
	}
	polya := strings.Fields(string(h))
	if len(polya) == 0 || len(polya[0]) != 64 {
		return nil, fmt.Errorf("%s.sha256 не содержит sha256", arhiv)
	}
	if _, err := hex.DecodeString(polya[0]); err != nil {
		return nil, fmt.Errorf("%s.sha256 не шестнадцатеричный", arhiv)
	}
	sv.Sha256 = strings.ToLower(polya[0])
	return sv, nil
}

// posledniyVypuskUporno это posledniyVypusk, который не сдаётся с первого раза.
//
// Срок вешается на КАЖДУЮ попытку, а не на весь заход: общий срок на три
// попытки означал бы, что первая, упёршаяся в таймаут, съедает время двух
// остальных, то есть повторов бы не было вовсе.
//
// Повторяются только отказы сети. Негодный выпуск возвращается сразу: см.
// oshibkaSeti.
func (s *Sluzhba) posledniyVypuskUporno(ctx context.Context) (*svedeniyaVypuska, error) {
	var posledn error
	for i := 0; i < popytokProverki; i++ {
		do, otm := context.WithTimeout(ctx, srokPopytkiProverki)
		v, err := s.posledniyVypusk(do)
		otm()
		if err == nil {
			if i > 0 {
				log.Printf("выпуск прочитан с попытки %d из %d", i+1, popytokProverki)
			}
			return v, nil
		}
		posledn = err
		var seti oshibkaSeti
		if !errors.As(err, &seti) {
			return nil, err
		}
		// Причина КАЖДОЙ попытки в журнал. Журнал команд пишет один код отказа,
		// а текст жил только в окне и исчезал вместе с ним: разбор жалобы
		// 20.09.2026 уткнулся ровно в это - отказы в журнале были, а чем они
		// вызваны, восстановить было нечем.
		log.Printf("чтение выпуска, попытка %d из %d: %v", i+1, popytokProverki, err)
		// Отступ удваивается от секунды. Отмена общего контекста (человек
		// закрыл окно, служба гасится) обрывает повторы: ждать некому.
		if i < popytokProverki-1 && !s.zhdat(ctx, time.Duration(1<<i)*time.Second) {
			break
		}
	}
	return nil, posledn
}

// proveritObnovlenie читает последний выпуск и запоминает находку. Ошибка сети
// это ошибка, а «новее нет» это nil без ошибки; отметка проверки ставится в
// обоих удачных случаях.
func (s *Sluzhba) proveritObnovlenie(ctx context.Context) (*protokol.ObnovlenieOtvet, error) {
	if versiyaProgrammy == "dev" {
		return nil, errNetVersii
	}
	v, err := s.posledniyVypuskUporno(ctx)
	if err != nil {
		// Причина остаётся в состоянии, а не только в журнале. Отметка проверки
		// ставится ТОЛЬКО при удаче, поэтому без этой строки окно неделю
		// показывало бы «проверено 20.09, новее нет» на проверке, которая с
		// тех пор падает каждый час.
		s.mu.Lock()
		s.obnovlenieOtkaz = tekstIz("проверка обновлений", err)
		s.mu.Unlock()
		return nil, err
	}
	seychas := s.seychas()
	var nahodka *protokol.ObnovlenieOtvet
	if novee(v.Versiya, versiyaProgrammy) {
		nahodka = &protokol.ObnovlenieOtvet{Versiya: v.Versiya, Razmer: v.Razmer, Provereno: seychas}
	}
	s.mu.Lock()
	bylo := s.obnovlenie
	s.obnovlenie, s.obnovlenieProvereno = nahodka, &seychas
	s.obnovlenieOtkaz = ""
	s.mu.Unlock()
	// Новая версия это событие состояния: трей скажет об этом один раз.
	if nahodka != nil && (bylo == nil || bylo.Versiya != nahodka.Versiya) {
		log.Printf("последний выпуск %s, у нас %s", nahodka.Versiya, versiyaProgrammy)
		s.izvestit("state", s.Status())
	}
	return nahodka, nil
}

// raspisanieObnovleniy: через минуту после старта, затем раз в сутки; при
// отказе сети через час. Сборка dev не проверяет ничего.
func (s *Sluzhba) raspisanieObnovleniy(ctx context.Context) {
	if versiyaProgrammy == "dev" {
		return
	}
	pauza := pervayaProverka
	for {
		if !s.zhdat(ctx, pauza) {
			return
		}
		if _, err := s.proveritObnovlenie(ctx); err != nil {
			log.Printf("проверка обновления не удалась: %v", err)
			pauza = otstupProverki
			continue
		}
		pauza = periodProverki
	}
}

func (s *Sluzhba) checkUpdate(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	if _, err := s.proveritObnovlenie(ctx); err != nil {
		// Итог нажатия в журнал службы. Журнал команд хранит только код
		// `update-check-failed`, и по нему видно, что человек жал кнопку
		// трижды, но не видно, обо что он бился.
		log.Printf("проверка обновления по команде не удалась: %v", err)
		return otkazIz(k, protokol.KodObnovlenieNeProvereno, err)
	}
	return otvet(k.Id, k.Imya, s.Status())
}

// downloadUpdate качает архив последнего выпуска в каталог данных, кладёт
// рядом .sha256 из выпуска и ставит его тем же путём, что installUpdate.
func (s *Sluzhba) downloadUpdate(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	// Скачанное сразу ставится, поэтому отказ до загрузки, а не после неё.
	if tekst := s.svoyVpsIdyot(); tekst != "" {
		return otkaz(k.Id, k.Imya, protokol.KodVpsZanyato, tekst)
	}
	do, otm := context.WithTimeout(ctx, srokZagruzki)
	defer otm()
	// Описание выпуска читается теми же повторами, что и при проверке: «скачать»
	// упирается в тот же прямой канал до GitHub, и падать на первой осечке
	// здесь так же нечестно.
	v, err := s.posledniyVypuskUporno(do)
	if err != nil {
		log.Printf("загрузка обновления не началась, выпуск не прочитан: %v", err)
		return otkazIz(k, protokol.KodObnovlenieNeProvereno, err)
	}
	if !novee(v.Versiya, versiyaProgrammy) {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, fmt.Sprintf("последний выпуск %s, это не новее %s", v.Versiya, versiyaProgrammy))
	}
	s.hodObnovleniya(protokol.HodObnovleniya{Shag: protokol.ShagSkachivanie, Versiya: v.Versiya, Vsego: v.Razmer})
	arhiv, err := s.skachatFayl(do, v.AdresArhiva, predelArhivaSeti, func(bylo, vsego int64) {
		if vsego <= 0 {
			vsego = v.Razmer
		}
		s.hodObnovleniya(protokol.HodObnovleniya{
			Shag: protokol.ShagSkachivanie, Versiya: v.Versiya, Skachano: bylo, Vsego: vsego,
		})
	})
	if err != nil {
		return s.otkazObnovleniya(k, protokol.KodObnovlenieNeSkachano, "архив не скачан: "+err.Error())
	}
	s.hodObnovleniya(protokol.HodObnovleniya{Shag: protokol.ShagSverka, Versiya: v.Versiya})
	if fakt := sha256.Sum256(arhiv); hex.EncodeToString(fakt[:]) != v.Sha256 {
		return s.otkazObnovleniya(k, protokol.KodObnovlenieNeSkachano, "sha256 скачанного архива не совпал с .sha256 выпуска")
	}
	kat := filepath.Join(s.dirDannyh, katalogObnovleniy)
	if err := os.MkdirAll(kat, 0o755); err != nil {
		return s.otkazObnovleniya(k, protokol.KodObnovlenieNeSkachano, "каталог обновлений не создан: "+err.Error())
	}
	put := filepath.Join(kat, v.Arhiv)
	if err := os.WriteFile(put, arhiv, 0o644); err != nil {
		return s.otkazObnovleniya(k, protokol.KodObnovlenieNeSkachano, "архив не записан: "+err.Error())
	}
	if err := os.WriteFile(put+".sha256", []byte(v.Sha256+"  "+v.Arhiv+"\n"), 0o644); err != nil {
		return s.otkazObnovleniya(k, protokol.KodObnovlenieNeSkachano, "хеш не записан: "+err.Error())
	}
	return s.ustanovitArhivSVersiey(k, put, v.Versiya)
}
