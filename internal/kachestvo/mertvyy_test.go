// Пакет kachestvo это ворота приёмки, а не код продукта. Здесь живут проверки,
// которые смотрят на ВЕСЬ проект целиком, и потому не помещаются ни в один
// пакет по существу.
package kachestvo_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kachestvo"
)

// Ш1 ворот приёмки, класс дефекта B: механизм написан, покрыт тестами и
// ВЫКЛЮЧЕН, потому что из прода его никто не зовёт. Класс поймали вручную
// дважды за одну сессию одним и тем же приёмом, и наращивание числа тестов
// против него бесполезно по построению: покрытие БЫЛО, тест звал функцию
// напрямую и был зелёный, а в проде вызова не существовало.
//
// Решение, отличающее эти ворота от черновика плана: `Tests: false`.
// Черновик грузил тесты вместе с продом и отделял их по имени варианта пакета,
// что хрупко (`pkg [pkg.test]`, `pkg_test [pkg.test]`, `pkg.test` это три разных
// написания одного). Без тестов вопрос ставится сам собой: если имя объявлено и
// ни разу не использовано, значит вне тестов его не зовут. Ровно то, что надо.
const korenProekta = "github.com/zxczxczxczxczxczxc1111/affory/..."

// Исключение ОБЯЗАНО нести причину. «Пригодится потом» причиной не является:
// пригодится, тогда и вернём из истории git.
//
// Ключ это полное имя с путём пакета, а не короткое: короткое `Zapisat`
// освободило бы разом все `Zapisat` проекта, включая те, что и правда умерли.
//
// Список короткий НЕ случайно. Из девяти находок первого прогона исключением
// закрыты две: те, где шов нужен тесту ДРУГОГО пакета и потому не убирается в
// `export_test.go`. Остальные семь чинились кодом, а не записью сюда.
var razresheno = map[string]string{
	pakets + "protokol.DolgieKomandy": "шов для теста чужого пакета: " +
		"cmd/affory-svc/komandy_test.go сверяет, что у каждой долгой команды есть " +
		"обработчик. Внутрипакетным export_test.go не закрывается, тест в другом пакете",
	pakets + "set.ServeryVZaprete": "шов для теста чужого пакета: " +
		"cmd/affory-svc/adresa_test.go сверяет адреса серверов в запрете со ВХОДОМ, " +
		"а не список со списком",
	pakets + "genkonfig.VseTransporty": "шов для теста чужого пакета: " +
		"internal/kachestvo/transporty_ekrana_test.go сверяет реестр транспортов " +
		"ядра со словарём имён на экране. Копия списка вместо реестра разошлась бы " +
		"с ним молча, а цена расхождения это пустая ячейка в списке серверов",
	pakets + "sostoyanie.PodmenitKatalogDannyh": "шов для TestMain чужого пакета " +
		"и он ОБЯЗАН молчать в проде: cmd/affory-svc/glavnyy_test.go уводит каталог " +
		"данных во временный на весь прогон. Без него конструктор службы подключает " +
		"настоящее хранилище секретов, и фикстура без подменённого шва записи " +
		"переписывает набор ЖИВОЙ машины. 20.09.2026 так и было: тестовая подписка " +
		"panel.example появлялась у владельца после каждой сборки выпуска. Вызов " +
		"из продуктового кода стережёт TestPodmenaKatalogaZovyotsyaTolkoIzTestov",
	pakets + "diagnostika.(Zhurnal).Zakryt": "шов для уборки тестов чужого пакета: " +
		"NovayaSluzhba открывает log/diagnostika.jsonl в каталоге прогона, и " +
		"cmd/affory-svc/glavnyy_test.go (zakrytZhurnalDiag) отпускает файл в конце " +
		"теста. В проде файл держится всю жизнь процесса и закрывается его выходом. " +
		"Без закрытия Windows не давал TestMain удалить каталог, и к 29.09.2026 в " +
		"%TEMP% скопилось 298 каталогов affory-testy-*",
}

const pakets = "github.com/zxczxczxczxczxczxc1111/affory/internal/"

