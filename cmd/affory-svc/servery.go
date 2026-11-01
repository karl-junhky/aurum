package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Nabor это всё, что служба помнит про серверы между запусками.
//
// Лежит внутри блоба DPAPI одним куском: разложить по трём файлам значило бы
// завести три момента, в которые они могут разъехаться, и ни одного места, где
// это заметно.
type Nabor struct {
	Servery []protokol.Server `json:"servery"`
	// Podpiska это АДРЕС, то есть секрет класса ключа. Наружу он не отдаётся
	// никогда, только признак «задана» и имя узла для экрана.
	//
	// С 12.09.2026 поле только ЧИТАЕТСЯ, ради наборов прежних установок:
	// PrivestiPodpiski переносит его в список. Писать сюда больше нельзя, иначе
	// два источника правды про одну подписку разъедутся на первом же
	// переключении активной.
	Podpiska string `json:"podpiska,omitempty"`
	// Подписки списком: активная одна, остальные лежат про запас. Устройство и
	// причины в podpiski.go.
	Podpiski  []ZapisPodpiski `json:"podpiski,omitempty"`
	Aktivnaya string          `json:"aktivnaya_podpiska,omitempty"`
	Vybran    string          `json:"vybran,omitempty"`
	// Пустое поле это НЕ «неизвестно», а «человек про режим ничего не говорил».
	// Пишут его ровно три места, и каждое это след явного действия человека:
	// setRouteMode, zapomnitVybor и removeServer при чистке выбора. Бланкетная
	// запись при каждом сохранении набора сделала бы будущую смену умолчания
	// недоставляемой: расписание подписки пишет набор раз в 12 часов, и поле
	// проштамповалось бы в каждой установке.
	Rezhim protokol.Rezhim `json:"rezhim,omitempty"`
	// Правила исключений (волна 5). Рядом с серверами, а не отдельным файлом:
	// один путь записи, один заслон, одно хранилище.
	Pravila PravilaNabora `json:"pravila"`
	// Серверы, убранные человеком из автовыбора (A5). Списком исключений, а не
	// полем у сервера: записи из подписки пересобираются каждым обходом, и флаг
	// на них не пережил бы ни одного обновления.
	VneAvto []string `json:"vne_avto,omitempty"`
	// Свои серверы (svoy_vps.go): как войти и что стоит. Ключи VPN лежат в
	// Servery, запись ссылается на них по id.
	SvoiServery []SvoyServer `json:"svoi_servery,omitempty"`
}

// rezhimNabora отвечает, каким режимом маршрута жить набору.
//
// Выбранный вручную сервер означает ручной режим: иначе после перезапуска
// человек оказывается на «авто» вместо того, что выбрал, и решит, что настройка
// не сохранилась. Это записанное решение, см. TestVybrannyyServerStanovitsyaUmolchaniem
// в genkonfig.
func rezhimNabora(n Nabor) protokol.Rezhim {
	if n.Rezhim != "" {
		return n.Rezhim
	}
	if n.Vybran != "" {
		return protokol.RezhimRuchnoy
	}
	return rezhimPoUmolchaniyu
}

// Авто, переключено 02.09.2026 задачей Ш7-8, после того
// как заслоны Ш7-1..Ш7-6 встали. Пока умолчание было ручным, расхождение
// выбранного и несущего не стреляло, потому что они совпадали; теперь стреляет,
// и всё, что об этом говорит человеку, уже написано.
//
// Юнит-тесты эту константу НЕ охраняют: ни один из них не читает поле default
// собранного конфига. Охраняют два места, оба заведены раньше:
// TestVAvtoUmolchanieSelektoraEtoAvto в генераторе и живой прогон оснастки.
const rezhimPoUmolchaniyu = protokol.RezhimAvto

var (
	ErrNetServerov  = errors.New("серверов нет")
	ErrServerNeNayd = errors.New("сервер не найден")
)

// Паузы дочитывания хранилища. Переменная ради теста: её читает только
// горутина дочитывания, а заводит её один main.
var pauzyDochityvaniya = []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute}

// DochitatKhranilishche перечитывает в фоне хранилище, не прочитанное при
// создании службы (M4 аудита 1.8.0). Команды и подъём читают его сами на
// каждом вызове, а поле режима в статусе считалось один раз: без дочитывания
// окно до первой записи набора показывало бы режим умолчанием.
func (s *Sluzhba) DochitatKhranilishche() {
	s.mu.Lock()
	nuzhno := s.rezhimNeProchitan
	s.mu.Unlock()
	if !nuzhno || !s.zavestiFonovuyu() {
		return
	}
	fon.Zapustit("дочитывании хранилища", func() {
		defer s.fon.Done()
		for _, pauza := range pauzyDochityvaniya {
			select {
			case <-s.fonCtx.Done():
				return
			case <-time.After(pauza):
			}
			n, err := s.nabor()
			if err != nil {
				log.Printf("хранилище по-прежнему не читается: %v", err)
				continue
			}
			s.mu.Lock()
			if s.rezhimNeProchitan {
				s.rezhim, s.rezhimNeProchitan = rezhimNabora(n), false
			}
			s.mu.Unlock()
			log.Printf("хранилище прочитано после отказа при старте")
			return
		}
	})
}

