package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Most is the whole surface the frontend gets. One method, on purpose.
//
// The shell does not know what "status" or "connect" mean; it moves frames.
// Every screen in wave 4 speaks to the service through this and nothing else,
// so swapping the shell later is this file plus main.go, not eleven screens.
//
// It is an interface, not just a struct, because the dead-export gate
// (internal/kachestvo/mertvyy_test.go) spares exported methods only when they
// satisfy an interface of this project. Nobody in Go calls Zvat; JS does.
type Most interface {
	Zvat(imya string, telo string) (string, error)
	// First run (§9.2): the shell must tell "no service" from "service not
	// answering" without admin, and start the install with one UAC prompt.
	SluzhbaUstanovlena() (bool, error)
	UstanovitSluzhbu() error
	// Uninstall (§"Удаление"): one elevated call, the keys question already
	// answered by the human. The window quits right after: its own directory
	// is about to go.
	UdalitProgrammu(steretKlyuchi bool) error
	// Update (wave 6.5): native archive dialog; "" when nothing was chosen.
	VybratArhiv() (string, error)
	// Профиль (03.09.2026): exportProfile и importProfile были только в CLI,
	// а окно про них не знало вовсе. Диалог и файл лежат тут, пароль сюда не
	// приходит никогда: он уходит телом команды по каналу.
	VybratKudaSohranit() (string, error)
	VybratOtkuda() (string, error)
	SohranitProfil(put string, profil string) error
	ProchitatProfil(put string) (string, error)
	// Ключ SSH для своего сервера (1.11.0): окно читает файл само, путь
	// службе не уходит.
	VybratKlyuchSsh() (*klyuchSsh, error)
	// Rules form (§5 п.1): processes of THIS session, which the service in
	// session 0 cannot see.
	SpisokProtsessov() ([]Protsess, error)
	VybratPrilozhenie() (string, error)
	OtkrytPapkuZhurnalov() error
	// Настройки даты и времени Windows, кнопка отказа clock-skew (1.10.0).
	OtkrytNastroykiVremeni() error
	// Журнал окна (1.10.0): показанные отказы и ошибки страницы, и строка
	// версии под отказом, которая не зависит от службы.
	ZapisatVZhurnal(tekst string)
	OpisanieVersii() string
	// QR с экрана (шесть удобств §3, О8 аудита 1.6.1): сначала снимок и
	// сводка без секретов, потом добавление по подтверждению либо отмена.
	// Ссылка в окно не попадает, окно получает сводку и строку итога.
	NaytiQrNaEkrane() (svodkaQr, error)
	DobavitNaydennoeQr() (string, error)
	ZabytQr()
	// Выгрузка диагностики (О6 аудита 1.6.1): диалог, порции от службы, файл.
	SohranitDiagnostiku() (string, error)
	// Выгрузка серверов (26.09.2026): ссылки из exportServers одним или
	// несколькими QR, PNG в data URI. Ключи приходят сюда из окна и никуда
	// не пишутся: картинка собирается в памяти.
	KodyQr(tekst string) ([]string, error)
	// Повышение прав окна (03.09.2026): служба часть команд отдаёт только
	// администратору, а окно стартует ярлыком без повышения, и без этого
	// метода действие «Повторить от администратора» ничего не делало.
	PerezapustitSPravami(vkladka string) error
	// Вкладка, с которой окно просили открыться (см. flagVkladka). Пустая
	// строка значит обычный запуск.
	StartovayaVkladka() string
	// Перезапуск окна ПОСЛЕ обновления (13.09.2026): подмена меняет файл, а
	// работающий процесс продолжает рисовать старый интерфейс.
	PerezapustitOkno() error
}

func (m *most) UdalitProgrammu(steretKlyuchi bool) error {
	args := "uninstall"
	if steretKlyuchi {
		args += " --steret-klyuchi"
	}
	// Ждём конца снятия и смотрим код. Прежде тут был запуск без ожидания и
	// немедленный Quit: упавшее снятие выглядело успешным, человек оставался с
	// установленной программой и уверенностью, что удалил её.
	kod, err := zapustitSnyatie(args)
	if err != nil {
		return err
	}
	if kod != 0 {
		if prichina := prochitatPrichinu(); prichina != "" {
			return errors.New(prichina)
		}
		return fmt.Errorf("снятие не удалось, код %d", kod)
	}
	// Quit ПОСЛЕ успеха. Обычно до него не доходит: снятие само закрывает окно,
	// освобождая каталог программы. Но если оно этого не сделало (окно запущено
	// из другого каталога), закрыться всё равно надо: кнопки ведут к бинарю,
	// которого больше нет.
	go zakrytOkno(m)
	return nil
}

