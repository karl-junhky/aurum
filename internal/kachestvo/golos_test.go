package kachestvo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kachestvo"
)

// Один голос на всё, что видит человек. Разбор 13.09.2026 нашёл в окне два
// обращения сразу: «Нажмите на сферу» на главном экране против «выбери другой»
// в словаре отказов. Фронт сторожит себя сам (golos.test.ts), а половина
// текстов приезжает от службы, и её никто не сторожил.
//
// Границы слова руками, а не \b: в Go, как и в JS, \b считается по латинскому
// \w и на кириллице не совпадает ни разу. Сторож с \b был бы зелёным всегда.
var reNaVy = regexp.MustCompile(`(?i)(^|[^а-яёА-ЯЁ])(вы|вам|вас|ваш[а-яёА-ЯЁ]*|[а-яёА-ЯЁ]{3,}?(ите|йте|ьте))([^а-яёА-ЯЁ]|$)`)

// Строка с кириллицей внутри кавычек: только такие доезжают до человека.
var reRusskayaStroka = regexp.MustCompile(`"[^"]*[а-яё][^"]*"`)

func TestTekstySluzhbyGovoryatNaTy(t *testing.T) {
	var nayden []string
	koren := filepath.FromSlash("../..")
	err := filepath.Walk(koren, func(put string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			vlozhen, err := kachestvo.VlozhennyyModul(put, koren)
			if err != nil {
				return err
			}
			if vlozhen {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		syroe, err := os.ReadFile(put)
		if err != nil {
			return err
		}
		for n, stroka := range strings.Split(string(syroe), "\n") {
			obrez := strings.TrimSpace(stroka)
			if strings.HasPrefix(obrez, "//") {
				continue
			}
			for _, s := range reRusskayaStroka.FindAllString(obrez, -1) {
				if reNaVy.MatchString(s) {
					nayden = append(nayden, filepath.ToSlash(put)+":"+itoa(n+1)+": "+s)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход не удался: %v", err)
	}
	if len(nayden) > 0 {
		t.Errorf("тексты на «вы», а всё окно говорит на «ты»:\n%s", strings.Join(nayden, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
