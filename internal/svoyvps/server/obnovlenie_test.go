package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
)

func TestZamenitProgrammu(t *testing.T) {
	ctx := context.Background()
	nachalo := func(hesh, dop string) ssh.Itog {
		return ssh.Itog{Vyvod: []byte("ustanovleno\t" + hesh + "\nversiya\tvpn-uzel 0.6.0, sing-box 1.14.2\n" + dop)}
	}

	t.Run("новая уходит в загрузку и встаёт", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie nachalo"] = nachalo(hesh(t, "staraya"), "")
		m.otvety["obnovlenie zamenit"] = ssh.Itog{Vyvod: []byte("versiya\tvpn-uzel 0.7.0, sing-box 1.14.2\notkat\t600\ngotovo\tda\n")}
		z, err := ZamenitProgrammu(ctx, m, strings.NewReader("novaya"))
		if err != nil {
			t.Fatal(err)
		}
		if z.Uzhe || z.Versiya != "vpn-uzel 0.7.0, sing-box 1.14.2" || z.Bylo.Versiya != "vpn-uzel 0.6.0, sing-box 1.14.2" {
			t.Errorf("%+v", z)
		}
		if m.fayly[katalogZagruzki+"/vpn-uzel"] != "novaya" || m.rezhimy[katalogZagruzki+"/vpn-uzel"] != 0o700 {
			t.Errorf("файлы %q", m.fayly)
		}
		if !slices.Equal(m.vyzovy, []string{"obnovlenie nachalo", "fayl " + katalogZagruzki + "/vpn-uzel", "obnovlenie zamenit"}) {
			t.Errorf("вызовы %q", m.vyzovy)
		}
	})

	t.Run("та же и подтверждена: менять нечего", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie nachalo"] = nachalo(hesh(t, "novaya"), "")
		z, err := ZamenitProgrammu(ctx, m, strings.NewReader("novaya"))
		if err != nil || !z.Uzhe || len(m.vyzovy) != 1 {
			t.Errorf("%+v, %v, вызовы %q", z, err, m.vyzovy)
		}
	})

	t.Run("та же, но не подтверждена: замена с новым таймером", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie nachalo"] = nachalo(hesh(t, "novaya"), "otkat\tidyot\n")
		m.otvety["obnovlenie zamenit"] = ssh.Itog{Vyvod: []byte("versiya\tvpn-uzel 0.6.0, sing-box 1.14.2\notkat\t600\ngotovo\tda\n")}
		z, err := ZamenitProgrammu(ctx, m, strings.NewReader("novaya"))
		if err != nil || z.Uzhe || z.Versiya != "vpn-uzel 0.6.0, sing-box 1.14.2" {
			t.Errorf("%+v, %v", z, err)
		}
		if !slices.Equal(m.vyzovy, []string{"obnovlenie nachalo", "fayl " + katalogZagruzki + "/vpn-uzel", "obnovlenie zamenit"}) {
			t.Errorf("вызовы %q", m.vyzovy)
		}
	})

	t.Run("обрыв входа на замене это не отказ", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie nachalo"] = nachalo(hesh(t, "staraya"), "")
		obryv := errors.New("соединение сброшено")
		m.oshibkaNa = map[string]error{"obnovlenie zamenit": obryv}
		_, err := ZamenitProgrammu(ctx, m, strings.NewReader("novaya"))
		var oz *ObryvZameny
		if !errors.As(err, &oz) || !errors.Is(err, obryv) {
			t.Errorf("%v", err)
		}
	})

	t.Run("отказ скрипта на замене это отказ", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie nachalo"] = nachalo(hesh(t, "staraya"), "")
		m.otvety["obnovlenie zamenit"] = ssh.Itog{Kod: 1, Oshibki: "новая программа не принимает пакет сервера: ядро\n"}
		_, err := ZamenitProgrammu(ctx, m, strings.NewReader("novaya"))
		var o *Oshibka
		if !errors.As(err, &o) || !strings.Contains(o.Tekst, "не принимает пакет") {
			t.Errorf("%v", err)
		}
	})

	t.Run("подтверждение и откат отдают версию на месте", func(t *testing.T) {
		m := novaya()
		m.otvety["obnovlenie podtverdit"] = ssh.Itog{Vyvod: []byte("versiya\tvpn-uzel 0.7.0, sing-box 1.14.2\ngotovo\tda\n")}
		m.otvety["obnovlenie otkatit"] = ssh.Itog{Vyvod: []byte("versiya\tvpn-uzel 0.6.0, sing-box 1.14.2\n")}
		if v, err := PodtverditObnovlenie(ctx, m); err != nil || v != "vpn-uzel 0.7.0, sing-box 1.14.2" {
			t.Errorf("подтверждение %q, %v", v, err)
		}
		// Без строки gotovo успех не доказан.
		if _, err := OtkatitObnovlenie(ctx, m); err == nil {
			t.Error("откат без gotovo прошёл")
		}
	})
}

