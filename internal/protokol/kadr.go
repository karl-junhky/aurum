// The wire is dumber than it looks: 4 bytes of length, then UTF-8 JSON. No gRPC,
// no protobuf, no schema registry. One machine, one reader, one writer, and a
// version number for the day the shapes stop matching.
package protokol

import (
	"encoding/json"
	"time"
)

type Kadr struct {
	Tip  string `json:"tip"`  // "cmd" | "otvet" | "sobytie"
	Id   uint64 `json:"id"`   // echoed back in otvet; 0 for sobytie
	Imya string `json:"imya"` // command or event name
	// RawMessage, NOT []byte. A []byte field marshals to base64, so the body of
	// every command would travel as a string of gibberish and every debugging
	// session would start with decoding it by hand.
	Telo  json.RawMessage `json:"telo,omitempty"`
	Oshib *Oshibka        `json:"oshibka,omitempty"`
}

type Oshibka struct {
	Kod   string `json:"kod"`   // one of kody.go, never free text
	Tekst string `json:"tekst"` // human-facing, Russian, already localized
}

// Sostoyanie is what the tray icon, the main screen and the tests all agree on.
// The service owns this value. The UI never computes it from parts, because two
// places computing the same truth is how they start disagreeing.
type Sostoyanie string

const (
	SostSluzhbaMolchit Sostoyanie = "sluzhba-molchit"
	SostVyklyuchen     Sostoyanie = "vyklyuchen"
	SostPodnimaetsya   Sostoyanie = "podnimaetsya"
	SostPodnyat        Sostoyanie = "podnyat"
	SostNeNeset        Sostoyanie = "ne-neset"
	SostVosstanavl     Sostoyanie = "vosstanavlivaetsya"
	SostOtkaz          Sostoyanie = "otkaz"
)

// Rezhim это способ выбирать сервер, а не сам сервер. Значений ровно два, и
// пустая строка не третье: она означает «человек про режим ничего не говорил»,
// живёт только в хранилище и до провода не доезжает.
type Rezhim string

const (
	RezhimAvto    Rezhim = "avto"
	RezhimRuchnoy Rezhim = "ruchnoy"
)

type StatusOtvet struct {
	TrafikPoUmolchaniyu MarshrutTrafika `json:"trafik_po_umolchaniyu,omitempty"`
	Sostoyanie          Sostoyanie      `json:"sostoyanie"`
	// Выбранный и несущий это РАЗНЫЕ вопросы, и одно поле на оба врало сразу.
	// В ручном режиме ответ совпадает, в автоматическом расходится: человек не
	// выбирал ничего, а трафик несёт кто-то конкретный. Оба с omitempty, потому
	// что «выбора нет» это законное состояние, а не пустая строка на экране.
	VybranId     string `json:"vybran_id,omitempty"`
	NesushchiyId string `json:"nesushchiy_id,omitempty"`
	// Имя несущего, чтобы трей мог сказать «несёт vpn-pc-hy2» без похода за
	// списком: смена несущего в авто-режиме случается без команды человека.
	NesushchiyImya string `json:"nesushchiy_imya,omitempty"`
	// Имя выбранного сервера, который не отвечает и временно подменён группой
	// авто (ручной режим, с 17.10.2026). Пусто: подмены нет или несёт снова
	// выбранный. Окно говорит это вслух: молча отдать человеку другую страну
	// нельзя.
	VremennoVmesto string `json:"vremenno_vmesto,omitempty"`
	// БЕЗ omitempty, и довод ровно один: поле службы не бывает пустым по
	// построению. omitempty на непустом поле это мёртвая разметка, которая
	// однажды прикроет настоящую пустоту. Ссылаться на соседей нельзя: из
	// восьми полей структуры omitempty несут три, и каждое по своей причине.
	RezhimMarshruta Rezhim `json:"rezhim_marshruta"`
	KillSwitch      bool   `json:"kill_switch"`
	Avtozapusk      bool   `json:"avtozapusk"`
	// Два решения, а не одно (задача 4.8): «программа поднимается при входе»
	// это Avtozapusk, «туннель поднимается сам» это это поле. Умолчания
	// разные (§9.2: первое включено, второе выключено), поэтому один флаг на
	// оба обещал бы человеку подключение, которого он не выбирал.
	PodklyuchatPriStarte bool `json:"podklyuchat_pri_starte"`
	Zhurnal              bool `json:"zhurnal"`
	Diagnostika          bool `json:"diagnostika"`
	// Полоса канала человека в мегабитах, добавлено 05.09.2026. Пара нулей это
	// «не измерена», и экран обязан отличать её от объявленной: без объявления
	// hysteria2 идёт на BBR, и это рабочее состояние, а не пустое поле.
	PolosaVverh int `json:"polosa_vverh,omitempty"`
	PolosaVniz  int `json:"polosa_vniz,omitempty"`
	// Порт локального прокси рядом с туннелем (задача 4.11). Ноль, и в JSON
	// отсутствие, означает «не поднят»: занятый порт молча отключает прокси,
	// и без этого поля человек об этом не узнаёт никак.
	PortProksi   int        `json:"port_proksi,omitempty"`
	VersiyaSluzh string     `json:"versiya_sluzhby"`
	PodnyatS     *time.Time `json:"podnyat_s,omitempty"`
	Oshib        *Oshibka   `json:"oshibka,omitempty"`
	// Когда восстановление попробует снова (с 1.9.1). Есть только на паузе
	// между попытками, во время самой попытки пусто: окно пишет тогда
	// «пробую подключиться», а не отсчёт до прошедшего срока.
	SledPopytka *time.Time `json:"sled_popytka,omitempty"`
	// Версия программы (сборка выпуска), а не протокола: экран показывает её
	// в строке обновления. У сборки dev пустая.
	VersiyaProgrammy string `json:"versiya_programmy,omitempty"`
	// Найденное обновление и отметка последней проверки (план «шесть
	// удобств» §5). Obnovlenie есть только когда версия на сервере новее.
	Obnovlenie          *ObnovlenieOtvet `json:"obnovlenie,omitempty"`
	ObnovlenieProvereno *time.Time       `json:"obnovlenie_provereno,omitempty"`
	// Чем кончилась ПОСЛЕДНЯЯ проверка, если она не удалась. Пусто значит
	// «удалась», и отметка выше тогда свежая.
	//
	// Без этого поля мёртвая проверка выглядела здоровой: отметка ставится
	// только при удаче, и окно неделю показывало бы «проверено 20.09, новее
	// нет», пока расписание каждый час молча упирается в отказ сети (найдено
	// 23.09.2026 разбором D3).
	ObnovlenieOtkaz string `json:"obnovlenie_otkaz,omitempty"`
	// Reklama это состояние списка блокировки рекламы (28.09.2026). В status, а
	// не в listRules: окно опрашивает status каждые 5 с, а правила перечитывает
	// только при смене вкладки. nil значит «списка нет и отказов не было».
	Reklama *ReklamaSostoyanie `json:"reklama,omitempty"`
	// Pomehi это чужие программы, которые ломают VPN прямо сейчас (1.10.0):
	// перехватчик пакетов или чужой VPN с маршрутом по умолчанию. Только
	// группа «ломает»: антивирус окно называет в отказе «данные не пошли», а
	// не на главном экране. Пусто значит «не найдено».
	Pomehi []Pomeha `json:"pomehi,omitempty"`
	// На сколько секунд часы компьютера впереди точного времени (1.10.0),
	// отрицательное значит «отстают». Есть только при расхождении больше двух
	// минут: меньшее подключению не мешает, и строка о нём была бы шумом.
	ChasyRashozhdenie int `json:"chasy_rashozhdenie,omitempty"`
	// SvoiTrevogi это беды сертификатов своих серверов (1.11.0, §3.4 плана
	// «Свой VPN»): продление не выходит или срок кончается. Здесь, а не
	// только во вкладке «Свой VPN»: её месяцами не открывают.
	SvoiTrevogi []SvoyaTrevoga `json:"svoi_trevogi,omitempty"`
}