// VybratArhiv opens the native file dialog for an update archive (wave 6.5).
// Empty string means the human closed the dialog; that is not an error and
// no command is sent. The service does the sha256 check, not the window.
func (m *most) VybratArhiv() (string, error) {
	put, err := m.app.Dialog.OpenFile().
		SetTitle("Архив сборки Affory").
		AddFilter("Архив сборки (*.zip)", "*.zip").
		PromptForSingleSelection()
	return otvetDialoga(put, err)
}

// tekstOtmenyDialoga это сообщение отмены из go-common-file-dialog, который
// Wails держит во ВНУТРЕННЕМ пакете: импортировать его сентинел нельзя, а
// сверять текст можно. Сверка по тексту некрасива и честна, а прежнее
// «любая ошибка это отмена» не было ни тем, ни другим.
const tekstOtmenyDialoga = "cancelled by user"

// otvetDialoga отделяет закрытый человеком диалог от сломанного.
//
// Пустая строка означает ровно одно: человек передумал. Всё остальное едет
// наверх ошибкой, потому что до 03.09.2026 сорванный диалог возвращал ту же
// пустую строку, окно на неё не делало ничего, и кнопка «Выбрать архив»
// молча не работала.
func otvetDialoga(put string, err error) (string, error) {
	if err == nil {
		return put, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), tekstOtmenyDialoga) {
		return "", nil
	}
	return "", err
}

// katalogDokumentov это «Документы» человека, с них открываются диалоги
// сохранения.
//
// Без каталога диалог открывается в рабочем каталоге процесса, а ярлык
// запускает окно из Program Files: человеку без прав туда не записать, и на
// «Сохранить» он первым делом получал отказ Windows (приёмка 1.7.0 в госте,
// 28.09.2026).
//
// Отказ оболочки не повод не показывать диалог: пустой каталог значит, что
// папку выберет сам диалог, как до этой правки.
func katalogDokumentov() string {
	put, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		log.Printf("папка «Документы» не найдена, диалог сохранения откроется где решит сам: %v", err)
		return ""
	}
	return put
}

// Шов для тестов: диалог сохранения профиля. Каталог приходит снаружи, чтобы
// тест видел, где диалог откроется.
var pokazatSohranenieProfilya = (*most).dialogProfilya

func (m *most) dialogProfilya(katalog string) (string, error) {
	// Заголовка у диалога сохранения в Wails v3 beta.26 нет: SetTitle есть
	// только у OpenFile. SetMessage на Windows ничего не показывает, в диалог
	// уходят только заголовок, фильтры, имя и каталог (dialogs_windows.go).
	// Заголовок ставит лишь SetOptions, а он переписывает все поля разом.
	d := m.app.Dialog.SaveFile().
		SetMessage("Куда вынести профиль Affory").
		SetFilename("profil.affory").
		AddFilter("Профиль Affory (*.affory)", "*.affory")
	if katalog != "" {
		d = d.SetDirectory(katalog)
	}
	return otvetDialoga(d.PromptForSingleSelection())
}

// VybratKudaSohranit это диалог сохранения для файла профиля. Пустая строка
// означает, что человек передумал.
func (m *most) VybratKudaSohranit() (string, error) {
	put, err := pokazatSohranenieProfilya(m, katalogDokumentov())
	m.puti.zapomnit(&m.puti.sohranit, put)
	return put, err
}

// VybratOtkuda это диалог открытия для файла профиля.
func (m *most) VybratOtkuda() (string, error) {
	put, err := otvetDialoga(m.app.Dialog.OpenFile().
		SetTitle("Откуда внести профиль Affory").
		AddFilter("Профиль Affory (*.affory)", "*.affory").
		PromptForSingleSelection())
	m.puti.zapomnit(&m.puti.prochitat, put)
	return put, err
}

// vybrannyePuti помнит пути, выбранные человеком в диалогах этого окна (О7
// аудита 1.6.1). Страница зовёт SohranitProfil и ProchitatProfil с любым
// путём, и без этой сверки чужой скрипт на ней писал бы и читал что угодно от
// имени человека.
type vybrannyePuti struct {
	mu                  sync.Mutex
	sohranit, prochitat string
}

