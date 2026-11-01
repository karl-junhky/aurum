// Package server ведёт серверную часть своего VPN: осмотр машины, установку
// программы vpn-uzel, порты в ufw, снос и чтение прошлой установки. Сами
// действия делают скрипты bash из skripty/ через SSH-слой, здесь их запуск
// и разбор ответа. Скрипты печатают строки «ключ<TAB>значение».
//
// Как и SSH-слой, пакет живёт в службе: каждый вход под перехватом паники,
// тексты для человека не пишутся (осмотр отдаёт замечания с кодами), чужой
// текст с сервера чистится до журнала.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

var (
	//go:embed skripty/osmotr.sh
	skriptOsmotra string
	//go:embed skripty/ochered.sh
	obshcheeOcheredi string
	//go:embed skripty/ustanovka.sh
	tekstUstanovki string
	//go:embed skripty/porty.sh
	skriptPortov string
	//go:embed skripty/snos.sh
	tekstSnosa string
	//go:embed skripty/vpn-uzel.service
	yunit []byte

	// Установка и снос одинаково решают, чей корень очереди интерфейса:
	// общий кусок идёт перед каждым из них.
	skriptUstanovki = obshcheeOcheredi + tekstUstanovki
	skriptSnosa     = obshcheeOcheredi + tekstSnosa
)

const (
	katalogZagruzki = "/var/lib/affory/zagruzka"
	putOtmetki      = "/etc/affory/sostav.json"
	putPaketa       = "/var/lib/vpn-uzel/sostoyanie.json"
)

// Mashina это вход на сервер с правами root, *ssh.Soedinenie.
type Mashina interface {
	Vypolnit(ctx context.Context, komanda string, vhod io.Reader) (ssh.Itog, error)
	Skript(ctx context.Context, tekst string, argumenty ...string) (ssh.Itog, error)
	OtpravitFayl(ctx context.Context, dannye io.ReadSeeker, kuda string, rezhim fs.FileMode) error
}

// Oshibka это отказ скрипта на сервере.
type Oshibka struct {
	// Shag: nastroyka, osmotr, ustanovka, porty, snos, chtenie.
	Shag string
	Kod  int
	// Tekst это последняя строка stderr, очищенная.
	Tekst string
	// NeVyshlo это шаги сноса, которые не снялись; отметка тогда на месте и
	// снос можно повторить.
	NeVyshlo []string
	// KlyuchOstalsya: снос прошёл весь, но строки наших ключей в
	// authorized_keys root остались.
	KlyuchOstalsya bool
}

func (o *Oshibka) Error() string {
	s := fmt.Sprintf("%s: код %d", o.Shag, o.Kod)
	if o.Tekst != "" {
		s += ": " + o.Tekst
	}
	if len(o.NeVyshlo) > 0 {
		s += "; не снялось: " + strings.Join(o.NeVyshlo, ", ")
	}
	if o.KlyuchOstalsya {
		s += "; строки ключей в authorized_keys root остались"
	}
	return s
}

// ErrNetUstanovki: на сервере нет отметки нашей установки.
var ErrNetUstanovki = errors.New("на сервере нет отметки установки")

func oshibkaSkripta(shag string, it ssh.Itog) *Oshibka {
	return &Oshibka{Shag: shag, Kod: it.Kod, Tekst: ssh.PoslednyayaStroka(it.Oshibki)}
}

// zashchita ставится первой строкой каждого входа в пакет: разбор чужого
// вывода не должен ронять службу вместе с туннелем.
func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/server."+gde, r)
		*err = fmt.Errorf("паника в server.%s: %v", gde, r)
	}
}

// stroki обходит строки «ключ<TAB>значение» вывода скрипта. Значение
// чистится от управляющих символов и режется: оно идёт в журнал и окно.
func stroki(vyvod []byte, fn func(k, z string)) {
	for s := range strings.Lines(string(vyvod)) {
		k, z, ok := strings.Cut(strings.TrimRight(s, "\r\n"), "\t")
		if ok {
			fn(k, z)
		}
	}
}

func chisto(z string) string { return ssh.Obrezat(z, 300) }

func chislo(z string) int {
	n, err := strconv.Atoi(strings.TrimSpace(z))
	if err != nil {
		return -1
	}
	return n
}

// Ustanovka это итог установки.
type Ustanovka struct {
	// Versiya это ответ vpn-uzel versiya.
	Versiya string
	// Perezapushchen: что сменилось и потребовало перезапуска службы
	// (programma, yunit), или ne-rabotala; пусто - служба шла как шла.
	Perezapushchen   string
	Preduprezhdeniya []string
}

