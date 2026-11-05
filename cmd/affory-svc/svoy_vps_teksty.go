package main

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// Тексты своего сервера (§9.1 спеки): код выбирает кнопку окна, текст пишет
// служба, потому что только она знает порт, найденное на сервере и
// отпечаток ключа. Строки сервера (имена программ, stderr) чистятся и
// режутся: их пишет чужая машина.

const dlinaSoServera = 200

// otkazVps переводит отказ задания в код и текст. Полная ошибка уходит в
// журнал службы в zapustitVps.
func otkazVps(err error) OtkazVps {
	var (
		molch   *upravlenie.Molchanie
		stop    *upravlenie.Ostanovka
		otk     *ssh.Otkaz
		osh     *server.Oshibka
		otsev   porty.Otsev
		zapis   *oshibkaZapisi
		ostatok *upravlenie.NeChisto
		chuzhoy *server.ParolChuzhoy
		klyuch  *upravlenie.KlyuchNePustil
		uzel    *oshibkaZagruzkiUzla
		negodna *upravlenie.MaskaNegodna
		brend   *upravlenie.MaskaIzvestnaya
	)
	// Отказ после замены программы несёт внутри и отказ SSH, и отказ
	// сервера: он разбирается раньше них.
	if o, ok := otkazObnovleniya(err); ok {
		return o
	}
	switch {
	case errors.As(err, &uzel):
		return OtkazVps{Kod: protokol.KodVpsUstanovka,
			Tekst: "программа для сервера не скачалась с GitHub (" + sboi.DlyaCheloveka(uzel.err) + "): проверь, открывается ли github.com, и повтори. На сервер ничего не ставилось"}
	case errors.As(err, &osh) && osh.Shag == "obnovlenie":
		t := "программа сервера не обновилась"
		if osh.Tekst != "" {
			t += ": " + ssh.Obrezat(osh.Tekst, dlinaSoServera)
		}
		return OtkazVps{Kod: protokol.KodVpsObnovlenie, Tekst: t + ". На сервере работает прежняя"}
	case errors.As(err, &chuzhoy):
		t := "пароль не выключен: вход по паролю держит включённым другая настройка SSH на сервере"
		if len(chuzhoy.Fayly) > 0 {
			t += ", в " + ssh.Obrezat(strings.Join(chuzhoy.Fayly, ", "), dlinaSoServera)
		} else {
			t += ", а в каком файле, найти не вышло"
		}
		return OtkazVps{Kod: protokol.KodVpsParolChuzhoy, Tekst: t + ". На сервере всё осталось как было"}
	case errors.As(err, &klyuch):
		// Отпечаток и срок ожидания вложенного отказа окну так же нужны.
		o := otkazVps(klyuch.Vhod)
		o.Kod, o.KlyuchOstalsya = protokol.KodVpsParol, false
		o.Tekst = "пароль не выключен: после выключения сервер не пустил ключом этого компьютера (" + o.Tekst + ")"
		if klyuch.Vozvrat != nil {
			o.Tekst += ", а сразу вернуть пароль не вышло: его вернёт таймер на сервере через 5 минут"
		} else {
			o.Tekst += ", и пароль включён обратно"
		}
		return o
	case errors.As(err, &osh) && osh.Shag == "parol":
		t := "вход по паролю не переключился"
		if osh.Tekst != "" {
			t += ": " + ssh.Obrezat(osh.Tekst, dlinaSoServera)
		}
		return OtkazVps{Kod: protokol.KodVpsParol, Tekst: t}
	case errors.As(err, &zapis):
		return OtkazVps{Kod: kodSohraneniya(zapis.err), Tekst: sboi.DlyaCheloveka(zapis.err)}
	case errors.Is(err, errSvoyZabyt):
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: err.Error()}
	case errors.As(err, &ostatok):
		return OtkazVps{Kod: protokol.KodVpsUstanovka,
			Tekst: "после сноса на сервере осталось: " + strings.Join(ostatok.Chto, ", ") + ". Ключи остались в списке, снос можно повторить"}
	case errors.Is(err, errProverkaDolgo):
		return OtkazVps{Kod: protokol.KodVpsNeOtvechaet, Tekst: err.Error()}
	case errors.Is(err, server.ErrNetUstanovki):
		return OtkazVps{Kod: protokol.KodVpsPostavitZanovo, Tekst: "на сервере нет установки Affory: поставь его заново, ключи VPN при этом сменятся"}
	case errors.Is(err, upravlenie.ErrChuzhayaUstanovka):
		return OtkazVps{Kod: protokol.KodVpsVzyat, Tekst: "сервер переставлен с другого компьютера, и ключи VPN этого компьютера к нему не подходят: возьми его под управление заново"}
	case errors.Is(err, server.ErrVhodyNeProvereny):
		return OtkazVps{Kod: protokol.KodVpsPosledniyVhod,
			Tekst: "не вышло проверить, какие входы root останутся без ключа этого компьютера: сервер не ответил на проверку своих настроек входа, поэтому ключ не снят. Сервер можно забыть, не снимая ключа"}
	case errors.Is(err, server.ErrPosledniyVhod):
		return OtkazVps{Kod: protokol.KodVpsPosledniyVhod,
			Tekst: "ключ этого компьютера - последний вход root: других ключей, которыми root пустят снаружи, нет, а паролем root не пускают. Без него root не войти, разве что под другим пользователем с sudo или через консоль хостера, поэтому ключ не снят. Сервер можно забыть, не снимая ключа"}
	case errors.Is(err, upravlenie.ErrNeDoshla):
		return OtkazVps{Kod: protokol.KodVpsPostavitZanovo, Tekst: "установка на сервере не дошла до ключей: поставь сервер заново"}
	case errors.As(err, &negodna):
		return OtkazVps{Kod: protokol.KodVpsMaska,
			Tekst: "маска " + ssh.Obrezat(negodna.Imya, 80) + " не годится: " + ssh.Obrezat(negodna.Prichina, dlinaSoServera) + ". На сервере прежняя маска"}
	case errors.As(err, &brend):
		return OtkazVps{Kod: protokol.KodVpsMaskaIzvestnaya,
			Tekst: "маска " + brend.Imya + " - известный сайт: на мобильном интернете такие имена глушат, и ключ reality с ним может не открыться. На сервере прежняя маска"}
	case errors.Is(err, upravlenie.ErrNetDrugoyMaski):
		return OtkazVps{Kod: protokol.KodVpsMaska,
			Tekst: "другой годной маски не нашлось ни рядом с сервером, ни в запасном списке: можно ввести свою. На сервере прежняя маска"}
	case errors.Is(err, upravlenie.ErrStarayaProgramma):
		return OtkazVps{Kod: protokol.KodVpsMaska, Tekst: "программа на сервере старая и маску не меняет: сначала обнови её"}
	case errors.As(err, &molch):
		return otkazMolchaniya(molch)
	case errors.As(err, &stop):
		z := stop.Zamechaniya[0]
		return OtkazVps{Kod: kodZamechaniya(z.Kod), Tekst: tekstZamechaniya(z)}
	case errors.As(err, &otk):
		return otkazSsh(otk)
	case errors.As(err, &osh):
		t := "установка на сервере не прошла"
		if osh.Tekst != "" {
			t += ": " + ssh.Obrezat(osh.Tekst, dlinaSoServera)
		}
		return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: t, KlyuchOstalsya: osh.KlyuchOstalsya}
	case errors.As(err, &otsev):
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: tekstOtseva(otsev)}
	case errors.Is(err, porty.ErrNetKandidatov):
		// Без «где»: при установке это «Дополнительно» мастера, при подборе
		// заново «Задать порты» в карточке.
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "все подходящие порты на сервере заняты чужими программами: задай порт руками"}
	case errors.Is(err, porty.ErrNeReshil):
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "подбор не нашёл порта, который проходит из твоей сети"}
	case errors.Is(err, upravlenie.ErrNetMaski):
		return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "с сервера не открывается ни один сайт для маскировки: похоже, у сервера нет выхода в интернет"}
	}
	return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: sboi.DlyaCheloveka(err)}
}

