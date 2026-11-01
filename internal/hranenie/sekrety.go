package hranenie

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// ImyaSekretov публично ради тестов и ради стенда: живая проверка обязана уметь
// подложить чужой блоб, а для этого знать имя.
const ImyaSekretov = "sekrety.dat"

// Пять попыток и удвоение паузы от секунды это около пятнадцати секунд.
//
// Потолок нужен обязательно. Цикл повторов без предела не проваливает старт
// службы, а ВЕШАЕТ его, и «висит» это единственный исход, под который в §9.1
// нет экрана вовсе.
const PopytokRasshifrovki = 5

var ErrSekretyNechitaemy = errors.New("секреты не расшифровываются на этой машине")

// ErrDPAPINedostupen это временный отказ: блоб цел, не ответила система (М4
// аудита 1.8.0). Следующее чтение пробует снова, блоб остаётся на месте.
var ErrDPAPINedostupen = errors.New("шифрование Windows сейчас не отвечает, секреты не прочитаны")

// porchaDannyh отличает испорченный блоб от временного отказа. Проверено на
// живом DPAPI 30.09.2026: мусор, обрезанный блоб и блоб с изменённым байтом в
// любом месте дают ERROR_INVALID_DATA. Коды плохого ключа NTE_BAD_DATA и
// NTE_BAD_KEY_STATE это чужая машина или сменившийся ключ, повторы им тоже не
// помогут. Ключа к блобу на машине нет вовсе: ERROR_PATH_NOT_FOUND, пойман
// приёмкой 1.9.0 на блобе без флага LOCAL_MACHINE, прочитанном из-под SYSTEM;
// ERROR_FILE_NOT_FOUND это то же отсутствие файлом, а не каталогом. Всё
// остальное, включая RPC к LSASS до входа в систему, временно.
func porchaDannyh(err error) bool {
	var kod syscall.Errno
	if !errors.As(err, &kod) {
		return false
	}
	switch uint32(kod) {
	case uint32(windows.ERROR_INVALID_DATA), 0x80090005, 0x8009000B,
		uint32(windows.ERROR_PATH_NOT_FOUND), uint32(windows.ERROR_FILE_NOT_FOUND):
		return true
	}
	return false
}

// Shifrovshchik это DPAPI МАШИННЫЙ, и это решение, а не умолчание.
//
// Служба стартует до входа пользователя в систему, и пользовательского
// контекста в этот момент физически нет. Цена принята осознанно и записана в
// спеке: администратор этой машины расшифрует блоб. Защита здесь от кражи
// файла, а не от локального администратора, которым мы и так являемся.
//
// Дополнительной энтропии нет намеренно. Она лежала бы в нашем же исполняемом
// файле рядом с блобом, то есть добавляла бы работу нам и ноль работы тому, кто
// унёс каталог целиком.
type Shifrovshchik struct {
	dir string

	// Швы. Настоящий DPAPI в тесте недостижим по временным отказам: LSASS у нас
	// всегда готов, а весь смысл повторов именно в том, что бывает наоборот.
	zashifrovat  func([]byte) ([]byte, error)
	rasshifrovat func([]byte) ([]byte, error)

	Chasy func() time.Time
	Spat  func(time.Duration)
}

func Novyy() *Shifrovshchik { return NovyyV(sostoyanie.KatalogDannyh()) }

func NovyyV(dir string) *Shifrovshchik {
	return &Shifrovshchik{
		dir: dir, zashifrovat: dpapiZashifrovat, rasshifrovat: dpapiRasshifrovat,
		Chasy: time.Now, Spat: time.Sleep,
	}
}

func (s *Shifrovshchik) put() string { return filepath.Join(s.dir, ImyaSekretov) }