// Ustanovit ставит программу и юнит. klyuch это ключ управления этого
// компьютера, уже лежащий в authorized_keys root: первым шагом, до загрузки
// файлов, он записывается в отметку, и снос снимет его строку, на каком бы
// шаге установка ни оборвалась. Файл, установленный с тем же sha256, по сети
// не идёт; остальные кладутся в каталог загрузки, и повтор после обрыва
// докачивает только несошедшиеся. Порты открывает OtkrytPorty после подбора.
func Ustanovit(ctx context.Context, m Mashina, programma io.ReadSeeker, klyuch gossh.PublicKey) (u Ustanovka, err error) {
	defer zashchita("Ustanovit", &err)
	telo := teloKlyucha(klyuch)
	it, err := m.Skript(ctx, skriptUstanovki, "nachalo", telo)
	if err != nil {
		return u, err
	}
	if it.Kod != 0 {
		return u, oshibkaSkripta("ustanovka", it)
	}
	ustanovleno := map[string]string{}
	stroki(it.Vyvod, func(k, z string) {
		if hesh, imya, ok := strings.Cut(z, " "); k == "ustanovleno" && ok {
			ustanovleno[imya] = hesh
		}
	})
	var lishnie []string
	for _, f := range []struct {
		imya   string
		dannye io.ReadSeeker
		rezhim fs.FileMode
	}{
		{"vpn-uzel", programma, 0o700},
		{"vpn-uzel.service", bytes.NewReader(yunit), 0o600},
	} {
		hesh, err := sha256Chitatelya(f.dannye)
		if err != nil {
			return u, err
		}
		kuda := katalogZagruzki + "/" + f.imya
		if ustanovleno[f.imya] == hesh {
			// Копия прошлой оборванной попытки в загрузке встала бы поверх
			// установленного.
			lishnie = append(lishnie, kuda)
			continue
		}
		if err := m.OtpravitFayl(ctx, f.dannye, kuda, f.rezhim); err != nil {
			return u, err
		}
	}
	if len(lishnie) > 0 {
		it, err := m.Vypolnit(ctx, "rm -f -- "+strings.Join(lishnie, " "), nil)
		if err != nil {
			return u, err
		}
		if it.Kod != 0 {
			return u, oshibkaSkripta("ustanovka", it)
		}
	}
	it, err = m.Skript(ctx, skriptUstanovki, telo)
	if err != nil {
		return u, err
	}
	gotovo := false
	stroki(it.Vyvod, func(k, z string) {
		switch k {
		case "versiya":
			u.Versiya = chisto(z)
		case "perezapushchen":
			u.Perezapushchen = chisto(z)
		case "preduprezhdenie":
			u.Preduprezhdeniya = append(u.Preduprezhdeniya, chisto(z))
		case "gotovo":
			gotovo = true
		}
	})
	if it.Kod != 0 || !gotovo {
		return u, oshibkaSkripta("ustanovka", it)
	}
	return u, nil
}

// ZapisatKlyuch записывает в отметку ключ управления ещё одного компьютера
// (захват): снос снимет и его строку.
func ZapisatKlyuch(ctx context.Context, m Mashina, klyuch gossh.PublicKey) (err error) {
	defer zashchita("ZapisatKlyuch", &err)
	it, err := m.Skript(ctx, skriptUstanovki, "klyuch", teloKlyucha(klyuch))
	if err != nil {
		return err
	}
	if it.Kod != 0 {
		return oshibkaSkripta("ustanovka", it)
	}
	return nil
}

func teloKlyucha(k gossh.PublicKey) string {
	return base64.StdEncoding.EncodeToString(k.Marshal())
}