// otkazMolchaniya: «не принимает вход» говорится, только если порт отказал и
// через VPN. Прямой отказ один ничего не доказывает: сброс подделывают и
// ТСПУ.
func otkazMolchaniya(m *upravlenie.Molchanie) OtkazVps {
	o := OtkazVps{Kod: protokol.KodVpsNeOtvechaet}
	switch {
	case m.Tunnel == nil && ssh.PrichinaIz(m.Pryamo) == ssh.PrichinaPortZakryt:
		o.Tekst = "на этом порту сервер не принимает вход, или соединение сбрасывают по дороге: проверь порт в письме хостера или включи VPN и повтори"
	case m.Tunnel == nil:
		o.Tekst = "сервер не отвечает на вход: он выключен, адрес неверный или дорогу к нему режут. Включи VPN и повтори, служба зайдёт через него"
	case ssh.PrichinaIz(m.Tunnel) == ssh.PrichinaPortZakryt:
		o.Tekst = "на этом порту сервер не принимает вход: проверь порт в письме хостера"
	default:
		o.Tekst = "сервер не отвечает на вход ни напрямую, ни через VPN: он выключен или адрес неверный"
	}
	return o
}

func otkazSsh(otk *ssh.Otkaz) OtkazVps {
	vhod := func(t string) OtkazVps {
		return OtkazVps{Kod: protokol.KodVpsVhodOtvergnut, Tekst: t, KlyuchOstalsya: otk.KlyuchOstalsya}
	}
	molchit := func(t string) OtkazVps {
		return OtkazVps{Kod: protokol.KodVpsNeOtvechaet, Tekst: t, Podozhdat: otk.Podozhdat}
	}
	switch otk.Prichina {
	case ssh.PrichinaKlyuchHostaSmenilsya:
		o := OtkazVps{Kod: protokol.KodVpsKlyuchHosta,
			Tekst: "сервер предъявил другой ключ. Так бывает после переустановки системы; если её не переустанавливали, сервер может подменять кто-то по дороге"}
		if otk.Predyavlen != nil {
			o.Otpechatok = otk.Predyavlen.Otpechatok()
		}
		return o
	case ssh.PrichinaTunnelCherezNego:
		return OtkazVps{Kod: protokol.KodVpsTunnelCherezNego, Tekst: "VPN сейчас идёт через этот же сервер: переключись на другой сервер или отключи VPN"}
	case ssh.PrichinaParolNeBeryot:
		return vhod("сервер не принимает вход по паролю: нужен ключ SSH")
	case ssh.PrichinaParolNePrinyat:
		return vhod("пароль не подошёл, или вход root по паролю на сервере запрещён")
	case ssh.PrichinaKlyuchNePrinyat:
		return vhod("сервер не принял ключ SSH")
	case ssh.PrichinaNuzhenParolKlyucha:
		return vhod("ключ SSH зашифрован: нужен его пароль")
	case ssh.PrichinaParolKlyuchaNeveren:
		return vhod("пароль ключа SSH не подошёл")
	case ssh.PrichinaKlyuchPpk:
		return vhod("ключ в формате PuTTY: сохрани его в PuTTYgen как ключ OpenSSH")
	case ssh.PrichinaKlyuchNeChitaetsya:
		return vhod("файл не читается как ключ SSH")
	case ssh.PrichinaParolProsrochen:
		return vhod("сервер требует сменить пароль при первом входе: зайди на него один раз сам, смени пароль и повтори")
	case ssh.PrichinaNetSudo:
		return vhod("вход не под root, а sudo на сервере нет: войди как root")
	case ssh.PrichinaSudoPrositParol:
		return vhod("sudo просит пароль: укажи пароль пользователя или войди как root")
	case ssh.PrichinaSudoNePustil:
		return vhod("пользователю не разрешён sudo: войди как root")
	case ssh.PrichinaRootZapreshchyon:
		return vhod("сервер не пускает root по ключу: в настройках SSH запрещён вход root или задан свой список пользователей")
	case ssh.PrichinaShumVhoda:
		return vhod("при входе сервер печатает текст раньше команды: убери вывод из .bashrc или .profile для входа без терминала")
	case ssh.PrichinaPodozhdi:
		o := molchit("сервер сразу закрывает соединение: так он наказывает адрес после неудачных входов. Подожди минуту")
		if o.Podozhdat == 0 {
			o.Podozhdat = 60
		}
		return o
	case ssh.PrichinaRanoPovtoryat:
		return molchit(fmt.Sprintf("два входа подряд не удались: следующий через %d с, иначе сервер запрёт твой адрес", otk.Podozhdat))
	case ssh.PrichinaNeSSH:
		return molchit("на этом порту отвечает другая программа, а не вход на сервер: проверь порт в письме хостера")
	case ssh.PrichinaPortZakryt:
		return molchit("на этом порту сервер не принимает вход: проверь порт в письме хостера")
	case ssh.PrichinaNetVhodaTunnelya:
		return molchit("VPN не отвечает на своём входе: переподключи VPN и повтори")
	case ssh.PrichinaTishina, ssh.PrichinaObryv, ssh.PrichinaSrok:
		return molchit("сервер перестал отвечать: повтори чуть позже")
	}
	return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "сбой связи с сервером"}
}

