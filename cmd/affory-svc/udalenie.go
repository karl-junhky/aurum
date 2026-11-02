package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Uninstall proper, spec §"Удаление": firewall rules → tunnel → service →
// autostart → data directory (keys, only if asked) → program directory.
// snyat() covers the first four; this file is the last two.

// flagSteretKlyuchi is the only spelling that erases the data directory.
// The interface passes it after the human answered the question; a typo
// means "keep", because losing keys to a misspelling is the worst reading.
const flagSteretKlyuchi = "--steret-klyuchi"

// popytokUdaleniya times four seconds of ping each: half a minute is longer
// than any sane window takes to close, and short enough that a directory the
// human genuinely locked (a shell sitting inside it) stops being retried.
const popytokUdaleniya = 8

func steretKlyuchiIz(args []string) bool {
	for _, a := range args[1:] {
		if a == flagSteretKlyuchi {
			return true
		}
	}
	return false
}

// snyatDannye removes the data directory when asked and leaves it alone
// otherwise. Absent directory is success either way.
func snyatDannye(dir string, steret bool) error {
	if !steret {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("каталог данных не удалён: %w", err)
	}
	return nil
}

// udalitKatalogProgrammy schedules the removal of the program directory from
// OUTSIDE this process: a running binary cannot delete itself, and the
// interface that launched us lives in the same directory. A detached cmd
// waits three seconds for both to exit, then removes the directory.
// katalogDlyaUdaleniya is a seam: the test aims the launch at a scratch dir.
var katalogDlyaUdaleniya = sostoyanie.KatalogProgrammy

// sluzhbaUborki это служба, по которой уборка узнаёт новую установку. Шов:
// на машине разработчика Affory стоит, и настоящая AfforySvc остановила бы
// уборку в каждом тесте.
var sluzhbaUborki = imyaSluzhby

// Что считается нашим в каталоге программы. Удаление идёт ТОЛЬКО по этим
// спискам, а сам каталог снимается последним и без рекурсии: не пуст, значит
// там лежит чужое, и он остаётся.
//
// До 1.6.2 здесь был `rmdir /s /q` по всему каталогу. Каталог программы это
// каталог бинаря, а установщик слушается пути, вписанного руками, поэтому
// Affory, поставленная в `D:\Games`, при удалении стирала `D:\Games` целиком
// (пункт К1 аудита 1.6.1).
//
// Тот же список повторяет раздел Uninstall в ustanovka\affory.nsi, и их
// расхождение ловит TestSpisokUdaleniyaSovpadaetSUstanovshchikom.
var (
	faylyProgrammy = append(append([]string{}, faylyUstanovki...),
		"GPL-3.0.txt", "YADRO-ISHODNIKI.txt", "UZEL-ISHODNIKI.txt", "LICENSE.opencck.txt",
		"LICENSE.simple-icons.txt", "OFL-Inter.txt",
		// Manrope ушёл 16.09.2026, но его лицензия лежит у всех, кто ставил 1.1.x.
		"OFL-Manrope.txt",
		imyaAvariynogo, imyaFaylaPrichiny, "Uninstall.exe")
	// Хвосты подмены файлов (internal/obnovlenie): расширения наши, чужих
	// файлов с ними не бывает.
	maskiProgrammy    = []string{"*.ubrat", "*.chast", "*.proba"}
	katalogiProgrammy = []string{obnovlenie.KatalogNovoy, obnovlenie.KatalogPredydushchey}
)

// peremennayaKataloga несёт путь каталога программы в команду уборщика.
const peremennayaKataloga = "AFFORY_KATALOG_UBORKI"

