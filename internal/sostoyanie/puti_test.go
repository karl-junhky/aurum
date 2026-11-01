package sostoyanie

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// propustitVlozhennyy уводит обход из каталога со своим go.mod: это uzel,
// программа для Linux, ни каталога данных, ни путей Windows у неё нет. Копия
// vlozhennyyModul из internal/kachestvo: пакеты тестов друг друга не видят.
func propustitVlozhennyy(katalog, koren string) error {
	if filepath.Clean(katalog) == filepath.Clean(koren) {
		return nil
	}
	_, err := os.Stat(filepath.Join(katalog, "go.mod"))
	switch {
	case err == nil:
		return filepath.SkipDir
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	return err
}

// Подмена каталога данных существует ради тестов и обязана оставаться их
// привилегией.
//
// Судья читает исходники, а не полагается на договорённость: вызов, случайно
// попавший в продуктовый код, уводит службу с пути, под который выставлены
// права ACL, и делает это молча. Заметить такое чтением диффа можно ровно один
// раз из десяти.
func TestPodmenaKatalogaZovyotsyaTolkoIzTestov(t *testing.T) {
	koren, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("корень модуля не вычислен: %v", err)
	}

	var naydeno []string
	err = filepath.WalkDir(koren, func(put string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Чужие деревья не наши: там свой код и свои правила.
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return propustitVlozhennyy(put, koren)
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		telo, err := os.ReadFile(put)
		if err != nil {
			return err
		}
		// Своё объявление не считается: оно и есть предмет охраны.
		if put == filepath.Join(koren, "internal", "sostoyanie", "puti.go") {
			return nil
		}
		if strings.Contains(string(telo), "PodmenitKatalogDannyh") {
			otn, _ := filepath.Rel(koren, put)
			naydeno = append(naydeno, otn)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход дерева не удался: %v", err)
	}

	if len(naydeno) > 0 {
		t.Fatalf("подмена каталога данных зовётся из продуктового кода: %v\n"+
			"это уводит службу с пути, под который выставлены права", naydeno)
	}
}

// Подмена и её снятие работают, и снятая подмена возвращает ПОСТОЯННЫЙ путь.
// Без второй половины судья выше зелёный при сломанной подмене.
func TestPodmenaKatalogaDeystvuetIShimaetsya(t *testing.T) {
	prezhniy := korenDannyh
	t.Cleanup(func() { korenDannyh = prezhniy })

	PodmenitKatalogDannyh(`C:\gde-to-eshchyo`)
	if KatalogDannyh() != `C:\gde-to-eshchyo` {
		t.Errorf("подмена не подействовала: %s", KatalogDannyh())
	}
	if KatalogZhurnalov() != filepath.Join(`C:\gde-to-eshchyo`, "log") {
		t.Errorf("журналы не поехали за каталогом: %s", KatalogZhurnalov())
	}

	PodmenitKatalogDannyh("")
	if KatalogDannyh() != filepath.Join(`C:\ProgramData`, "Affory") {
		t.Errorf("снятая подмена не вернула постоянный путь: %s", KatalogDannyh())
	}
}

// Каталог программы читается от СВОЕГО бинаря, а не из константы.
//
// Ровно этого не было до 21.09.2026, и установка в любой каталог, кроме
// `C:\Program Files\Affory`, падала на снятии отпечатков: файлы лежали там,
// куда человек их поставил, а служба смотрела в постоянный путь.
func TestKatalogProgrammyChitaetsyaOtSvoegoBinarya(t *testing.T) {
	prezhniy := katalogProgrammy
	katalogProgrammy = ""
	t.Cleanup(func() { katalogProgrammy = prezhniy })

	svoy, err := os.Executable()
	if err != nil {
		t.Fatalf("свой путь не читается: %v", err)
	}
	hotim := filepath.Dir(svoy)
	if KatalogProgrammy() != hotim {
		t.Errorf("каталог программы %s, а бинарь лежит в %s", KatalogProgrammy(), hotim)
	}
	// Тестовый бинарь лежит во временном каталоге, то есть совпадение с
	// постоянным путём означало бы, что читается всё-таки константа.
	if KatalogProgrammy() == filepath.Join(`C:\Program Files`, "Affory") {
		t.Error("каталог программы совпал с прежней константой: путь зашит")
	}
}

// Подмена пути нужна подменщику обновления и обязана работать в обе стороны.
// Без второй половины сторож ниже зелёный при сломанной подмене.
func TestPodmenaKatalogaProgrammyDeystvuetIShimaetsya(t *testing.T) {
	prezhniy := katalogProgrammy
	t.Cleanup(func() { katalogProgrammy = prezhniy })

	PodmenitKatalogProgrammy(`D:\gde-to\Affory`)
	if KatalogProgrammy() != `D:\gde-to\Affory` {
		t.Errorf("подмена не подействовала: %s", KatalogProgrammy())
	}

	PodmenitKatalogProgrammy("")
	svoy, err := os.Executable()
	if err != nil {
		t.Fatalf("свой путь не читается: %v", err)
	}
	if KatalogProgrammy() != filepath.Dir(svoy) {
		t.Errorf("снятая подмена не вернула путь своего бинаря: %s", KatalogProgrammy())
	}
}

// Постоянный путь не зашит нигде, кроме последней опоры.
//
// Судья читает исходники, потому что дефект возвращается не правкой этой
// функции, а новой строкой `C:\Program Files\Affory` в чужом файле: она
// выглядит безобидно, работает на машине разработчика и ломается только у
// того, кто поставил программу в другое место.
//
// Смотрит СТРОКОВЫЕ ЛИТЕРАЛЫ, а не текст файла. Первая редакция искала
// подстроку и тут же обвинила комментарий, который объясняет, почему путь
// больше не зашит: судья, запрещающий называть дефект по имени, заставляет
// писать вокруг него иносказания.
func TestPostoyannyyPutNeZashitVKode(t *testing.T) {
	koren, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("корень модуля не вычислен: %v", err)
	}

	// Свой файл это и есть предмет охраны: последняя опора при отказе
	// os.Executable. Прибор проверки ссылок не ходит по этому пути, а
	// подставляет его в правила маршрутизации конфига, который разбирает
	// ядром: там это имя процесса, а не файл, который надо открыть.
	isklyucheniya := map[string]string{
		filepath.Join("internal", "sostoyanie", "puti.go"): "последняя опора при отказе os.Executable",
		filepath.Join("cmd", "affory-proverka", "main.go"): "имя процесса в правилах конфига, а не путь к файлу",
	}

	var naydeno []string
	err = filepath.WalkDir(koren, func(put string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return propustitVlozhennyy(put, koren)
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		otn, _ := filepath.Rel(koren, put)
		if _, est := isklyucheniya[otn]; est {
			return nil
		}
		derevo, err := parser.ParseFile(token.NewFileSet(), put, nil, 0)
		if err != nil {
			return fmt.Errorf("%s не разобран: %w", otn, err)
		}
		ast.Inspect(derevo, func(u ast.Node) bool {
			lit, ok := u.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			znachenie, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.Contains(znachenie, `Program Files\Affory`) {
				naydeno = append(naydeno, otn)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("обход дерева не удался: %v", err)
	}

	if len(naydeno) > 0 {
		t.Fatalf("постоянный путь к программе зашит в %v\n"+
			"каталог программы читается от своего бинаря: sostoyanie.KatalogProgrammy", naydeno)
	}
}