// programmaUzla кладёт подставную программу сервера: versiya отвечает
// версией v, proverit отказывает, пока задана PROVERIT_SBOY.
func programmaUzla(t *testing.T, put, v string) {
	t.Helper()
	zapisat(t, put, "#!/bin/sh",
		`case "$1" in`,
		`versiya) echo "vpn-uzel `+v+`, sing-box 1.14.2" ;;`,
		`proverit) if [ -n "${PROVERIT_SBOY:-}" ]; then echo "ядро: пакет не годится" >&2; exit 1; fi ;;`,
		`esac`)
	if err := os.Chmod(put, 0o755); err != nil {
		t.Fatal(err)
	}
}

// koreniObnovleniya это сервер после установки 0.6.0 с новой программой 0.7.0
// в загрузке и файл таймера отката.
func korenObnovleniya(t *testing.T) (koren, string) {
	t.Helper()
	k := novyyKoren(t)
	zapisat(t, k.f("/etc/affory/sostav.json"), `{"format":1,"sostoyanie":"gotovo","vpn_uzel":"vpn-uzel 0.6.0, sing-box 1.14.2","klyuchi":["AAAAnash1"]}`)
	programmaUzla(t, k.f("/usr/local/sbin/vpn-uzel"), "0.6.0")
	programmaUzla(t, k.f("/var/lib/affory/zagruzka/vpn-uzel"), "0.7.0")
	if err := os.MkdirAll(k.f("/run"), 0o700); err != nil {
		t.Fatal(err)
	}
	return k, k.put + "/otkat-taymer"
}

// prognatSOshibkami гоняет obnovlenie.sh и отдаёт ещё и его stderr: по
// нему видно, каким отказом он кончился.
func (k koren) prognatSOshibkami(t *testing.T, env []string, rezhim string) (int, string) {
	t.Helper()
	fayl := k.put + "/stderr"
	_, kod := k.prognat(t, "exec 2>>\"$OSHIBKI\"\n"+skriptObnovleniya, append(env, "OSHIBKI="+fayl), rezhim)
	b, err := os.ReadFile(fayl)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return kod, string(b)
}

func versiyaFayla(t *testing.T, put string) string {
	t.Helper()
	b, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"0.6.0", "0.7.0", "0.5.0"} {
		if strings.Contains(string(b), "vpn-uzel "+v+",") {
			return v
		}
	}
	return "?"
}