func udalitKatalogProgrammy() error {
	dir := katalogDlyaUdaleniya()
	// Каталог освобождается ДО планирования уборки: иначе уборщик отрабатывает
	// свои восемь кругов о живое окно и уходит ни с чем.
	osvoboditKatalog(dir)
	sistemnyy, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("системный каталог не найден: %w", err)
	}
	// exec.Command would wrap the whole /c argument in quotes; cmd.exe strips
	// the outer pair and trips over the inner ones ("The filename, directory
	// name, or volume label syntax is incorrect"), so the raw command line is
	// handed over verbatim. Guest run 02.09.2026 caught this: the directory
	// stayed whole. timeout.exe refuses to run without a console, which a
	// detached process has not got, so ping does the waiting.
	// Попыток НЕСКОЛЬКО, а не одна. Каталог держит не только эта служба: рядом
	// лежит affory-ui.exe. Само окно уходит лишь тогда, когда снятие пошло
	// ЧЕРЕЗ НЕГО (most.go зовёт app.Quit), поэтому его теперь закрывает
	// osvoboditKatalog выше, а не надежда на то, что оно догадается. Повтор
	// остаётся ради всех остальных: антивирус, индексатор и просто открытая в
	// каталоге оболочка держат файл секунду-другую и никого не слушают. Цикл
	// прекращается сразу, как каталога не станет, поэтому обычный случай не
	// стал дольше ни на шаг. С чужими файлами в каталоге он отрабатывает все
	// круги и уходит, оставив каталог.
	//
	// Путь каталога едет переменной окружения, а не текстом команды (L12
	// аудита 1.8.0): cmd раскрывает %ИМЯ% в своей строке, и каталог
	// с процентом в имени подменялся значением переменной. Значение
	// переменной cmd раскрывает один раз и повторно не разбирает.
	const kat = "%" + peremennayaKataloga + "%"
	var fayly []string
	for _, f := range append(append([]string{}, faylyProgrammy...), maskiProgrammy...) {
		fayly = append(fayly, `"`+kat+`\`+f+`"`)
	}
	var katalogi []string
	for _, k := range katalogiProgrammy {
		katalogi = append(katalogi, fmt.Sprintf(`rmdir /s /q "%s\%s" 2>nul`, kat, k))
	}
	cmdExe := filepath.Join(sistemnyy, "cmd.exe")
	cmd := exec.Command(cmdExe)
	cmd.Env = append(os.Environ(), peremennayaKataloga+"="+dir)
	// Рабочий каталог системный, а не родитель каталога программы: отвязанный
	// процесс живёт до тридцати секунд и держал бы родителя всё это время.
	cmd.Dir = sistemnyy
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008,
		CmdLine: fmt.Sprintf(
			`"%s" /c for /l %%i in (1,1,%d) do ("%s" -n 4 127.0.0.1 >nul & (%s) & del /f /q %s 2>nul & %s & rmdir "%s" 2>nul & if not exist "%s" exit)`,
			cmdExe, popytokUdaleniya, filepath.Join(sistemnyy, "PING.EXE"),
			proverkaNovoyUstanovki(sistemnyy, sluzhbaUborki),
			strings.Join(fayly, " "), strings.Join(katalogi, " & "), kat, kat),
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("удаление каталога программы не запланировано: %w", err)
	}
	return nil
}

