package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

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
	)
	switch {
	case errors.As(err, &zapis):
		return OtkazVps{Kod: kodSohraneniya(zapis.err), Tekst: sboi.DlyaCheloveka(zapis.err)}
	case errors.As(err, &ostatok):
		return OtkazVps{Kod: protokol.KodVpsUstanovka,
			Tekst: "после сноса на сервере осталось: " + strings.Join(ostatok.Chto, ", ") + ". Ключи остались в списке, снос можно повторить"}
	case errors.Is(err, errProverkaDolgo):
		return OtkazVps{Kod: protokol.KodVpsNeOtvechaet, Tekst: err.Error()}
	case errors.Is(err, server.ErrNetUstanovki):
		return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "на сервере нет установки Affory: его можно поставить заново"}
	case errors.Is(err, upravlenie.ErrChuzhayaUstanovka):
		return OtkazVps{Kod: protokol.KodVpsVzyat, Tekst: "сервер переставлен с другого компьютера, и ключи VPN этого компьютера к нему не подходят: возьми его под управление заново"}
	case errors.Is(err, upravlenie.ErrNeDoshla):
		return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "установка на сервере не дошла до ключей: поставь сервер заново, она продолжится"}
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
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "все подходящие порты на сервере заняты чужими программами: задай порт руками в «Дополнительно»"}
	case errors.Is(err, porty.ErrNeReshil):
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "подбор не нашёл порта, который проходит из твоей сети"}
	case errors.Is(err, upravlenie.ErrNetMaski):
		return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "с сервера не открывается ни один сайт для маскировки: похоже, у сервера нет выхода в интернет"}
	}
	return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: sboi.DlyaCheloveka(err)}
}

// otkazMolchaniya: «никто не слушает» говорится, только если порт отказал и
// через VPN. Прямой отказ один ничего не доказывает: сброс подделывают и
// ТСПУ.
func otkazMolchaniya(m *upravlenie.Molchanie) OtkazVps {
	o := OtkazVps{Kod: protokol.KodVpsNeOtvechaet}
	switch {
	case m.Tunnel == nil && ssh.PrichinaIz(m.Pryamo) == ssh.PrichinaPortZakryt:
		o.Tekst = "на этом порту SSH никто не слушает, или соединение сбрасывают по дороге: проверь порт в письме хостера или включи VPN и повтори"
	case m.Tunnel == nil:
		o.Tekst = "сервер не отвечает по SSH: он выключен, адрес неверный или дорогу к нему режут. Включи VPN и повтори, служба зайдёт через него"
	case ssh.PrichinaIz(m.Tunnel) == ssh.PrichinaPortZakryt:
		o.Tekst = "на этом порту SSH никто не слушает: проверь порт в письме хостера"
	default:
		o.Tekst = "сервер не отвечает по SSH ни напрямую, ни через VPN: он выключен или адрес неверный"
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
		return molchit("на этом порту отвечает не SSH: проверь порт в письме хостера")
	case ssh.PrichinaPortZakryt:
		return molchit("на этом порту SSH никто не слушает: проверь порт в письме хостера")
	case ssh.PrichinaNetVhodaTunnelya:
		return molchit("VPN не отвечает на своём входе: переподключи VPN и повтори")
	case ssh.PrichinaTishina, ssh.PrichinaObryv, ssh.PrichinaSrok:
		return molchit("сервер перестал отвечать по SSH: повтори чуть позже")
	}
	return OtkazVps{Kod: protokol.KodVpsUstanovka, Tekst: "сбой связи с сервером"}
}

