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
	"net/netip"
	"regexp"
	"slices"
	"strconv"

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
	// Маска это имя сайта: буквы, цифры, дефис, хотя бы одна точка.
	reMaska = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

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
	case !reMaska.MatchString(s.Maska):
		return fmt.Errorf("%w: маска %q", errNegodno, s.Maska)
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
	Versiya   int        `json:"versiya"`
	Yadro     any        `json:"yadro"`
	SertIP    *Sert      `json:"sert_ip,omitempty"`
	SertIPSam *sertIPSam `json:"sert_ip_sam,omitempty"`
	Proba     *proba     `json:"proba,omitempty"`
}

type sertIPSam struct {
	IP      string `json:"ip"`
	Staging bool   `json:"staging,omitempty"`
}

type proba struct {
	Srok int `json:"srok"`
}

// Paket собирает пакет программы сервера.
func Paket(s Sekrety, v Vhody, o Opcii) ([]byte, error) {
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
	p := paket{Versiya: versiyaPaketa, Yadro: yadro(s, v, adres.String())}
	// Сертификат нужен одному hy2. Без hy2 выпуск только копил бы отказы и
	// тревоги про срок на сервере, где 80 и 443/tcp закрыты, поэтому без hy2
	// он идёт лишь по прямой просьбе.
	if len(v.Hy2) > 0 || o.Vypusk {
		p.SertIPSam = &sertIPSam{IP: adres.String(), Staging: o.Staging}
		p.SertIP = o.Kopiya
	}
	if o.Proba != 0 {
		p.Proba = &proba{Srok: o.Proba}
	}
	return json.Marshal(p)
}

// PaketSerta это пакет «только сертификат» (§3.8, шаг 7): ядро без входов, и
// программа сервера выпускает сертификат на адрес, пока 80 и 443/tcp ещё
// свободны. Каким путём он вышел, служба узнаёт из sostoyanie и по нему
// отсеивает кандидатов подбора.
func PaketSerta(adres string, o Opcii) ([]byte, error) {
	if o.Proba != 0 {
		return nil, fmt.Errorf("%w: пакет сертификата не пробный", errNegodno)
	}
	a, err := publichnyyAdres(adres)
	if err != nil {
		return nil, err
	}
	return json.Marshal(paket{
		Versiya: versiyaPaketa, Yadro: yadro(Sekrety{}, Vhody{}, a.String()),
		SertIPSam: &sertIPSam{IP: a.String(), Staging: o.Staging}, SertIP: o.Kopiya,
	})
}

// yadro это конфиг sing-box сервера: входы, выход напрямую и маршрут, как у
// серверов центра.
func yadro(s Sekrety, v Vhody, adres string) map[string]any {
	vhody := make([]any, 0, len(v.Reality)+len(v.Hy2))
	for _, port := range v.Reality {
		vhody = append(vhody, map[string]any{
			"type": "vless", "tag": "reality-" + strconv.Itoa(port), "listen": "::", "listen_port": port,
			"users": []any{map[string]any{"name": chelovek, "uuid": s.Uuid, "flow": flowVision}},
			"tls": map[string]any{"enabled": true, "server_name": s.Maska, "reality": map[string]any{
				"enabled": true, "handshake": map[string]any{"server": s.Maska, "server_port": 443},
				"private_key": s.RealityZakr, "short_id": []any{s.ShortId}}},
		})
	}
	for _, port := range v.Hy2 {
		vhody = append(vhody, map[string]any{
			"type": "hysteria2", "tag": "hy2-" + strconv.Itoa(port), "listen": "::", "listen_port": port,
			"users":                   []any{map[string]any{"name": chelovek, "password": s.Parol}},
			"ignore_client_bandwidth": true, "bbr_profile": "aggressive",
			"masquerade": map[string]any{"type": "proxy", "url": "https://" + s.Maska, "rewrite_host": true},
			// Файлы это заглушка до первого выпуска: сам сертификат программа
			// отдаёт входу на каждом рукопожатии.
			"tls": map[string]any{"enabled": true, "server_name": adres, "alpn": []any{"h3"},
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
		Proba *proba `json:"proba"`
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
	// «Мой сервер, reality» и «Мой сервер, hysteria2»: так ключи видны и в
	// списке, и на телефоне, вид назван тем же словом, что в окне.
	imya := func(vid string, port, vsego int) string {
		if vsego > 1 {
			return v.Imya + ", " + vid + " " + strconv.Itoa(port)
		}
		return v.Imya + ", " + vid
	}
	zadumannye := make([]protokol.Server, 0, len(v.Reality)+len(v.Hy2))
	for _, port := range v.Reality {
		zadumannye = append(zadumannye, protokol.Server{
			Imya: imya("reality", port, len(v.Reality)), Transport: "reality-tcp", Host: adres, Port: port,
			Uuid: s.Uuid, PublicKey: s.RealityOtkr, ShortId: s.ShortId, Flow: flowVision, Sni: s.Maska, Fp: fpTelefona,
		})
	}
	for _, port := range v.Hy2 {
		zadumannye = append(zadumannye, protokol.Server{
			Imya: imya("hysteria2", port, len(v.Hy2)), Transport: "hy2", Host: adres, Port: port,
			Parol: s.Parol, Obfs: obfsHy2, ObfsParol: s.ObfsParol, Sni: adres,
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