func sha256Chitatelya(r io.ReadSeeker) (string, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("файл не перемотан: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("файл не прочитан: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DozhdatsyaNastroyki ждёт, пока cloud-init закончит первую настройку
// машины, не дольше 5 минут (§3.8 плана): в первые минуты после создания
// сервера она ещё ставит пакеты и правит sshd. Нет cloud-init - ждать
// нечего.
func DozhdatsyaNastroyki(ctx context.Context, m Mashina) (err error) {
	defer zashchita("DozhdatsyaNastroyki", &err)
	it, err := m.Vypolnit(ctx, `command -v cloud-init >/dev/null 2>&1 || exit 0
command -v timeout >/dev/null 2>&1 || exit 0
timeout 300 cloud-init status --wait >/dev/null 2>&1
[ $? -ne 124 ] || exit 124`, nil)
	if err != nil {
		return err
	}
	if it.Kod == 124 {
		return &Oshibka{Shag: "nastroyka", Kod: it.Kod, Tekst: "cloud-init не закончил за 5 минут"}
	}
	if it.Kod != 0 {
		return oshibkaSkripta("nastroyka", it)
	}
	return nil
}

// Porty это итог правки ufw.
type Porty struct {
	// UFW: active, inactive (порты не открывались, снимались только наши
	// прежние правила) или net (ufw нет, правки не было).
	UFW string
	// Otkryty: наши правила на месте. UzheOtkryty: порт открыт чужим
	// правилом, его не трогали. Вид «443/tcp».
	Otkryty, UzheOtkryty []string
	// Zakryto: сколько наших правил снято; правило это сразу IPv4 и IPv6.
	Zakryto int
}

// OtkrytPorty приводит наши правила ufw к портам пакета: недостающие
// открывает при включённом ufw, лишние наши закрывает при любом. Пустые
// списки закрывают все наши.
func OtkrytPorty(ctx context.Context, m Mashina, tcp, udp []int) (p Porty, err error) {
	defer zashchita("OtkrytPorty", &err)
	// Повтор порта в списке скрипт открывал бы дважды.
	spisok := func(porty []int) (string, error) {
		var s []string
		for _, n := range porty {
			if n < 1 || n > 65535 {
				return "", fmt.Errorf("порт %d вне 1-65535", n)
			}
			if !slices.Contains(s, strconv.Itoa(n)) {
				s = append(s, strconv.Itoa(n))
			}
		}
		return strings.Join(s, ","), nil
	}
	t, err := spisok(tcp)
	if err != nil {
		return p, err
	}
	u, err := spisok(udp)
	if err != nil {
		return p, err
	}
	it, err := m.Skript(ctx, skriptPortov, t, u)
	if err != nil {
		return p, err
	}
	if it.Kod != 0 {
		return p, oshibkaSkripta("porty", it)
	}
	stroki(it.Vyvod, func(k, z string) {
		switch k {
		case "ufw":
			p.UFW = chisto(z)
		case "otkryt":
			p.Otkryty = append(p.Otkryty, chisto(z))
		case "uzhe_otkryt":
			p.UzheOtkryty = append(p.UzheOtkryty, chisto(z))
		case "zakryt":
			p.Zakryto++
		}
	})
	return p, nil
}

// Snos это итог сноса.
type Snos struct {
	// BezOtmetki: отметки не было, снимать было нечего.
	BezOtmetki bool
	// ParolVernulsya: наш файл sshd снят, вход по паролю снова такой, каким
	// был до выключения.
	ParolVernulsya bool
}

// Snesti снимает с сервера всё своё по отметке (§3.9 плана): сначала наши
// правила ufw, потом остальное. Отказ на середине оставляет отметку, и снос
// повторяется с начала.
func Snesti(ctx context.Context, m Mashina) (s Snos, err error) {
	defer zashchita("Snesti", &err)
	if _, err := OtkrytPorty(ctx, m, nil, nil); err != nil {
		return s, err
	}
	it, err := m.Skript(ctx, skriptSnosa)
	if err != nil {
		return s, err
	}
	gotovo := false
	var neVyshlo []string
	stroki(it.Vyvod, func(k, z string) {
		switch k {
		case "otmetki":
			s.BezOtmetki, gotovo = true, true
		case "parol":
			s.ParolVernulsya = true
		case "ne_vyshlo":
			neVyshlo = append(neVyshlo, chisto(z))
		case "gotovo":
			gotovo = true
		}
	})
	switch {
	case it.Kod == 4:
		o := oshibkaSkripta("snos", it)
		o.KlyuchOstalsya = true
		return s, o
	case it.Kod != 0 || !gotovo:
		o := oshibkaSkripta("snos", it)
		o.NeVyshlo = neVyshlo
		return s, o
	}
	return s, nil
}

// Sostav это отметка установки, /etc/affory/sostav.json.
type Sostav struct {
	Format int `json:"format"`
	// Sostoyanie: ustanovka (установка не дошла до конца) или gotovo.
	Sostoyanie string `json:"sostoyanie"`
	// VpnUzel это ответ vpn-uzel versiya на момент установки.
	VpnUzel string `json:"vpn_uzel"`
	// Klyuchi это тела ключей управления в base64.
	Klyuchi []string `json:"klyuchi"`
}

// ProchitatUstanovku читает отметку и принятый программой пакет: из них
// захват собирает те же ключи без перевыпуска. Пакета нет (установка
// оборвалась до него) - paket пустой. Отметки нет - ErrNetUstanovki.
// В пакете секреты: в журнал он не идёт.
func ProchitatUstanovku(ctx context.Context, m Mashina) (s Sostav, paket []byte, err error) {
	defer zashchita("ProchitatUstanovku", &err)
	prochitat := func(put string) ([]byte, bool, error) {
		it, err := m.Vypolnit(ctx, fmt.Sprintf("if [ -f %[1]s ]; then cat -- %[1]s; else exit 3; fi", put), nil)
		if err != nil {
			return nil, false, err
		}
		switch it.Kod {
		case 0:
			return it.Vyvod, true, nil
		case 3:
			return nil, false, nil
		}
		return nil, false, oshibkaSkripta("chtenie", it)
	}
	otmetka, est, err := prochitat(putOtmetki)
	if err != nil {
		return s, nil, err
	}
	if !est {
		return s, nil, ErrNetUstanovki
	}
	if s, err = razobratOtmetku(otmetka); err != nil {
		return s, nil, err
	}
	paket, _, err = prochitat(putPaketa)
	if err != nil {
		return s, nil, err
	}
	return s, paket, nil
}