func tekstOtseva(o porty.Otsev) string {
	switch o.Prichina {
	case porty.PrichinaVne:
		return fmt.Sprintf("порт %d вне 1-65535", o.Port)
	case porty.PrichinaSsh:
		return fmt.Sprintf("на порту %d вход на сервер", o.Port)
	case porty.PrichinaSert:
		return fmt.Sprintf("порт %d нужен для продления сертификата", o.Port)
	case porty.PrichinaZanyat:
		return fmt.Sprintf("порт %d/%s занят программой %s", o.Port, o.Proto, ssh.Obrezat(o.Process, 60))
	}
	return fmt.Sprintf("порт %d не годится", o.Port)
}

// kodZamechaniya выбирает кнопку окна по замечанию, которое остановило
// установку.
func kodZamechaniya(k server.Kod) string {
	switch k {
	case server.KodNePustoy:
		return protokol.KodVpsNePustoy
	case server.KodUzheStoit:
		return protokol.KodVpsVzyat
	case server.KodNastraivaetsya, server.KodSboyOsmotra:
		return protokol.KodVpsUstanovka
	}
	return protokol.KodVpsNeGoditsya
}

// tekstZamechaniya пишет пункт осмотра. Stop у KodUzheStoit значит, что
// отметка чужая.
func tekstZamechaniya(z server.Zamechanie) string {
	chto := ssh.Obrezat(strings.Join(z.Chto, ", "), dlinaSoServera)
	pervoe := ""
	if len(z.Chto) > 0 {
		pervoe = ssh.Obrezat(z.Chto[0], 60)
	}
	switch z.Kod {
	case server.KodNeUbuntu:
		return "на сервере не Ubuntu (" + pervoe + "): нужна Ubuntu 20.04 или новее, лучше 24.04"
	case server.KodStarayaUbuntu:
		return "Ubuntu старше 20.04 (" + pervoe + "): переустанови систему на Ubuntu 24.04"
	case server.KodUbuntu2004:
		return "у Ubuntu 20.04 с мая 2025 нет обновлений безопасности: лучше переустановить на 24.04"
	case server.KodUbuntuNeProverena:
		return "эту версию (" + pervoe + ") живьём не проверяли: установка пойдёт, но может споткнуться"
	case server.KodNeX86:
		return "процессор " + pervoe + ", а нужен x86_64"
	case server.KodNeSystemd:
		return "система сервера не умеет запускать службы так, как нужно Affory (нет systemd)"
	case server.KodKonteyner:
		return "это контейнер " + pervoe + ", а не виртуальная машина: нужен VPS на KVM"
	case server.KodMaloPamyati:
		return "памяти " + pervoe + " МБ, нужно хотя бы 128"
	case server.KodMaloDiska:
		return "свободного места " + pervoe + " МБ, нужно хотя бы 256"
	case server.KodNetUtility:
		return "на сервере нет нужных программ: " + chto
	case server.KodNetUtilityZhelatelno:
		return "нет программ " + chto + ": установка обойдётся без них"
	case server.KodSboyOsmotra:
		return "осмотр не смог проверить: " + chto
	case server.KodNetIPv4:
		return "у сервера нет публичного IPv4"
	case server.KodChastnyyIPv4:
		return "на сервере частный адрес " + pervoe + ": порты проверятся с твоего компьютера"
	case server.KodChasy:
		return "часы сервера расходятся на " + pervoe + ": сертификат может не выпуститься"
	case server.KodNastraivaetsya:
		return "машина ещё настраивается после создания: установка подождёт до 5 минут"
	case server.KodNePustoy:
		return "сервер не пустой: " + chuzhoeDlyaCheloveka(z.Chto) + ". Ставь на чистую систему"
	case server.KodPodCentrom:
		return "сервер уже раздаёт VPN под управлением центра: свой VPN на него не ставится"
	case server.KodUzheStoit:
		// Ключа нет и тогда, когда сервер забыли на этом же компьютере.
		if z.Stop {
			return "на сервере уже стоит Affory, а ключа управления им на этом компьютере нет: возьми его под управление"
		}
		// Программа уже стоит, а пакета с ключами может и не быть: по отметке
		// не видно. Сервер из раздела служба поверх записи ставит только из
		// «Поставить заново» в карточке, поэтому обещания «пройдёт» тут нет.
		if pervoe == "gotovo" {
			return "на сервере уже стоит Affory с этого компьютера: новая установка сменит ключи VPN"
		}
		return "на сервере незаконченная установка с этого компьютера: она продолжится"
	case server.KodChuzhoyBrandmauer:
		return "входящие режет свой брандмауэр сервера (" + chto + "): порты, может быть, придётся открыть руками"
	case server.KodNetBBR:
		return "система сервера не умеет ускорять дальние соединения (нет BBR): reality издалека будет медленнее"
	}
	log.Printf("свой сервер: замечание без текста %q", z.Kod)
	return string(z.Kod)
}