// Sohranit шифрует и кладёт на диск атомарно.
//
// Атомарность здесь важнее, чем у файла состояния. Недописанный блоб не
// расшифруется НИКОГДА, то есть выглядит ровно как мёртвый ключ, а мёртвый ключ
// мы хороним. Оборванная запись иначе стоила бы человеку всех его серверов.
func (s *Shifrovshchik) Sohranit(telo []byte) error {
	blob, err := s.zashifrovat(telo)
	if err != nil {
		return fmt.Errorf("секреты не шифруются: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("каталог данных недоступен: %w", err)
	}
	// Прежняя версия остаётся рядом (Н3 аудита 1.6.1): если эту запись
	// испортит пропавшее питание, Zagruzit вернёт хотя бы её. Годность
	// прежнего блоба не проверяется: мёртвый блоб хоронится при чтении, до
	// всякой новой записи.
	if err := sostoyanie.ZapisatNadyozhno(s.put(), blob, nil); err != nil {
		return fmt.Errorf("секреты не записаны: %w", err)
	}
	return nil
}

// Zagruzit читает блоб, повторяя при отказе.
//
// Отсутствие файла это НЕ ошибка: первый запуск, серверов ещё не добавляли.
// Показать на этом secrets-unreadable значило бы напугать человека тем, что всё
// идёт по плану.
func (s *Shifrovshchik) Zagruzit() ([]byte, error) {
	blob, err := os.ReadFile(s.put())
	if os.IsNotExist(err) {
		return s.voskresit()
	}
	if err != nil {
		return nil, fmt.Errorf("секреты не читаются: %w", err)
	}

	// Повторы, а не приговор с первой попытки. CryptUnprotectData это вызов в
	// LSASS, и до входа пользователя он отказывает по причинам, не имеющим
	// отношения к целости ключа. Похоронить блоб на первом же отказе значит
	// превратить временную занятость системы в окончательный вердикт.
	pauza := time.Second
	var posledn error
	for i := 0; i < PopytokRasshifrovki; i++ {
		telo, err := s.rasshifrovat(blob)
		if err == nil {
			return telo, nil
		}
		posledn = err
		// Испорченный блоб повторами не чинится: ждать незачем.
		if porchaDannyh(err) {
			break
		}
		if i < PopytokRasshifrovki-1 {
			s.Spat(pauza)
			pauza *= 2
		}
	}

	// Временный отказ не приговор (M4 аудита 1.8.0). Прежде блоб хоронился и
	// после него: следующее чтение видело пустое место и отдавало набор
	// первого запуска, а серверы человека оставались в отложенном файле.
	if !porchaDannyh(posledn) {
		return nil, fmt.Errorf("%w: %v", ErrDPAPINedostupen, posledn)
	}

	// Блоб испорчен. Теперь это приговор, и блоб убирается с дороги, иначе
	// следующий запуск упрётся в него же и так до конца времён.
	imya, err := s.pohoronit()
	if err != nil {
		return nil, fmt.Errorf("%w: %v (и убрать блоб не удалось: %v)",
			ErrSekretyNechitaemy, posledn, err)
	}
	if telo, ok := s.vernutZapas(); ok {
		log.Printf("секреты не расшифровались (%v), блоб отложен как %s; взята прежняя версия", posledn, imya)
		return telo, nil
	}
	return nil, fmt.Errorf("%w: %v (блоб отложен как %s)", ErrSekretyNechitaemy, posledn, imya)
}

// vernutZapas пробует прежнюю версию блоба и ставит её на место основного.
//
// Одна попытка, а не повторы: повторы уже показали, что LSASS отвечает или
// не отвечает, и второй круг ожидания только отодвинул бы отказ.
func (s *Shifrovshchik) vernutZapas() ([]byte, bool) {
	zapas, err := os.ReadFile(s.put() + sostoyanie.RasshirenieZapasa)
	if err != nil {
		return nil, false
	}
	telo, err := s.rasshifrovat(zapas)
	if err != nil {
		return nil, false
	}
	if err := sostoyanie.ZapisatNadyozhno(s.put(), zapas, nil); err != nil {
		// Секреты уже в руках, и это главное. Незаписанное место просто
		// заставит следующий старт пройти этот путь ещё раз.
		log.Printf("прежняя версия секретов не встала на место: %v", err)
	}
	return telo, true
}

// voskresit отвечает на пустое место основного блоба.
//
// Пустое место с отложенными блобами рядом это не первый запуск. До 1.9.0
// блоб хоронился и при временном отказе DPAPI, то есть в отложенном может
// лежать целый набор человека (M4 аудита 1.8.0). Читается: встаёт на место.
// Не отвечает система: отказ, а не пустой набор, иначе первая же запись
// нового сервера оставила бы старые серверы только в отложенном файле.
// Испорчены все: набор начинается заново, как и прежде.
func (s *Shifrovshchik) voskresit() ([]byte, error) {
	otlozhennye, err := filepath.Glob(filepath.Join(s.dir, ImyaSekretov+".mertvyy-*"))
	if err != nil {
		return nil, fmt.Errorf("отложенные секреты не ищутся: %w", err)
	}
	// Новые первыми: в имени дата и время, и строковый порядок это их порядок.
	sort.Sort(sort.Reverse(sort.StringSlice(otlozhennye)))
	var vremennyy error
	for _, put := range otlozhennye {
		blob, err := os.ReadFile(put)
		if err != nil {
			log.Printf("отложенные секреты %s не читаются: %v", filepath.Base(put), err)
			continue
		}
		telo, err := s.rasshifrovat(blob)
		if err != nil {
			if !porchaDannyh(err) {
				vremennyy = err
			}
			continue
		}
		if err := sostoyanie.ZapisatNadyozhno(s.put(), blob, nil); err != nil {
			return nil, fmt.Errorf("секреты из %s прочитаны, но не встали на место: %w", filepath.Base(put), err)
		}
		// Копия стоит на месте основного: отложенный файл больше не нужен, а
		// оставленный, он вернул бы старый набор после следующей порчи.
		if err := os.Remove(put); err != nil {
			log.Printf("вернувшийся блоб %s не удалён: %v", filepath.Base(put), err)
		}
		log.Printf("секреты вернулись из %s: блоб был отложен без порчи", filepath.Base(put))
		return telo, nil
	}
	if vremennyy != nil {
		return nil, fmt.Errorf("%w: отложенные секреты не проверены: %v", ErrDPAPINedostupen, vremennyy)
	}
	if len(otlozhennye) > 0 {
		log.Printf("основного блоба нет, отложенные (%d) испорчены: набор начинается заново", len(otlozhennye))
	}
	return nil, nil
}

// pohoronit переименовывает мёртвый блоб.
//
// В суффиксе дата И ВРЕМЯ. С одной датой второй отказ за те же сутки перетирает
// первый, переживший блоб оказывается вторым, и по журналу этого не видно.
func (s *Shifrovshchik) pohoronit() (string, error) {
	imya := fmt.Sprintf("%s.mertvyy-%s", ImyaSekretov, s.Chasy().Format("2006-01-02-150405"))
	novyy := filepath.Join(s.dir, imya)
	// Секунды всё-таки конечны, а два отказа подряд бывают быстрее секунды.
	// Затирать уже отложенный блоб нельзя по той же причине, по какой заведено
	// время в имени.
	for i := 1; ; i++ {
		if _, err := os.Stat(novyy); os.IsNotExist(err) {
			break
		}
		novyy = filepath.Join(s.dir, fmt.Sprintf("%s-%d", imya, i))
	}
	if err := os.Rename(s.put(), novyy); err != nil {
		return "", err
	}
	return filepath.Base(novyy), nil
}

// dpapiZashifrovat шифрует с флагом CRYPTPROTECT_LOCAL_MACHINE.
//
// Без флага блоб привязывается к учётной записи, и служба из-под SYSTEM
// прочитать его не сможет вовсе. Это и есть контроль к живой проверке в госте:
// тот же блоб без флага обязан дать secrets-unreadable.
func dpapiZashifrovat(telo []byte) ([]byte, error) {
	return dpapi(telo, true)
}

func dpapiRasshifrovat(blob []byte) ([]byte, error) {
	return dpapi(blob, false)
}

// ZashifrovatDlyaMashiny шифрует тем же DPAPI с привязкой к машине, что и
// sekrety.dat. Так служба хранит секреты, которым не место в наборе: ключи
// управления своих серверов (план «Свой VPN» §3.6) не должны уезжать в
// экспорт профиля.
func ZashifrovatDlyaMashiny(telo []byte) ([]byte, error) { return dpapiZashifrovat(telo) }

// RasshifrovatDlyaMashiny обратна ZashifrovatDlyaMashiny.
func RasshifrovatDlyaMashiny(blob []byte) ([]byte, error) { return dpapiRasshifrovat(blob) }

func dpapi(telo []byte, shifrovat bool) ([]byte, error) {
	// Пустой срез нельзя отдавать как указатель на первый элемент: его нет.
	if len(telo) == 0 {
		return nil, errors.New("пустое тело")
	}
	vhod := windows.DataBlob{Size: uint32(len(telo)), Data: &telo[0]}
	var vyhod windows.DataBlob

	var err error
	if shifrovat {
		err = windows.CryptProtectData(&vhod, nil, nil, 0, nil,
			windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &vyhod)
	} else {
		err = windows.CryptUnprotectData(&vhod, nil, nil, 0, nil,
			windows.CRYPTPROTECT_UI_FORBIDDEN, &vyhod)
	}
	if err != nil {
		return nil, err
	}
	// LocalFree обязателен: DPAPI выделяет буфер сам, и без освобождения служба,
	// живущая месяцами, течёт на каждом чтении секретов.
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(vyhod.Data)))

	// Копия, а не срез поверх чужой памяти: буфер освобождается прямо здесь
	// отложенным вызовом, и срез пережил бы его на любой срок.
	nazad := make([]byte, vyhod.Size)
	copy(nazad, unsafe.Slice(vyhod.Data, vyhod.Size))
	return nazad, nil
}