func (v *vybrannyePuti) zapomnit(kuda *string, put string) {
	if put == "" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	*kuda = put
}

// vybran сверяет путь с выбранным. Регистр букв Windows в путях не различает.
func (v *vybrannyePuti) vybran(kuda *string, put string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if *kuda == "" || !strings.EqualFold(filepath.Clean(*kuda), filepath.Clean(put)) {
		return errors.New("файл не выбран в диалоге этого окна")
	}
	return nil
}

// SohranitProfil кладёт на диск то, что отдала команда exportProfile.
func (m *most) SohranitProfil(put string, profil string) error {
	if err := m.puti.vybran(&m.puti.sohranit, put); err != nil {
		return err
	}
	return sohranitProfil(put, profil)
}

// ProchitatProfil поднимает файл профиля для команды importProfile.
func (m *most) ProchitatProfil(put string) (string, error) {
	if err := m.puti.vybran(&m.puti.prochitat, put); err != nil {
		return "", err
	}
	return prochitatProfil(put)
}

// sohranitProfil пишет ИСХОДНЫЕ байты, а не base64: по каналу профиль едет
// в base64, потому что тело кадра это JSON, но файл должен совпадать байт в
// байт с тем, что пишет affory-cli profile export.
//
// Разбор до записи, а не после: молча записанный мусор человек обнаружит
// только при попытке восстановиться на другой машине.
func sohranitProfil(put string, profil string) error {
	blob, err := base64.StdEncoding.DecodeString(profil)
	if err != nil {
		return fmt.Errorf("профиль не декодируется: %w", err)
	}
	if len(blob) == 0 {
		return errors.New("профиль пуст: записывать нечего")
	}
	// 0600 бессмысленны на NTFS без ACL, но файл в любом случае не должен
	// создаваться с правами каталога.
	if err := os.WriteFile(put, blob, 0o600); err != nil {
		return fmt.Errorf("файл не записан: %w", err)
	}
	return nil
}

// prochitatProfil читает файл профиля и отдаёт его в base64 для тела команды.
func prochitatProfil(put string) (string, error) {
	blob, err := os.ReadFile(put)
	if err != nil {
		return "", fmt.Errorf("файл не прочитан: %w", err)
	}
	if len(blob) == 0 {
		return "", errors.New("файл профиля пуст")
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

// SluzhbaUstanovlena asks the service manager whether AfforySvc exists.
func (m *most) SluzhbaUstanovlena() (bool, error) {
	return sluzhbaUstanovlena(imyaSluzhby)
}

// UstanovitSluzhbu launches `affory-svc install` elevated. ShellExecute with
// the runas verb is the one UAC prompt §4.4 allows; the call returns as soon
// as the prompt is answered, so the screen polls status to learn the rest.
// The service binary is looked for next to this one: that is the install
// layout, and a UI running from somewhere else has no service to install.
func (m *most) UstanovitSluzhbu() error {
	return zapustitSluzhbuSPravami("install")
}

// zapustitSluzhbuSPravami runs affory-svc.exe from next to this binary with
// the given arguments, elevated. Returns once UAC is answered.
func zapustitSluzhbuSPravami(argumenty string) error {
	svoy, err := os.Executable()
	if err != nil {
		return fmt.Errorf("свой путь не читается: %w", err)
	}
	put := filepath.Join(filepath.Dir(svoy), "affory-svc.exe")
	if _, err := os.Stat(put); err != nil {
		return fmt.Errorf("рядом с программой нет affory-svc.exe: %w", err)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	fayl, _ := windows.UTF16PtrFromString(put)
	args, _ := windows.UTF16PtrFromString(argumenty)
	// SW_HIDE: the subcommand prints a line and exits; a console flashing on
	// top of the window would read as an error to anyone watching.
	if err := windows.ShellExecute(0, verb, fayl, args, nil, windows.SW_HIDE); err != nil {
		// ERROR_CANCELLED это человек, нажавший «Нет» в запросе прав, а не
		// поломка. Текст свой, как и при повышении окна ниже: Windows отвечает
		// по-английски («The operation was canceled by the user»), и эта строка
		// уезжала прямо на экран первого запуска, где рядом стоит наша русская
		// фраза. Имя команды оттуда тоже убрано: «affory-svc install» человеку,
		// который просто нажал «Установить службу», сказать нечего.
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("права не выданы: запрос отклонён")
		}
		return fmt.Errorf("служба не установилась: %w", err)
	}
	return nil
}

const imyaSluzhby = "AfforySvc"

// sluzhbaUstanovlena needs only SC_MANAGER_CONNECT and SERVICE_QUERY_STATUS,
// which every user has. A missing service is false, not an error.
func sluzhbaUstanovlena(imya string) (bool, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, fmt.Errorf("диспетчер служб недоступен: %w", err)
	}
	defer windows.CloseServiceHandle(m)
	imya16, err := windows.UTF16PtrFromString(imya)
	if err != nil {
		return false, err
	}
	h, err := windows.OpenService(m, imya16, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		return false, fmt.Errorf("служба %s не открылась: %w", imya, err)
	}
	windows.CloseServiceHandle(h)
	return true, nil
}

// most talks to the service over the named pipe, reconnecting lazily.
type most struct {
	app *application.App
	// naStatus is the tray's ear: every status that passes through here,
	// answer or event, is repeated to it. nil until the tray exists.
	naStatus func(protokol.StatusOtvet)
	// okno is hidden for the screen shot and shown again; nil in tests.
	okno *application.WebviewWindow

	mu sync.Mutex
	k  *kanal.Klient

	puti vybrannyePuti
	qr   qrNaPodtverzhdenii
}

// podklyuchen reports whether the pipe is currently dialled.
func (m *most) podklyuchen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.k != nil
}