// chuzhoeDlyaCheloveka пишет словами окна находки осмотра, которые скрипт и
// Osmotr.chuzhoe отдают метками: «programma nginx», «yunit nginx.service»,
// «katalog /etc/xray», «port tcp/443 nginx», «programma vpn-uzel». Программа
// и её служба называются одним именем один раз. Список режется по целым
// пунктам: обрыв посреди слова читается как опечатка. Сам пункт режется,
// только если он длиннее 60 знаков: так бывает разве что у службы с длинным
// именем конфига вроде openvpn-server@….
func chuzhoeDlyaCheloveka(chto []string) string {
	var slova []string
	for _, c := range chto {
		vid, ostatok, _ := strings.Cut(c, " ")
		switch vid {
		case "programma", "yunit":
			c = strings.TrimSuffix(ostatok, ".service")
			// vpn-uzel без отметки: установку Affory оборвали или снесли не до конца.
			if c == "vpn-uzel" {
				c = "остатки прежней установки Affory"
			}
		case "katalog":
			c = "папка " + ostatok
		case "port":
			adres, kto, _ := strings.Cut(ostatok, " ")
			if proto, port, ok := strings.Cut(adres, "/"); ok {
				// Протокол человеку нужен, только если это не обычный TCP.
				if proto != "tcp" {
					port += "/" + proto
				}
				c = "порт " + port + " занят"
				// «?»: ss не назвал процесс.
				if kto != "" && kto != "?" {
					c += " " + kto
				}
			}
		}
		c = ssh.Obrezat(c, 60)
		if c != "" && !slices.Contains(slova, c) {
			slova = append(slova, c)
		}
	}
	tekst := ""
	for i, s := range slova {
		if i > 0 && utf8.RuneCountInString(tekst)+2+utf8.RuneCountInString(s) > dlinaSoServera {
			return tekst + " и ещё " + strconv.Itoa(len(slova)-i)
		}
		if i > 0 {
			tekst += ", "
		}
		tekst += s
	}
	return tekst
}

