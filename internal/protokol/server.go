package protokol

// Everything the parser produces and the generator consumes. Adding a transport
// means adding a case in genkonfig, so the string is not free-form either.
type Server struct {
	Id        string `json:"id"`
	Imya      string `json:"imya"`
	Transport string `json:"transport"` // reality-tcp | reality-grpc | tls-tcp | ws | grpc | httpupgrade | hy2 | ss | trojan | trojan-ws | vmess | vmess-ws | anytls | tuic
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Uuid      string `json:"uuid,omitempty"`
	PublicKey string `json:"pbk,omitempty"`
	ShortId   string `json:"sid,omitempty"`
	Flow      string `json:"flow,omitempty"`

	// Добавлено волной 2. Без этих четырёх полей модель не выражает половину
	// собственной таблицы транспортов: у ss нет ни метода, ни пароля, у hy2
	// пароля, у ws и grpc пути. Обнаружено при сборке фикстур генератора, то
	// есть ровно там, где такие дыры и обязаны обнаруживаться.
	Parol      string `json:"parol,omitempty"` // hy2, ss
	Metod      string `json:"metod,omitempty"` // ss
	Put        string `json:"put,omitempty"`   // ws: path, grpc: service_name
	Sni        string `json:"sni,omitempty"`   // имя в TLS, если отличается от Host
	IzPodpiski bool   `json:"iz_podpiski"`

	// Запись пропала из публикации, но ядро её ещё держит кандидатом, поэтому она
	// осталась в наборе до переподключения. Ни «из подписки» (подписка про неё уже
	// не знает), ни ручная (ручную слияние тащит вечно, а эту надо выбросить, как
	// только ядро её отпустит).
	Uderzhan bool `json:"uderzhan,omitempty"`

	// Svoy: ключ своего сервера, поставленного разделом «Свой VPN» (1.11.0).
	// По нему ядро ставит своему reality отпечаток firefox, тот же, с которым
	// ключ проверяли при установке. Чужой ссылке firefox не годится (см.
	// otpechatokReality в genkonfig), а свой сервер на sing-box его принимает.
	Svoy bool `json:"svoy,omitempty"`

	// Добавлено волной 3.
	//
	// NebezopasnyyIgnorirovan это пометка «в ссылке стоял insecure=1 либо
	// allowInsecure=1, и мы его НЕ выполнили». Выполнить значит отключить
	// единственную проверку, мешающую выдать себя за наш сервер, то есть
	// превратить любую подписку в примитив для подмены, причём невидимо.
	// Отказывать всей ссылке тоже неверно: сервер при этом совершенно рабочий.
	// Поэтому разбираем, флаг игнорируем, а в списке говорим об этом вслух.
	NebezopasnyyIgnorirovan bool `json:"nebezopasnyy_ignorirovan,omitempty"`

	// OtpechatokNeProveryaetsya: в ссылке стоял отпечаток сертификата целиком
	// (pinSHA256 в hex), а ядро умеет сверять только хеш публичного ключа.
	// Сервер работает с обычной проверкой цепочки, без обещанной ссылкой, и
	// человек видит это в списке (П4 аудита 1.6.1).
	OtpechatokNeProveryaetsya bool `json:"otpechatok_ne_proveryaetsya,omitempty"`

	// Три поля, добавленные после разбора ЖИВОЙ чужой подписки 01.09.2026.
	// Своих фикстур не хватало: ни одна из них их не содержала, а панели
	// ставят их в каждой ссылке.
	//
	// Fp это отпечаток TLS (utls fingerprint, обычно chrome). Без него
	// рукопожатие выглядит не так, как договаривались, и REALITY это замечает.
	// Alpn нужен grpc и http/2. HostZagolovka это заголовок Host для ws:
	// без него фронт вернёт чужую страницу вместо туннеля.
	// BezTLS добавлено 02.09.2026 разбором корпуса чужих ссылок.
	//
	// Ссылки без TLS существуют: в сборнике igareck/vpn-configs-for-russia их
	// шесть из 145, `security=none` и `security=false`, порты 80 и 2200. Модель
	// этого не выражала вовсе, разбор считал `security` локально и хранил только
	// `pbk`. Поэтому генератор для ws и grpc включал TLS БЕЗУСЛОВНО.
	//
	// Это молчаливая версия находки 43: там ядро отвергало конфиг и отказ был
	// виден, здесь `check` проходит, а соединения не будет никогда.
	//
	// Правило то же, что у остальных клиентов: TLS есть только при
	// `security=tls` или `security=reality`. Всё прочее, включая отсутствие
	// параметра, означает открытый транспорт.
	BezTLS bool `json:"bez_tls,omitempty"`

	// Hysteria2, добавлено 03.09.2026 (план «Стабильность и hy2» §4.1).
	// Obfs это тип обфускации (salamander, gecko) с паролем; Porty это
	// диапазоны перебора портов в записи ссылки («20000-21000,443»); Pin это
	// pinSHA256 публичного ключа сертификата в base64. Пин это ПРОВЕРКА, а не
	// её отключение: insecure по-прежнему игнорируется.
	Obfs      string `json:"obfs,omitempty"`
	ObfsParol string `json:"obfs_parol,omitempty"`
	Porty     string `json:"porty,omitempty"`
	Pin       string `json:"pin,omitempty"`
	// SPinom это экранная пометка «сервер закреплён пином»: в файле набора её
	// нет, ставит dlyaEkrana, сам пин при этом остаётся дома.
	SPinom bool `json:"s_pinom,omitempty"`

	// VneAvto это экранная пометка «сервер убран из автовыбора» (A5). В записи
	// сервера её тоже нет: исключения живут списком у набора, потому что записи
	// из подписки пересобираются каждым обходом.
	VneAvto bool `json:"vne_avto,omitempty"`

	// vmess, добавлено 03.09.2026. AlterId это устаревший режим совместимости
	// (в живых панелях почти всегда 0, но встречается), Shifr это способ
	// шифрования полезной нагрузки: auto, none, zero, aes-128-gcm,
	// chacha20-poly1305. Оба поля ядру нужны явно, умолчание «auto» ставит
	// генератор, а не разбор: разбор говорит только то, что было в ссылке.
	AlterId int    `json:"alter_id,omitempty"`
	Shifr   string `json:"shifr,omitempty"`

	Fp            string `json:"fp,omitempty"`
	Alpn          string `json:"alpn,omitempty"`
	HostZagolovka string `json:"host_zagolovka,omitempty"`

	// TUIC, добавлено 05.09.2026. Peregruzka это congestion_control (bbr, cubic,
	// new_reno), RezhimUDP это udp_relay_mode (native либо quic).
	//
	// Оба поля берутся ИЗ ССЫЛКИ и не подставляются нами. Замер 05.09.2026 дал
	// по трём вариантам управления перегрузкой 259-271 Мбит при разбросе до 3%,
	// то есть разница в пределах шума: выдумывать за чужую панель нечего, а
	// молча заменять её выбор своим значит менять протокол под тем же именем.
	Peregruzka string `json:"peregruzka,omitempty"`
	RezhimUDP  string `json:"rezhim_udp,omitempty"`

	// Полоса, объявленная САМОЙ ссылкой hysteria2 (upmbps и downmbps), Мбит.
	// У hysteria2 объявление полосы это единственный переключатель Brutal,
	// отдельного флага нет, поэтому панели пишут эти два параметра прямо в
	// ссылку. Наш собственный сборщик подписки делает то же самое для входа
	// hy2-brutal.
	//
	// Пара или ничего: одно число без второго означало бы Brutal с неизвестной
	// половиной канала.
	PolosaVverh int `json:"polosa_vverh,omitempty"`
	PolosaVniz  int `json:"polosa_vniz,omitempty"`
}