// zvatSluzhbu это шов для тестов: команды, которые оболочка шлёт службе сама
// (QR с экрана, выгрузка диагностики), идут через него.
var zvatSluzhbu = (*most).Zvat

// komandaSluzhbe шлёт команду службе и разворачивает её кадр. Отказ службы
// возвращается её же текстом, чтобы окно показало его той строкой, что и всё
// остальное.
func (m *most) komandaSluzhbe(imya string, telo []byte) (protokol.Kadr, error) {
	var k protokol.Kadr
	otvet, err := zvatSluzhbu(m, imya, string(telo))
	if err != nil {
		return k, err
	}
	if err := json.Unmarshal([]byte(otvet), &k); err != nil {
		return k, fmt.Errorf("ответ службы не разобран: %w", err)
	}
	if k.Oshib != nil {
		return k, errors.New(k.Oshib.Tekst)
	}
	return k, nil
}

// zvatFonovo runs a command for the tray: no answer wanted, the state event
// (or the answer's status, repeated to naStatus) is the whole result.
func (m *most) zvatFonovo(komanda string) {
	_, _ = m.Zvat(komanda, "")
}

// soobshchitStatus repeats a frame's status body to the tray, if it is one.
func (m *most) soobshchitStatus(kadr protokol.Kadr) {
	if m.naStatus == nil || len(kadr.Telo) == 0 {
		return
	}
	var st protokol.StatusOtvet
	if err := json.Unmarshal(kadr.Telo, &st); err != nil || st.Sostoyanie == "" {
		return
	}
	m.naStatus(st)
}

var _ Most = (*most)(nil)