// preduprezhdeniyaVps переводит «не так» установки, дошедшей до ключей.
// «Не прошёл» одного детектора называет все виды разом: вердикт у них
// общий, и иначе одна и та же строка стояла бы дважды. Вид, чей порт
// перехватывает программа на этом компьютере, идёт своей строкой.
func preduprezhdeniyaVps(pr []upravlenie.Preduprezhdenie, pom pomehiKompyutera) []OtkazVps {
	out := make([]OtkazVps, 0, len(pr))
	vzyato := make([]bool, len(pr))
	for i, p := range pr {
		if vzyato[i] {
			continue
		}
		gruppa := []upravlenie.Preduprezhdenie{p}
		if p.Vid == upravlenie.NeProshyol {
			kto := pom.perehvatchik(p)
			for j := i + 1; j < len(pr); j++ {
				q := pr[j]
				if !vzyato[j] && q.Vid == upravlenie.NeProshyol && q.Detektor == p.Detektor && q.Otkaz == p.Otkaz &&
					pom.perehvatchik(q) == kto {
					gruppa, vzyato[j] = append(gruppa, q), true
				}
			}
		}
		out = append(out, preduprezhdenieVps(gruppa, pom))
	}
	return out
}

// preduprezhdenieVps пишет одно предупреждение. Виды в gruppa отличаются
// только ключом: вид и вердикт берутся у первого.
func preduprezhdenieVps(gruppa []upravlenie.Preduprezhdenie, pom pomehiKompyutera) OtkazVps {
	p := gruppa[0]
	var klyuchi, porty []string
	for _, x := range gruppa {
		klyuchi = append(klyuchi, fmt.Sprintf("%s на порту %d", imyaVida(x.Transport), x.Port))
		proto := "TCP"
		if x.Transport == "hy2" {
			proto = "UDP"
		}
		porty = append(porty, fmt.Sprintf("%d (%s)", x.Port, proto))
	}
	klyuch := strings.Join(klyuchi, " и ")
	neProshyol := " не прошёл"
	if len(gruppa) > 1 {
		neProshyol = " не прошли"
	}
	kto := pom.perehvatchik(p)
	switch p.Vid {
	case upravlenie.NetSerta:
		return OtkazVps{Kod: protokol.KodVpsNetSerta,
			Tekst: "сертификат на адрес не выпущен: у хостера закрыты порты 80 и 443. Поставлен только reality; открой у хостера порт 80 и подбери порты заново, тогда встанет hysteria2"}
	case upravlenie.NeVezyot:
		if kto != "" {
			return OtkazVps{Kod: protokol.KodVpsPorty, IzSeti: true, Tekst: tekstPerehvata(klyuch, neProshyol, kto)}
		}
		return OtkazVps{Kod: protokol.KodVpsPorty, IzSeti: true, Tekst: "ключ " + klyuch + " не провёз проверку из твоей сети"}
	case upravlenie.NeProvereno:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "ключи поставлены, но проверить их не вышло"}
	case upravlenie.PortyNeOtkryty:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "ключи поставлены, но открыть их порты в брандмауэре сервера не вышло: подбери порты заново"}
	}
	var vid blok.Vid
	if p.Detektor != nil {
		vid = p.Detektor.Vid
	}
	o := OtkazVps{Kod: protokol.KodVpsPorty, IzSeti: true}
	// Перехватчик встаёт посреди соединения, и детектор мог принять это за
	// глушение или подмену, а сервер для него доступен. Белые списки,
	// молчащую сеть или сервер, закрытый адрес и брандмауэр хостера он не
	// объясняет: о них слово детектора.
	if kto != "" && slices.Contains([]blok.Vid{"", blok.Dostupen, blok.Glushat, blok.Podmena}, vid) {
		o.Tekst = tekstPerehvata(klyuch, neProshyol, kto)
		return o
	}
	switch vid {
	case blok.BelyeSpiski:
		o.Kod, o.Tekst = protokol.KodVpsSetOgranichena, "твоя сеть пускает только разрешённые сайты: свой сервер из неё не виден. Ключи стоят, проверь их с телефона по мобильному интернету"
	case blok.NeSostoyalas:
		o.Kod, o.Tekst = protokol.KodVpsSetOgranichena, "твоя сеть не отвечает, проверить "+klyuch+" не вышло"
	case blok.OtvetNeVozvrashchaetsya:
		o.Kod, o.Tekst = protokol.KodVpsBrandmauerHostera, "попытки подключиться доходят до сервера, а ответ не возвращается: открой у хостера "+slovoPorta(len(porty))+" "+strings.Join(porty, " и ")
	case blok.NeDohodit:
		// Другой вид мог пройти: тогда проверять с телефона только этот.
		chto, otkroyutsya := "их", "не откроются"
		if len(gruppa) == 1 {
			chto, otkroyutsya = imyaVida(p.Transport), "не откроется"
		}
		o.Kod, o.Tekst = protokol.KodVpsAdresZakryt, "сервер жив, ключи стоят. Проверь "+chto+" с телефона по мобильному интернету: если и там "+otkroyutsya+", попроси у хостера другой адрес"
	case blok.Glushat:
		o.Tekst = "по дороге к серверу глушат шифрованное соединение: " + klyuch + neProshyol
	case blok.Podmena:
		o.Tekst = "на адрес сервера отвечает кто-то другой: " + klyuch + neProshyol
	default:
		o.Tekst = klyuch + neProshyol + " проверку из твоей сети"
	}
	return o
}

