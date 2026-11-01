//go:build zhivoy

package ssh

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// zhivyeSroki ставит настоящие пределы вместо сжатых в TestMain и
// возвращает прежние после теста.
func zhivyeSroki(t *testing.T) {
	popytka, puls, proverki, pauzy := srokPopytki, intervalPulsa, srokProverok, pauzyPovtora
	t.Cleanup(func() {
		srokPopytki, intervalPulsa, srokProverok, pauzyPovtora = popytka, puls, proverki, pauzy
	})
	srokPopytki, intervalPulsa, srokProverok = 30*time.Second, 15*time.Second, 20*time.Second
	pauzyPovtora = []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second}
}

// Живая проверка слоя на настоящей машине, только по метке:
//
//	AFFORY_SSH_ADRES=<адрес> AFFORY_SSH_KLYUCH=<файл ключа root> AFFORY_SSH_PUT=pryamo|tunnel \
//	go test -tags zhivoy -run TestZhivoy -v ./internal/svoyvps/ssh/
//
// Адреса в репозиторий не попадают: он публичный. Машина остаётся как была:
// пробный каталог и пробный ключ root убираются в конце.
func TestZhivoy(t *testing.T) {
	adres, putKlyucha := os.Getenv("AFFORY_SSH_ADRES"), os.Getenv("AFFORY_SSH_KLYUCH")
	if adres == "" || putKlyucha == "" {
		t.Skip("нет AFFORY_SSH_ADRES и AFFORY_SSH_KLYUCH")
	}
	klyuch, err := os.ReadFile(putKlyucha)
	if err != nil {
		t.Fatal(err)
	}
	zhivyeSroki(t)
	var put Put
	switch os.Getenv("AFFORY_SSH_PUT") {
	case "tunnel":
		put = PutCherezTunnel("127.0.0.1:10809")
	default:
		a, err := FizicheskiyAdapter()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("адаптер %s (%s), индекс %d", a.Imya, a.Opisanie, a.Indeks)
		put = PutPryamo(a.Indeks, a.Imya)
	}
	ctx, otmena := context.WithTimeout(context.Background(), 3*time.Minute)
	defer otmena()

	z := Zapros{Adres: adres, Vhod: Vhod{Polzovatel: "root", Klyuch: klyuch}, Put: put}
	nachalo := time.Now()
	s, err := Podklyuchit(ctx, z)
	if err != nil {
		t.Fatalf("вход: %v", err)
	}
	defer s.Zakryt()
	t.Logf("вход %v %s, ключ хоста %s %s", time.Since(nachalo).Round(time.Millisecond), s.Put, s.KlyuchHosta.Tip, s.KlyuchHosta.Otpechatok())
	t.Logf("сервер видит подключение с %s, свои адреса %v", s.otkuda, s.svoi)

	it, err := s.Vypolnit(ctx, ". /etc/os-release; echo \"$PRETTY_NAME\"; locale | head -n 1; readlink -f \"$(command -v sudo)\" \"$(command -v install)\"", nil)
	if err != nil || it.Kod != 0 {
		t.Fatalf("система: %v %d %q", err, it.Kod, it.Oshibki)
	}
	t.Logf("система:\n%s", it.Vyvod)

	it, err = s.Skript(ctx, `printf '%s|' "$@"; echo; echo "$LC_ALL"`, "a b", "c'd", `$(id)`, "")
	if err != nil || it.Kod != 0 || string(it.Vyvod) != "a b|c'd|$(id)||\nC\n" {
		t.Fatalf("скрипт с аргументами: %v %d %q %q", err, it.Kod, it.Vyvod, it.Oshibki)
	}

	const katalog = "/tmp/affory-proba-ssh"
	defer s.Vypolnit(context.Background(), "rm -rf -- "+katalog, nil)
	if it, err := s.Vypolnit(ctx, "mkdir -p -- "+katalog, nil); err != nil || it.Kod != 0 {
		t.Fatalf("каталог: %v %q", err, it.Oshibki)
	}
	dannye := make([]byte, 3<<20)
	rand.Read(dannye)
	nachalo = time.Now()
	if err := s.OtpravitFayl(ctx, bytes.NewReader(dannye), katalog+"/f", 0o755); err != nil {
		t.Fatalf("файл: %v", err)
	}
	t.Logf("3 МБ за %v", time.Since(nachalo).Round(time.Millisecond))
	it, err = s.Vypolnit(ctx, "stat -c '%a %s' -- "+katalog+"/f; ls -a -- "+katalog, nil)
	if err != nil || !strings.HasPrefix(string(it.Vyvod), "755 3145728\n") || strings.Contains(string(it.Vyvod), "affory-chast") {
		t.Fatalf("файл на месте: %v %q %q", err, it.Vyvod, it.Oshibki)
	}
	// Тот же файл второй раз не передаётся, права меняются.
	nachalo = time.Now()
	if err := s.OtpravitFayl(ctx, bytes.NewReader(dannye), katalog+"/f", 0o600); err != nil {
		t.Fatalf("тот же файл: %v", err)
	}
	t.Logf("сошедшийся файл за %v", time.Since(nachalo).Round(time.Millisecond))
	// Поверх существующего другим содержимым: путь, на котором uutils
	// install ломался.
	dannye[0] ^= 0xff
	if err := s.OtpravitFayl(ctx, bytes.NewReader(dannye), katalog+"/f", 0o644); err != nil {
		t.Fatalf("замена файла: %v", err)
	}
	it, err = s.Vypolnit(ctx, "stat -c '%a' -- "+katalog+"/f; sha256sum -- "+katalog+"/f", nil)
	if err != nil || !strings.HasPrefix(string(it.Vyvod), "644\n") {
		t.Fatalf("после замены: %v %q", err, it.Vyvod)
	}

	it, err = s.Skript(ctx, skriptVhodaRoot, "198.51.100.7")
	if err != nil || it.Kod != 0 {
		t.Fatalf("sshd -T: %v %q", err, it.Oshibki)
	}
	t.Logf("permitrootlogin: %q", strings.TrimSpace(string(it.Vyvod)))

	zhivyeSkriptyKlyucha(ctx, t, s, katalog+"/ak")

	// Пробный ключ root: ложится один раз, слой входит им сам, потом
	// строка убирается тем же скриптом, что и при откате.
	probnyy := novyyKlyuch(t)
	stroka := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(probnyy.PublicKey()))) + " affory-proba-ssh"
	teloProbnogo := strings.Fields(stroka)[1]
	it, err = s.Vypolnit(ctx, "md5sum /root/.ssh/authorized_keys; stat -c '%a' /root/.ssh /root/.ssh/authorized_keys", nil)
	if err != nil {
		t.Fatal(err)
	}
	do := string(it.Vyvod)
	defer func() {
		ctx := context.Background()
		if err := s.ubratKlyuchRoot(ctx, stroka); err != nil {
			t.Errorf("пробный ключ root не убран: %v", err)
		}
		it, err := s.Vypolnit(ctx, "md5sum /root/.ssh/authorized_keys; stat -c '%a' /root/.ssh /root/.ssh/authorized_keys", nil)
		if err != nil || string(it.Vyvod) != do {
			t.Errorf("authorized_keys не вернулся: было %q, стало %q, %v", do, it.Vyvod, err)
		}
	}()
	for range 2 {
		r, err := s.PolozhitKlyuchRoot(ctx, probnyy, "affory-proba-ssh", true)
		if err != nil {
			t.Fatalf("ключ root: %v", err)
		}
		if !r.Root || r.KlyuchHosta.Otpechatok() != s.KlyuchHosta.Otpechatok() {
			t.Fatalf("вход пробным ключом: root %v, ключ хоста %s", r.Root, r.KlyuchHosta.Otpechatok())
		}
		r.Zakryt()
	}
	it, err = s.Vypolnit(ctx, "grep -cF -- '"+teloProbnogo+"' /root/.ssh/authorized_keys; stat -c '%a' /root/.ssh /root/.ssh/authorized_keys", nil)
	_, pravaDo, _ := strings.Cut(do, "\n")
	if err != nil || string(it.Vyvod) != "1\n"+pravaDo {
		t.Fatalf("ключ root лёг один раз, права прежние (%q): %v %q", pravaDo, err, it.Vyvod)
	}

	// Чужой записанный ключ хоста: отказ до входа.
	zk := z
	zk.Vhod = Vhod{Polzovatel: "root", Podpis: probnyy}
	chuzhoy := klyuchHostaIz(novyyKlyuch(t).PublicKey())
	zk.Izvesten = &chuzhoy
	_, err = Podklyuchit(ctx, zk)
	if PrichinaIz(err) != PrichinaKlyuchHostaSmenilsya {
		t.Fatalf("чужой ключ хоста: %v", err)
	}

	if os.Getenv("AFFORY_SSH_SUDO") == "1" {
		zhivoySudo(ctx, t, z, s)
	}
}

