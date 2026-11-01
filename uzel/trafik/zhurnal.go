package trafik

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Zapis это трафик по людям за один отрезок, обычно минуту.
type Zapis struct {
	N uint64              `json:"n"` // номер, растёт на единицу без пропусков
	T int64               `json:"t"` // конец отрезка, секунды unix
	L map[string][2]int64 `json:"l"` // имя -> [вверх, вниз] в байтах
	// LV это тот же трафик по входам: имя -> вход -> [вверх, вниз]. Сумма по
	// входам равна L. С 0.3.0, старый центр поле не читает.
	LV map[string]map[string][2]int64 `json:"lv,omitempty"`
	// O это отметки входов за отрезок. С 0.3.0.
	O []Otmetka `json:"o,omitempty"`
}

// Otmetka это сколько раз за отрезок случилось одно событие входа с одного
// адреса.
type Otmetka struct {
	Vhod   string `json:"v"`           // тег входа
	Adres  string `json:"a"`           // адрес клиента без порта
	Imya   string `json:"i,omitempty"` // человек, если вход удался
	Vid    string `json:"k"`           // что случилось
	Chislo int64  `json:"n"`
}

func (o Otmetka) menshe(d Otmetka) bool {
	if o.Vhod != d.Vhod {
		return o.Vhod < d.Vhod
	}
	if o.Adres != d.Adres {
		return o.Adres < d.Adres
	}
	if o.Imya != d.Imya {
		return o.Imya < d.Imya
	}
	return o.Vid < d.Vid
}

// PredelZapisey это сколько неподтверждённых записей журнал держит. Запись
// пишется раз в минуту и только если был трафик или отметки, так что это
// больше двух месяцев без центра. Сверх предела старые уходят, и это видно в
// здоровье.
const PredelZapisey = 100000

// Zhurnal это трафик на диске до тех пор, пока центр не подтвердил, что
// забрал его.
//
// Центр спрашивает записи после последней подтверждённой и подтверждает
// номер. Ни лежащий центр, ни оборванный ответ, ни перезапуск программы
// цифр не теряют: неподтверждённое просто отдаётся ещё раз, а центр
// отбрасывает номера, которые уже видел.
type Zhurnal struct {
	mu            sync.Mutex
	put           string
	putPodtv      string
	sleduyushchiy uint64
	podtverzhdeno uint64
	zapisey       int
	poteryano     uint64
}

// Otkryt поднимает журнал из каталога. Предупреждения это то, что пришлось
// поправить на диске; работе они не мешают, но в журнал программы их надо
// вывести.
func Otkryt(katalog string) (*Zhurnal, []string, error) {
	z := &Zhurnal{
		put:      filepath.Join(katalog, "trafik.jsonl"),
		putPodtv: filepath.Join(katalog, "trafik.podtverzhdeno"),
	}
	var preduprezhdeniya []string

	syroe, err := os.ReadFile(z.putPodtv)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, nil, fmt.Errorf("отметка подтверждения трафика не читается: %w", err)
	default:
		n, perr := strconv.ParseUint(strings.TrimSpace(string(syroe)), 10, 64)
		if perr != nil {
			// Потерянная отметка значит только повтор уже отданных записей,
			// центр их отбросит. Из-за неё VPN не должен стоять.
			preduprezhdeniya = append(preduprezhdeniya, fmt.Sprintf("отметка подтверждения трафика испорчена (%q), считаю с нуля", string(syroe)))
		} else {
			z.podtverzhdeno = n
		}
	}

	zapisi, celyh, err := prochitat(z.put)
	if err != nil {
		return nil, nil, err
	}
	if celyh >= 0 {
		// Хвост без перевода строки или нечитаемая строка в конце остаются
		// от сбоя посреди записи. Он отрезается, иначе следующая запись
		// приклеилась бы к обрывку.
		if err := os.Truncate(z.put, celyh); err != nil {
			return nil, nil, fmt.Errorf("обрезать испорченный хвост журнала трафика: %w", err)
		}
		preduprezhdeniya = append(preduprezhdeniya, "у журнала трафика был оборванный хвост, отрезан")
	}
	posledniy := z.podtverzhdeno
	for _, zp := range zapisi {
		if zp.N > posledniy {
			posledniy = zp.N
		}
		if zp.N > z.podtverzhdeno {
			z.zapisey++
		}
	}
	z.sleduyushchiy = posledniy + 1
	return z, preduprezhdeniya, nil
}

// prochitat читает журнал целиком. celyh это длина целой части файла, если
// за ней остался мусор, иначе -1.
func prochitat(put string) ([]Zapis, int64, error) {
	dannye, err := os.ReadFile(put)
	if errors.Is(err, os.ErrNotExist) {
		return nil, -1, nil
	}
	if err != nil {
		return nil, -1, fmt.Errorf("журнал трафика не читается: %w", err)
	}
	var zapisi []Zapis
	var smeshchenie int64
	for len(dannye) > 0 {
		konec := bytes.IndexByte(dannye, '\n')
		if konec < 0 {
			return zapisi, smeshchenie, nil
		}
		var zp Zapis
		if err := json.Unmarshal(dannye[:konec], &zp); err != nil {
			return zapisi, smeshchenie, nil
		}
		zapisi = append(zapisi, zp)
		smeshchenie += int64(konec + 1)
		dannye = dannye[konec+1:]
	}
	return zapisi, -1, nil
}