// tekstPerehvata: ключ не прошёл, а его порт на этом компьютере
// перехватывает программа kto. Подбор заново возьмёт порт, который
// прошёл, а Avast перехватывает только почтовые.
func tekstPerehvata(klyuch, neProshyol, kto string) string {
	return klyuch + neProshyol + ": порт перехватывает " + kto + " на этом компьютере. Подбери порты заново, подбор возьмёт другой порт"
}

func slovoPorta(n int) string {
	if n > 1 {
		return "порты"
	}
	return "порт"
}

// tekstKruga это что показал круг детектора, после которого будут ещё:
// окно говорит это сразу и даёт не ждать повторов (И7). Пусто - круг не
// отрицательный, говорить нечего.
func tekstKruga(v blok.Vid) string {
	switch v {
	case blok.NeDohodit, blok.ServerMolchit, blok.MolchitNapryamuyu:
		return "из твоей сети адрес сервера сейчас не открывается. Так бывает временно"
	case blok.Glushat:
		return "по дороге к серверу сейчас глушат шифрованное соединение"
	case blok.Podmena:
		return "на адрес сервера сейчас отвечает кто-то другой"
	case blok.OtvetNeVozvrashchaetsya:
		return "сервер получает попытки подключиться, а ответ до тебя не доходит: похоже, порт закрыт у хостера"
	}
	return ""
}

