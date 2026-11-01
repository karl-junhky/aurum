package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Ключ управления своим сервером (§3.6 плана «Свой VPN»): своя пара ed25519 на
// каждый сервер, выпускает служба. Отдельный файл под DPAPI машины, а не в
// sekrety.dat: в экспорт профиля он не едет, наружу по каналу не отдаётся.
// Каталог данных заперт на SYSTEM и администраторов (sostoyanie.acl.go).

const katalogKlyuchey = "svoy-vpn"

// reIdSvoego: id своего сервера идёт в имя файла, поэтому только свои знаки.
var reIdSvoego = regexp.MustCompile(`^svoy-[0-9a-f]{16}$`)

func (s *Sluzhba) putKlyuchaUpravleniya(id string) (string, error) {
	if !reIdSvoego.MatchString(id) {
		return "", fmt.Errorf("негодный id своего сервера %q", id)
	}
	return filepath.Join(s.dirDannyh, katalogKlyuchey, id+".klyuch"), nil
}

// klyuchUpravleniya читает ключ управления сервера id. ok false - ключа на
// этом компьютере нет: сервер ставили с другого или запись пришла импортом.
func (s *Sluzhba) klyuchUpravleniya(id string) (k gossh.Signer, ok bool, err error) {
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		return nil, false, err
	}
	blob, err := os.ReadFile(put)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("ключ управления не прочитан: %w", err)
	}
	telo, err := hranenie.RasshifrovatDlyaMashiny(blob)
	if err != nil {
		return nil, false, fmt.Errorf("ключ управления не расшифрован: %w", err)
	}
	if k, err = gossh.ParsePrivateKey(telo); err != nil {
		return nil, false, fmt.Errorf("ключ управления не разобран: %w", err)
	}
	return k, true, nil
}

// zavestiKlyuchUpravleniya отдаёт ключ сервера id, а нет его - заводит. Ключ
// остаётся и после неудачной установки: он мог уже лечь в authorized_keys
// root, и повтор установки входит тем же ключом.
func (s *Sluzhba) zavestiKlyuchUpravleniya(id string) (gossh.Signer, error) {
	if k, ok, err := s.klyuchUpravleniya(id); err != nil || ok {
		return k, err
	}
	_, zakr, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ключ управления не создан: %w", err)
	}
	blok, err := gossh.MarshalPrivateKey(zakr, "")
	if err != nil {
		return nil, fmt.Errorf("ключ управления не упакован: %w", err)
	}
	blob, err := hranenie.ZashifrovatDlyaMashiny(pem.EncodeToMemory(blok))
	if err != nil {
		return nil, fmt.Errorf("ключ управления не зашифрован: %w", err)
	}
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(put), 0o700); err != nil {
		return nil, fmt.Errorf("каталог ключей управления не создан: %w", err)
	}
	if err := sostoyanie.ZapisatNadyozhno(put, blob, nil); err != nil {
		return nil, fmt.Errorf("ключ управления не записан: %w", err)
	}
	return gossh.NewSignerFromKey(zakr)
}

// ubratKlyuchUpravleniya стирает ключ сервера id вместе с запасной копией.
// Нет файла - стирать нечего. Копия первой: отказ на ней оставляет ключ
// целым, и «Забыть» с отказом правда не меняет ничего.
func (s *Sluzhba) ubratKlyuchUpravleniya(id string) error {
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		return err
	}
	for _, p := range []string{put + sostoyanie.RasshirenieZapasa, put} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("ключ управления не стёрт: %w", err)
		}
	}
	return nil
}

// kommentariyKlyucha подписывает строку ключа в authorized_keys именем
// компьютера и датой (З8: «оставить только этот компьютер»). Строка ключа
// пускает только простые знаки, остальные выпадают.
func kommentariyKlyucha(imyaKompyutera string, seychas time.Time) string {
	chisto := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return -1
	}, imyaKompyutera)
	if chisto == "" {
		chisto = "pc"
	}
	if len(chisto) > 63 {
		chisto = chisto[:63]
	}
	return "affory " + chisto + " " + seychas.Format("2006-01-02")
}
