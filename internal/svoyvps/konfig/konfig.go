// Package konfig собирает входы своего VPN из одних секретов: пакет для
// программы сервера vpn-uzel и ключи клиента, reality и hy2 (решение
// владельца 01.11: остальные транспорты не добавляют ни скорости, ни
// устойчивости).
//
// Сборка повторяет центр подписок, но на два входа и одного человека вместо
// образца, порты приходят из подбора, а сертификат на IP программа сервера
// выпускает сама (sert_ip_sam). Пробный
// пакет подбора портов собирается тем же кодом с теми же секретами, что уйдут
// в ключи: проверка кандидата это проверка будущего ключа.
package konfig

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

const (
	// versiyaPaketa: третья версия это свой VPN без центра (vpn-uzel 0.6.0).
	versiyaPaketa = 3
	// chelovek это имя единственного человека во входах: по нему программа
	// сервера ведёт учёт трафика.
	chelovek = "vladelec"
	// katalogSertaIP это каталог своего сертификата на сервере: туда его
	// кладёт выпуск программы, а до первого выпуска лежит заглушка.
	katalogSertaIP = "/var/lib/vpn-uzel/sert-ip/"
	flowVision     = "xtls-rprx-vision"
	// fpTelefona: ссылки уходят и на телефон, с edge работает Happ
	// (проверено 31.10). Своё ядро клиента ставит отпечаток само.
	fpTelefona = "edge"
	obfsHy2    = "salamander"

	// PortSayta это порт сайта режима домена на петле сервера, цель reality.
	// Входы слушают «::», то есть и петлю, поэтому reality и hy2 на нём не
	// ставятся.
	PortSayta = 10443
	// obnovlyatPodpisku: раз во сколько часов Happ забирает подписку сам.
	obnovlyatPodpisku = "12"
)

// Пределы срока пробного пакета: те же, что у программы сервера (ProbaMin и
// ProbaMax в uzel/yadro), иначе она отвергнет пакет.
const (
	probaMin = 10
	probaMax = 3600
)

var errNegodno = errors.New("входы своего VPN не собраны")

// Sekrety это секреты одной установки. Живут у службы с первой сборки до
// сноса: пробный пакет, окончательный и ключи берут их отсюда.
type Sekrety struct {
	Uuid        string `json:"uuid"`         // vless
	RealityZakr string `json:"reality_zakr"` // закрытый ключ reality, base64url
	RealityOtkr string `json:"reality_otkr"` // открытый, он же pbk в ссылке
	ShortId     string `json:"short_id"`
	Parol       string `json:"parol"`      // hy2
	ObfsParol   string `json:"obfs_parol"` // salamander
	Maska       string `json:"maska"`      // имя, под чей сайт reality прячется
}

// NovyeSekrety заводит секреты под маску maska.
func NovyeSekrety(maska string) (Sekrety, error) {
	s := Sekrety{Maska: maska}
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Sekrety{}, fmt.Errorf("ключ reality: %w", err)
	}
	s.RealityZakr = base64.RawURLEncoding.EncodeToString(k.Bytes())
	s.RealityOtkr = base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
	b, err := sluchaynye(16)
	if err != nil {
		return Sekrety{}, err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	s.Uuid = h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	if b, err = sluchaynye(4); err != nil {
		return Sekrety{}, err
	}
	s.ShortId = hex.EncodeToString(b)
	if b, err = sluchaynye(18); err != nil {
		return Sekrety{}, err
	}
	s.Parol = base64.RawURLEncoding.EncodeToString(b)
	if b, err = sluchaynye(18); err != nil {
		return Sekrety{}, err
	}
	s.ObfsParol = base64.RawURLEncoding.EncodeToString(b)
	return s, s.Proverit()
}

func sluchaynye(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("случайные байты: %w", err)
	}
	return b, nil
}