// zhivyeSkriptyKlyucha гоняет скрипты укладки и уборки на временном файле:
// чужая строка без перевода строки в конце, права 644, потом файл из одной
// нашей строки.
func zhivyeSkriptyKlyucha(ctx context.Context, t *testing.T, s *Soedinenie, f string) {
	nasha := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(novyyKlyuch(t).PublicKey()))) + " affory-proba"
	chuzhaya := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(novyyKlyuch(t).PublicKey()))) + " chuzhoy"
	sostoyanie := func() string {
		it, err := s.Vypolnit(ctx, "stat -c '%a' -- "+kavychki(f)+"; cat -- "+kavychki(f), nil)
		if err != nil || it.Kod != 0 {
			t.Fatalf("временный authorized_keys: %v %q", err, it.Oshibki)
		}
		return string(it.Vyvod)
	}
	skript := func(tekst, ozhidaem string) {
		it, err := s.Skript(ctx, tekst, nasha, f)
		if err != nil || it.Kod != 0 || string(it.Vyvod) != ozhidaem {
			t.Fatalf("скрипт ключа: %v %d %q %q, ждала вывод %q", err, it.Kod, it.Vyvod, it.Oshibki, ozhidaem)
		}
	}
	if it, err := s.Vypolnit(ctx, "printf '%s' "+kavychki(chuzhaya)+" > "+kavychki(f)+"; chmod 644 -- "+kavychki(f), nil); err != nil || it.Kod != 0 {
		t.Fatalf("временный authorized_keys: %v %q", err, it.Oshibki)
	}
	skript(skriptKlyuchaRoot, "dobavlen\n")
	skript(skriptKlyuchaRoot, "")
	if got, nado := sostoyanie(), "644\n"+chuzhaya+"\n"+nasha+"\n"; got != nado {
		t.Fatalf("после укладки %q, ждала %q", got, nado)
	}
	skript(skriptUbratKlyuchRoot, "")
	if got, nado := sostoyanie(), "644\n"+chuzhaya+"\n"; got != nado {
		t.Fatalf("после уборки %q, ждала %q", got, nado)
	}
	// Файл из одной нашей строки: grep -v ничего не находит и выходит с 1.
	if it, err := s.Vypolnit(ctx, "printf '%s\\n' "+kavychki(nasha)+" > "+kavychki(f), nil); err != nil || it.Kod != 0 {
		t.Fatalf("временный authorized_keys: %v %q", err, it.Oshibki)
	}
	skript(skriptUbratKlyuchRoot, "")
	if got := sostoyanie(); got != "644\n" {
		t.Fatalf("после уборки единственной строки %q, ждала пустой файл 644", got)
	}
	// Пустое тело совпало бы с каждой строкой: уборка обязана отказать и не
	// тронуть файл.
	if it, err := s.Vypolnit(ctx, "printf '%s\\n' "+kavychki(chuzhaya)+" > "+kavychki(f), nil); err != nil || it.Kod != 0 {
		t.Fatalf("временный authorized_keys: %v %q", err, it.Oshibki)
	}
	if it, err := s.Skript(ctx, skriptUbratKlyuchRoot, "ssh-ed25519", f); err != nil || it.Kod != 2 {
		t.Fatalf("уборка с пустым телом: %v, код %d, %q", err, it.Kod, it.Oshibki)
	}
	if got, nado := sostoyanie(), "644\n"+chuzhaya+"\n"; got != nado {
		t.Fatalf("после уборки с пустым телом %q, ждала %q", got, nado)
	}
	it, err := s.Vypolnit(ctx, "ls -a -- "+kavychki(f[:strings.LastIndexByte(f, '/')]), nil)
	if err != nil || strings.Contains(string(it.Vyvod), "affory-chast") {
		t.Fatalf("временный файл остался: %v %q", err, it.Vyvod)
	}
}