// SvoyaTrevoga это тревога о своём сервере. Tekst готов целиком, Imya для
// заголовка строки, Id - чтобы окно открыло карточку.
type SvoyaTrevoga struct {
	Id    string `json:"id"`
	Imya  string `json:"imya"`
	Tekst string `json:"tekst"`
}

// Pomeha это одна программа из группы «ломает». Vid выбирает фразу окна:
// obhod просят выключить, vpn отключить.
type Pomeha struct {
	Imya string `json:"imya"`
	Vid  string `json:"vid"`
}

// ReklamaSostoyanie описывает список, который ЛЕЖИТ В ФАЙЛЕ, а не выбранный
// человеком: пока новый уровень готовится, они расходятся.
type ReklamaSostoyanie struct {
	Uroven     string     `json:"uroven"`
	Pravil     int        `json:"pravil"`
	Versiya    string     `json:"versiya,omitempty"`
	Sobran     *time.Time `json:"sobran,omitempty"`
	Proveren   *time.Time `json:"proveren,omitempty"`
	Vstroennyy bool       `json:"vstroennyy,omitempty"`
	// БЕЗ omitempty: false значит «конфиг живого ядра блока не несёт».
	Deystvuet bool `json:"deystvuet"`
	// Голая причина последнего отказа, БЕЗ префикса: фразу вокруг неё строит
	// окно. Держится до первой удачи, как ObnovlenieOtkaz.
	Otkaz string `json:"otkaz,omitempty"`
}

// ObnovlenieOtvet это то, что служба знает о новой версии на сервере
// обновлений: номер, размер архива и когда это узнано.
type ObnovlenieOtvet struct {
	Versiya   string    `json:"versiya"`
	Razmer    int64     `json:"razmer"`
	Provereno time.Time `json:"provereno"`
}

// Шаги обновления. Ответ на downloadUpdate приходит ОДИН и в самом конце, а
// между нажатием и им лежат два десятка мегабайт и подмена файлов. Без событий
// окно всё это время показывает человеку ровно ничего.
const (
	ShagSkachivanie = "skachivanie"
	ShagSverka      = "sverka"
	ShagRaspakovka  = "raspakovka"
	// ShagPodmena последний, который успевает уйти: сразу за ним служба
	// останавливается сама, и следующую новость окно узнаёт, когда новая версия
	// поднимется и ответит. Ожидание этих секунд ведёт окно, а не служба.
	ShagPodmena = "podmena"
	ShagOtkaz   = "otkaz"
)

// HodObnovleniya это тело события obnovlenie-hod.
type HodObnovleniya struct {
	Shag string `json:"shag"`
	// Versiya выпуска, ради которого всё затеяно: окно подписывает им полосу,
	// потому что своя версия в это время уже ничего не значит.
	Versiya string `json:"versiya,omitempty"`
	// Доля загрузки. Vsego берётся из описания выпуска, поэтому известна до
	// первого байта; ноль значит, что сервер размера не назвал.
	Skachano int64 `json:"skachano,omitempty"`
	Vsego    int64 `json:"vsego,omitempty"`
	// SrokS у шага подмены: сколько секунд служба даёт новой версии на ответ,
	// прежде чем вернуть прежнюю.
	SrokS int `json:"srok_s,omitempty"`
	// Tekst у отказа: та же причина, что ушла бы в ответ.
	Tekst string `json:"tekst,omitempty"`
}