// Zvat sends one command by name with a JSON body and returns the answer frame
// as JSON text. Strings on both sides, deliberately: the binding layer would
// otherwise type json.RawMessage as a byte array, and every screen would be
// decoding base64 by hand.
func (m *most) Zvat(imya string, telo string) (string, error) {
	k, err := m.klient()
	var ov kanal.OtkazVersii
	if errors.As(err, &ov) {
		// Разные версии это ответ, а не сбой оболочки: экран рисует по коду
		// protocol-mismatch свой §9.1 «обнови программу» (О3 аудита 1.6.1).
		b, errJ := json.Marshal(protokol.Kadr{Tip: "otvet", Imya: imya,
			Oshib: &protokol.Oshibka{Kod: protokol.KodProtocolMismatch, Tekst: ov.Tekst}})
		if errJ != nil {
			return "", fmt.Errorf("отказ версии не сериализуется: %w", errJ)
		}
		return string(b), nil
	}
	if err != nil {
		return "", err
	}
	var syroe json.RawMessage
	if telo != "" {
		syroe = json.RawMessage(telo)
	}
	otvet, err := zvatKanal(k, imya, syroe)
	if obryvKanala(otvet, err) {
		// A dead pipe stays dead. Drop the client so the next call redials
		// instead of reporting the same corpse forever.
		m.sbrosit(k)
		if m.naStatus != nil {
			m.naStatus(protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit})
		}
		return "", err
	}
	if err != nil && otvet.Oshib == nil {
		// Вышедший срок канал не рвёт (О2 аудита 1.6.1), но и успехом не
		// является. До 01.10.2026 ошибка здесь терялась: ниже err перезаписывал
		// json.Marshal, и окно получало пустой кадр без отказа, то есть «сделано».
		// Хуже всех от этого было SnyatRezhim: выход из трея считал защиту снятой,
		// когда служба просто не успела ответить.
		//
		// Свой код у вышедшего срока (16.10.2026): служба, может быть, ещё
		// работает, и подключение с переключением окно досматривает по
		// состоянию, а не рисует провалом (жалоба 15.10.2026).
		kod := kodObolochki
		if errors.Is(err, kanal.ErrSrokOtveta) {
			kod = kodSroka
		}
		otvet = protokol.Kadr{Tip: "otvet", Imya: imya,
			Oshib: &protokol.Oshibka{Kod: kod, Tekst: err.Error()}}
	}
	// A refusal frame is an answer: the screen reads oshibka off the frame
	// and shows the §9.1 screen for it. The pipe stays open.
	m.soobshchitStatus(otvet)
	b, err := json.Marshal(otvet)
	if err != nil {
		return "", fmt.Errorf("ответ %s не сериализуется: %w", imya, err)
	}
	return string(b), nil
}

// otkazVOtvete достаёт отказ службы из ответа, отданного Zvat строкой.
//
// Zvat возвращает отказ КАДРОМ и без ошибки Go, и это правильно: экран читает
// код отказа из кадра и рисует по нему свой §9.1. Но вызывающему, которому
// нужен факт «сделано или нет», одного err мало, а тишина здесь означала бы
// успех по умолчанию.
func otkazVOtvete(otvet string) error {
	if otvet == "" {
		return fmt.Errorf("служба не ответила")
	}
	var k protokol.Kadr
	if err := json.Unmarshal([]byte(otvet), &k); err != nil {
		return fmt.Errorf("ответ службы неразборчив: %w", err)
	}
	if k.Oshib != nil {
		return fmt.Errorf("%s (%s)", k.Oshib.Tekst, k.Oshib.Kod)
	}
	return nil
}

// SnyatRezhim выключает режим «весь трафик» и отвечает, ПОЛУЧИЛОСЬ ЛИ.
//
// Той же командой, что и переключатель на экране настроек: два пути к одному
// поступку разошлись бы молча. Зовётся выходом из трея, которому нельзя
// закрывать программу над запертой машиной.
func (m *most) SnyatRezhim() error {
	otvet, err := m.Zvat("setKillSwitch", `{"vkl":false}`)
	if err != nil {
		return err
	}
	return otkazVOtvete(otvet)
}

// Otklyuchit опускает туннель и отвечает, ПОЛУЧИЛОСЬ ЛИ.
//
// Той же командой, что и кнопка «Отключить» на главном экране, и по той же
// причине, что и SnyatRezhim: два пути к одному поступку разошлись бы молча.
// Зовётся выходом из трея, которому с 10.09.2026 нельзя закрывать программу над
// поднятым туннелем: значка не останется, а трафик пойдёт.
//
// Отказ приезжает КАДРОМ без ошибки Go, поэтому одного err мало. Ровно на этом
// однажды закрылась программа над запертой машиной.
func (m *most) Otklyuchit() error {
	otvet, err := m.Zvat("disconnect", "")
	if err != nil {
		return err
	}
	return otkazVOtvete(otvet)
}

// obryvKanala tells a dead pipe from a refusal. kanal.Klient.Zvat returns
// the frame AND an error for a refusal; only an error without a frame is
// the transport failing. Closing the pipe on a refusal cost the screen every
// refusal frame and killed concurrent calls (seen live 02.09.2026).
//
// Вышедший срок тоже не обрыв (О2 аудита 1.6.1): служба занята, а канал жив.
func obryvKanala(otvet protokol.Kadr, err error) bool {
	return err != nil && otvet.Oshib == nil && !errors.Is(err, kanal.ErrSrokOtveta)
}

