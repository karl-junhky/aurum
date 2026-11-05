package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Kvitanciya это ответ программы сервера на пакет (yadro.Kvitanciya в uzel).
type Kvitanciya struct {
	// Hesh это sha256 пакета, который работает сейчас.
	Hesh string `json:"hesh"`
	// Kak: без изменений, на лету, перезапуск, ядро прежнее.
	Kak        string `json:"kak,omitempty"`
	YadroZhivo bool   `json:"yadro_zhivo"`
	// Oshibka это отказ программы: пакет не применён, работает прежний.
	Oshibka string `json:"oshibka,omitempty"`
	// Zanyat это порт, который ядро пакета не заняло: его держит чужая
	// программа. Подбор тогда берёт следующего кандидата.
	Zanyat *Zanyat `json:"zanyat,omitempty"`
}

// Zanyat это порт, не доставшийся ядру пакета.
type Zanyat struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"`
	// Vhod это тег входа пакета на этом порту: reality-443, hy2-8443.
	Vhod     string `json:"vhod,omitempty"`
	Prichina string `json:"prichina"`
}

// Primenit отдаёт пакет программе сервера и сверяет квитанцию: работает
// ровно присланный пакет и ядро живо. Отказ программы приходит *Oshibka с
// квитанцией рядом: в ней Zanyat, если пакету не хватило порта.
func Primenit(ctx context.Context, m Mashina, paket []byte) (k Kvitanciya, err error) {
	defer zashchita("Primenit", &err)
	if len(paket) == 0 {
		return Kvitanciya{}, errors.New("пустой пакет")
	}
	it, err := m.Vypolnit(ctx, putProgrammy+" pult primenit", bytes.NewReader(paket))
	if err != nil {
		return Kvitanciya{}, err
	}
	// Отказ пульта и отказ применения приходят одним полем oshibka с кодом
	// выхода 1.
	if jerr := json.Unmarshal(it.Vyvod, &k); jerr != nil {
		if it.Kod != 0 {
			return Kvitanciya{}, oshibkaSkripta("primenit", it)
		}
		return Kvitanciya{}, &Oshibka{Shag: "primenit", Tekst: "квитанция не разобрана: " + chisto(jerr.Error())}
	}
	if k.Zanyat != nil {
		k.Zanyat.Vhod, k.Zanyat.Prichina = chisto(k.Zanyat.Vhod), chisto(k.Zanyat.Prichina)
	}
	switch {
	case k.Oshibka != "":
		k.Oshibka = chisto(k.Oshibka)
		return k, &Oshibka{Shag: "primenit", Kod: it.Kod, Tekst: k.Oshibka}
	case it.Kod != 0:
		return k, oshibkaSkripta("primenit", it)
	case k.Hesh != heshPaketa(paket):
		return k, &Oshibka{Shag: "primenit", Tekst: "программа сервера работает не с присланным пакетом"}
	case !k.YadroZhivo:
		return k, &Oshibka{Shag: "primenit", Tekst: "ядро программы сервера не поднялось с пакетом"}
	}
	return k, nil
}

// heshPaketa считает хэш так же, как программа сервера (yadro.Hesh).
func heshPaketa(paket []byte) string {
	h := sha256.Sum256(paket)
	return hex.EncodeToString(h[:])
}

// SostoyanieUzla это ответ программы сервера на sostoyanie: что работает и
// как идёт выпуск своего сертификата.
type SostoyanieUzla struct {
	Versiya    string `json:"versiya"`
	Hesh       string `json:"hesh"`
	YadroZhivo bool   `json:"yadro_zhivo"`
	// Oshibka это последний отказ ядра.
	Oshibka string `json:"oshibka,omitempty"`
	// ProbaDo: идёт проба портов, до этого времени (Unix-секунды).
	ProbaDo int64 `json:"proba_do,omitempty"`
	// Sert пуст, если в пакете нет самостоятельного выпуска.
	Sert *SertUzla `json:"sert_ip_sam,omitempty"`
	// Maska пуст у программы старше 0.7.0, у ядра без reality и в режиме
	// своего домена.
	Maska *StorozhMaski `json:"maska,omitempty"`
	// Sayt это сайт режима своего домена (0.7.0); простой режим - nil.
	Sayt *SaytUzla `json:"sayt,omitempty"`
}

