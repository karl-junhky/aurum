package zaglushka

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
)

// kosti делают выборы по зерну. Каждый выбор считается от своей метки, а не
// из общего потока: новая ось разнообразия не сдвигает старые, и результат
// не зависит от порядка вызовов. Только sha256, без math/rand: поток
// генератора может поменяться между версиями Go, а сайт должен остаться тем же.
type kosti struct{ zerno []byte }

func (k kosti) bayty(metka string) []byte {
	h := sha256.New()
	var dlina [8]byte
	binary.BigEndian.PutUint64(dlina[:], uint64(len(k.zerno)))
	h.Write([]byte("zaglushka/1"))
	h.Write(dlina[:])
	h.Write(k.zerno)
	h.Write([]byte(metka))
	return h.Sum(nil)
}

func (k kosti) chislo(metka string) uint64 {
	return binary.BigEndian.Uint64(k.bayty(metka)[:8])
}

// iz: число от 0 до n-1. Перекос от остатка при n в десятки ничтожен
func (k kosti) iz(metka string, n int) int {
	if n <= 1 {
		return 0
	}
	return int(k.chislo(metka) % uint64(n))
}

// dolya: число в [0, 1)
func (k kosti) dolya(metka string) float64 {
	return float64(k.chislo(metka)>>11) / (1 << 53)
}

// mezhdu: число в [ot, do), округлённое до сотых, чтобы в настройках кота
// не тянулись длинные хвосты знаков
func (k kosti) mezhdu(metka string, ot, do float64) float64 {
	return math.Round((ot+(do-ot)*k.dolya(metka))*100) / 100
}

// hex: n шестнадцатеричных знаков, n не больше 64
func (k kosti) hex(metka string, n int) string {
	return hex.EncodeToString(k.bayty(metka))[:n]
}

// hexSoderzhimogo: хвост имени файла от зерна и содержимого. Новая версия
// файла получает новое имя, и браузер не возьмёт старую из кэша
func (k kosti) hexSoderzhimogo(metka string, n int, soderzhimoe []byte) string {
	h := sha256.New()
	h.Write(k.bayty(metka))
	h.Write(soderzhimoe)
	return hex.EncodeToString(h.Sum(nil))[:n]
}

func vybor[T any](k kosti, metka string, iz []T) T {
	return iz[k.iz(metka, len(iz))]
}

// neskolko: m разных элементов из iz в порядке, который задаёт зерно
func neskolko[T any](k kosti, metka string, iz []T, m int) []T {
	type para struct {
		i int
		v uint64
	}
	p := make([]para, len(iz))
	for i := range iz {
		p[i] = para{i, k.chislo(metka + "/" + strconv.Itoa(i))}
	}
	sort.Slice(p, func(a, b int) bool {
		if p[a].v != p[b].v {
			return p[a].v < p[b].v
		}
		return p[a].i < p[b].i
	})
	if m > len(p) {
		m = len(p)
	}
	out := make([]T, m)
	for i := range out {
		out[i] = iz[p[i].i]
	}
	return out
}