// kodObolochki это KOD_OBOLOCHKI из frontend/src/ekrany/otkazy.ts: отказ,
// который родился в окне, а не в службе. Экран рисует по нему свою общую
// фразу, а текст ошибки мелким шрифтом под ней.
const kodObolochki = "oshibka-obolochki"

// kodSroka это KOD_SROKA оттуда же: служба не ответила в срок канала. Канал
// жив, а команда, может быть, ещё идёт.
const kodSroka = "srok-otveta"

// podklyuchitsya это шов над kanal.Podklyuchitsya для тестов моста.
var podklyuchitsya = kanal.Podklyuchitsya

// zvatKanal это шов над kanal.Klient.Zvat: канал из тестов окна не собрать,
// а проверять надо, что мост делает с его ответом.
var zvatKanal = func(k *kanal.Klient, imya string, telo any) (protokol.Kadr, error) {
	return k.Zvat(imya, telo)
}

// klient returns the live connection, dialing once if there is none.
func (m *most) klient() (*kanal.Klient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.k != nil {
		return m.k, nil
	}
	k, err := podklyuchitsya()
	if err != nil {
		return nil, err
	}
	m.k = k
	// Events flow one way, service to screen. Forward them as JSON text under
	// a single event name; the frontend fans them out by frame name.
	go m.peredavatSobytiya(k)
	return k, nil
}

// sbrosit forgets the client, but only if it is still the one that failed.
// A concurrent redial may already have replaced it.
func (m *most) sbrosit(k *kanal.Klient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.k == k {
		_ = k.Zakryt()
		m.k = nil
	}
}

func (m *most) peredavatSobytiya(k *kanal.Klient) {
	for kadr := range k.Sobytiya() {
		b, err := json.Marshal(kadr)
		if err != nil {
			continue
		}
		if kadr.Imya == "state" {
			m.soobshchitStatus(kadr)
		}
		m.app.Event.Emit(sobytieKanala, string(b))
	}
	// Channel closed: the reader died, so the pipe is gone. Tell the screen
	// once, so it can show "служба молчит" instead of a stale status.
	m.sbrosit(k)
	m.app.Event.Emit(sobytieKanala, kanalZakrytKadr())
	if m.naStatus != nil {
		m.naStatus(protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit})
	}
}

// sobytieKanala is the single Wails event name that carries service events.
const sobytieKanala = "kanal"

// sobytieOkna несёт ОДИН вопрос: видно окно или нет.
//
// Своё событие, а не родные WindowShow и WindowHide Wails, по двум причинам,
// и обе замерены. Первая записана рядом в Pokazat: при первом показе окна,
// созданного скрытым, WindowShow не приходит вовсе. Вторая наша: крестик у нас
// прячет окно в трей вместо закрытия, и с точки зрения родных событий это не
// отличается ни от чего.
const sobytieOkna = "okno"

// sobytieVkladki просит окно открыть названную вкладку. Нужно трею: его пункт
// «Обновить до X» обязан привести человека туда, где обновляются, а не просто
// показать окно на брошенной вкладке.
const sobytieVkladki = "vkladka"

// PerezapustitOkno поднимает НОВОЕ окно из каталога программы и закрывает это.
//
// Зачем. Подмена при обновлении отодвигает работающий affory-ui.exe в .ubrat и
// кладёт на его место новый файл; Windows держит открытый образ, поэтому
// процесс живёт дальше и рисует СТАРЫЙ интерфейс. 13.09.2026 в госте это видно
// числом: служба после подмены называет себя 1.1.2, окно остаётся тем же
// процессом со сборкой 0.9.9. Человек читает это как «программа не
// обновилась» и лезет ставить выпуск установщиком руками.
//
// Имя файла добавляется к КАТАЛОГУ своего процесса, а не берётся у него
// целиком: os.Executable у отодвинутого процесса отдаёт affory-ui.exe.ubrat,
// то есть прежний образ, и перезапуск поднял бы ровно то, от чего уходим.
// Каталог переименование файла не меняет.
//
// Каталог берётся у себя, а не у internal/sostoyanie: оболочке тот пакет
// запрещён границей (granitsa_test.go), и обходить её ради одной строки нельзя.
//
// Без повышения прав: окно после обновления должно вернуться таким же, каким
// человек его оставил, а запрос UAC на ровном месте выглядит как новая беда.
func (m *most) PerezapustitOkno() error {
	svoy, err := os.Executable()
	if err != nil {
		return fmt.Errorf("свой путь не читается: %w", err)
	}
	katalog := filepath.Dir(svoy)
	novoe := filepath.Join(katalog, "affory-ui.exe")
	if _, err := os.Stat(novoe); err != nil {
		return fmt.Errorf("новый файл окна не найден: %w", err)
	}
	// flagSmena: это окно ещё живо, и без флага новое ушло бы, увидев его.
	cmd := exec.Command(novoe, flagSmena)
	cmd.Dir = katalog
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("новое окно не запустилось: %w", err)
	}
	// Старое уходит ПОСЛЕ удачного запуска и с паузой: два окна это два трея и
	// две подписки, а пустой экран между ними человек читает как падение.
	go func() {
		time.Sleep(700 * time.Millisecond)
		if p := application.Get(); p != nil {
			p.Quit()
		}
	}()
	return nil
}