// Dopisat добавляет запись и дожидается, пока она ляжет на диск.
func (z *Zhurnal) Dopisat(t int64, s Snimok) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	// В записи с одними отметками L пустой объект, а не null: поле
	// обязательное, и читающему не надо гадать, старая ли это запись.
	lyudi := s.Lyudi
	if lyudi == nil {
		lyudi = map[string][2]int64{}
	}
	zp := Zapis{N: z.sleduyushchiy, T: t, L: lyudi, LV: s.PoVhodam, O: s.Otmetki}
	if len(zp.LV) == 0 {
		zp.LV = nil
	}
	stroka, err := json.Marshal(zp)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(z.put, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("журнал трафика не открывается: %w", err)
	}
	_, err = f.Write(append(stroka, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if errZ := f.Close(); err == nil {
		err = errZ
	}
	if err != nil {
		return fmt.Errorf("запись в журнал трафика: %w", err)
	}
	z.sleduyushchiy++
	z.zapisey++
	if z.zapisey > PredelZapisey {
		return z.perepisat(0, PredelZapisey)
	}
	return nil
}

// Posle отдаёт до predel записей с номером больше n и больше
// подтверждённого.
func (z *Zhurnal) Posle(n uint64, predel int) ([]Zapis, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	if n < z.podtverzhdeno {
		n = z.podtverzhdeno
	}
	zapisi, _, err := prochitat(z.put)
	if err != nil {
		return nil, err
	}
	itog := make([]Zapis, 0)
	for _, zp := range zapisi {
		if zp.N > n {
			itog = append(itog, zp)
			if len(itog) == predel {
				break
			}
		}
	}
	return itog, nil
}

// Podtverdit отмечает, что центр забрал всё по номер n включительно, и
// убирает это с диска.
func (z *Zhurnal) Podtverdit(n uint64) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if posledniy := z.sleduyushchiy - 1; n > posledniy {
		n = posledniy
	}
	if n <= z.podtverzhdeno {
		return nil
	}
	// Сначала отметка, потом чистка: упади программа между ними, лишние
	// строки останутся на диске, но наружу не уйдут, их отсекает отметка.
	if err := zapisatAtomarno(z.putPodtv, []byte(strconv.FormatUint(n, 10)+"\n")); err != nil {
		return fmt.Errorf("отметка подтверждения трафика: %w", err)
	}
	z.podtverzhdeno = n
	return z.perepisat(n, PredelZapisey)
}

// perepisat оставляет в журнале записи с номером больше posle, не больше
// ostavit последних.
func (z *Zhurnal) perepisat(posle uint64, ostavit int) error {
	zapisi, _, err := prochitat(z.put)
	if err != nil {
		return err
	}
	var nuzhnye []Zapis
	for _, zp := range zapisi {
		if zp.N > posle {
			nuzhnye = append(nuzhnye, zp)
		}
	}
	if lishnih := len(nuzhnye) - ostavit; lishnih > 0 {
		z.poteryano += uint64(lishnih)
		nuzhnye = nuzhnye[lishnih:]
	}
	var buf bytes.Buffer
	for _, zp := range nuzhnye {
		stroka, err := json.Marshal(zp)
		if err != nil {
			return err
		}
		buf.Write(stroka)
		buf.WriteByte('\n')
	}
	if err := zapisatAtomarno(z.put, buf.Bytes()); err != nil {
		return fmt.Errorf("переписать журнал трафика: %w", err)
	}
	z.zapisey = len(nuzhnye)
	return nil
}

// Sostoyanie это то, что журнал говорит о себе в здоровье сервера.
type Sostoyanie struct {
	Posledniy     uint64 `json:"posledniy"`
	Podtverzhdeno uint64 `json:"podtverzhdeno"`
	Zhdut         int    `json:"zhdut"`
	Poteryano     uint64 `json:"poteryano,omitempty"`
}

func (z *Zhurnal) Sostoyanie() Sostoyanie {
	z.mu.Lock()
	defer z.mu.Unlock()
	return Sostoyanie{
		Posledniy:     z.sleduyushchiy - 1,
		Podtverzhdeno: z.podtverzhdeno,
		Zhdut:         z.zapisey,
		Poteryano:     z.poteryano,
	}
}

// zapisatAtomarno пишет файл целиком: рядом, на диск, потом подмена.
func zapisatAtomarno(put string, dannye []byte) error {
	vremennyy := put + ".novyy"
	f, err := os.OpenFile(vremennyy, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, bytes.NewReader(dannye))
	if err == nil {
		err = f.Sync()
	}
	if errZ := f.Close(); err == nil {
		err = errZ
	}
	if err != nil {
		_ = os.Remove(vremennyy) // недописанный файл не нужен, ошибка удаления ничего не меняет
		return err
	}
	return os.Rename(vremennyy, put)
}
