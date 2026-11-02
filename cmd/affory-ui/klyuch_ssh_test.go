package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProchitatKlyuchSsh(t *testing.T) {
	kat := t.TempDir()
	zapisat := func(imya string, data []byte) string {
		t.Helper()
		put := filepath.Join(kat, imya)
		if err := os.WriteFile(put, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return put
	}
	zakrytyy := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n")

	k, err := prochitatKlyuchSsh(zapisat("id_ed25519", zakrytyy))
	if err != nil {
		t.Fatalf("закрытый ключ не прочитан: %v", err)
	}
	if k.Imya != "id_ed25519" {
		t.Errorf("имя %q, а нужно имя файла без пути: путь экрану ни к чему", k.Imya)
	}
	if data, _ := base64.StdEncoding.DecodeString(k.Base64); string(data) != string(zakrytyy) {
		t.Errorf("содержимое пришло не байт в байт")
	}

	sluchai := []struct {
		chto, imya string
		data       []byte
		ozhidaem   string
	}{
		{"открытая часть", "id_ed25519.pub", []byte("ssh-ed25519 AAAA kto@gde\n"), "открытая часть"},
		{"открытая ECDSA", "id_ecdsa.pub", []byte("ecdsa-sha2-nistp256 AAAA kto@gde\n"), "открытая часть"},
		{"пустой", "pusto", []byte(" \n"), "пуст"},
		{"больше предела", "bolshoy", make([]byte, predelKlyuchaSsh+1), "64 КБ"},
	}
	for _, s := range sluchai {
		_, err := prochitatKlyuchSsh(zapisat(s.imya, s.data))
		if err == nil || !strings.Contains(err.Error(), s.ozhidaem) {
			t.Errorf("%s: ждали отказ про %q, получили %v", s.chto, s.ozhidaem, err)
		}
	}

	if _, err := prochitatKlyuchSsh(kat); err == nil || !strings.Contains(err.Error(), "папку") {
		t.Errorf("папка вместо файла: ждали отказ, получили %v", err)
	}
	if _, err := prochitatKlyuchSsh(filepath.Join(kat, "net-takogo")); err == nil {
		t.Errorf("пропавший файл прочитан без ошибки")
	}
}

func TestVybratKlyuchSshBezOkna(t *testing.T) {
	k, err := (&most{}).VybratKlyuchSsh()
	if err == nil || k != nil {
		t.Fatalf("без окна выбор ключа обязан отказать, а не вернуть %v", k)
	}
}
