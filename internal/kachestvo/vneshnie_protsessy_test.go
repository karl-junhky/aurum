package kachestvo

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Каждый запуск внешней программы стоит на учёте, и рядом записано, что с её
// выводом.
//
// Класс дефектов, ради которого заведён этот судья, стоил двух жалоб за один
// день 21.09.2026. Консольные программы Windows отвечают в кодовой странице
// КОНСОЛИ, а Go читает их вывод как UTF-8. Пока перевода не было:
//
//   - netsh на русской машине говорил «Ни одно правило не соответствует…», мы
//     видели «ЌЁ ®¤­® Їа ўЁ«®», не узнавали фразу и считали отсутствие правила
//     отказом. Установка поверх прежней падала на подготовке;
//   - собственная подкоманда `install`, которая пишет наружу в кодовой странице
//     машины ради установщика, читалась обновлением как UTF-8, и причина отката
//     доезжала до человека рядом ромбиков — JSON заменяет негодные байты на
//     U+FFFD безвозвратно.
//
// Новый вызов в новом файле роняет этот тест. Это и есть его работа: автор
// обязан назвать, что делает с выводом, и либо перевести его через
// internal/kodirovki, либо объяснить, почему перевод не нужен.
var vneshnieProtsessy = map[string]string{
	filepath.Join("cmd", "affory-proverka", "main.go"): "sing-box check: ядро это программа на Go, " +
		"её вывод уже UTF-8. Перевод ИСПОРТИЛ бы его",
	filepath.Join("cmd", "affory-svc", "obnovlenie.go"): "свой affory-svc.exe install: вывод переводится " +
		"из кодовой страницы машины (kodirovki.Ansi), потому что подкоманды пишут наружу именно в ней",
	filepath.Join("cmd", "affory-svc", "udalenie.go"): "cmd.exe и taskkill: вывод не читается вовсе, " +
		"оба зовутся ради действия",
	filepath.Join("cmd", "affory-ui", "most.go"): "перезапуск своего окна: вывод не читается",
	filepath.Join("internal", "diagnostika", "istochniki_windows.go"): "netsh show dynamicport: из вывода " +
		"берутся только ЧИСЛА, а цифры одинаковы в любой кодовой странице. Текст никуда не показывается",
	filepath.Join("internal", "reklama", "obnovit", "main.go"): "инструмент разработчика, sing-box " +
		"rule-set convert/match и version: вывод ядра уже UTF-8",
	filepath.Join("internal", "set", "ipv6.go"): "netsh: единственное место, где он зовётся, " +
		"и вывод переводится из кодовой страницы консоли (kodirovki.Konsoli)",
	filepath.Join("internal", "yadra", "proverka.go"): "sing-box check: вывод ядра уже UTF-8",
	filepath.Join("internal", "yadra", "spisok.go"): "sing-box rule-set convert/match: вывод ядра " +
		"уже UTF-8, читается только признак совпадения",
	filepath.Join("internal", "yadra", "zapusk.go"): "sing-box run: жалобы ядра уже UTF-8",
}

// VlozhennyyModul: каталог со своим go.mod внутри модуля клиента. Это uzel,
// программа для Linux со своими правилами: судьи клиента её не обходят.
func VlozhennyyModul(katalog, koren string) (bool, error) {
	if filepath.Clean(katalog) == filepath.Clean(koren) {
		return false, nil
	}
	_, err := os.Stat(filepath.Join(katalog, "go.mod"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

func TestVneshnieProtsessyPodUchyotom(t *testing.T) {
	koren, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("корень модуля не вычислен: %v", err)
	}

	nayden := map[string]bool{}
	err = filepath.WalkDir(koren, func(put string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			vlozhen, err := VlozhennyyModul(put, koren)
			if err != nil {
				return err
			}
			if vlozhen {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		// Разбор, а не поиск подстроки: первая редакция обвинила
		// internal/yadra/zhurnal.go за упоминание exec.Command в комментарии,
		// который как раз ОБЪЯСНЯЕТ старую находку. Судья, запрещающий называть
		// вещи своими именами в прозе, заставляет писать иносказания.
		derevo, err := parser.ParseFile(token.NewFileSet(), put, nil, 0)
		if err != nil {
			return fmt.Errorf("%s не разобран: %w", put, err)
		}
		ast.Inspect(derevo, func(u ast.Node) bool {
			vyzov, ok := u.(*ast.CallExpr)
			if !ok {
				return true
			}
			pole, ok := vyzov.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			paket, ok := pole.X.(*ast.Ident)
			if !ok || paket.Name != "exec" {
				return true
			}
			if pole.Sel.Name == "Command" || pole.Sel.Name == "CommandContext" {
				otn, _ := filepath.Rel(koren, put)
				nayden[otn] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("обход дерева не удался: %v", err)
	}

	for put := range nayden {
		if _, est := vneshnieProtsessy[put]; !est {
			t.Errorf("%s зовёт внешнюю программу, а в реестре его нет.\n"+
				"Скажите, что делаете с её выводом: консольные программы Windows отвечают "+
				"в кодовой странице консоли, а не в UTF-8 (internal/kodirovki)", put)
		}
	}
	for put := range vneshnieProtsessy {
		if !nayden[put] {
			t.Errorf("в реестре есть %s, а вызова внешней программы там больше нет: "+
				"строку пора убрать, иначе реестр начнёт врать", put)
		}
	}
}