func TestNetMyortvogoEksporta(t *testing.T) {
	rezhim := packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
		packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports
	pkgs, err := packages.Load(&packages.Config{Mode: rezhim, Dir: "../.."}, korenProekta)
	if err != nil {
		t.Fatal(err)
	}
	// packages.Load возвращает ошибки ВНУТРИ пакетов, а не в err. Ровно на этом
	// уже сломалась одна проверка проекта: она смотрела только на err и потому
	// проходила на пустой загрузке.
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("пакеты не загрузились: %d ошибок", n)
	}
	if len(pkgs) == 0 {
		t.Fatal("не загружено ни одного пакета: ворота ничего не проверяют")
	}

	obyavleno := map[string]*types.Func{}
	// Переменные и константы держатся ОТДЕЛЬНО от функций: у них нет ни
	// приёмника, ни интерфейса, через который их могли бы звать не по имени,
	// поэтому и разбор у них короче. Появились они здесь не из красоты: удаление
	// мёртвой функции осиротило пакетную переменную `pauzy`, и ворота на
	// функциях этого не заметили. Компилятор Go тоже молчит, он ловит только
	// неиспользованные ЛОКАЛЬНЫЕ переменные.
	obyavlenyZnacheniya := map[string]types.Object{}
	zovut := map[string]bool{}
	interfeysy := kontraktyOshibok()

	for _, p := range pkgs {
		for _, t := range p.TypesInfo.Types {
			sobratInterfeysy(t.Type, &interfeysy)
		}
		for _, o := range p.TypesInfo.Uses {
			sobratInterfeysy(o.Type(), &interfeysy)
		}
		for _, f := range p.Syntax {
			// По f.Decls, а не через Inspect: так в разбор попадает только
			// верхний уровень файла. Inspect зашёл бы и внутрь функций, где
			// каждая локальная переменная выглядит объявлением ровно так же.
			for _, d := range f.Decls {
				gd, est := d.(*ast.GenDecl)
				if !est || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
					continue
				}
				for _, spec := range gd.Specs {
					vs, est := spec.(*ast.ValueSpec)
					if !est {
						continue
					}
					for _, imya := range vs.Names {
						if imya.Name == "_" {
							continue
						}
						if o := p.TypesInfo.Defs[imya]; paketnoe(o) {
							obyavlenyZnacheniya[imyaZnacheniya(o)] = o
						}
					}
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.FuncDecl:
					if !v.Name.IsExported() {
						return true
					}
					if o, est := p.TypesInfo.Defs[v.Name].(*types.Func); est {
						obyavleno[imyaObyekta(o)] = o
					}
				case *ast.Ident:
					// Uses, а не Defs: объявление собственного имени не
					// считается его вызовом. Ident, а не только SelectorExpr:
					// внутрипакетный вызов идёт голым именем, и черновик плана
					// счёл бы такую функцию мёртвой при живом вызывающем.
					if o, est := p.TypesInfo.Uses[v].(*types.Func); est {
						zovut[imyaObyekta(o)] = true
					}
					// Проверка на пакетный уровень тут обязательна. Локальная
					// переменная тоже принадлежит пакету, и без этого условия
					// любой `pauzy` внутри чужой функции воскрешал бы мёртвую
					// пакетную переменную с тем же именем.
					if o := p.TypesInfo.Uses[v]; paketnoe(o) {
						zovut[imyaZnacheniya(o)] = true
					}
				}
				return true
			})
		}
	}

	var mertvye []string
	for imya, o := range obyavleno {
		if zovut[imya] {
			continue
		}
		if kto := cherezInterfeys(o, interfeysy); kto != "" {
			t.Logf("пропущено %s: зовётся через интерфейс %s", imya, kto)
			continue
		}
		if prichina, est := razresheno[imya]; est {
			t.Logf("пропущено %s: %s", imya, prichina)
			continue
		}
		mertvye = append(mertvye, imya)
	}
	nesletCodes := 0
	for imya, o := range obyavlenyZnacheniya {
		if zovut[imya] {
			continue
		}
		// Граница проведена по ПАКЕТУ, а не поимённо, и это осознанно.
		// internal/protokol это словарь на проводе: коды отказа и состояния
		// объявлены по §9.1 спеки и обязаны существовать все сразу, иначе
		// служба и интерфейс договариваются на разных языках. Непосланный код
		// это не мёртвый код, это НЕЗАКРЫТЫЙ ПУТЬ ОТКАЗА, и вопрос у него свой:
		// не «удалить ли», а «кто обязан его слать». Поимённый список
		// исключений на восемь строк ответил бы на первый вопрос и похоронил
		// второй, поэтому счёт печатается, а разбор живёт в аудите 04c.
		if strings.HasSuffix(o.Pkg().Path(), "/internal/protokol") {
			nesletCodes++
			t.Logf("пропущено %s: словарь протокола, вопрос не «мёртв ли», а «кто шлёт» (04c)", imya)
			continue
		}
		if prichina, est := razresheno[imya]; est {
			t.Logf("пропущено %s: %s", imya, prichina)
			continue
		}
		mertvye = append(mertvye, imya)
	}
	sort.Strings(mertvye)

	for _, imya := range mertvye {
		t.Errorf("%s не зовётся ниоткуда вне тестов: механизм написан и выключен", imya)
	}
	t.Logf("проверено пакетов %d, экспортированных функций %d, пакетных значений %d, мёртвых %d",
		len(pkgs), len(obyavleno), len(obyavlenyZnacheniya), len(mertvye))
	t.Logf("словарь протокола: %d значений не шлёт никто, разбор в 04c", nesletCodes)

	proveritIsklyucheniya(t, obyavleno)
}