func TestObnovlenieSkript(t *testing.T) {
	prog, prezh := "/usr/local/sbin/vpn-uzel", "/usr/local/sbin/vpn-uzel.prezhnyaya"

	t.Run("замена: копия прежней, таймер раньше подмены, перезапуск очередью", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		out, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit")
		if kod != 0 {
			t.Fatalf("код %d, вывод %q", kod, out)
		}
		if !slices.Equal(out, []string{"versiya vpn-uzel 0.7.0, sing-box 1.14.2", "otkat 600", "gotovo da"}) {
			t.Errorf("вывод %q", out)
		}
		if versiyaFayla(t, k.f(prog)) != "0.7.0" || versiyaFayla(t, k.f(prezh)) != "0.6.0" {
			t.Errorf("на месте %s, копия %s", versiyaFayla(t, k.f(prog)), versiyaFayla(t, k.f(prezh)))
		}
		if !est(otkat) || est(k.f(prog+".novyy")) {
			t.Errorf("таймер %v, остался .novyy %v", est(otkat), est(k.f(prog+".novyy")))
		}
		log := strings.Join(chitat(t, k.log), "\n")
		iRun, iRestart := strings.Index(log, "systemd-run"), strings.Index(log, "systemctl --no-block restart vpn-uzel.service")
		if iRun < 0 || iRestart < iRun || !strings.Contains(log, "--on-active=600") {
			t.Errorf("журнал %q", log)
		}
		// Сработавшая служба таймера прошлого раза снимается вместе с ним.
		if !strings.Contains(log, "systemctl stop affory-obnovlenie-otkat.timer affory-obnovlenie-otkat.service") {
			t.Errorf("служба таймера не снималась: %q", log)
		}
		// Сам перезапуск не ждётся: SSH может оборваться вместе с программой.
		if slices.Contains(chitat(t, k.log), "systemctl restart vpn-uzel.service") {
			t.Errorf("перезапуск без очереди: %q", log)
		}
	})

	t.Run("новая не принимает живой пакет: ничего не тронуто", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		_, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat, "PROVERIT_SBOY=1"}, "zamenit")
		if kod != 1 {
			t.Fatalf("код %d", kod)
		}
		if versiyaFayla(t, k.f(prog)) != "0.6.0" || est(k.f(prezh)) || est(otkat) || est(k.f(prog+".novyy")) {
			t.Errorf("тронуто: на месте %s, копия %v, таймер %v", versiyaFayla(t, k.f(prog)), est(k.f(prezh)), est(otkat))
		}
		// Загрузка своё отслужила и место на диске не держит.
		if est(k.f("/var/lib/affory")) {
			t.Error("загрузка осталась")
		}
	})

	t.Run("новая не отвечает на versiya: понятный отказ", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, k.f("/var/lib/affory/zagruzka/vpn-uzel"), "#!/bin/sh", "exit 1")
		kod, oshibki := k.prognatSOshibkami(t, []string{"OTKAT=" + otkat}, "zamenit")
		if kod != 1 || !strings.Contains(oshibki, "новая программа не запускается") || versiyaFayla(t, k.f(prog)) != "0.6.0" {
			t.Errorf("код %d, stderr %q", kod, oshibki)
		}
	})

	t.Run("таймер не завёлся: программа не тронута, своей копии нет", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		_, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat, "SYSTEMD_RUN_SBOY=1"}, "zamenit")
		if kod != 1 || versiyaFayla(t, k.f(prog)) != "0.6.0" || est(k.f(prog+".novyy")) || est(k.f(prezh)) {
			t.Errorf("код %d, на месте %s, копия %v", kod, versiyaFayla(t, k.f(prog)), est(k.f(prezh)))
		}
		if slices.ContainsFunc(chitat(t, k.log), func(s string) bool {
			return strings.HasPrefix(s, "systemctl ") && strings.Contains(s, "restart")
		}) {
			t.Error("перезапуск без таймера")
		}
	})

	t.Run("прошлое обновление не подтверждено: копия остаётся проверенной", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		programmaUzla(t, k.f(prezh), "0.5.0")
		zapisat(t, otkat)
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 0 {
			t.Fatalf("код %d", kod)
		}
		if versiyaFayla(t, k.f(prezh)) != "0.5.0" || versiyaFayla(t, k.f(prog)) != "0.7.0" || !est(otkat) {
			t.Errorf("копия %s, на месте %s, таймер %v", versiyaFayla(t, k.f(prezh)), versiyaFayla(t, k.f(prog)), est(otkat))
		}
	})

	// Таймер пропал с перезагрузкой сервера: на месте всё равно
	// неподтверждённая, и проверенной остаётся копия.
	t.Run("копия без таймера тоже остаётся проверенной", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		programmaUzla(t, k.f(prezh), "0.5.0")
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 0 {
			t.Fatalf("код %d", kod)
		}
		if versiyaFayla(t, k.f(prezh)) != "0.5.0" || !est(otkat) {
			t.Errorf("копия %s, таймер %v", versiyaFayla(t, k.f(prezh)), est(otkat))
		}
	})

	t.Run("перезапуск не встал в очередь: прежняя на месте, таймер снят", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		kod, oshibki := k.prognatSOshibkami(t, []string{"OTKAT=" + otkat, "RESTART_SBOY=1"}, "zamenit")
		if kod != 1 || !strings.Contains(oshibki, "прежняя программа на месте") {
			t.Fatalf("код %d, stderr %q", kod, oshibki)
		}
		if versiyaFayla(t, k.f(prog)) != "0.6.0" || est(k.f(prezh)) || est(otkat) || est(k.f("/var/lib/affory")) {
			t.Errorf("на месте %s, копия %v, таймер %v, загрузка %v", versiyaFayla(t, k.f(prog)), est(k.f(prezh)), est(otkat), est(k.f("/var/lib/affory")))
		}
	})

	// Работает непроверенная программа прошлого раза: снять таймер значило бы
	// оставить её без отката.
	t.Run("перезапуск не встал, копия с прошлого раза: на диске проверенная, таймер идёт", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		programmaUzla(t, k.f(prezh), "0.5.0")
		zapisat(t, otkat)
		kod, oshibki := k.prognatSOshibkami(t, []string{"OTKAT=" + otkat, "RESTART_SBOY=1"}, "zamenit")
		if kod != 1 || !strings.Contains(oshibki, "прежнюю программу вернёт таймер") {
			t.Fatalf("код %d, stderr %q", kod, oshibki)
		}
		if versiyaFayla(t, k.f(prog)) != "0.5.0" || versiyaFayla(t, k.f(prezh)) != "0.5.0" || !est(otkat) || est(k.f(prog+".chast")) {
			t.Errorf("на месте %s, копия %s, таймер %v", versiyaFayla(t, k.f(prog)), versiyaFayla(t, k.f(prezh)), est(otkat))
		}
	})

	t.Run("замок занят: загрузка всё равно убрана", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, k.f(prog+".novyy"), "чужая")
		kod, oshibki := k.prognatSOshibkami(t, []string{"OTKAT=" + otkat, "FLOCK_ZANYAT=1"}, "zamenit")
		if kod != 1 || !strings.Contains(oshibki, "замок обновления занят") {
			t.Fatalf("код %d, stderr %q", kod, oshibki)
		}
		// Загрузка своё отслужила, а .novyy до замка может быть чужим.
		if est(k.f("/var/lib/affory")) || !est(k.f(prog+".novyy")) {
			t.Errorf("загрузка %v, .novyy %v", est(k.f("/var/lib/affory")), est(k.f(prog+".novyy")))
		}
	})

	t.Run("подтверждение: таймер и копия сняты, версия в отметке", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 0 {
			t.Fatalf("замена: код %d", kod)
		}
		out, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "podtverdit")
		if kod != 0 || !slices.Contains(out, "versiya vpn-uzel 0.7.0, sing-box 1.14.2") {
			t.Fatalf("код %d, вывод %q", kod, out)
		}
		if est(otkat) || est(k.f(prezh)) || est(k.f("/var/lib/affory")) {
			t.Errorf("таймер %v, копия %v, загрузка %v", est(otkat), est(k.f(prezh)), est(k.f("/var/lib/affory")))
		}
		otm := strings.Join(chitat(t, k.f("/etc/affory/sostav.json")), "")
		if otm != `{"format":1,"sostoyanie":"gotovo","vpn_uzel":"vpn-uzel 0.7.0, sing-box 1.14.2","klyuchi":["AAAAnash1"]}` {
			t.Errorf("отметка %s", otm)
		}
	})

	t.Run("подтверждение без снятого таймера отказывает", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, otkat)
		programmaUzla(t, k.f(prezh), "0.5.0")
		_, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat, "OTKAT_NE_SNIMAETSYA=1"}, "podtverdit")
		if kod != 1 || !est(k.f(prezh)) {
			t.Errorf("код %d, копия %v", kod, est(k.f(prezh)))
		}
	})

	t.Run("программа не назвала версию: подтверждения нет, таймер остаётся", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, otkat)
		programmaUzla(t, k.f(prezh), "0.6.0")
		zapisat(t, k.f(prog), "#!/bin/sh", "exit 1")
		_, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "podtverdit")
		otm := strings.Join(chitat(t, k.f("/etc/affory/sostav.json")), "")
		if kod != 1 || !est(otkat) || !est(k.f(prezh)) || !strings.Contains(otm, `"vpn_uzel":"vpn-uzel 0.6.0, sing-box 1.14.2"`) {
			t.Errorf("код %d, таймер %v, копия %v, отметка %s", kod, est(otkat), est(k.f(prezh)), otm)
		}
	})

	t.Run("откат: прежняя на месте, перезапуск, таймер снят", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 0 {
			t.Fatalf("замена: код %d", kod)
		}
		out, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "otkatit")
		if kod != 0 || !slices.Contains(out, "versiya vpn-uzel 0.6.0, sing-box 1.14.2") {
			t.Fatalf("код %d, вывод %q", kod, out)
		}
		if versiyaFayla(t, k.f(prog)) != "0.6.0" || est(k.f(prezh)) || est(otkat) {
			t.Errorf("на месте %s, копия %v, таймер %v", versiyaFayla(t, k.f(prog)), est(k.f(prezh)), est(otkat))
		}
		if n := strings.Count(strings.Join(chitat(t, k.log), "\n"), "systemctl --no-block restart vpn-uzel.service"); n != 2 {
			t.Errorf("перезапусков %d", n)
		}
	})

	t.Run("состояние называет программу и таймер", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, otkat)
		out, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "sostoyanie")
		if kod != 0 || len(out) != 3 || !strings.HasPrefix(out[0], "ustanovleno ") || out[1] != "versiya vpn-uzel 0.6.0, sing-box 1.14.2" || out[2] != "otkat idyot" {
			t.Errorf("код %d, вывод %q", kod, out)
		}
		if est(k.f("/var/lib/affory/zagruzka/.")) && !est(k.f("/var/lib/affory/zagruzka/vpn-uzel")) {
			t.Error("состояние создало каталог загрузки")
		}
		// Таймер пропал с перезагрузкой, копия лежит: всё равно не подтверждена.
		k, otkat = korenObnovleniya(t)
		programmaUzla(t, k.f(prezh), "0.5.0")
		out, kod = k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "sostoyanie")
		if kod != 0 || !slices.Contains(out, "otkat idyot") {
			t.Errorf("без таймера: код %d, вывод %q", kod, out)
		}
		k, otkat = korenObnovleniya(t)
		if out, kod = k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "sostoyanie"); kod != 0 || slices.Contains(out, "otkat idyot") {
			t.Errorf("подтверждённая: код %d, вывод %q", kod, out)
		}
		// Таймер сработал и вернул прежнюю: копии нет, а сам он ещё числится
		// active.
		k, otkat = korenObnovleniya(t)
		zapisat(t, otkat)
		if out, kod = k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat, "OTKAT_SRABOTAL=1"}, "sostoyanie"); kod != 0 || slices.Contains(out, "otkat idyot") {
			t.Errorf("сработавший таймер: код %d, вывод %q", kod, out)
		}
	})

	t.Run("срок таймера в скрипте тот же, что знает клиент", func(t *testing.T) {
		if !strings.Contains(skriptObnovleniya, fmt.Sprintf("\nsrok_otkata=%d\n", int(SrokOtkata.Seconds()))) {
			t.Errorf("в obnovlenie.sh другой srok_otkata, чем SrokOtkata %v", SrokOtkata)
		}
	})

	t.Run("сервер под центром и незаконченная установка не трогаются", func(t *testing.T) {
		k, otkat := korenObnovleniya(t)
		zapisat(t, k.f("/usr/local/sbin/vpn-nablyudenie"), "x")
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 1 || versiyaFayla(t, k.f(prog)) != "0.6.0" {
			t.Errorf("под центром: код %d", kod)
		}
		k, otkat = korenObnovleniya(t)
		zapisat(t, k.f("/etc/affory/sostav.json"), `{"format":1,"sostoyanie":"ustanovka"}`)
		if _, kod := k.prognat(t, skriptObnovleniya, []string{"OTKAT=" + otkat}, "zamenit"); kod != 1 || versiyaFayla(t, k.f(prog)) != "0.6.0" {
			t.Errorf("незаконченная установка: код %d", kod)
		}
	})
}