// naborIzHranilishcha читает набор из хранилища секретов.
//
// Имя не совпадает с полем s.nabor намеренно: поле это ШОВ, а это его настоящее
// значение по умолчанию. Одно имя на оба Go и не позволил бы.
func (s *Sluzhba) naborIzHranilishcha() (Nabor, error) {
	telo, err := s.sekretyChitat()
	if err != nil {
		return Nabor{}, err
	}
	var n Nabor
	if len(telo) == 0 {
		// Пустое хранилище это первый запуск, а не поломка. Отказ здесь показал
		// бы secrets-unreadable человеку, который просто ещё не добавил сервер.
		return n, nil
	}
	if err := json.Unmarshal(telo, &n); err != nil {
		return Nabor{}, fmt.Errorf("набор серверов не разбирается: %w", err)
	}
	// Неизвестный режим это НЕ отказ читать набор: блоб мог прийти импортом
	// чужого профиля, а отказ оставил бы человека без единого сервера из-за
	// одной строки. Приводим к пустому, дальше его посчитает rezhimNabora.
	//
	// Проверка живёт ЗДЕСЬ, а не в команде, которая режим ставит: importProfilya
	// пишет расшифрованный блоб в sekretyPisat как есть, без разбора в Nabor.
	// Замерено: без этой строки мусорное значение проходит насквозь.
	if n.Rezhim != protokol.RezhimAvto && n.Rezhim != protokol.RezhimRuchnoy {
		n.Rezhim = ""
	}
	// Настройка рекламы приводится здесь по той же причине: importProfilya
	// пишет блоб как есть, а негодный уровень или имя роняли бы сборку правил.
	var pochinki []string
	n.Pravila.Reklama, pochinki = privestiReklamu(n.Pravila.Reklama)
	for _, p := range pochinki {
		skazatRedko("набор починен на чтении: " + p)
	}
	// Подписки приводятся ЗДЕСЬ, на единственном чтении, а не в каждой команде:
	// иначе всякая новая дверь к набору это шанс забыть приведение, а забытое
	// выглядит как «подписка не задана» при заданной подписке.
	// Починки говорят о себе вслух. Каждая из них меняет состав подписок или
	// выбор активной БЕЗ команды человека, то есть ровно то, что разбор жалобы
	// 20.09.2026 три раза подряд не смог ни увидеть, ни опровергнуть. Сито
	// внутри skazatRedko: чтение идёт на каждую команду, а починка, не доехавшая
	// до диска, повторяется на каждом чтении.
	for _, p := range n.PrivestiPodpiski() {
		skazatRedko("набор починен на чтении: " + p + "; " + sledSekretov())
	}
	// Правила сервисов приводятся ЗДЕСЬ же и по той же причине: каталог едет
	// внутри программы, и обновление оставляет в наборе правило на сервис,
	// которого больше нет. Без приведения такое правило роняло сборку конфига
	// на каждой попытке подключения (жалоба 21.09.2026).
	for _, p := range n.PrivestiPravila() {
		skazatRedko("набор починен на чтении: " + p)
	}
	// Чистка старых правил приложений идёт ПОСЛЕ снятия сирот и по той же
	// причине: список приложений и список сервисов слились, и один Discord не
	// должен лежать в обоих сразу. Чистка однократная, флаг внутри.
	for _, p := range n.snyatPravilaStavshieServisami() {
		skazatRedko("набор починен на чтении: " + p)
	}
	// Пути с номером версии внутри (Discord, Claude, ChatGPT) чинятся ПОСЛЕ
	// чистки: снятому правилу путь обновлять незачем. Трогает только те, чей
	// файл исчез, поэтому на здоровом наборе это один os.Stat на программу.
	for _, p := range n.osvezhitPutiProgramm() {
		skazatRedko("набор починен на чтении: " + p)
	}
	n.Servery = aktualnyeServery(n.Servery)
	for i := range n.Podpiski {
		n.Podpiski[i].Servery = aktualnyeServery(n.Podpiski[i].Servery)
	}
	// После чистки серверов, а не до: исключение автовыбора живо ровно до тех
	// пор, пока жив сервер, на который оно указывает.
	n.PrivestiAvtovybor()
	return n, nil
}

func (s *Sluzhba) zapisatNabor(n Nabor) error {
	telo, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("набор серверов не сериализуется: %w", err)
	}
	return s.sekretyPisat(telo)
}

// VybrannyyServer отдаёт сервер, к которому подключаемся.
//
// Если выбранного нет (первый запуск, либо выбранный убрали из подписки), берём
// первый по списку, а не отказываем: человек, у которого сервер один, не должен
// сначала «выбирать» его.
func (n Nabor) VybrannyyServer() (protokol.Server, error) {
	if len(n.Servery) == 0 {
		return protokol.Server{}, ErrNetServerov
	}
	for _, s := range n.Servery {
		if s.Id == n.Vybran {
			return s, nil
		}
	}
	return n.Servery[0], nil
}

// dlyaEkrana убирает из сервера всё, что не должно уезжать в канал.
//
// Канал пускает INTERACTIVE осознанно (интерфейс не требует админа), а список
// серверов при этом содержит uuid, публичный ключ, shortId и пароли. Отдавать
// их наружу значит раздавать доступ к VPN всякому, кто дотянулся до канала, при
// том что показывать на экране их всё равно нечего.
func dlyaEkrana(s protokol.Server) protokol.Server {
	return protokol.Server{
		Id: s.Id, Imya: s.Imya, Transport: s.Transport,
		Host: s.Host, Port: s.Port,
		IzPodpiski:                s.IzPodpiski,
		Uderzhan:                  s.Uderzhan,
		Svoy:                      s.Svoy,
		NebezopasnyyIgnorirovan:   s.NebezopasnyyIgnorirovan,
		OtpechatokNeProveryaetsya: s.OtpechatokNeProveryaetsya,
		SPinom:                    s.Pin != "",
	}
}
