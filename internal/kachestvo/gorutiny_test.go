package kachestvo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Горутины процесса службы запускаются только через internal/fon.
//
// Необработанная паника в любой горутине Go завершает процесс целиком, а
// процесс службы держит туннель и защиту. Голый `go` в этих пакетах значит
// горутину, чья паника уносит сеть человека. Список каталогов это всё, что
// крутится внутри службы и заводит свои горутины; окно живёт отдельным
// процессом и сюда не входит.
var katalogiSluzhby = []string{
	filepath.Join("cmd", "affory-svc"),
	filepath.Join("internal", "yadra"),
	filepath.Join("internal", "skorost"),
	// Свой VPN: каждый пакет internal/svoyvps своей строкой, обход не
	// рекурсивный.
	filepath.Join("internal", "svoyvps", "ssh"),
	filepath.Join("internal", "svoyvps", "server"),
	filepath.Join("internal", "svoyvps", "konfig"),
	filepath.Join("internal", "svoyvps", "proverka"),
	filepath.Join("internal", "svoyvps", "porty"),
	filepath.Join("internal", "svoyvps", "blok"),
}

func TestGorutinySluzhbyCherezFon(t *testing.T) {
	koren, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("корень модуля не вычислен: %v", err)
	}
	proshli := 0
	for _, katalog := range katalogiSluzhby {
		fayly, err := os.ReadDir(filepath.Join(koren, katalog))
		if err != nil {
			t.Fatalf("каталог %s не прочитан: %v", katalog, err)
		}
		for _, f := range fayly {
			imya := f.Name()
			if f.IsDir() || !strings.HasSuffix(imya, ".go") || strings.HasSuffix(imya, "_test.go") {
				continue
			}
			put := filepath.Join(koren, katalog, imya)
			nabor := token.NewFileSet()
			derevo, err := parser.ParseFile(nabor, put, nil, 0)
			if err != nil {
				t.Fatalf("%s не разобран: %v", put, err)
			}
			proshli++
			ast.Inspect(derevo, func(u ast.Node) bool {
				if g, ok := u.(*ast.GoStmt); ok {
					poz := nabor.Position(g.Pos())
					t.Errorf("%s:%d: голый go. В службе горутина запускается через fon.Zapustit "+
						"(или fon.SPovtorom для долгого цикла): её паника иначе роняет процесс с туннелем",
						filepath.Join(katalog, imya), poz.Line)
				}
				return true
			})
		}
	}
	// Пустой обход проходил бы молча: пустой судья выглядит здоровым.
	if proshli == 0 {
		t.Fatalf("ни одного файла в %v: судья ничего не проверил", katalogiSluzhby)
	}
}
