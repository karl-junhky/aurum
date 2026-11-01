package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kodirovki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Обновление (задача 6.5, усечённый объём): installUpdate{path} к архиву, который
// уже лежит на диске. Служба сверяет sha256, распаковывает рядом, опускает
// туннель и запускает подменщика: копию ТЕКУЩЕГО affory-svc.exe во временном
// каталоге в режиме swap. Подменщик останавливает службу, подменяет файлы,
// ставит заново и ждёт ответа по каналу; не дождался за 20 секунд, возвращает
// прежнюю версию. Исход ложится в obnovlenie.json и показывается один раз.
//
// Подменщик именно текущий, а не новый: первый живой прогон 03.09.2026 запускал
// новый, и заведомо битая сборка (не служба вовсе) молча не сделала ничего,
// то есть откат не проверялся тем самым случаем, ради которого он написан.

const rezhimPodmeny = "swap"

// prefiksPodmeny начинает имя каталога, который подменщик заводит себе внутри
// каталога данных.
const prefiksPodmeny = "podmena-"

func (s *Sluzhba) installUpdate(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Put string `json:"path"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkazIz(k, protokol.KodProtocolMismatch, err)
		}
	}
	if telo.Put == "" {
		return otkaz(k.Id, k.Imya, protokol.KodArhivNegoden, "нужен путь к архиву сборки")
	}
	return s.ustanovitArhiv(k, telo.Put)
}

// sobytieHodaObnovleniya это имя события с шагами. Одно на весь путь: окно
// различает шаги по полю, а не по имени события, иначе подписок было бы пять.
const sobytieHodaObnovleniya = "obnovlenie-hod"

func (s *Sluzhba) hodObnovleniya(h protokol.HodObnovleniya) {
	s.izvestit(sobytieHodaObnovleniya, h)
}

// otkazObnovleniya гасит полосу в окне и отвечает отказом. Разделять эти два
// действия нельзя: отказ без события оставляет окно с полосой навсегда.
func (s *Sluzhba) otkazObnovleniya(k protokol.Kadr, kod, tekst string) protokol.Kadr {
	s.hodObnovleniya(protokol.HodObnovleniya{Shag: protokol.ShagOtkaz, Tekst: tekst})
	return otkaz(k.Id, k.Imya, kod, tekst)
}

// ustanovitArhiv это общая часть installUpdate и downloadUpdate: сверка,
// распаковка, опускание туннеля, подменщик.
func (s *Sluzhba) ustanovitArhiv(k protokol.Kadr, put string) protokol.Kadr {
	return s.ustanovitArhivSVersiey(k, put, "")
}

// ustanovitArhivSVersiey знает номер выпуска, когда архив приехал из сети.
// Архив с диска номера не несёт, и окно тогда подписывает полосу без него.
func (s *Sluzhba) ustanovitArhivSVersiey(k protokol.Kadr, put, versiya string) protokol.Kadr {
	// Подмена опускает службу, а с ней дело со своим сервером: установка
	// оборвалась бы между шагами на чужой машине. Поэтому идущее дело
	// отказывает обновлению, а обновление запирает новые дела. Запуск
	// подменщика запор не снимает: служба вот-вот остановится, а не
	// остановилась - запор кончится сам.
	otperet, tekst := s.zaperetVps(time.Now().Add(zaporNaObnovlenie), otkazObnovlenieIdyot)
	if otperet == nil {
		return s.otkazObnovleniya(k, protokol.KodVpsZanyato, tekst)
	}
	podmenshchikZapushchen := false
	defer func() {
		if !podmenshchikZapushchen {
			otperet()
		}
	}()
	if err := obnovlenie.Sverit(put); err != nil {
		return s.otkazObnovleniya(k, protokol.KodArhivNegoden, err.Error())
	}
	s.hodObnovleniya(protokol.HodObnovleniya{Shag: protokol.ShagRaspakovka, Versiya: versiya})
	novaya := filepath.Join(s.dirProgrammy, obnovlenie.KatalogNovoy)
	if err := obnovlenie.Raspakovat(put, novaya); err != nil {
		return s.otkazObnovleniya(k, protokol.KodArhivNegoden, err.Error())
	}
	if _, err := os.Stat(filepath.Join(novaya, "affory-svc.exe")); err != nil {
		return s.otkazObnovleniya(k, protokol.KodArhivNegoden, "в архиве нет affory-svc.exe")
	}
	if err := proveritNePonizhenie(novaya); err != nil {
		return s.otkazObnovleniya(k, protokol.KodArhivNegoden, err.Error())
	}
	// Был ли туннель поднят, запоминается ДО опускания (Н10 аудита 1.6.1):
	// новая служба поднимет его сама, человек в VPN не должен после
	// обновления оказаться без него. Защита отдельно не передаётся: её
	// настройка живёт в файле состояния и встаёт вместе с туннелем.
	bylPodnyat := s.vnutriSost() == protokol.SostPodnyat
	// Туннель опускается ДО подмены: правила и политика снимаются штатно, а
	// не остаются сиротами от службы, которую сейчас убьют.
	s.Otklyuchit()
	if oshib := s.vnutriOshib(); oshib != nil {
		return s.otkazObnovleniya(k, oshib.Kod, oshib.Tekst)
	}
	// Событие уходит ДО запуска подменщика: тот останавливает службу первым
	// делом, и после него до окна уже ничего не долетит.
	s.hodObnovleniya(protokol.HodObnovleniya{
		Shag: protokol.ShagPodmena, Versiya: versiya, SrokS: int(obnovlenie.SrokPodyoma.Seconds()),
	})
	if err := s.zapustitPodmenshchika(s.dirProgrammy, novaya, bylPodnyat); err != nil {
		return s.otkazObnovleniya(k, protokol.KodUpdateRollback, "подменщик не запущен: "+err.Error())
	}
	podmenshchikZapushchen = true
	return otvet(k.Id, k.Imya, map[string]any{"zapushchena": true, "srok_s": int(obnovlenie.SrokPodyoma.Seconds())})
}

// zaporNaObnovlenie держит задания своего сервера от проверки архива до
// остановки службы подменщиком: распаковка, опускание туннеля и запуск
// подменщика укладываются в него с запасом.
const zaporNaObnovlenie = 5 * time.Minute

const otkazObnovlenieIdyot = "ставится обновление клиента: со своим сервером можно будет работать после него"

// Шов для тестов: номер сборки из ресурсов exe.
var versiyaSborki = versiyaIzResursov

// proveritNePonizhenie отвергает архив старее установленной версии (Б1 аудита
// 1.6.1). Та же версия ставится: так чинят испорченную установку.
//
// Номер берётся из ресурсов affory-ui.exe, куда его прошивает sobrat-reliz.ps1
// (go-winres --file-version): запускать новый exe от SYSTEM ради номера нельзя,
// а имя архива человек волен назвать как угодно. Сборка без ресурсов (стенд,
// ручная сборка) и служба dev не судятся: по ним нечего сравнивать, и отказ
// сломал бы приёмку обновления в госте.
func proveritNePonizhenie(novaya string) error {
	if versiyaProgrammy == "dev" {
		return nil
	}
	vArhive, ok := versiyaSborki(filepath.Join(novaya, "affory-ui.exe"))
	if !ok {
		log.Printf("номер сборки в архиве не прочитан, понижение не проверено")
		return nil
	}
	if novee(versiyaProgrammy, vArhive) {
		return fmt.Errorf("архив версии %s старее установленной %s, понижение версии не ставится", vArhive, versiyaProgrammy)
	}
	return nil
}

// versiyaIzResursov читает X.Y.Z из VERSIONINFO файла, не запуская его.
func versiyaIzResursov(put string) (string, bool) {
	razmer, err := windows.GetFileVersionInfoSize(put, nil)
	if err != nil || razmer == 0 {
		return "", false
	}
	blok := make([]byte, razmer)
	if err := windows.GetFileVersionInfo(put, 0, razmer, unsafe.Pointer(&blok[0])); err != nil {
		return "", false
	}
	var info *windows.VS_FIXEDFILEINFO
	var dlina uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&blok[0]), `\`, unsafe.Pointer(&info), &dlina); err != nil || info == nil || dlina == 0 {
		return "", false
	}
	return fmt.Sprintf("%d.%d.%d", info.FileVersionMS>>16, info.FileVersionMS&0xffff, info.FileVersionLS>>16), true
}

// zapustitPodmenshchika копирует ТЕКУЩУЮ службу в свой каталог внутри каталога
// данных и запускает её в режиме swap отвязанным процессом: служба, которая
// сейчас отвечает, через секунды будет остановлена вместе со всеми своими
// детьми.
func zapustitPodmenshchika(prog, novaya string, podnyat bool) error {
	exe, err := prigotovitPodmenshchika(sostoyanie.KatalogDannyh())
	if err != nil {
		return err
	}
	argumenty := []string{rezhimPodmeny, prog, novaya}
	if podnyat {
		argumenty = append(argumenty, flagPodnyat)
	}
	cmd := exec.Command(exe, argumenty...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	return cmd.Start()
}

// prigotovitPodmenshchika кладёт копию службы в новый каталог внутри каталога
// данных и отдаёт путь к ней.
//
// До 1.6.2 копия ложилась в `%TEMP%\affory-podmena`. У SYSTEM на старых
// Windows 10 это `C:\Windows\Temp`, где обычный пользователь может завести
// каталог заранее: MkdirAll принимал его молча, и копию можно было подменить
// между записью и запуском от SYSTEM. Каталог данных закрыт от записи всем,
// кроме SYSTEM и администраторов, а MkdirTemp заводит каталог сам и никогда не
// берёт уже существующий.
func prigotovitPodmenshchika(dannye string) (string, error) {
	svoy, err := os.Executable()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(svoy)
	if err != nil {
		return "", err
	}
	vrem, err := os.MkdirTemp(dannye, prefiksPodmeny+"*")
	if err != nil {
		return "", err
	}
	exe := filepath.Join(vrem, "affory-svc.exe")
	if err := os.WriteFile(exe, b, 0o700); err != nil {
		return "", errors.Join(err, os.RemoveAll(vrem))
	}
	return exe, nil
}

// ubratKatalogiPodmeny убирает каталоги прошлых подменщиков из каталога данных
// и возвращает то, что убрать не вышло.
//
// Служба стартует раньше, чем её подменщик дождался ответа, поэтому каталог
// работающего подменщика здесь не убирается: его exe занят. Он уйдёт на
// следующем старте.
func ubratKatalogiPodmeny(dannye string) []string {
	zapisi, err := os.ReadDir(dannye)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []string{fmt.Sprintf("каталог %s не прочитан: %v", dannye, err)}
	}
	var zhaloby []string
	for _, z := range zapisi {
		if !z.IsDir() || !strings.HasPrefix(z.Name(), prefiksPodmeny) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dannye, z.Name())); err != nil {
			zhaloby = append(zhaloby, fmt.Sprintf("%s не убран: %v", z.Name(), err))
		}
	}
	return zhaloby
}

// flagPodnyat просит поднять туннель после обновления: у подменщика и у
// подкоманды install.
const flagPodnyat = "--podnyat"

func estFlagPodnyat(args []string) bool {
	return slices.Contains(args, flagPodnyat)
}

// argumentyUstanovki это подкоманда, которой подменщик ставит службу: и
// новую, и прежнюю при откате. Флаг едет в обоих случаях: туннель был поднят
// до обновления, и откат не повод оставить человека без него.
func argumentyUstanovki(podnyat bool) []string {
	if podnyat {
		return []string{"install", flagPodnyat}
	}
	return []string{"install"}
}

// podmenit это режим swap: тело подменщика. Пишет в log\obnovlenie.log: у
// отвязанного процесса нет ни консоли, ни родителя, которому жаловаться.
func podmenit(prog, novaya string, podnyat bool) {
	// Подменщик работает из временного каталога, а каталог программы знает
	// только из аргумента. Называем его сразу: иначе KatalogProgrammy, который
	// у всех остальных читается от своего бинаря, указал бы здесь в %TEMP%.
	sostoyanie.PodmenitKatalogProgrammy(prog)
	if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "obnovlenie.log"); err == nil {
		log.SetOutput(zh)
		defer zh.Close()
	}
	log.Printf("подмена начата: %s <- %s, туннель поднять после: %v", prog, novaya, podnyat)
	p := obnovlenie.Podmena{
		KatalogProgrammy: prog,
		Novaya:           novaya,
		Ostanovit:        ostanovitSluzhbu,
		Ustanovit: func() error {
			out, err := exec.Command(filepath.Join(prog, "affory-svc.exe"), argumentyUstanovki(podnyat)...).CombinedOutput()
			if err != nil {
				// Свой же бинарь, и всё равно перевод. Подкоманды пишут наружу в
				// кодовой странице машины: их вывод забирает установщик, который
				// читает трубу именно так. Прочитанный здесь как UTF-8, русский
				// текст не просто портится, а гибнет: причину отката мы кладём в
				// JSON (ZapisatItog), а маршалер заменяет негодные байты на U+FFFD.
				// Человек получил бы вместо объяснения ряд ромбиков.
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(kodirovki.Iz(out, kodirovki.Ansi)))
			}
			return nil
		},
		ZhdatOtveta: zhdatOtvetaSluzhby,
		Srok:        obnovlenie.SrokPodyoma,
	}
	itog := p.Vypolnit()
	// Подменщик это копия ПРЕЖНЕЙ службы, значит его версия и есть та, что
	// останется работать после отката. Ровно её и надо сравнивать потом.
	itog.Versiya = versiyaProgrammy
	log.Printf("подмена закончена: ok=%v %s %s", itog.Ok, itog.Kod, itog.Tekst)
	if err := obnovlenie.ZapisatItog(sostoyanie.KatalogDannyh(), itog); err != nil {
		log.Printf("исход обновления не записан: %v", err)
	}
}

func ostanovitSluzhbu() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(imyaSluzhby)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return err
	}
	return zhdatSostoyaniya(s, svc.Stopped)
}

// zhdatOtvetaSluzhby ждёт не «Running» у диспетчера, а ОТВЕТ по каналу: служба,
// висящая в Running с мёртвым каналом, это ровно тот случай, ради которого
// откат и заведён.
func zhdatOtvetaSluzhby(srok time.Duration) error {
	do := time.Now().Add(srok)
	var posledn error
	for time.Now().Before(do) {
		k, err := kanal.Podklyuchitsya()
		if err == nil {
			ctxOtv, otm := context.WithTimeout(context.Background(), 3*time.Second)
			_, err = zvatHello(ctxOtv, k)
			otm()
			_ = k.Zakryt()
			if err == nil {
				return nil
			}
		}
		posledn = err
		time.Sleep(500 * time.Millisecond)
	}
	return posledn
}

func zvatHello(ctx context.Context, k *kanal.Klient) (protokol.Kadr, error) {
	tip := make(chan struct{})
	var kadr protokol.Kadr
	var err error
	// Паника здесь закрыть tip не успеет, и ответ придёт сроком ctx.
	fon.Zapustit("проверке новой службы", func() {
		kadr, err = k.Zvat("hello", map[string]int{"protocol": protokol.Versiya})
		close(tip)
	})
	select {
	case <-tip:
		return kadr, err
	case <-ctx.Done():
		return protokol.Kadr{}, ctx.Err()
	}
}

// pokazatItogObnovleniya забирает исход прошлой подмены и показывает его в
// статусе. Успех идёт только в журнал. Зовётся на каждом status: исход
// появляется ПОСЛЕ старта новой службы, когда подменщик дождался её ответа.
func (s *Sluzhba) pokazatItogObnovleniya() {
	i, est, err := obnovlenie.ProchitatItog(s.dirDannyh)
	if err != nil {
		log.Printf("исход обновления не прочитан: %v", err)
		return
	}
	if !est {
		return
	}
	if i.Ok {
		log.Printf("обновление прошло, новая версия отвечает")
		return
	}
	// Жалоба старше текущей версии никого не касается: раз версия сменилась,
	// выпуск встал другим путём (установщиком руками), и совет «переустановите
	// из установщика» человек уже выполнил. Файл при этом забран чтением выше,
	// то есть всплыть повторно исходу нечем.
	if i.Versiya != "" && i.Versiya != versiyaProgrammy {
		log.Printf("исход обновления от версии %s отброшен: сейчас %s", i.Versiya, versiyaProgrammy)
		return
	}
	log.Printf("обновление откачено: %s %s", i.Kod, i.Tekst)
	s.mu.Lock()
	s.oshib = oshibkaDlyaOkna("обновление", i.Kod, i.Tekst)
	s.mu.Unlock()
}

// ubratHvostyPodmeny стирает *.ubrat в каталоге программы: файл работавшего
// окна, отодвинутый подменой, удалить в тот момент было нельзя. Заодно
// убираются каталоги прошлых подменщиков.
func ubratHvostyPodmeny() {
	hvosty, _ := filepath.Glob(filepath.Join(sostoyanie.KatalogProgrammy(), "*.ubrat"))
	for _, h := range hvosty {
		_ = os.Remove(h)
	}
	for _, z := range ubratKatalogiPodmeny(sostoyanie.KatalogDannyh()) {
		log.Printf("уборка после подмены: %s", z)
	}
}

// Паузы повторной уборки каталога подменщика. Переменная ради теста.
var pauzyUborkiPodmeny = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// DoubratPodmenu повторяет уборку каталогов подменщика в фоне (Х1 аудита
// 1.8.0). Подменщик жив, пока ждёт ответа новой службы, то есть дольше её
// старта, и уборка на старте его каталог не снимала: «Access is denied», и
// 20 МБ лежали до следующего перезапуска службы.
func (s *Sluzhba) DoubratPodmenu(dannye string) {
	if !estKatalogiPodmeny(dannye) || !s.zavestiFonovuyu() {
		return
	}
	fon.Zapustit("уборке после подмены", func() {
		defer s.fon.Done()
		var zhaloby []string
		for _, pauza := range pauzyUborkiPodmeny {
			select {
			case <-s.fonCtx.Done():
				return
			case <-time.After(pauza):
			}
			if zhaloby = ubratKatalogiPodmeny(dannye); len(zhaloby) == 0 {
				log.Printf("каталог подменщика убран после ответа новой версии")
				return
			}
		}
		for _, z := range zhaloby {
			log.Printf("уборка после подмены не удалась: %s", z)
		}
	})
}

// estKatalogiPodmeny отвечает, остался ли хоть один каталог подменщика.
func estKatalogiPodmeny(dannye string) bool {
	zapisi, err := os.ReadDir(dannye)
	if err != nil {
		return false
	}
	for _, z := range zapisi {
		if z.IsDir() && strings.HasPrefix(z.Name(), prefiksPodmeny) {
			return true
		}
	}
	return false
}

// Уборка скачанных обновлений.
//
// downloadUpdate кладёт каждый выпуск в obnovleniya и больше к нему не
// возвращается. На живой машине к 26.09.2026 там лежали одиннадцать архивов по
// 27 МБ, от 1.0.3 до 1.5.0, всего 284 МБ.
//
// Откату архивы не нужны: подменщик хранит прежнюю версию в predydushchaya
// рядом с программой и возвращает её оттуда копированием. Поэтому служба на
// старте убирает архивы СТАРШЕ работающей версии. Старт новой версии после
// подмены и есть «после успешной установки», а заодно так убирается и то, что
// накопилось до этой правки, и то, что осталось после установщика руками.
//
// Архив самой работающей версии остаётся: служба стартует ДО того, как
// подменщик дождался её ответа, и убрать архив, который ещё ставится, значило
// бы убирать до успеха. Архив новее работающей тоже остаётся: это след
// неудавшейся попытки, и следующая загрузка его всё равно перепишет. Итого
// в каталоге живёт один архив, а не история всех выпусков.
func ubratStaryeObnovleniya() {
	kat := filepath.Join(sostoyanie.KatalogDannyh(), katalogObnovleniy)
	ubrano, bayt, zhaloby := ubratStaryeArhivy(kat, versiyaProgrammy)
	if len(ubrano) > 0 {
		log.Printf("убраны скачанные обновления старше %s: %d файлов, %.1f МБ (%s)",
			versiyaProgrammy, len(ubrano), float64(bayt)/(1<<20), strings.Join(ubrano, ", "))
	}
	for _, z := range zhaloby {
		log.Printf("уборка скачанных обновлений: %s", z)
	}
}

// ubratStaryeArhivy удаляет из katalog архивы старше tekushchaya вместе с их
// .sha256 и говорит, что убрано, сколько байт и что не вышло.
func ubratStaryeArhivy(katalog, tekushchaya string) (ubrano []string, bayt int64, zhaloby []string) {
	// Сборка dev не знает, что старше неё, и не трогает ничего.
	if _, ok := razobratVersiyu(tekushchaya); !ok {
		return nil, 0, nil
	}
	zapisi, err := os.ReadDir(katalog)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, []string{fmt.Sprintf("каталог %s не прочитан: %v", katalog, err)}
	}
	for _, z := range zapisi {
		versiya, nash := versiyaArhiva(z.Name())
		if !nash || z.IsDir() || !novee(tekushchaya, versiya) {
			continue
		}
		var razmer int64
		if info, err := z.Info(); err == nil {
			razmer = info.Size()
		}
		if err := os.Remove(filepath.Join(katalog, z.Name())); err != nil {
			zhaloby = append(zhaloby, fmt.Sprintf("%s не удалён: %v", z.Name(), err))
			continue
		}
		ubrano = append(ubrano, z.Name())
		bayt += razmer
	}
	return ubrano, bayt, zhaloby
}

// versiyaArhiva узнаёт только свои имена: affory-X.Y.Z.zip и
// affory-X.Y.Z.zip.sha256, ровно те, что пишет downloadUpdate. Чужой файл в
// каталоге данных не наш, и решать за него нельзя.
func versiyaArhiva(imya string) (string, bool) {
	osnova := strings.TrimSuffix(imya, ".sha256")
	if !strings.HasPrefix(osnova, "affory-") || !strings.HasSuffix(osnova, ".zip") {
		return "", false
	}
	v := strings.TrimSuffix(strings.TrimPrefix(osnova, "affory-"), ".zip")
	if _, ok := razobratVersiyu(v); !ok {
		return "", false
	}
	return v, true
}
