package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

var programmaUzlaDlyaTesta = []byte("программа сервера для теста")

type zagruzkiUzla struct {
	adresa []string
	otvet  []byte
	err    error
}

// sProgrammoyUzla: служба версии versiya ждёт programmaUzlaDlyaTesta, а
// загрузка с выпуска подменена.
func sProgrammoyUzla(t *testing.T, versiya string) (*Sluzhba, *zagruzkiUzla) {
	t.Helper()
	byla := versiyaProgrammy
	versiyaProgrammy = versiya
	t.Cleanup(func() { versiyaProgrammy = byla })
	s := podstavnaya(t, nil)
	h := sha256.Sum256(programmaUzlaDlyaTesta)
	s.vshityyUzel = func() (vypuskUzla, error) {
		return vypuskUzla{Versiya: "0.6.0", Sha256: hex.EncodeToString(h[:])}, nil
	}
	s.istochnikUzla = ""
	z := &zagruzkiUzla{otvet: programmaUzlaDlyaTesta}
	s.skachatFayl = func(ctx context.Context, adres string, predel int64, _ func(bylo, vsego int64)) ([]byte, error) {
		z.adresa = append(z.adresa, adres)
		if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > srokZagruzkiUzla {
			t.Errorf("загрузка без срока или дольше %s", srokZagruzkiUzla)
		}
		if predel != predelProgrammyUzla {
			t.Errorf("потолок загрузки %d", predel)
		}
		if z.err != nil {
			return nil, z.err
		}
		return z.otvet, ctx.Err()
	}
	return s, z
}

func prochitatVsyo(t *testing.T, s *Sluzhba) ([]byte, error) {
	t.Helper()
	p, err := s.programmaUzla(context.Background())
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return io.ReadAll(p)
}

func TestProgrammaUzlaSVypuska(t *testing.T) {
	s, z := sProgrammoyUzla(t, "1.11.0")
	s.programmaUzla = s.programmaSVypuska
	b, err := prochitatVsyo(t, s)
	if err != nil || !bytes.Equal(b, programmaUzlaDlyaTesta) {
		t.Fatalf("программа %q, %v", b, err)
	}
	nado := "https://github.com/zxczxczxczxczxczxc1111/affory/releases/download/v1.11.0/vpn-uzel-0.6.0-linux-amd64"
	if len(z.adresa) != 1 || z.adresa[0] != nado {
		t.Fatalf("загрузки %v", z.adresa)
	}
	kopiya, err := os.ReadFile(filepath.Join(s.dirDannyh, "vpn-uzel"))
	if err != nil || !bytes.Equal(kopiya, programmaUzlaDlyaTesta) {
		t.Fatalf("копия %q, %v", kopiya, err)
	}
	if _, err := os.Stat(filepath.Join(s.dirDannyh, "vpn-uzel.chast")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("временный файл остался: %v", err)
	}
	// Вторая установка берёт сверенную копию и в сеть не ходит.
	if b, err := prochitatVsyo(t, s); err != nil || !bytes.Equal(b, programmaUzlaDlyaTesta) || len(z.adresa) != 1 {
		t.Fatalf("второй раз %q, %v, загрузки %v", b, err, z.adresa)
	}
}

