package protokol

import (
	"testing"
	"time"
)

func TestDolgieKomandyPoluchayutDolgiySrok(t *testing.T) {
	// Замерено на стенде 01.09.2026: при едином сроке в пять секунд connect
	// отвечал отказом «служба не ответила», а туннель в это время стоял
	// поднятым, оба ядра жили и внешний адрес был адресом сервера. Отказ на
	// сработавшей команде посылает человека чинить исправное.
	for _, imya := range []string{
		"connect", "disconnect", "setKillSwitch", "refreshSubscription",
		"addServer", "addServers", "removeServer", "setSubscription",
		"exportProfile", "importProfile", "exportServers",
		"setAutoMember", "removeSubscription",
		"svoyVpsZabyt", "svoyVpsPrinyatKlyuchHosta",
	} {
		// Не меньше, а не ровно: команды подписок ждут дольше минуты (L14).
		if SrokOtveta(imya) < SrokDolgoy {
			t.Fatalf("команда %s получила быстрый срок, а за ней стоит работа", imya)
		}
	}
}

func TestChteniePolyaNeZhdyotMinutu(t *testing.T) {
	// Зеркальный случай, и он важнее первого. Задрать срок всем это интерфейс,
	// висящий минуту на мёртвой службе вместо честного «не отвечает».
	//
	// listServers ушла отсюда 03.09.2026 и вернуться не должна: она читает не
	// поле, а хранилище секретов, и при негодном блобе честно работает
	// пятнадцать секунд. Быстрым остаётся только чтение поля под мьютексом.
	for _, imya := range []string{"status", "hello", "subscribeStats"} {
		if SrokOtveta(imya) != SrokBystroy {
			t.Fatalf("команда %s ждёт долго, хотя только читает состояние", imya)
		}
	}
}

func TestNeizvestnayaKomandaZhdyotMalo(t *testing.T) {
	// Умолчание в пользу короткого срока: неизвестная команда это опечатка либо
	// чужой клиент, и минута ожидания отказа на ней никому не помогает.
	if SrokOtveta("takoy-komandy-net") != SrokBystroy {
		t.Fatal("неизвестная команда ждёт долго")
	}
}

func TestDolgiyeKomandyPokryvayutRabotuKotoruyuOniDelayut(t *testing.T) {
	// Срок клиента 5 секунд (internal/kanal/klient.go:200). Всё, что работает
	// дольше, обязано лежать в dolgie, иначе клиент рвёт команду на середине.
	//
	// Карта неэкспортируемая, и тест живёт с ней в одном пакете намеренно:
	// DolgieKomandy отдаёт только имена, а проверять надо ещё и величину.
	nuzhny := map[string]time.Duration{
		"downloadUpdate":  4 * time.Minute,
		"installUpdate":   4 * time.Minute,
		"checkUpdate":     30 * time.Second,
		"setRules":        60 * time.Second,
		"checkExitIp":     30 * time.Second,
		"checkLeaks":      60 * time.Second,
		"getServerHealth": 30 * time.Second,
		// Найдено живой пробой в госте 07.09.2026: обе команды замера
		// отвечали «служба не ответила за 5s», работая при этом честно.
		// Юнит-тесты службы зовут Obrabotat напрямую, минуя канал, и потому
		// не видели этого двое суток.
		//
		// measureBandwidth меряет ДВА направления подряд, каждое до
		// yadra.SrokMax (30 с), то есть минуту чистой работы плюс соединения.
		"measureBandwidth": 90 * time.Second,
		// measureDelays: tcping до 3 с на сервер, по четыре разом, плюс
		// realping через ядро на каждый. Сорок серверов это десять кругов.
		"measureDelays": 60 * time.Second,
	}
	for imya, minimum := range nuzhny {
		srok, est := dolgie[imya]
		if !est {
			t.Errorf("команда %q работает дольше пяти секунд, а срока у неё нет", imya)
			continue
		}
		if srok < minimum {
			t.Errorf("срок %q равен %v, а работа занимает до %v", imya, srok, minimum)
		}
	}
}

// Команда, читающая набор, платит за расшифровку хранилища, сколько бы полей
// она потом ни отдала.
//
// Замерено чтением: `PopytokRasshifrovki` равно пяти, паузы между попытками
// удваиваются от секунды, то есть 1+2+4+8 = 15 секунд сна плюс пять вызовов
// DPAPI плюс похороны блоба. Обычный замер такой команды покажет миллисекунды
// и не докажет ничего: дорог здесь ХУДШИЙ случай, битый sekrety.dat.
//
// Без срока это выглядит как «служба не ответила» на пятой секунде, притом что
// служба в этот момент честно работает и через пятнадцать секунд назовёт
// настоящую причину, secrets-unreadable. Ровно тот дефект, ради которого
// заведена вся эта карта.
func TestKomandyChitayushchieNaborNeUkladyvayutsyaVBystryySrok(t *testing.T) {
	// Пятнадцать секунд сна плюс сами вызовы: минимум с запасом, а не впритык.
	const minimum = 30 * time.Second
	for _, imya := range []string{
		"listServers", "listRules", "setRules",
		"addServer", "addServers", "exportServers", "removeServer", "setSubscription", "setServer", "setRouteMode",
		"refreshSubscription", "connect", "getServerHealth", "checkLeaks",
		// measureDelays обходит набор целиком, значит платит за расшифровку
		// хранилища раньше, чем сделает первый замер.
		"measureDelays",
		// Свой сервер: запись о сервере читается до запуска задания.
		"svoyVpsSpisok", "svoyVpsOsmotret", "svoyVpsUstanovit", "svoyVpsSnesti", "svoyVpsVzyat",
		"svoyVpsProverit", "svoyVpsOtpustit", "svoyVpsParol", "svoyVpsZabyt", "svoyVpsPrinyatKlyuchHosta",
		"svoyVpsPodobratPorty",
	} {
		srok, est := dolgie[imya]
		if !est {
			t.Errorf("команда %q читает набор, а срока у неё нет", imya)
			continue
		}
		if srok < minimum {
			t.Errorf("срок %q равен %v, а одна расшифровка набора стоит до %v", imya, srok, minimum)
		}
	}
}
