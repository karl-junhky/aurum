package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Ключ SSH для раздела «Свой VPN» (план 2026-11-01, §3.6). Окно читает файл
// само и отдаёт содержимое: путь служба не принимает, иначе служба SYSTEM
// прочла бы по просьбе любого процесса сеанса любой файл машины.

// klyuchSsh это выбранный файл: имя для экрана и содержимое в base64, как
// его ждёт поле klyuch команд svoyVps* ([]byte в JSON).
type klyuchSsh struct {
	Imya   string `json:"imya"`
	Base64 string `json:"base64"`
}

// predelKlyuchaSsh с запасом: закрытый ключ RSA 16384 в формате OpenSSH
// занимает меньше 13 КБ. Файл больше предела ключом не бывает, а читать его
// в память целиком незачем.
const predelKlyuchaSsh = 64 << 10

// VybratKlyuchSsh открывает выбор файла ключа и читает выбранный. nil без
// ошибки: человек закрыл диалог.
func (m *most) VybratKlyuchSsh() (*klyuchSsh, error) {
	if m.app == nil {
		return nil, errors.New("окно выбора файла недоступно")
	}
	d := m.app.Dialog.OpenFile().
		SetTitle("Закрытый ключ SSH").
		CanChooseFiles(true).
		CanChooseDirectories(false)
	if kat := katalogSsh(); kat != "" {
		d = d.SetDirectory(kat)
	}
	if m.okno != nil {
		d = d.AttachToWindow(m.okno)
	}
	put, err := otvetDialoga(d.PromptForSingleSelection())
	if err != nil || put == "" {
		return nil, err
	}
	return prochitatKlyuchSsh(put)
}

// katalogSsh это папка .ssh человека, если она есть: ключи по умолчанию
// лежат там. Без неё диалог откроется там, где решит сам.
func katalogSsh() string {
	dom, err := os.UserHomeDir()
	if err != nil {
		log.Printf("папка пользователя не найдена, выбор ключа откроется где решит сам: %v", err)
		return ""
	}
	kat := filepath.Join(dom, ".ssh")
	if info, err := os.Stat(kat); err != nil || !info.IsDir() {
		return ""
	}
	return kat
}

func prochitatKlyuchSsh(put string) (*klyuchSsh, error) {
	f, err := os.Open(put)
	if err != nil {
		return nil, fmt.Errorf("файл ключа не открылся: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("файл ключа не читается: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("выбери файл ключа, а не папку")
	}
	if info.Size() > predelKlyuchaSsh {
		return nil, errors.New("файл больше 64 КБ, ключ SSH таким не бывает")
	}
	// Предел и при чтении: файл мог вырасти между Stat и чтением.
	data, err := io.ReadAll(io.LimitReader(f, predelKlyuchaSsh+1))
	if err != nil {
		return nil, fmt.Errorf("файл ключа не прочитан: %w", err)
	}
	if len(data) > predelKlyuchaSsh {
		return nil, errors.New("файл больше 64 КБ, ключ SSH таким не бывает")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, errors.New("файл ключа пуст")
	}
	// Открытую часть путают с закрытой чаще всего: у них одно имя, у
	// открытой только хвост .pub.
	if strings.HasPrefix(strings.TrimSpace(string(data)), "ssh-") || strings.HasPrefix(strings.TrimSpace(string(data)), "ecdsa-") {
		return nil, errors.New("это открытая часть ключа, нужен закрытый файл: тот же, только без .pub")
	}
	return &klyuchSsh{Imya: filepath.Base(put), Base64: base64.StdEncoding.EncodeToString(data)}, nil
}