// proveritIsklyucheniya не даёт списку исключений стать кладбищем.
//
// Это главная болезнь таких списков и единственная причина, по которой ворота
// со временем перестают работать: имя удалили или подключили, а строчка с
// оправданием осталась и молча прикрывает следующее имя, попавшее под тот же
// ключ. Поэтому исключение обязано ЖИТЬ: имя должно существовать в проекте и
// упоминаться хотя бы в одном тесте, иначе оправдание протухло.
//
// Сверка с тестами текстовая, и это осознанно: здесь нужен не анализ вызовов
// (его делает сам тест выше), а признак «предмет исключения ещё существует».
func proveritIsklyucheniya(t *testing.T, obyavleno map[string]*types.Func) {
	t.Helper()

	upominaniya := map[string]bool{}
	koren := filepath.Join("..", "..")
	err := filepath.WalkDir(koren, func(put string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			vlozhen, err := kachestvo.VlozhennyyModul(put, koren)
			if err != nil {
				return err
			}
			if vlozhen {
				return filepath.SkipDir
			}
			return nil
		}
		if err != nil || !strings.HasSuffix(put, "_test.go") {
			return err
		}
		telo, err := os.ReadFile(put)
		if err != nil {
			return err
		}
		for imya := range razresheno {
			korotkoe := imya[strings.LastIndex(imya, ".")+1:]
			if strings.Contains(string(telo), korotkoe) {
				upominaniya[imya] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход тестов не удался: %v", err)
	}

	for imya := range razresheno {
		if _, est := obyavleno[imya]; !est {
			t.Errorf("исключение %s протухло: такой функции в проекте больше нет", imya)
			continue
		}
		if !upominaniya[imya] {
			t.Errorf("исключение %s протухло: причина ссылается на тест, а ни один тест его не поминает", imya)
		}
	}
}

// kontraktyOshibok собирает четыре интерфейса, которых НЕТ ни в одном нашем
// типе и быть не может: пакет `errors` объявляет их у себя
// неэкспортированными. `Unwrap() []error` это обёртка нескольких ошибок
// (Go 1.20), её разворачивают те же `errors.Is` и `errors.As`.
// `errors.As` зовётся из нашего прода 32 раза, то есть `Unwrap` и `Is` на наших
// типах ошибок достижимы по-настоящему, просто не по имени.
//
// Без этой добавки ворота обвиняют в смерти каждую обёртку ошибки, а это
// половина ложных срабатываний.
func kontraktyOshibok() []*types.Interface {
	oshibka := types.Universe.Lookup("error").Type()
	lyuboy := types.NewInterfaceType(nil, nil)
	lyuboy.Complete()

	odin := func(imya string, vhod, vyhod types.Type) *types.Interface {
		var params, rezultaty []*types.Var
		if vhod != nil {
			params = append(params, types.NewParam(token.NoPos, nil, "", vhod))
		}
		if vyhod != nil {
			rezultaty = append(rezultaty, types.NewParam(token.NoPos, nil, "", vyhod))
		}
		sig := types.NewSignatureType(nil, nil, nil,
			types.NewTuple(params...), types.NewTuple(rezultaty...), false)
		i := types.NewInterfaceType([]*types.Func{types.NewFunc(token.NoPos, nil, imya, sig)}, nil)
		i.Complete()
		return i
	}

	return []*types.Interface{
		odin("Unwrap", nil, oshibka),
		odin("Unwrap", nil, types.NewSlice(oshibka)),
		odin("Is", oshibka, types.Typ[types.Bool]),
		odin("As", lyuboy, types.Typ[types.Bool]),
	}
}

// sobratInterfeysy складывает все интерфейсы, встреченные в типах проекта:
// и объявленные у нас, и приехавшие из чужих сигнатур. Именно вторые важнее:
// `svc.Handler` в проекте не объявлен нигде, но метод `Execute` зовёт по нему
// диспетчер служб Windows.
func sobratInterfeysy(t types.Type, kuda *[]*types.Interface) {
	if t == nil {
		return
	}
	if i, est := t.Underlying().(*types.Interface); est && i.NumMethods() > 0 {
		*kuda = append(*kuda, i)
		return
	}
	// Сигнатуры разворачиваются: интерфейс чаще всего приезжает параметром
	// чужой функции, а не отдельным выражением.
	if sig, est := t.Underlying().(*types.Signature); est {
		for _, spisok := range []*types.Tuple{sig.Params(), sig.Results()} {
			for i := 0; spisok != nil && i < spisok.Len(); i++ {
				if v, est := spisok.At(i).Type().Underlying().(*types.Interface); est && v.NumMethods() > 0 {
					*kuda = append(*kuda, v)
				}
			}
		}
	}
}

// cherezInterfeys отвечает на вопрос «этот метод могут звать не по имени».
//
// Без него ворота обвиняют в смерти каждый `Error()`, `Unwrap()` и `Execute()`,
// то есть ровно те методы, которые в Go по имени и не зовут никогда. Ловушка
// не косметическая: список исключений рос бы с каждым новым типом ошибки, и
// через месяц в него дописывали бы не глядя, а вместе с ложняками уехала бы и
// настоящая находка.
func cherezInterfeys(o *types.Func, interfeysy []*types.Interface) string {
	sig, _ := o.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return ""
	}
	poluchatel := sig.Recv().Type()
	for _, i := range interfeysy {
		if !imeetMetod(i, o.Name()) {
			continue
		}
		// Проверяются оба написания: метод с приёмником-значением попадает и в
		// набор указателя, обратное неверно.
		if types.Implements(poluchatel, i) || types.Implements(types.NewPointer(poluchatel), i) {
			return i.String()
		}
	}
	return ""
}

func imeetMetod(i *types.Interface, imya string) bool {
	for n := 0; n < i.NumMethods(); n++ {
		if i.Method(n).Name() == imya {
			return true
		}
	}
	return false
}

// paketnoe отделяет объявление верхнего уровня от локального. У пакетного
// объекта родитель это область видимости самого пакета; у локального это
// область видимости функции или блока.
func paketnoe(o types.Object) bool {
	if o == nil || o.Pkg() == nil {
		return false
	}
	switch o.(type) {
	case *types.Var, *types.Const:
	default:
		return false
	}
	return o.Parent() == o.Pkg().Scope()
}

func imyaZnacheniya(o types.Object) string { return o.Pkg().Path() + "." + o.Name() }

// imyaObyekta строит опознание вида `путь/пакет.Имя` или
// `путь/пакет.(Тип).Метод`. Полный путь обязателен: коротких имён в проекте
// хватает одинаковых, и `Prochitat` живёт в трёх пакетах сразу.
func imyaObyekta(o *types.Func) string {
	put := ""
	if o.Pkg() != nil {
		put = o.Pkg().Path() + "."
	}
	sig, _ := o.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return put + o.Name()
	}
	tip := sig.Recv().Type().String()
	// Указатель и значение это один и тот же метод для наших целей, и путь
	// внутри строки типа только мешает читать.
	tip = strings.TrimPrefix(tip, "*")
	if i := strings.LastIndex(tip, "."); i >= 0 {
		tip = tip[i+1:]
	}
	return put + "(" + tip + ")." + o.Name()
}
