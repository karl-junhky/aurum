package protokol

// Кадры, тело которых НЕ ПЕЧАТАЕТСЯ никогда.
//
// Пароль профиля идёт по каналу открытым текстом, и для локального именованного
// канала с разорванным наследованием ACL это приемлемо. Но из этого прямо
// следует запрет: одна отладочная печать кадра целиком, добавленная через
// полгода «чтобы посмотреть, что приходит», кладёт пароль от всех ключей в
// журнал, который читают все админы машины.
//
// Список живёт ЗДЕСЬ, рядом с проверкой, а не в голове того, кто будет добавлять
// печать.
var telaNeLogiruem = map[string]bool{
	"listConnections":    true, // снимок посещений не должен включать историю через отладочный журнал
	"inspectApplication": true, // список работающих программ остаётся в окне
	"exportProfile":      true,
	"importProfile":      true,
	"setSubscription":    true, // адрес подписки это секрет класса ключа
	"addSubscription":    true, // и здесь он же
	"addServers":         true, // пачка ключей, 26.09.2026
	"exportServers":      true, // ответ это все ключи машины
	"svoyVpsOsmotret":    true, // пароль и ключ входа на свой сервер, 01.11.2026
	"svoyVpsUstanovit":   true, // и здесь они же
	"svoyVpsVzyat":       true, // и при захвате
}

// TeloMozhnoLogirovat отвечает, можно ли печатать тело кадра.
func TeloMozhnoLogirovat(imya string) bool { return !telaNeLogiruem[imya] }

// TeloDlyaZhurnala отдаёт то, что печатать МОЖНО.
//
// Функция, а не «не забудь проверить»: забыть проверку легко, а вызвать
// функцию, которая уже есть, естественно.
func TeloDlyaZhurnala(imya string, telo []byte) string {
	if !TeloMozhnoLogirovat(imya) {
		return "<тело скрыто: команда несёт секрет>"
	}
	return string(telo)
}
