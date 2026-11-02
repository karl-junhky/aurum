package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Программа сервера (У5 плана «Свой VPN»). Её отпечаток вшит в сборку
// клиента, сам файл лежит в выпуске клиента на GitHub. Служба качает его при
// первой установке и держит сверенную копию в каталоге данных. Сверка sha256
// идёт при каждой выдаче, и установке отдаётся сверенное из памяти: файл,
// подменённый на диске между сверкой и отправкой, на сервер не уедет.

// vpn-uzel.otpechatok.json это вывод uzel/sobrat.sh как есть. Выпуск
// (ustanovka\sobrat-reliz.ps1) кладёт рядом только файл с тем же sha256.
//
//go:embed vpn-uzel.otpechatok.json
var otpechatokUzlaFayl []byte

const (
	repozitoriyVypuskov = "https://github.com/zxczxczxczxczxczxc1111/affory"
	// Программа весит около 40 МБ, потолок с запасом на рост.
	predelProgrammyUzla = 96 << 20
	// 40 МБ на канале в 2 Мбит/с идут почти три минуты.
	srokZagruzkiUzla = 5 * time.Minute
	// shagZagruzka: служба готовит программу сервера, а без копии качает её.
	shagZagruzka = "zagruzka"
	// peremennayaUzla задаёт стенду файл программы: приёмка идёт раньше
	// выкладки. Отпечаток сверяется и у него.
	peremennayaUzla   = "AFFORY_UZEL"
	imyaProgrammyUzla = "vpn-uzel"
)

// vypuskUzla это программа сервера, которую ждёт эта сборка клиента.
type vypuskUzla struct {
	Versiya string
	Sha256  string
}

var versiyaUzlaRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var prochitatVshityyUzel = sync.OnceValues(func() (vypuskUzla, error) {
	return razobratOtpechatokUzla(otpechatokUzlaFayl)
})

func razobratOtpechatokUzla(b []byte) (vypuskUzla, error) {
	var o struct {
		Programma string `json:"programma"`
		Sha256    string `json:"sha256"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return vypuskUzla{}, fmt.Errorf("отпечаток программы сервера в сборке не разобран: %w", err)
	}
	// Строго X.Y.Z, как у сборки выпуска: из версии собирается имя файла.
	v, ok := strings.CutPrefix(o.Programma, "vpn-uzel ")
	if !ok || !versiyaUzlaRe.MatchString(v) {
		return vypuskUzla{}, fmt.Errorf("в отпечатке программы сервера нет версии: %q", o.Programma)
	}
	if _, err := hex.DecodeString(o.Sha256); err != nil || len(o.Sha256) != 64 || strings.ToLower(o.Sha256) != o.Sha256 {
		return vypuskUzla{}, fmt.Errorf("в отпечатке программы сервера не sha256: %q", o.Sha256)
	}
	return vypuskUzla{Versiya: v, Sha256: o.Sha256}, nil
}

func imyaFaylaUzla(versiya string) string {
	return "vpn-uzel-" + versiya + "-linux-amd64"
}

var errSborkaBezVersii = errors.New("в сборке без версии программу сервера взять негде: укажи файл переменной " + peremennayaUzla)

// oshibkaZagruzkiUzla: программа сервера не пришла с GitHub. Окну это отказ
// установки с кнопкой «Повторить».
type oshibkaZagruzkiUzla struct{ err error }

func (o *oshibkaZagruzkiUzla) Error() string {
	return "программа сервера не скачалась с GitHub: " + o.err.Error()
}
func (o *oshibkaZagruzkiUzla) Unwrap() error { return o.err }

// programmaSVypuska отдаёт сверенную программу сервера: копию из каталога
// данных, а если её нет или она не та, файл из AFFORY_UZEL или с выпуска.
func (s *Sluzhba) programmaSVypuska(ctx context.Context) (io.ReadSeekCloser, error) {
	u, err := s.vshityyUzel()
	if err != nil {
		return nil, err
	}
	kopiya := filepath.Join(s.dirDannyh, imyaProgrammyUzla)
	b, err := prochitatSverennuyu(kopiya, u.Sha256)
	if err == nil {
		return programmaVPamyati{bytes.NewReader(b)}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		log.Printf("копия программы сервера не годится (%v), беру заново", err)
	}
	if b, err = s.dostatProgrammuUzla(ctx, u); err != nil {
		return nil, err
	}
	if err := zapisatKopiyuUzla(kopiya, b); err != nil {
		// Установка идёт и без копии: файл сверен и лежит в памяти, а в
		// следующий раз скачается заново.
		log.Printf("копия программы сервера не записана: %v", err)
	}
	return programmaVPamyati{bytes.NewReader(b)}, nil
}

func (s *Sluzhba) dostatProgrammuUzla(ctx context.Context, u vypuskUzla) ([]byte, error) {
	if s.istochnikUzla != "" {
		b, err := prochitatSverennuyu(s.istochnikUzla, u.Sha256)
		if err != nil {
			return nil, fmt.Errorf("программа сервера из %s не годится: %w", peremennayaUzla, err)
		}
		return b, nil
	}
	if versiyaProgrammy == "dev" {
		return nil, errSborkaBezVersii
	}
	do, otm := context.WithTimeout(ctx, srokZagruzkiUzla)
	defer otm()
	adres := repozitoriyVypuskov + "/releases/download/v" + versiyaProgrammy + "/" + imyaFaylaUzla(u.Versiya)
	b, err := s.skachatFayl(do, adres, predelProgrammyUzla, nil)
	switch {
	case err == nil:
	case ctx.Err() != nil:
		// Отмену сверху человек сделал сам, это не отказ GitHub.
		return nil, ctx.Err()
	case errors.Is(err, errKod404):
		// Стенд до выкладки или выпуск без программы: сеть тут ни при чём.
		return nil, fmt.Errorf("в выпуске Affory %s на GitHub нет программы для сервера %s", versiyaProgrammy, imyaFaylaUzla(u.Versiya))
	default:
		return nil, &oshibkaZagruzkiUzla{err}
	}
	if err := sveritUzel(b, u.Sha256); err != nil {
		return nil, fmt.Errorf("программа сервера с GitHub не та, что ждёт эта версия Affory: %w", err)
	}
	return b, nil
}

func prochitatSverennuyu(put, sha string) ([]byte, error) {
	f, err := os.Open(put)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, predelProgrammyUzla+1))
	if err != nil {
		return nil, err
	}
	if len(b) > predelProgrammyUzla {
		return nil, fmt.Errorf("файл больше %d МБ", predelProgrammyUzla>>20)
	}
	if err := sveritUzel(b, sha); err != nil {
		return nil, err
	}
	return b, nil
}

func sveritUzel(b []byte, sha string) error {
	h := sha256.Sum256(b)
	if est := hex.EncodeToString(h[:]); est != sha {
		return fmt.Errorf("sha256 %s вместо %s", est, sha)
	}
	return nil
}

// zapisatKopiyuUzla пишет копию через временный файл: оборванная запись
// оставила бы копию, которую каждая установка качала бы заново.
func zapisatKopiyuUzla(put string, b []byte) error {
	vremen := put + ".chast"
	if err := os.WriteFile(vremen, b, 0o600); err != nil {
		_ = os.Remove(vremen)
		return err
	}
	if err := os.Rename(vremen, put); err != nil {
		_ = os.Remove(vremen)
		return err
	}
	return nil
}

type programmaVPamyati struct{ *bytes.Reader }

func (programmaVPamyati) Close() error { return nil }
