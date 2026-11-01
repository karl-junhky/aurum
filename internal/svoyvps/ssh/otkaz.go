// Package ssh ходит по SSH на сервер человека: вход паролем или ключом,
// сверка ключа хоста, права root, команды и файлы.
//
// Слой живёт в службе, а служба держит туннель и защиту. Поэтому каждый вход
// сюда под перехватом паники, горутины только через internal/fon, а на всё,
// что ждёт сеть, есть срок.
//
// Тексты для человека слой не пишет: он различает причины, а что сказать и
// что посоветовать, решает служба. Пароль, ключи и вывод команд с секретами
// не попадают ни в Podrobno, ни в журнал.
package ssh

import (
	"errors"
	"fmt"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
)

// Prichina различает отказы так, чтобы каждой отвечал свой совет.
type Prichina string

const (
	// Соединение не открылось до срока: путь молчит. Что с ним, решает
	// детектор блокировки, а не слой.
	PrichinaTishina Prichina = "tishina"
	// Порт ответил отказом: на нём никто не слушает, чаще всего неверный
	// порт SSH.
	PrichinaPortZakryt Prichina = "port-zakryt"
	// Локального входа туннеля нет: туннель не поднят или 10809 держит
	// чужая программа.
	PrichinaNetVhodaTunnelya Prichina = "net-vhoda-tunnelya"
	// Соединение рвалось до входа все попытки подряд: сброс по дороге.
	PrichinaObryv Prichina = "obryv"
	// Сервер закрывал соединение, не поздоровавшись. Так OpenSSH 9.8 и новее
	// наказывает адрес после неудачных входов, так же отбрасывает лишние
	// подключения MaxStartups. Это «подожди минуту», а не «сервер мёртв».
	PrichinaPodozhdi Prichina = "podozhdi"
	// На порту ответил не SSH.
	PrichinaNeSSH Prichina = "ne-ssh"
	// Ключ хоста не тот, что был записан, и новый не одобрен. До этого
	// отказа на сервер не ушло ничего, кроме подписи входа.
	PrichinaKlyuchHostaSmenilsya Prichina = "klyuch-hosta-smenilsya"
	// Сервер не берёт пароль вовсе: после попытки none в списке способов нет
	// ни password, ни keyboard-interactive.
	PrichinaParolNeBeryot Prichina = "parol-ne-beryot"
	// Пароль отвергнут: неверный, или root по паролю запрещён. Различить их
	// по протоколу нельзя, текст называет оба.
	PrichinaParolNePrinyat  Prichina = "parol-ne-prinyat"
	PrichinaKlyuchNePrinyat Prichina = "klyuch-ne-prinyat"
	// Файл ключа зашифрован, а пароль ключа не дан.
	PrichinaNuzhenParolKlyucha  Prichina = "nuzhen-parol-klyucha"
	PrichinaParolKlyuchaNeveren Prichina = "parol-klyucha-neveren"
	// Ключ в формате PuTTY: его надо сохранить в PuTTYgen как OpenSSH.
	PrichinaKlyuchPpk          Prichina = "klyuch-ppk"
	PrichinaKlyuchNeChitaetsya Prichina = "klyuch-ne-chitaetsya"
	// Сервер требует сменить пароль при первом входе. Без терминала этого не
	// сделать: человеку зайти один раз самому и сменить.
	PrichinaParolProsrochen Prichina = "parol-prosrochen"
	// Вошли не root, а sudo на машине нет.
	PrichinaNetSudo Prichina = "net-sudo"
	// Вошли ключом не root, а sudo просит пароль, которого у слоя нет.
	PrichinaSudoPrositParol Prichina = "sudo-prosit-parol"
	// sudo с паролем входа отказал: пользователь не в sudoers.
	PrichinaSudoNePustil Prichina = "sudo-ne-pustil"
	// sshd не пускает root нашим ключом: PermitRootLogin no, AllowUsers без
	// root, свой AuthorizedKeysFile. Строка нашего ключа убирается; не
	// вышло - Otkaz.KlyuchOstalsya.
	PrichinaRootZapreshchyon Prichina = "root-zapreshchyon"
	// Оболочка входа печатает текст раньше команды (echo в .bashrc или
	// .profile): этот текст лёг бы перед ответом каждой команды и сломал бы
	// его разбор. Человеку убрать вывод для неинтерактивного входа.
	PrichinaShumVhoda Prichina = "shum-vhoda"
	// Два неудачных входа подряд на этот сервер: третий только после паузы,
	// иначе OpenSSH 9.8 и новее запрёт адрес, а через туннель и соседей по
	// выходу.
	PrichinaRanoPovtoryat Prichina = "rano-povtoryat"
	// Путь через туннель выходит в интернет с этого же сервера: перезапуск
	// программы на нём оборвал бы и туннель, и SSH.
	PrichinaTunnelCherezNego Prichina = "tunnel-cherez-nego"
	// Срок операции вышел.
	PrichinaSrok Prichina = "srok"
	// Внутренняя ошибка слоя, в том числе перехваченная паника.
	PrichinaSboy Prichina = "sboy"
)

// Otkaz это ошибка слоя с причиной.
type Otkaz struct {
	Prichina Prichina
	// Podrobno идёт в журнал службы: адрес и порт можно, секреты нельзя.
	Podrobno string
	// Predyavlen это ключ хоста, который показал сервер, когда причина
	// PrichinaKlyuchHostaSmenilsya: его отпечаток человек сверяет перед
	// «Я переустановил систему».
	Predyavlen *KlyuchHosta
	// Podozhdat это сколько ждать до следующей попытки при
	// PrichinaRanoPovtoryat, в секундах.
	Podozhdat int
	// KlyuchOstalsya: строка нашего ключа в authorized_keys root осталась на
	// сервере, хотя войти ею нельзя, и убрать её не удалось.
	KlyuchOstalsya bool
	Err            error
}

// Error идёт в журнал службы, а в тексте Err бывает слово сервера
// (сообщение разрыва SSH): поэтому чистится целиком.
func (o *Otkaz) Error() string {
	s := string(o.Prichina)
	if o.Podrobno != "" {
		s += ": " + o.Podrobno
	}
	if o.Err != nil {
		s += ": " + o.Err.Error()
	}
	return ochistit(s)
}

func (o *Otkaz) Unwrap() error { return o.Err }

// PrichinaIz достаёт причину из ошибки слоя. Чужая ошибка это PrichinaSboy:
// пустая причина ушла бы в окно экраном, которого никто не проектировал.
// Нет ошибки - нет и причины.
func PrichinaIz(err error) Prichina {
	if err == nil {
		return ""
	}
	var o *Otkaz
	if errors.As(err, &o) {
		return o.Prichina
	}
	return PrichinaSboy
}

func otkaz(p Prichina, podrobno string, err error) *Otkaz {
	return &Otkaz{Prichina: p, Podrobno: podrobno, Err: err}
}

// zashchita ставится первой строкой каждого входа в слой: паника внутри
// становится отказом, а не падением службы вместе с туннелем.
func zashchita(gde string, err *error) {
	if r := recover(); r != nil {
		fon.ZapisatPaniku("svoyvps/ssh."+gde, r)
		*err = otkaz(PrichinaSboy, "паника в "+gde, fmt.Errorf("%v", r))
	}
}