var (
	reUuid    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reShortId = regexp.MustCompile(`^([0-9a-f]{2}){1,8}$`)
	// Пароли свои, base64url: в ссылке они идут в userinfo и в параметр, и
	// знаки вне этого набора чужой клиент вправе прочитать по-своему.
	reParol = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	// Маска это имя сайта: буквы, цифры, дефис, хотя бы одна точка; зона из
	// букв или в punycode (.рф это xn--p1ai). Своим доменом маска бывает и
	// в зоне .рф.
	reMaska = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+([a-z]{2,63}|xn--[a-z0-9-]{1,59})$`)
	reHex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// ProveritMasku отвергает маску, которую не примут reality и ссылки.
func ProveritMasku(m string) error {
	if len(m) > 253 || !reMaska.MatchString(m) {
		return fmt.Errorf("%w: маска %q", errNegodno, m)
	}
	return nil
}

// Proverit отвергает секреты, с которыми вход не поднимется или ссылка
// разъедется с сервером. Секреты приходят из хранилища службы, а не только
// из NovyeSekrety.
func (s Sekrety) Proverit() error {
	switch {
	case !reUuid.MatchString(s.Uuid):
		return fmt.Errorf("%w: uuid %q", errNegodno, s.Uuid)
	case !reShortId.MatchString(s.ShortId):
		return fmt.Errorf("%w: short_id %q", errNegodno, s.ShortId)
	case !reParol.MatchString(s.Parol):
		return fmt.Errorf("%w: пароль hy2", errNegodno)
	case !reParol.MatchString(s.ObfsParol):
		return fmt.Errorf("%w: пароль obfs", errNegodno)
	}
	if err := ProveritMasku(s.Maska); err != nil {
		return err
	}
	zakr, err := base64.RawURLEncoding.DecodeString(s.RealityZakr)
	if err != nil {
		return fmt.Errorf("%w: закрытый ключ reality: %v", errNegodno, err)
	}
	k, err := ecdh.X25519().NewPrivateKey(zakr)
	if err != nil {
		return fmt.Errorf("%w: закрытый ключ reality: %v", errNegodno, err)
	}
	// Открытый в ссылке обязан быть парой закрытому на сервере, иначе ключ
	// мёртв при живом сервере, и детектор искал бы блокировку.
	if base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()) != s.RealityOtkr {
		return fmt.Errorf("%w: открытый ключ reality не пара закрытому", errNegodno)
	}
	return nil
}

// Vhody это где сервер и на каких портах его входы.
type Vhody struct {
	Imya  string // имя сервера в списке клиента
	Adres string // IP сервера: сертификат и ссылки на адрес, без имени
	// Reality это TCP-порты reality, Hy2 - UDP-порты hy2. В окончательном
	// пакете не больше одного каждого, в пробном все кандидаты. Пустой Hy2
	// значит, что сертификат не выпустить (закрыты 80 и 443/tcp), и ставится
	// один reality.
	Reality []int
	Hy2     []int
	// Sayt это режим своего домена; nil - простой режим.
	Sayt *Sayt
}

// Sayt это режим своего домена (ДМ, ПД): маска reality это сам домен, reality
// ведёт чужих на сайт-заглушку программы сервера, сертификат hy2 выпущен на
// домен, ключи едут подпиской с того же сайта.
type Sayt struct {
	Domen string // имя в ASCII (punycode), A-запись ведёт на Adres
	Put   string // секретный путь подписки, 32 знака hex
	Zerno string // вид сайта-заглушки, 32 знака hex
	// Routing это ссылка профиля Happ для заголовка routing подписки; пусто
	// - подписка без правил.
	Routing string
}

// NovyySayt заводит режим домена со своим путём подписки и видом заглушки.
func NovyySayt(domen, routing string) (*Sayt, error) {
	put, err := sluchaynye(16)
	if err != nil {
		return nil, err
	}
	zerno, err := sluchaynye(16)
	if err != nil {
		return nil, err
	}
	s := &Sayt{Domen: domen, Put: hex.EncodeToString(put), Zerno: hex.EncodeToString(zerno), Routing: routing}
	return s, s.proverit()
}

// NovyyPut это тот же режим домена с новым путём подписки: прежняя ссылка
// перестаёт отвечать.
func (s Sayt) NovyyPut() (*Sayt, error) {
	put, err := sluchaynye(16)
	if err != nil {
		return nil, err
	}
	s.Put = hex.EncodeToString(put)
	return &s, s.proverit()
}

func (s Sayt) proverit() error {
	switch {
	case ProveritMasku(s.Domen) != nil:
		return fmt.Errorf("%w: домен %q", errNegodno, s.Domen)
	case !reHex32.MatchString(s.Put):
		return fmt.Errorf("%w: путь подписки", errNegodno)
	case !reHex32.MatchString(s.Zerno):
		return fmt.Errorf("%w: зерно сайта", errNegodno)
	case strings.ContainsAny(s.Routing, "\r\n"):
		return fmt.Errorf("%w: перевод строки в правилах Happ", errNegodno)
	}
	return nil
}

// PutPodpiski достаёт секретный путь из ссылки подписки SsylkaPodpiski;
// пусто - ссылки нет или путь в ней не наш.
func PutPodpiski(ssylka string) string {
	u, err := url.Parse(ssylka)
	if err != nil {
		return ""
	}
	if put := strings.TrimPrefix(u.Path, "/"); reHex32.MatchString(put) {
		return put
	}
	return ""
}

// SsylkaPodpiski это адрес подписки: сайт и секретный путь. Пусто - режим
// простой.
func (v Vhody) SsylkaPodpiski() string {
	if v.Sayt == nil || len(v.Reality) == 0 {
		return ""
	}
	return v.adresSayta() + "/" + v.Sayt.Put
}

// adresSayta это сайт режима домена снаружи: домен и порт reality, если он
// не 443.
func (v Vhody) adresSayta() string {
	host := v.Sayt.Domen
	if v.Reality[0] != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(v.Reality[0]))
	}
	return "https://" + host
}

func (v Vhody) proverit(proba bool) (netip.Addr, error) {
	if v.Imya == "" {
		return netip.Addr{}, fmt.Errorf("%w: нет имени сервера", errNegodno)
	}
	a, err := publichnyyAdres(v.Adres)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(v.Reality)+len(v.Hy2) == 0 {
		return netip.Addr{}, fmt.Errorf("%w: нет ни одного порта", errNegodno)
	}
	if v.Sayt != nil {
		if err := v.Sayt.proverit(); err != nil {
			return netip.Addr{}, err
		}
		// Сайт и подписка живут за reality: без него домену нечего отдавать.
		if len(v.Reality) == 0 {
			return netip.Addr{}, fmt.Errorf("%w: режим домена без reality", errNegodno)
		}
		for _, p := range []int{PortSayta, 80} {
			if slices.Contains(v.Reality, p) {
				return netip.Addr{}, fmt.Errorf("%w: порт %d/tcp занят сайтом режима домена", errNegodno, p)
			}
		}
	}
	if !proba && (len(v.Reality) > 1 || len(v.Hy2) > 1) {
		return netip.Addr{}, fmt.Errorf("%w: в окончательном пакете по одному порту на ключ", errNegodno)
	}
	for _, porty := range [][]int{v.Reality, v.Hy2} {
		for i, p := range porty {
			if p < 1 || p > 65535 {
				return netip.Addr{}, fmt.Errorf("%w: порт %d", errNegodno, p)
			}
			if slices.Contains(porty[:i], p) {
				return netip.Addr{}, fmt.Errorf("%w: порт %d дважды", errNegodno, p)
			}
		}
	}
	return a, nil
}

// publichnyyAdres разбирает адрес сервера. Сертификат на IP выпускается
// только на публичный адрес, и ссылку на петлю разбор клиента отвергает.
func publichnyyAdres(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("%w: адрес %q не IP", errNegodno, s)
	}
	if a = a.Unmap(); !a.IsGlobalUnicast() || a.IsPrivate() {
		return netip.Addr{}, fmt.Errorf("%w: адрес %s не публичный", errNegodno, a)
	}
	return a, nil
}

// Sert это сертификат с ключом в PEM, как он едет в пакете.
type Sert struct {
	Cert   string `json:"cert"`
	Klyuch string `json:"klyuch"`
}

// Opcii это то, чем пакеты одной установки отличаются друг от друга.
type Opcii struct {
	// Staging: сертификат от тестового каталога Let's Encrypt (прогоны
	// приёмки, настоящий лимитирован пятью в неделю на адрес).
	Staging bool
	// Kopiya это сертификат прошлой установки на тот же адрес: программа
	// сервера ставит её вместо нового выпуска, пока срок позволяет.
	Kopiya *Sert
	// Proba это срок пробного пакета подбора портов в секундах, 0 значит
	// окончательный.
	Proba int
	// Vypusk: выпуск сертификата и без hy2. Подбор заново на сервере, где
	// сертификат при установке не вышел, пробует его снова, не трогая входы.
	Vypusk bool
}

type paket struct {
	Versiya   int         `json:"versiya"`
	Yadro     any         `json:"yadro"`
	SertIP    *Sert       `json:"sert_ip,omitempty"`
	SertIPSam *sertIPSam  `json:"sert_ip_sam,omitempty"`
	Proba     *proba      `json:"proba,omitempty"`
	Sayt      *saytPaketa `json:"sayt,omitempty"`
}

type sertIPSam struct {
	IP      string `json:"ip"`
	Domen   string `json:"domen,omitempty"`
	Staging bool   `json:"staging,omitempty"`
}

type proba struct {
	Srok int `json:"srok"`
}

// saytPaketa это поле sayt пакета (uzel/sayt): сайт на петле и подписка.
type saytPaketa struct {
	Domen    string         `json:"domen"`
	Port     int            `json:"port"`
	Slushat  string         `json:"slushat"`
	Zerno    string         `json:"zerno"`
	Podpiska podpiskaPaketa `json:"podpiska"`
}

type podpiskaPaketa struct {
	Put       string            `json:"put"`
	Zagolovki map[string]string `json:"zagolovki,omitempty"`
	Telo      string            `json:"telo"`
}

// Paket собирает пакет программы сервера.
func Paket(s Sekrety, v Vhody, o Opcii) ([]byte, error) {
	sertNa := ""
	if v.Sayt != nil {
		sertNa = v.Sayt.Domen
	}
	return sobratPaket(s, v, o, sertNa)
}

// PaketPeredSmenoy это пакет нынешних входов простого режима, в котором
// сертификат уже выпускается на домен: первый шаг перехода в режим домена.
// Ядро прежнее, VPN работает, пока выпуск ждёт Let's Encrypt, а сайт и
// входы на домене встают следующим пакетом, когда сертификат уже есть.
func PaketPeredSmenoy(s Sekrety, v Vhody, domen string, o Opcii) ([]byte, error) {
	switch {
	case v.Sayt != nil:
		return nil, fmt.Errorf("%w: сервер уже в режиме домена", errNegodno)
	case ProveritMasku(domen) != nil:
		return nil, fmt.Errorf("%w: домен %q", errNegodno, domen)
	}
	o.Vypusk = true
	return sobratPaket(s, v, o, domen)
}

func sobratPaket(s Sekrety, v Vhody, o Opcii, sertNa string) ([]byte, error) {
	if err := s.Proverit(); err != nil {
		return nil, err
	}
	if o.Proba != 0 && (o.Proba < probaMin || o.Proba > probaMax) {
		return nil, fmt.Errorf("%w: срок пробы %d с, можно от %d до %d", errNegodno, o.Proba, probaMin, probaMax)
	}
	adres, err := v.proverit(o.Proba != 0)
	if err != nil {
		return nil, err
	}
	if v.Sayt != nil && s.Maska != v.Sayt.Domen {
		return nil, fmt.Errorf("%w: в режиме домена маска %q не домен %q", errNegodno, s.Maska, v.Sayt.Domen)
	}
	p := paket{Versiya: versiyaPaketa, Yadro: yadro(s, v, adres.String())}
	// Сертификат нужен hy2 и сайту режима домена. Без них выпуск только
	// копил бы отказы и тревоги про срок на сервере, где 80 и 443/tcp
	// закрыты, поэтому без них он идёт лишь по прямой просьбе.
	if len(v.Hy2) > 0 || o.Vypusk || v.Sayt != nil {
		p.SertIPSam = &sertIPSam{IP: adres.String(), Domen: sertNa, Staging: o.Staging}
		// Копия бывает только на IP.
		if sertNa == "" {
			p.SertIP = o.Kopiya
		}
	}
	if o.Proba != 0 {
		p.Proba = &proba{Srok: o.Proba}
	}
	if v.Sayt != nil {
		zag, telo, err := podpiska(s, v)
		if err != nil {
			return nil, err
		}
		p.Sayt = &saytPaketa{Domen: v.Sayt.Domen, Port: v.Reality[0], Slushat: slushatSayta(), Zerno: v.Sayt.Zerno,
			Podpiska: podpiskaPaketa{Put: v.Sayt.Put, Zagolovki: zag, Telo: telo}}
	}
	return json.Marshal(p)
}

func slushatSayta() string { return "127.0.0.1:" + strconv.Itoa(PortSayta) }

// podpiska это ответ сайта по секретному пути: ключи reality и hy2 с именем
// сервера, заголовки Happ. Имя профиля идёт в base64: в заголовке HTTP
// кириллица не едет.
func podpiska(s Sekrety, v Vhody) (map[string]string, string, error) {
	klyuchi, err := Klyuchi(s, v)
	if err != nil {
		return nil, "", err
	}
	ssylki := make([]string, 0, len(klyuchi))
	for _, k := range klyuchi {
		ssylki = append(ssylki, k.Ssylka)
	}
	zag := map[string]string{
		"content-type":            "text/plain; charset=utf-8",
		"profile-title":           "base64:" + base64.StdEncoding.EncodeToString([]byte(v.Imya)),
		"profile-update-interval": obnovlyatPodpisku,
	}
	if v.Sayt.Routing != "" {
		zag["routing"] = v.Sayt.Routing
	}
	return zag, base64.StdEncoding.EncodeToString([]byte(strings.Join(ssylki, "\n"))), nil
}

// PaketSerta это пакет «только сертификат» (§3.8, шаг 7): ядро без входов, и
// программа сервера выпускает сертификат на адрес (или на домен, если он
// задан), пока 80 и 443/tcp ещё свободны. Каким путём он вышел, служба
// узнаёт из sostoyanie и по нему отсеивает кандидатов подбора.
func PaketSerta(adres, domen string, o Opcii) ([]byte, error) {
	if o.Proba != 0 {
		return nil, fmt.Errorf("%w: пакет сертификата не пробный", errNegodno)
	}
	a, err := publichnyyAdres(adres)
	if err != nil {
		return nil, err
	}
	p := paket{
		Versiya: versiyaPaketa, Yadro: yadro(Sekrety{}, Vhody{}, a.String()),
		SertIPSam: &sertIPSam{IP: a.String(), Staging: o.Staging}, SertIP: o.Kopiya,
	}
	if domen != "" {
		if ProveritMasku(domen) != nil {
			return nil, fmt.Errorf("%w: домен %q", errNegodno, domen)
		}
		p.SertIPSam.Domen, p.SertIP = domen, nil
	}
	return json.Marshal(p)
}

// yadro это конфиг sing-box сервера: входы, выход напрямую и маршрут, как у
// серверов центра.
func yadro(s Sekrety, v Vhody, adres string) map[string]any {
	// Простой режим: reality прячется за чужой сайт маски. Режим домена:
	// за свой сайт на петле («сам у себя»), имя в приветствии это домен, и
	// hy2 стоит на сертификате домена.
	cel, celPort, imyaHy2, saytHy2 := s.Maska, 443, adres, "https://"+s.Maska
	if v.Sayt != nil && len(v.Reality) > 0 {
		cel, celPort, imyaHy2, saytHy2 = "127.0.0.1", PortSayta, v.Sayt.Domen, v.adresSayta()
	}
	vhody := make([]any, 0, len(v.Reality)+len(v.Hy2))
	for _, port := range v.Reality {
		vhody = append(vhody, map[string]any{
			"type": "vless", "tag": "reality-" + strconv.Itoa(port), "listen": "::", "listen_port": port,
			"users": []any{map[string]any{"name": chelovek, "uuid": s.Uuid, "flow": flowVision}},
			"tls": map[string]any{"enabled": true, "server_name": s.Maska, "reality": map[string]any{
				"enabled": true, "handshake": map[string]any{"server": cel, "server_port": celPort},
				"private_key": s.RealityZakr, "short_id": []any{s.ShortId}}},
		})
	}
	for _, port := range v.Hy2 {
		vhody = append(vhody, map[string]any{
			"type": "hysteria2", "tag": "hy2-" + strconv.Itoa(port), "listen": "::", "listen_port": port,
			"users":                   []any{map[string]any{"name": chelovek, "password": s.Parol}},
			"ignore_client_bandwidth": true, "bbr_profile": "aggressive",
			"masquerade": map[string]any{"type": "proxy", "url": saytHy2, "rewrite_host": true},
			// Файлы это заглушка до первого выпуска: сам сертификат программа
			// отдаёт входу на каждом рукопожатии.
			"tls": map[string]any{"enabled": true, "server_name": imyaHy2, "alpn": []any{"h3"},
				"certificate_path": katalogSertaIP + "fullchain.pem", "key_path": katalogSertaIP + "privkey.pem"},
			"obfs": map[string]any{"type": obfsHy2, "password": s.ObfsParol},
		})
	}
	return map[string]any{
		"log": map[string]any{"level": "error", "timestamp": true},
		"dns": map[string]any{"servers": []any{map[string]any{"type": "udp", "tag": "obshchiy", "server": "1.1.1.1"}},
			"strategy": "prefer_ipv4"},
		"inbounds":  vhody,
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
		"route": map[string]any{
			"rules": []any{
				map[string]any{"action": "sniff"},
				map[string]any{"protocol": []any{"bittorrent"}, "action": "reject"},
				map[string]any{"action": "resolve", "server": "obshchiy"},
				map[string]any{"ip_is_private": true, "action": "reject"},
			},
			"final": "direct",
		},
	}
}

// vhodPaketa это вход ядра сервера в том виде, в каком его пишет yadro.
type vhodPaketa struct {
	Type       string `json:"type"`
	ListenPort int    `json:"listen_port"`
	Users      []struct {
		Uuid     string `json:"uuid"`
		Password string `json:"password"`
	} `json:"users"`
	Tls struct {
		ServerName string `json:"server_name"`
		Reality    struct {
			PrivateKey string   `json:"private_key"`
			ShortId    []string `json:"short_id"`
		} `json:"reality"`
	} `json:"tls"`
	Obfs struct {
		Password string `json:"password"`
	} `json:"obfs"`
}

// IzPaketa собирает секреты и входы из окончательного пакета, который принят
// программой сервера: захват (§3.9 плана) отдаёт те же ключи без
// перевыпуска, сервер остаётся источником правды. adres это IP, на который
// вошли, imya - имя сервера в списке. Пробный пакет и пакет «только
// сертификат» ключей не дают.
//
// Пароли hy2 живут только во входе hy2. Без него в пакете их нет, и они
// заводятся новыми: входа, который бы их ждал, на сервере всё равно нет.
func IzPaketa(b []byte, adres, imya string) (Sekrety, Vhody, error) {
	var p struct {
		Versiya int `json:"versiya"`
		Yadro   struct {
			Inbounds []vhodPaketa `json:"inbounds"`
		} `json:"yadro"`
		Proba *proba      `json:"proba"`
		Sayt  *saytPaketa `json:"sayt"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return Sekrety{}, Vhody{}, fmt.Errorf("%w: пакет сервера не разобран: %v", errNegodno, err)
	}
	switch {
	case p.Versiya != versiyaPaketa:
		return Sekrety{}, Vhody{}, fmt.Errorf("%w: пакет версии %d, а нужна %d", errNegodno, p.Versiya, versiyaPaketa)
	case p.Proba != nil:
		return Sekrety{}, Vhody{}, fmt.Errorf("%w: на сервере пробный пакет, подбор портов не закончен", errNegodno)
	case len(p.Yadro.Inbounds) == 0:
		return Sekrety{}, Vhody{}, fmt.Errorf("%w: в пакете сервера нет входов", errNegodno)
	}
	var s Sekrety
	v := Vhody{Imya: imya, Adres: adres}
	for _, in := range p.Yadro.Inbounds {
		if len(in.Users) != 1 {
			return Sekrety{}, Vhody{}, fmt.Errorf("%w: у входа %s %d пользователей", errNegodno, in.Type, len(in.Users))
		}
		switch in.Type {
		case "vless":
			if len(in.Tls.Reality.ShortId) != 1 {
				return Sekrety{}, Vhody{}, fmt.Errorf("%w: у reality %d short_id", errNegodno, len(in.Tls.Reality.ShortId))
			}
			s.Uuid, s.Maska = in.Users[0].Uuid, in.Tls.ServerName
			s.RealityZakr, s.ShortId = in.Tls.Reality.PrivateKey, in.Tls.Reality.ShortId[0]
			v.Reality = append(v.Reality, in.ListenPort)
		case "hysteria2":
			s.Parol, s.ObfsParol = in.Users[0].Password, in.Obfs.Password
			v.Hy2 = append(v.Hy2, in.ListenPort)
		default:
			return Sekrety{}, Vhody{}, fmt.Errorf("%w: незнакомый вход %q", errNegodno, in.Type)
		}
	}
	if len(v.Reality) == 0 {
		return Sekrety{}, Vhody{}, fmt.Errorf("%w: в пакете сервера нет reality", errNegodno)
	}
	if zakr, err := base64.RawURLEncoding.DecodeString(s.RealityZakr); err == nil {
		if k, err := ecdh.X25519().NewPrivateKey(zakr); err == nil {
			s.RealityOtkr = base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
		}
	}
	if len(v.Hy2) == 0 {
		novye, err := NovyeSekrety(s.Maska)
		if err != nil {
			return Sekrety{}, Vhody{}, err
		}
		s.Parol, s.ObfsParol = novye.Parol, novye.ObfsParol
	}
	// Режим домена переживает круг пакет - секреты - пакет: подбор портов и
	// захват собирают пакет заново, и сайт с подпиской не должны пропасть.
	if p.Sayt != nil {
		v.Sayt = &Sayt{Domen: p.Sayt.Domen, Put: p.Sayt.Podpiska.Put, Zerno: p.Sayt.Zerno,
			Routing: p.Sayt.Podpiska.Zagolovki["routing"]}
		if s.Maska != v.Sayt.Domen {
			return Sekrety{}, Vhody{}, fmt.Errorf("%w: маска reality %q не домен сайта %q", errNegodno, s.Maska, v.Sayt.Domen)
		}
	}
	if err := s.Proverit(); err != nil {
		return Sekrety{}, Vhody{}, err
	}
	if _, err := v.proverit(false); err != nil {
		return Sekrety{}, Vhody{}, err
	}
	return s, v, nil
}