func tekstOtseva(o porty.Otsev) string {
	switch o.Prichina {
	case porty.PrichinaVne:
		return fmt.Sprintf("порт %d вне 1-65535", o.Port)
	case porty.PrichinaSsh:
		return fmt.Sprintf("на порту %d вход SSH", o.Port)
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
		return "на сервере не Ubuntu (" + pervoe + "): нужна Ubuntu 22.04 или 24.04"
	case server.KodStarayaUbuntu:
		return "Ubuntu старше 20.04 (" + pervoe + "): переустанови систему на Ubuntu 24.04"
	case server.KodUbuntu2004:
		return "у Ubuntu 20.04 с мая 2025 нет обновлений безопасности: лучше переустановить на 24.04"
	case server.KodUbuntuNeProverena:
		return "эту версию (" + pervoe + ") живьём не проверяли: установка пойдёт, но может споткнуться"
	case server.KodNeX86:
		return "процессор " + pervoe + ", а нужен x86_64"
	case server.KodNeSystemd:
		return "система запускается не через systemd"
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
		return "на машине уже есть чужое: " + chto + ". Ставь на чистую систему"
	case server.KodPodCentrom:
		return "сервер уже раздаёт VPN под управлением центра: свой VPN на него не ставится"
	case server.KodUzheStoit:
		// Ключа нет и тогда, когда сервер забыли на этом же компьютере.
		if z.Stop {
			return "на сервере уже стоит Affory, а ключа управления им на этом компьютере нет: возьми его под управление"
		}
		return "на сервере незаконченная установка с этого компьютера: она продолжится"
	case server.KodChuzhoyBrandmauer:
		return "входящие режет свой брандмауэр сервера (" + chto + "): порты, может быть, придётся открыть руками"
	case server.KodNetBBR:
		return "в ядре нет BBR: reality на дальних каналах будет медленнее"
	}
	log.Printf("свой сервер: замечание без текста %q", z.Kod)
	return string(z.Kod)
}

// preduprezhdenieVps переводит «не так» установки, дошедшей до ключей.
func preduprezhdenieVps(p upravlenie.Preduprezhdenie) OtkazVps {
	klyuch := fmt.Sprintf("%s на порту %d", imyaVida(p.Transport), p.Port)
	switch p.Vid {
	case upravlenie.NetSerta:
		return OtkazVps{Kod: protokol.KodVpsNetSerta,
			Tekst: "сертификат на адрес не выпущен: у хостера закрыты порты 80 и 443. Поставлен только reality; открой у хостера порт 80 и подбери порты заново, тогда встанет hysteria2"}
	case upravlenie.NeVezyot:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "ключ " + klyuch + " не провёз проверку"}
	case upravlenie.NeProvereno:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "ключи поставлены, но проверить их не вышло"}
	}
	var vid blok.Vid
	if p.Detektor != nil {
		vid = p.Detektor.Vid
	}
	switch vid {
	case blok.BelyeSpiski:
		return OtkazVps{Kod: protokol.KodVpsSetOgranichena, Tekst: "твоя сеть пускает только разрешённые сайты: свой сервер из неё не виден. Поставлен " + klyuch}
	case blok.NeSostoyalas:
		return OtkazVps{Kod: protokol.KodVpsSetOgranichena, Tekst: "твоя сеть не отвечает, проверить " + klyuch + " не вышло"}
	case blok.OtvetNeVozvrashchaetsya:
		return OtkazVps{Kod: protokol.KodVpsBrandmauerHostera, Tekst: fmt.Sprintf("стук доходит до сервера, а ответ не возвращается: открой у хостера порт %d", p.Port)}
	case blok.NeDohodit:
		return OtkazVps{Kod: protokol.KodVpsAdresZakryt, Tekst: "сервер жив, но твоя сеть не пускает к его адресу: попроси у хостера другой адрес"}
	case blok.Glushat:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "по дороге к серверу глушат шифрованное соединение: " + klyuch + " не прошёл"}
	case blok.Podmena:
		return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: "на адрес сервера отвечает кто-то другой: " + klyuch + " не прошёл"}
	}
	return OtkazVps{Kod: protokol.KodVpsPorty, Tekst: klyuch + " не прошёл проверку из твоей сети"}
}

func imyaVida(transport string) string {
	if transport == "hy2" {
		return "hysteria2"
	}
	return "reality"
}