// Копия, испорченная или подменённая на диске, не уезжает на сервер, а
// качается заново.
func TestProgrammaUzlaKopiyaNeTa(t *testing.T) {
	s, z := sProgrammoyUzla(t, "1.11.0")
	s.programmaUzla = s.programmaSVypuska
	put := filepath.Join(s.dirDannyh, "vpn-uzel")
	if err := os.WriteFile(put, []byte("подменённая"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := prochitatVsyo(t, s)
	if err != nil || !bytes.Equal(b, programmaUzlaDlyaTesta) || len(z.adresa) != 1 {
		t.Fatalf("программа %q, %v, загрузки %v", b, err, z.adresa)
	}
	if kopiya, _ := os.ReadFile(put); !bytes.Equal(kopiya, programmaUzlaDlyaTesta) {
		t.Fatalf("копия не переписана: %q", kopiya)
	}
}

func TestProgrammaUzlaSGitHubNeTa(t *testing.T) {
	s, z := sProgrammoyUzla(t, "1.11.0")
	s.programmaUzla = s.programmaSVypuska
	z.otvet = []byte("другой файл")
	_, err := prochitatVsyo(t, s)
	if err == nil || !strings.Contains(err.Error(), "не та") {
		t.Fatalf("ошибка %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dirDannyh, "vpn-uzel")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("несверенный файл лёг в копию: %v", err)
	}
}

func TestProgrammaUzlaGitHubNedostupen(t *testing.T) {
	s, z := sProgrammoyUzla(t, "1.11.0")
	s.programmaUzla = s.programmaSVypuska
	z.err = errors.New("dial tcp: i/o timeout")
	_, err := prochitatVsyo(t, s)
	var uzel *oshibkaZagruzkiUzla
	if !errors.As(err, &uzel) {
		t.Fatalf("ошибка %v", err)
	}
	o := otkazVps(err)
	if o.Kod != protokol.KodVpsUstanovka || !strings.Contains(o.Tekst, "github.com") || !strings.Contains(o.Tekst, "повтори") {
		t.Fatalf("отказ %+v", o)
	}
}

// Выпуск без файла программы это не отказ сети: звать проверять github.com
// незачем.
func TestProgrammaUzlaNetVVypuske(t *testing.T) {
	s, z := sProgrammoyUzla(t, "1.11.0")
	z.err = fmt.Errorf("%w: список выпусков не отдан", errKod404)
	_, err := s.programmaSVypuska(context.Background())
	var uzel *oshibkaZagruzkiUzla
	if err == nil || errors.As(err, &uzel) || !strings.Contains(err.Error(), "1.11.0") || !strings.Contains(err.Error(), "vpn-uzel-0.6.0-linux-amd64") {
		t.Fatalf("ошибка %v", err)
	}
	if o := otkazVps(err); o.Kod != protokol.KodVpsUstanovka || strings.Contains(o.Tekst, "повтори") {
		t.Fatalf("отказ %+v", o)
	}
}

// Отмена сверху это не отказ GitHub: окно не должно звать проверять сеть.
func TestProgrammaUzlaOtmena(t *testing.T) {
	s, _ := sProgrammoyUzla(t, "1.11.0")
	ctx, otmena := context.WithCancel(context.Background())
	otmena()
	_, err := s.programmaSVypuska(ctx)
	var uzel *oshibkaZagruzkiUzla
	if !errors.Is(err, context.Canceled) || errors.As(err, &uzel) {
		t.Fatalf("ошибка %v", err)
	}
}

func TestProgrammaUzlaIzPeremennoy(t *testing.T) {
	s, z := sProgrammoyUzla(t, "dev")
	s.programmaUzla = s.programmaSVypuska
	put := filepath.Join(t.TempDir(), "vpn-uzel")
	s.istochnikUzla = put

	if _, err := prochitatVsyo(t, s); err == nil || !strings.Contains(err.Error(), peremennayaUzla) {
		t.Fatalf("нет файла: %v", err)
	}
	if err := os.WriteFile(put, []byte("чужая сборка"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prochitatVsyo(t, s); err == nil || !strings.Contains(err.Error(), peremennayaUzla) || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("чужой файл: %v", err)
	}
	if err := os.WriteFile(put, programmaUzlaDlyaTesta, 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := prochitatVsyo(t, s); err != nil || !bytes.Equal(b, programmaUzlaDlyaTesta) {
		t.Fatalf("программа %q, %v", b, err)
	}
	if len(z.adresa) != 0 {
		t.Fatalf("при заданном файле ходили в сеть: %v", z.adresa)
	}
}

func TestProgrammaUzlaBezVersii(t *testing.T) {
	s, z := sProgrammoyUzla(t, "dev")
	if _, err := s.programmaSVypuska(context.Background()); !errors.Is(err, errSborkaBezVersii) || len(z.adresa) != 0 {
		t.Fatalf("ошибка %v, загрузки %v", err, z.adresa)
	}
}

// Вшитый отпечаток разбирается и говорит о той версии, что в исходниках
// uzel: поднятая версия без пересборки и нового отпечатка ловится здесь.
func TestOtpechatokUzlaVSborke(t *testing.T) {
	u, err := prochitatVshityyUzel()
	if err != nil {
		t.Fatal(err)
	}
	ish, err := os.ReadFile("../../uzel/cmd/vpn-uzel/main.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^var Versiya = "([^"]+)"$`).FindSubmatch(ish)
	if m == nil || string(m[1]) != u.Versiya {
		t.Fatalf("в сборке vpn-uzel %s, в исходниках %q", u.Versiya, m)
	}
}

func TestRazobratOtpechatokUzla(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	if u, err := razobratOtpechatokUzla([]byte(`{"programma":"vpn-uzel 0.6.0","sha256":"` + sha + `"}`)); err != nil || u.Versiya != "0.6.0" || u.Sha256 != sha {
		t.Fatalf("годный: %+v, %v", u, err)
	}
	for imya, telo := range map[string]string{
		"не json":          `{`,
		"без версии":       `{"programma":"vpn-uzel","sha256":"` + sha + `"}`,
		"чужое имя":        `{"programma":"uzel 0.6.0","sha256":"` + sha + `"}`,
		"версия не X.Y.Z":  `{"programma":"vpn-uzel 0.6","sha256":"` + sha + `"}`,
		"версия с v":       `{"programma":"vpn-uzel v0.6.0","sha256":"` + sha + `"}`,
		"версия с хвостом": `{"programma":"vpn-uzel 0.6.0 ","sha256":"` + sha + `"}`,
		"заглавные":        `{"programma":"vpn-uzel 0.6.0","sha256":"` + strings.ToUpper(sha) + `"}`,
		"короткий":         `{"programma":"vpn-uzel 0.6.0","sha256":"abcd"}`,
		"не hex":           `{"programma":"vpn-uzel 0.6.0","sha256":"` + strings.Repeat("zz", 32) + `"}`,
	} {
		if _, err := razobratOtpechatokUzla([]byte(telo)); err == nil {
			t.Errorf("%s: разобран", imya)
		}
	}
}