// tekstSeti это почему ключи не прошли из сети человека, для записи о
// сервере: карточка показывает его, пока ключи не пройдут.
func tekstSeti(pr []OtkazVps) string {
	var chasti []string
	for _, p := range pr {
		if p.IzSeti && p.Tekst != "" {
			chasti = append(chasti, p.Tekst)
		}
	}
	return skleitPrichiny(chasti)
}

// skleitPrichiny пишет причины одной строкой: вторая и дальше с большой
// буквы, как новые предложения.
func skleitPrichiny(chasti []string) string {
	chasti = slices.Clone(chasti)
	for i := 1; i < len(chasti); i++ {
		r := []rune(chasti[i])
		chasti[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(chasti, ". ")
}

// prichinySeti это причина для записи о сервере: общая на виды vidy и,
// когда видов больше одного, своя у каждого. Своя нужна, когда проверка
// снимет один вид: общая могла назвать и его, а своя сохранит совет
// оставшемуся, например какой порт открыть у хостера.
func prichinySeti(pr []upravlenie.Preduprezhdenie, vidy []string, pom pomehiKompyutera) (string, map[string]string) {
	obshchaya := tekstSeti(preduprezhdeniyaVps(pr, pom))
	if len(vidy) < 2 {
		return obshchaya, nil
	}
	poVidam := make(map[string]string, len(vidy))
	for _, t := range vidy {
		svoi := slices.DeleteFunc(slices.Clone(pr), func(p upravlenie.Preduprezhdenie) bool { return p.Transport != t })
		poVidam[t] = tekstSeti(preduprezhdeniyaVps(svoi, pom))
	}
	return obshchaya, poVidam
}

// tekstVidov это причина для видов vidy из причин по видам: так она не
// назовёт снятый вид. Одинаковые причины пишутся раз.
func tekstVidov(vidy []string, poVidam map[string]string) string {
	var chasti []string
	for _, t := range vidy {
		if tekst := poVidam[t]; tekst != "" && !slices.Contains(chasti, tekst) {
			chasti = append(chasti, tekst)
		}
	}
	return skleitPrichiny(chasti)
}

func imyaVida(transport string) string {
	if transport == "hy2" {
		return "hysteria2"
	}
	return "reality"
}
