package kachestvo_test

import (
	"regexp"
	"sort"
	"testing"
)

// Долг 4 волны 4: критерий волны звучит «у каждого кода свой текст И СВОЁ
// ДЕЙСТВИЕ», а проверялся только текст. Действие не проверялось ничем, и самой
// таблицы действий не существовало: в плане лежал список слов без привязки к
// кодам.
//
// Почему это ворота, а не аккуратность. Экран отказа без действия оставляет
// человека с фразой и ничем: «сервер не принял ключ» это не задача, которую
// можно выполнить. Двадцать девять кодов расписать один раз дёшево, а забыть
// действие у тридцатого невидимо: текст-то будет.
//
// Словарь ЗАКРЫТ. Незнакомое действие в спеке это либо опечатка, либо новая
// кнопка, которую никто не проектировал, и оба случая обязаны быть красными.
var razreshennyeDeystviya = map[string]string{
	"povtorit":           "повторить ту же операцию",
	"perepodklyuchitsya": "переподнять туннель целиком: живого переключения тут мало",
	"otkryt-servery":     "открыть вкладку серверов и выбрать другой",
	"obnovit-podpisku":   "обновить подписку",
	"obnovit-programmu":  "поставить свежую версию: половинки разных версий",
	"postavit-sluzhbu":   "экран первого запуска, установка службы",
	"zaprosit-prava":     "повторить с повышением прав",
	"proverit-set":       "проверить сеть: перезапускать нечего",
	"pokazat-vinovnika":  "назвать чужую программу по имени",
	// 1.10.0, clock-skew: окно открывает настройки даты и времени Windows.
	"nastroit-vremya": "открыть настройки даты и времени Windows",
	"nichego":         "действия нет, экран только сообщает",
	// 1.11.0, свой VPN (Е3 плана «Свой VPN»).
	"prinyat-klyuch-hosta":  "«Систему переустановили»: принять новый ключ хоста своего сервера",
	"pereustanovit-os":      "переустановить ОС в панели хостера и поставить на чистую",
	"vzyat-pod-upravlenie":  "взять свой сервер под управление с этого компьютера",
	"postavit-zanovo":       "поставить свой сервер заново поверх записи о нём",
	"otkryt-port-u-hostera": "открыть порт в панели хостера",
	"poprosit-drugoy-adres": "попросить у хостера другой адрес",
}

// Строка таблицы §9.1 после добавления колонки: код первой ячейкой, действие
// последней. Разбор идёт по началу и концу строки, а не по счёту разделителей:
// внутри ячеек живут и обратные кавычки, и полужирный текст.
// Хвостовой `\r` разрешён намеренно. Спека лежит в СОСЕДНЕМ репозитории, а у
// того core.autocrlf true: любой `git checkout` этого файла переписывает концы
// строк на CRLF. Разбор без `\r` находит тогда ноль строк и валит тест
// сообщением «сломан разбор», хотя сломаны концы строк, а не разбор.
// 05.09.2026 это стоило получаса поисков не там.
var reDeystvie = regexp.MustCompile("(?m)^[|] `([a-z0-9-]+)` [|].*[|] `([a-z0-9-]+)` [|][ \t]*\r?$")

func TestUKazhdogoKodaEstDeystvie(t *testing.T) {
	spec, est := razdel91(trebovatSpeku(t))
	if !est {
		t.Fatal("в спеке не найден раздел 9.1: сверять не с чем")
	}

	// Пустой словарь сделал бы проверку зелёной при любом содержимом таблицы.
	// Ровно так уже промолчал тест границы импортов, и урок стоил двух заходов.
	if len(razreshennyeDeystviya) == 0 {
		t.Fatal("словарь действий пуст: проверка ничего не проверяет")
	}

	uKoda := map[string]string{}
	for _, m := range reDeystvie.FindAllStringSubmatch(spec, -1) {
		uKoda[m[1]] = m[2]
	}
	if len(uKoda) == 0 {
		t.Fatal("в §9.1 не разобрана ни одна строка с действием: сломан разбор, а не спека")
	}

	vKode := kodyIzPaketa(t)
	if len(vKode) == 0 {
		t.Fatal("в пакете protokol не нашлось ни одного кода: сверка ничего не проверяет")
	}

	var bezDeystviya []string
	for kod := range vKode {
		if _, mozhno := kodyBezStroki[kod]; mozhno {
			continue
		}
		if uKoda[kod] == "" {
			bezDeystviya = append(bezDeystviya, kod)
		}
	}
	sort.Strings(bezDeystviya)
	for _, kod := range bezDeystviya {
		t.Errorf("у кода %q в §9.1 нет действия: человеку показали фразу и ничего, что можно сделать", kod)
	}

	var chuzhie []string
	for kod, d := range uKoda {
		if _, mozhno := razreshennyeDeystviya[d]; !mozhno {
			chuzhie = append(chuzhie, kod+" -> "+d)
		}
	}
	sort.Strings(chuzhie)
	for _, s := range chuzhie {
		t.Errorf("действие вне словаря: %s; либо опечатка, либо кнопка, которую никто не проектировал", s)
	}

	// Обратная сторона: действие, которого не просит ни один код, это мёртвая
	// строка словаря. Она переживёт удаление последнего своего кода молча.
	ispolzuetsya := map[string]bool{}
	for _, d := range uKoda {
		ispolzuetsya[d] = true
	}
	var mertvye []string
	for d := range razreshennyeDeystviya {
		if !ispolzuetsya[d] {
			mertvye = append(mertvye, d)
		}
	}
	sort.Strings(mertvye)
	for _, d := range mertvye {
		t.Errorf("действие %q (%s) не просит ни один код: словарь растёт мёртвыми строками",
			d, razreshennyeDeystviya[d])
	}

	t.Logf("кодов с действием %d, различных действий %d из %d в словаре",
		len(uKoda), len(ispolzuetsya), len(razreshennyeDeystviya))
}