// kanalZakrytKadr is the synthetic frame the shell emits when the pipe dies.
// It is shaped like a real event so the frontend needs no second code path.
func kanalZakrytKadr() string {
	b, _ := json.Marshal(protokol.Kadr{Tip: "sobytie", Imya: "kanal-zakryt"})
	return string(b)
}

// PerezapustitSPravami перезапускает ОКНО с повышением прав.
//
// Зачем. Служба намеренно отдаёт часть команд только администратору
// (см. komandyDlyaAdmina в affory-svc): угроза тут не человек за машиной, а
// другая программа, запущенная от него же. Но окно у человека стартует
// ярлыком и ключом Run, то есть БЕЗ повышения, и до 03.09.2026 кнопка
// «Повторить от администратора» не делала ничего вовсе: живой прогон в госте
// показал, что из обычного окна нельзя ни добавить сервер, ни удалить его, ни
// обновить подписку, ни включить автозапуск. Программа при этом честно писала
// «эта команда только для администратора машины» и не давала способа им стать.
//
// Повышается ОКНО, а не отдельная команда: служба смотрит на токен того, кто
// пришёл в канал, и разовое повышение одной команды ей не показать никак.
// vkladkiOkna это ЗАКРЫТЫЙ набор: строка уходит в командную строку нового
// процесса, и принимать сюда что угодно значит пускать чужой текст в запуск.
var vkladkiOkna = map[string]bool{"podklyuchenie": true, "pravila": true, "servery": true, "svoy-vpn": true, "nastroyki": true}

// StartovayaVkladka читает флаг вкладки из своей же командной строки.
func (m *most) StartovayaVkladka() string {
	for _, a := range os.Args[1:] {
		v, est := strings.CutPrefix(a, flagVkladka+"=")
		if est && vkladkiOkna[v] {
			return v
		}
	}
	return ""
}

func (m *most) PerezapustitSPravami(vkladka string) error {
	svoy, err := os.Executable()
	if err != nil {
		return fmt.Errorf("свой путь не читается: %w", err)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	fayl, _ := windows.UTF16PtrFromString(svoy)
	// flagSmena: это окно ещё живо, и без флага окно с правами ушло бы, увидев
	// его (О1 аудита 1.6.1).
	stroka := flagSmena
	if vkladkiOkna[vkladka] {
		stroka += " " + flagVkladka + "=" + vkladka
	}
	parametry, _ := windows.UTF16PtrFromString(stroka)
	if err := windows.ShellExecute(0, verb, fayl, parametry, nil, windows.SW_SHOWNORMAL); err != nil {
		// Отказ от запроса прав это не поломка: человек нажал «Нет». Свой текст,
		// а не системный: Windows отвечает по-английски («The operation was
		// canceled by the user»), и живой прогон 13.09.2026 показал эту строку
		// в русском окне.
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("права не выданы: запрос отклонён")
		}
		return fmt.Errorf("повышение не состоялось: %w", err)
	}
	// Старое окно уходит: два экземпляра означают два трея, два наблюдателя и
	// две подписки на события службы. Пауза даёт новому окну стартовать, чтобы
	// человек не увидел пустой экран между ними.
	go func() {
		time.Sleep(700 * time.Millisecond)
		if p := application.Get(); p != nil {
			p.Quit()
		}
	}()
	return nil
}