// Klyuch это один ключ своего VPN: запись для набора клиента и ссылка для
// телефона и экспорта.
type Klyuch struct {
	Server protokol.Server
	Ssylka string
}

// ImyaKlyucha это имя ключа своего сервера imya вида transport:
// «Мой сервер, reality» и «Мой сервер, hysteria2», а при нескольких портах
// вида (vsego) ещё и порт. Так ключи видны и в списке, и на телефоне, вид
// назван тем же словом, что в окне. Переименование правит ключи им же.
func ImyaKlyucha(imya, transport string, port, vsego int) string {
	vid := "reality"
	if transport == "hy2" {
		vid = "hysteria2"
	}
	if vsego > 1 {
		return imya + ", " + vid + " " + strconv.Itoa(port)
	}
	return imya + ", " + vid
}

// Klyuchi собирает ключи на входы v. Ссылка собирается сборкой клиента и
// разбирается его же разбором: запись в набор кладётся та, что даёт разбор,
// как при вставке ссылки руками, и обязана совпасть с задуманной. Иначе
// ссылка на телефоне и запись на компьютере вели бы на разные входы.
func Klyuchi(s Sekrety, v Vhody) ([]Klyuch, error) {
	if err := s.Proverit(); err != nil {
		return nil, err
	}
	a, err := v.proverit(true)
	if err != nil {
		return nil, err
	}
	adres := a.String()
	// hy2 проверяет сертификат сервера по имени: на IP в простом режиме, на
	// домен в режиме домена. Адрес в ключе IP в обоих: DNS телефону не
	// нужен.
	imyaHy2 := adres
	if v.Sayt != nil {
		imyaHy2 = v.Sayt.Domen
	}
	zadumannye := make([]protokol.Server, 0, len(v.Reality)+len(v.Hy2))
	for _, port := range v.Reality {
		zadumannye = append(zadumannye, protokol.Server{
			Imya: ImyaKlyucha(v.Imya, "reality-tcp", port, len(v.Reality)), Transport: "reality-tcp", Host: adres, Port: port,
			Uuid: s.Uuid, PublicKey: s.RealityOtkr, ShortId: s.ShortId, Flow: flowVision, Sni: s.Maska, Fp: fpTelefona,
		})
	}
	for _, port := range v.Hy2 {
		zadumannye = append(zadumannye, protokol.Server{
			Imya: ImyaKlyucha(v.Imya, "hy2", port, len(v.Hy2)), Transport: "hy2", Host: adres, Port: port,
			Parol: s.Parol, Obfs: obfsHy2, ObfsParol: s.ObfsParol, Sni: imyaHy2,
		})
	}
	klyuchi := make([]Klyuch, 0, len(zadumannye))
	for _, z := range zadumannye {
		z.Id = ssylki.Id(z.Host, z.Port, z.Transport)
		ssylka, err := ssylki.Sobrat(z)
		if err != nil {
			return nil, fmt.Errorf("%w: ссылка %s: %v", errNegodno, z.Imya, err)
		}
		r, err := ssylki.Razobrat(ssylka)
		if err != nil {
			return nil, fmt.Errorf("%w: своя ссылка %s не разбирается: %v", errNegodno, z.Imya, err)
		}
		if r != z {
			return nil, fmt.Errorf("%w: ссылка %s после разбора ведёт не туда", errNegodno, z.Imya)
		}
		// Происхождения в ссылке нет: пометка ставится после сверки, и по ней
		// ядро клиента берёт своему reality отпечаток firefox.
		r.Svoy = true
		klyuchi = append(klyuchi, Klyuch{Server: r, Ssylka: ssylka})
	}
	return klyuchi, nil
}