// proverkaNovoyUstanovki это кусок команды уборщика: выйти, если служба снова
// зарегистрирована и не помечена к удалению.
//
// Уборщик живёт полминуты после снятия. `affory-svc.exe install`, набранный за
// это время из того же каталога, кладёт службу обратно, а уборщик стирал бы
// всё, что не занято, и оставлял программу из одной работающей службы
// (28.09.2026, гость).
//
// Одного «служба есть» мало. Снятие удаляет службу через DeleteService, и
// пока на неё открыт чужой дескриптор, она остаётся в реестре с DeleteFlag=1:
// выход по одному наличию ключа оставлял бы каталог после снятия из окна, где
// каталог убирает только уборщик. Помеченная служба не стартует уже никогда,
// поэтому новой установкой не считается.
//
// Запрос к реестру, а не sc.exe: наличие ключа и флаг читаются одной
// утилитой одинаково.
func proverkaNovoyUstanovki(sistemnyy, sluzhba string) string {
	reg := filepath.Join(sistemnyy, "reg.exe")
	klyuch := `HKLM\SYSTEM\CurrentControlSet\Services\` + sluzhba
	return fmt.Sprintf(`"%s" query "%s" >nul 2>&1 && ("%s" query "%s" /v DeleteFlag >nul 2>&1 || exit)`,
		reg, klyuch, reg, klyuch)
}

// osvoboditKatalog просит закрыться наши программы, запущенные ИЗ каталога
// программы.
//
// Без этого снятие из командной строки не доводит дело до конца. Окно
// закрывается само, только когда снятие пошло ЧЕРЕЗ НЕГО (most.go зовёт
// app.Quit после ухода команды); про `affory-svc.exe uninstall`, набранный
// руками или запущенный оснасткой, оно не узнаёт никогда, живёт дальше и держит
// каталог своим образом. Уборщик отрабатывает все восемь кругов впустую, а
// uninstall к тому времени уже отчитался кодом 0.
//
// Ищем по ПУТИ ОБРАЗА, а не по имени процесса. Имя `affory-ui.exe` может носить
// что угодно на чужой машине, а убивать по имени значит однажды убить не своё.
// И по пути мало: каталог программы бывает общим (`D:\Games`), и всё, что
// запущено оттуда, не наше. Поэтому путь и имя вместе: образ лежит ПРЯМО в
// каталоге и называется одним из наших exe.
//
// Сначала просьба (`taskkill` без /F шлёт окну WM_CLOSE), через три секунды
// принуждение: программу снимают, и окно, пережившее снятие, показывало бы
// кнопки к удалённому бинарю.
func osvoboditKatalog(dir string) {
	nashi := procesyIz(dir)
	if len(nashi) == 0 {
		return
	}
	taskkill := putSistemnoy("taskkill.exe")
	for _, pid := range nashi {
		// Ошибка здесь не повод останавливаться: процесс мог закончиться сам
		// между снимком и просьбой, и это ровно то, чего мы добивались.
		_ = exec.Command(taskkill, "/PID", strconv.FormatUint(uint64(pid), 10)).Run()
	}
	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if len(procesyIz(dir)) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, pid := range procesyIz(dir) {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(h, 1)
		_ = windows.CloseHandle(h)
	}
}

// procesyIz отдаёт pid наших процессов из dir: образ лежит прямо в dir и
// называется одним из наших exe. Свой процесс не в счёт: служба сама лежит там
// же и умрёт сразу после uninstall.
func procesyIz(dir string) []uint32 {
	snimok, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snimok)

	svoy := uint32(os.Getpid())
	katalog := filepath.Clean(dir)
	var out []uint32
	// dwSize это договор, а не подсказка: Process32First отвергает нулевой.
	var zapis windows.ProcessEntry32
	zapis.Size = uint32(unsafe.Sizeof(zapis))
	for err = windows.Process32First(snimok, &zapis); err == nil; err = windows.Process32Next(snimok, &zapis) {
		if zapis.ProcessID == svoy || zapis.ProcessID == 0 {
			continue
		}
		put := putObraza(zapis.ProcessID)
		if put == "" {
			continue
		}
		if strings.EqualFold(filepath.Dir(put), katalog) && nashExe(filepath.Base(put)) {
			out = append(out, zapis.ProcessID)
		}
	}
	return out
}

// nashExe узнаёт имена наших программ, без учёта регистра.
func nashExe(imya string) bool {
	for _, f := range faylyUstanovki {
		if strings.EqualFold(imya, f) {
			return true
		}
	}
	return false
}

// putSistemnoy отдаёт полный путь к программе из System32. Поиск по PATH от
// имени SYSTEM или администратора находит первое попавшееся с этим именем, и
// каталог, стоящий в PATH раньше System32, подменил бы `taskkill.exe` своим.
// Голое имя остаётся только на случай, когда системный каталог не назван.
func putSistemnoy(imya string) string {
	sistemnyy, err := windows.GetSystemDirectory()
	if err != nil {
		return imya
	}
	return filepath.Join(sistemnyy, imya)
}

// putObraza отдаёт полный путь образа процесса или пустую строку.
//
// QUERY_LIMITED_INFORMATION, а не QUERY_INFORMATION: первого хватает на путь и
// он даётся к процессам, к которым второе не даётся вовсе.
func putObraza(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	bufer := make([]uint16, windows.MAX_LONG_PATH)
	dlina := uint32(len(bufer))
	if err := windows.QueryFullProcessImageName(h, 0, &bufer[0], &dlina); err != nil || dlina == 0 {
		return ""
	}
	return windows.UTF16ToString(bufer[:dlina])
}
