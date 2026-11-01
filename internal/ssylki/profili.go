package ssylki

import (
	"crypto/rand"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Конфигурация сравнивается только в памяти. Секреты и их отпечатки не входят
// в публичный ID: дополнительная запись получает независимый случайный суффикс.
func profil(s protokol.Server) protokol.Server {
	if s.Host != "" {
		s.Id = ""
	}
	s.Imya = ""
	s.IzPodpiski, s.Uderzhan, s.SPinom, s.NebezopasnyyIgnorirovan = false, false, false, false
	s.OtpechatokNeProveryaetsya, s.Svoy = false, false
	s.Host = strings.ToLower(s.Host)
	return s
}

func TotZheProfil(a, b protokol.Server) bool { return profil(a) == profil(b) }

func bazaProfilya(s protokol.Server) string {
	if s.Porty != "" {
		return IdHoppinga(s.Host, s.Port, s.Transport)
	}
	return Id(s.Host, s.Port, s.Transport)
}

func svobodnyyId(s protokol.Server, used map[string]bool) string {
	id := s.Id
	if id == "" {
		id = bazaProfilya(s)
	}
	for used[id] {
		id = bazaProfilya(s) + "-" + rand.Text()[:12]
	}
	return id
}

// Одинаковая конфигурация не удваивается из-за имени. Разные параметры на
// одном endpoint остаются разными записями, даже если парсер дал общий ID.
func unikalnyeProfili(input []protokol.Server) []protokol.Server {
	out := make([]protokol.Server, 0, len(input))
	seen := make(map[protokol.Server]bool)
	ids := make(map[string]bool)
	for _, s := range input {
		if s.Uderzhan {
			continue
		}
		key := profil(s)
		key.IzPodpiski = s.IzPodpiski
		if seen[key] {
			continue
		}
		seen[key] = true
		s.Id = svobodnyyId(s, ids)
		ids[s.Id] = true
		out = append(out, s)
	}
	return out
}

// Slit сопоставляет новое поколение с тем же источником. Сначала точный
// профиль, затем однозначные параметры без ключей и имя. Последняя уступка
// для старых ID допустима только при одном профиле endpoint с обеих сторон.
// Неоднозначная ротация не должна переносить выбор на угаданный профиль.
func Slit(bylo, stalo []protokol.Server) []protokol.Server {
	fresh := unikalnyeProfili(stalo)
	matched := make([]bool, len(fresh))
	usedOld := make([]bool, len(bylo))
	reserved := make(map[string]bool)
	for _, s := range bylo {
		reserved[s.Id] = true
	}
	endpoint := func(s protokol.Server) protokol.Server {
		k := protokol.Server{Host: strings.ToLower(s.Host), Port: s.Port, Transport: s.Transport, IzPodpiski: s.IzPodpiski}
		if s.Porty != "" {
			k.Porty = "hop"
		}
		if s.Host == "" {
			k.Id = s.Id
		}
		return k
	}
	oldCount, newCount := make(map[protokol.Server]int), make(map[protokol.Server]int)
	for _, s := range bylo {
		if !s.Uderzhan {
			oldCount[endpoint(s)]++
		}
	}
	for _, s := range fresh {
		newCount[endpoint(s)]++
	}
	keys := []func(protokol.Server) protokol.Server{
		func(s protokol.Server) protokol.Server { k := profil(s); k.IzPodpiski = s.IzPodpiski; return k },
		func(s protokol.Server) protokol.Server {
			k := profil(s)
			k.IzPodpiski = s.IzPodpiski
			k.Uuid, k.Parol, k.PublicKey, k.ShortId, k.ObfsParol, k.Pin = "", "", "", "", "", ""
			return k
		},
		func(s protokol.Server) protokol.Server { k := endpoint(s); k.Imya = s.Imya; return k },
		endpoint,
	}
	for pass, keyOf := range keys {
		oldKeys, newKeys := make(map[protokol.Server][]int), make(map[protokol.Server][]int)
		oldTotals, newTotals := make(map[protokol.Server]int), make(map[protokol.Server]int)
		for i, s := range bylo {
			if !s.Uderzhan {
				oldTotals[keyOf(s)]++
			}
			if !usedOld[i] && !s.Uderzhan {
				k := keyOf(s)
				oldKeys[k] = append(oldKeys[k], i)
			}
		}
		for i, s := range fresh {
			newTotals[keyOf(s)]++
			if !matched[i] {
				k := keyOf(s)
				newKeys[k] = append(newKeys[k], i)
			}
		}
		for k, news := range newKeys {
			olds := oldKeys[k]
			if len(news) != 1 || len(olds) != 1 {
				continue
			}
			i, j := news[0], olds[0]
			if (pass == 1 || pass == 2) && (oldTotals[k] != 1 || newTotals[k] != 1) {
				continue
			}
			if pass == 2 && fresh[i].Imya == "" {
				continue
			}
			if pass == 3 && (oldCount[endpoint(fresh[i])] != 1 || newCount[endpoint(fresh[i])] != 1) {
				continue
			}
			fresh[i].Id = bylo[j].Id
			matched[i], usedOld[j] = true, true
		}
	}
	for i := range fresh {
		if !matched[i] {
			fresh[i].Id = svobodnyyId(fresh[i], reserved)
		}
		reserved[fresh[i].Id] = true
	}
	for i, s := range bylo {
		if !s.IzPodpiski && !s.Uderzhan && !usedOld[i] {
			fresh = append(fresh, s)
		}
	}
	return fresh
}

// Добавление ссылки не является командой ротации ключей: другая конфигурация
// сохраняется рядом. Повтор точной ссылки возвращает прежний ID и источник.
func DobavitProfil(bylo []protokol.Server, novyy protokol.Server) ([]protokol.Server, protokol.Server) {
	out := append([]protokol.Server(nil), bylo...)
	used := make(map[string]bool, len(bylo))
	for i, s := range out {
		used[s.Id] = true
		if !s.Uderzhan && profil(s) == profil(novyy) {
			if !s.IzPodpiski {
				out[i].Imya = novyy.Imya
			}
			return out, out[i]
		}
	}
	novyy.Id = svobodnyyId(novyy, used)
	return append(out, novyy), novyy
}