// SaytUzla это сайт режима домена глазами программы сервера (sayt.Sostoyanie
// в uzel).
type SaytUzla struct {
	Domen string `json:"domen"`
	// Https: поднят HTTPS для reality, Http: поднят 80/tcp.
	Https bool `json:"https"`
	Http  bool `json:"http"`
	// Sert: сайту есть чем ответить на домене.
	Sert    bool   `json:"sert"`
	Oshibka string `json:"oshibka,omitempty"`
	// Zabrana: когда подписку последний раз забирали, Unix-секунды.
	Zabrana int64 `json:"zabrana,omitempty"`
}

// SertUzla это выпуск сертификата на IP глазами программы сервера
// (sertip.Sostoyanie в uzel). Время в Unix-секундах.
type SertUzla struct {
	IP string `json:"ip"`
	// Domen: выпуск на имя домена (режим своего домена), а не на IP.
	Domen   string `json:"domen,omitempty"`
	Staging bool   `json:"staging,omitempty"`
	// Est: настоящий сертификат на этот адрес лежит на диске.
	Est bool  `json:"est"`
	Ot  int64 `json:"ot,omitempty"`
	Do  int64 `json:"do,omitempty"`
	// Sposob это путь проверки, которым получен текущий: http-01 или
	// tls-alpn-01.
	Sposob string `json:"sposob,omitempty"`
	Idyot  bool   `json:"idyot,omitempty"`
	// Popytka это когда была последняя попытка, удачная или нет.
	Popytka int64 `json:"popytka,omitempty"`
	// Otkaz это чем кончилась последняя попытка, если неудачей; OtkazS - с
	// какого времени отказы идут подряд.
	Otkaz          string `json:"otkaz,omitempty"`
	OtkazS         int64  `json:"otkaz_s,omitempty"`
	Sleduyushchaya int64  `json:"sleduyushchaya,omitempty"`
	// Trevoga: сертификата нет и выпуск уже отказывал, или до конца срока
	// меньше суток, или продлить пора, а не выходит дольше шести часов.
	Trevoga bool `json:"trevoga,omitempty"`
}

// Sostoyanie спрашивает программу сервера, как она.
func Sostoyanie(ctx context.Context, m Mashina) (s SostoyanieUzla, err error) {
	defer zashchita("Sostoyanie", &err)
	it, err := m.Vypolnit(ctx, putProgrammy+" pult sostoyanie", nil)
	if err != nil {
		return SostoyanieUzla{}, err
	}
	// Код 1 с полем oshibka это отказ пульта; то же поле при коде 0 это
	// последний отказ ядра, часть ответа.
	if it.Kod != 0 {
		var o struct {
			Oshibka string `json:"oshibka"`
		}
		if json.Unmarshal(it.Vyvod, &o) == nil && o.Oshibka != "" {
			return SostoyanieUzla{}, &Oshibka{Shag: "sostoyanie", Kod: it.Kod, Tekst: chisto(o.Oshibka)}
		}
		return SostoyanieUzla{}, oshibkaSkripta("sostoyanie", it)
	}
	if err := json.Unmarshal(it.Vyvod, &s); err != nil {
		return SostoyanieUzla{}, &Oshibka{Shag: "sostoyanie", Tekst: "ответ не разобран: " + chisto(err.Error())}
	}
	if s.Versiya == "" {
		return SostoyanieUzla{}, &Oshibka{Shag: "sostoyanie", Tekst: "в ответе нет версии программы"}
	}
	s.Versiya, s.Oshibka = chisto(s.Versiya), chisto(s.Oshibka)
	if s.Sert != nil {
		s.Sert.Sposob, s.Sert.Otkaz = chisto(s.Sert.Sposob), chisto(s.Sert.Otkaz)
	}
	if s.Maska != nil {
		s.Maska.Imya, s.Maska.Prichina = chisto(s.Maska.Imya), chisto(s.Maska.Prichina)
	}
	return s, nil
}

// StorozhMaski это маска reality глазами программы сервера (maska.Storozh в
// uzel, МА3): она проверяет маску раз в час и после трёх провалов подряд
// ставит тревогу. Время в Unix-секундах.
type StorozhMaski struct {
	Imya string `json:"imya"`
	// Provereno ноль - эту маску программа ещё не проверяла.
	Provereno int64  `json:"provereno,omitempty"`
	Godna     bool   `json:"godna"`
	Provalov  int    `json:"provalov,omitempty"`
	ProvalS   int64  `json:"proval_s,omitempty"`
	Prichina  string `json:"prichina,omitempty"`
	Trevoga   bool   `json:"trevoga,omitempty"`
}