const (
	probnyyPolzovatel          = "affory-proba"
	skriptProbnogoPolzovatelya = `set -e
u=affory-proba
id -u "$u" >/dev/null 2>&1 || useradd -m -s /bin/bash "$u"
h=$(getent passwd "$u" | cut -d: -f6)
mkdir -p -- "$h/.ssh"
printf '%s\n' "$1" > "$h/.ssh/authorized_keys"
chown -R "$u:$u" -- "$h/.ssh"
chmod 700 -- "$h/.ssh"
chmod 600 -- "$h/.ssh/authorized_keys"
printf '%s ALL=(ALL) NOPASSWD:ALL\n' "$u" > /etc/sudoers.d/affory-proba
chmod 440 -- /etc/sudoers.d/affory-proba`
	skriptUbratProbnogo = `u=affory-proba
rm -f -- /etc/sudoers.d/affory-proba
loginctl terminate-user "$u" 2>/dev/null
pkill -KILL -u "$u" 2>/dev/null
sleep 1
userdel -r "$u" 2>/dev/null
id -u "$u" 2>/dev/null && echo "пользователь остался"
ls /etc/sudoers.d/affory-proba 2>/dev/null && echo "sudoers остался"
true`
)

// zhivoySudo входит временным пользователем с sudo без пароля: команды и
// файлы идут через sudo -n, ключ root кладётся после sshd -T. Пользователь и
// его sudoers убираются, authorized_keys root сверяется с прежним.
func zhivoySudo(ctx context.Context, t *testing.T, z Zapros, root *Soedinenie) {
	klyuchPolzovatelya := novyyKlyuch(t)
	stroka := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(klyuchPolzovatelya.PublicKey())))
	if it, err := root.Skript(ctx, skriptProbnogoPolzovatelya, stroka); err != nil || it.Kod != 0 {
		t.Fatalf("пробный пользователь: %v %q", err, it.Oshibki)
	}
	probnyyRoot := novyyKlyuch(t)
	strokaRoot := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(probnyyRoot.PublicKey()))) + " affory-proba-sudo"
	defer func() {
		it, err := root.Skript(context.Background(), skriptUbratProbnogo)
		if err != nil || strings.TrimSpace(string(it.Vyvod)) != "" {
			t.Errorf("пробный пользователь не убран: %v %q %q", err, it.Vyvod, it.Oshibki)
		}
		// Сверку с прежним authorized_keys делает TestZhivoy, когда уберёт и
		// свой пробный ключ.
		if err := root.ubratKlyuchRoot(context.Background(), strokaRoot); err != nil {
			t.Errorf("пробный ключ root не убран: %v", err)
		}
	}()

	zp := z
	zp.Vhod = Vhod{Polzovatel: probnyyPolzovatel, Podpis: klyuchPolzovatelya}
	zp.Izvesten = &root.KlyuchHosta
	s, err := Podklyuchit(ctx, zp)
	if err != nil {
		t.Fatalf("вход пробным пользователем: %v", err)
	}
	defer s.Zakryt()
	if s.Root || s.prava != pravaSudo {
		t.Fatalf("пробный пользователь: root %v, права %d", s.Root, s.prava)
	}
	it, err := s.Skript(ctx, `printf '%s|' "$@"; id -u`, "a b", "c'd")
	if err != nil || string(it.Vyvod) != "a b|c'd|0\n" {
		t.Fatalf("скрипт через sudo -n: %v %q %q", err, it.Vyvod, it.Oshibki)
	}
	const katalog = "/tmp/affory-proba-sudo"
	defer root.Vypolnit(context.Background(), "rm -rf -- "+katalog, nil)
	if it, err := s.Vypolnit(ctx, "mkdir -p -- "+katalog, nil); err != nil || it.Kod != 0 {
		t.Fatalf("каталог через sudo: %v %q", err, it.Oshibki)
	}
	// Двоичный файл: stdin через sudo -n не должен терять и менять байты.
	dannye := make([]byte, 1<<20)
	rand.Read(dannye)
	h := sha256.Sum256(dannye)
	if err := s.OtpravitFayl(ctx, bytes.NewReader(dannye), katalog+"/f", 0o640); err != nil {
		t.Fatalf("файл через sudo: %v", err)
	}
	it, err = root.Vypolnit(ctx, "stat -c '%U %a %s' -- "+katalog+"/f; sha256sum -- "+katalog+"/f", nil)
	nado := "root 640 1048576\n" + hex.EncodeToString(h[:]) + "  " + katalog + "/f\n"
	if err != nil || string(it.Vyvod) != nado {
		t.Fatalf("файл через sudo на месте: %v %q, ждала %q", err, it.Vyvod, nado)
	}
	r, err := s.PolozhitKlyuchRoot(ctx, probnyyRoot, "affory-proba-sudo", true)
	if err != nil {
		t.Fatalf("ключ root через sudo: %v", err)
	}
	if !r.Root {
		t.Fatal("вход ключом, положенным через sudo, дал не root")
	}
	r.Zakryt()
}
