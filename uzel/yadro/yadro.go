// Пакет yadro держит sing-box библиотекой: поднимает его из последнего
// принятого состояния, принимает новое на лету или перезапуском, откатывается,
// если новое не поднялось.
package yadro

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	sjson "github.com/sagernet/sing/common/json"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/brandmauer"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/podpiska"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/sertip"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/trafik"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/vhody"
)

const (
	imyaSostoyaniya  = "sostoyanie.json"
	imyaPrezhnego    = "sostoyanie.prezhnee.json"
	PovtorPodnyatiya = 30 * time.Second
	// zhdatVypuska: сколько смена ядра ждёт идущую попытку выпуска
	// сертификата. Пульт держит соединение минуту.
	zhdatVypuska = 45 * time.Second
)

// Как меняется ядро в ответ на пакет.
const (
	BezIzmeneniy  = "без изменений"
	NaLetu        = "на лету"
	Perezapusk    = "перезапуск"
	YadroPrezhnee = "ядро прежнее" // поменялись только подписка или сертификат
)

// Сертификаты из пакета ложатся сюда, в каталог состояния, и входы ядра
// ссылаются на эти пути: sert это имя подписки, sert-ip свой на IP сервера.
const (
	katalogSerta   = "sert"
	katalogSertaIP = "sert-ip"
	imyaSerta      = "fullchain.pem"
	imyaKlyucha    = "privkey.pem"
	// katalogVypuska: ключ аккаунта Let's Encrypt и учёт попыток
	// самостоятельного выпуска.
	katalogVypuska = "acme"
)

// Kvitanciya это ответ центру на присланный пакет.
type Kvitanciya struct {
	Hesh       string  `json:"hesh"` // что работает сейчас
	Kak        string  `json:"kak,omitempty"`
	Zakryto    int     `json:"zakryto"` // закрыто соединений выбывших
	YadroZhivo bool    `json:"yadro_zhivo"`
	Oshibka    string  `json:"oshibka,omitempty"`
	Zanyat     *Zanyat `json:"zanyat,omitempty"`
}

// Zanyat это порт, который новое ядро не смогло занять, чаще всего потому,
// что его держит чужая программа. Подбору портов не нужно вынимать порт из
// текста ошибки.
type Zanyat struct {
	Port     int    `json:"port"`
	Proto    string `json:"proto"`          // tcp или udp
	Vhod     string `json:"vhod,omitempty"` // тег входа пакета на этом порту
	Prichina string `json:"prichina"`
}

// Zdorove это ответ на вопрос центра «как ты».
type Zdorove struct {
	SingBox    string `json:"sing_box"`
	Hesh       string `json:"hesh"`
	YadroZhivo bool   `json:"yadro_zhivo"`
	ZhivoS     int64  `json:"zhivo_s,omitempty"`
	Primeneno  int64  `json:"primeneno,omitempty"`
	Oshibka    string `json:"oshibka,omitempty"`
	ProbaDo    int64  `json:"proba_do,omitempty"` // идёт проба портов, до этого времени
	Vhody      []Vhod `json:"vhody"`
}

// Brandmauer открывает порты пробного пакета. По умолчанию iptables с
// пометкой PometkaProby, в тестах подставной.
type Brandmauer interface {
	Otkryt(proto string, port int) error
	// Ubrat снимает все правила пробы, в том числе от упавшей программы.
	Ubrat() error
}

// PometkaProby стоит в комментарии временных правил подбора портов.
const PometkaProby = "affory-podbor"

type iptablesProby struct{}

// Otkryt пускает порт и по IPv4, и по IPv6: каким адресом сервера ходит
// клиент, пробе всё равно.
func (iptablesProby) Otkryt(proto string, port int) error {
	return errors.Join(brandmauer.Otkryt(false, proto, port, PometkaProby),
		brandmauer.Otkryt(true, proto, port, PometkaProby))
}

func (iptablesProby) Ubrat() error { return brandmauer.Ubrat(PometkaProby) }

// edinicaSroka это секунда срока пробы; тесты её укорачивают.
var edinicaSroka = time.Second

type idushchayaProba struct {
	hesh   string
	do     time.Time
	tajmer *time.Timer
	// vozvrat: срок пробы вышел, а принятое не вернулось, это повтор
	// возврата. Клиенту проба уже не показывается.
	vozvrat bool
}

type zapushchennoe struct {
	box    *box.Box
	uchet  *vhody.Uchet
	otmena context.CancelFunc
	karkas string
	paket  []byte
	yadro  []byte // конфиг ядра из пакета, байт в байт
	vhody  []Vhod
	s      time.Time
}

func (z *zapushchennoe) zakryt() error {
	err := z.box.Close()
	z.otmena()
	return err
}

type Yadro struct {
	mu         sync.Mutex
	katalog    string
	schyotchik *trafik.Schyotchik
	zhurnal    *slog.Logger
	podpiska   *podpiska.Server
	vypusk     *sertip.Vypusk
	brandmauer Brandmauer

	rabotaet  *zapushchennoe
	paket     []byte // последний принятый, даже если ядро с ним не поднялось
	hesh      string // что работает: принятый пакет или проба
	primeneno time.Time
	oshibka   string
	proba     *idushchayaProba // nil - пробы портов нет

	// Сертификаты входов собирает и пакет (под mu), и выпуск после продления
	// (без mu), поэтому у них свой замок. Порядок: mu, потом sertyMu.
	sertyMu     sync.Mutex
	sertyPaketa vhody.Serty
	sam         bool // свой на IP ведёт выпуск, а не пакет
}

func Novoe(katalog string, schyotchik *trafik.Schyotchik, zhurnal *slog.Logger) *Yadro {
	return &Yadro{katalog: katalog, schyotchik: schyotchik, zhurnal: zhurnal, brandmauer: iptablesProby{}}
}

// PodklyuchitBrandmauer заменяет iptables для пробы портов. Зовётся до
// Zapustit.
func (y *Yadro) PodklyuchitBrandmauer(b Brandmauer) { y.brandmauer = b }

// PodklyuchitPodpisku отдаёт ядру раздачу подписки: она меняется вместе с
// пакетом и поднимается из него же после перезапуска. Зовётся до Zapustit.
func (y *Yadro) PodklyuchitPodpisku(s *podpiska.Server) { y.podpiska = s }

// PodklyuchitVypusk создаёт самостоятельный выпуск сертификата на IP: пакет
// третьей версии его включает, прежние выключают. Зовётся до Zapustit, а
// Rabotat выпуска вызывающий запускает после Zapustit.
func (y *Yadro) PodklyuchitVypusk(o sertip.Opcii) *sertip.Vypusk {
	y.vypusk = sertip.Novyy(filepath.Join(y.katalog, katalogVypuska), filepath.Join(y.katalog, katalogSertaIP),
		y.zhurnal, y.postavitSerty, o)
	return y.vypusk
}

func Hesh(paket []byte) string {
	summa := sha256.Sum256(paket)
	return hex.EncodeToString(summa[:])
}

// sobrat создаёт ядро, не запуская его. Каждое ядро получает свой реестр
// входов и свой учёт соединений.
func (y *Yadro) sobrat(paket []byte) (*zapushchennoe, error) {
	p, err := razobratPaket(paket)
	if err != nil {
		return nil, err
	}
	karkas, err := Karkas(p.Yadro)
	if err != nil {
		return nil, err
	}
	uchet := vhody.NovyyUchet(y.schyotchik)
	ctx, otmena := context.WithCancel(context.Background())
	ctx = kontekstYadra(ctx, uchet)
	opcii, err := sjson.UnmarshalExtendedContext[option.Options](ctx, p.Yadro)
	if err != nil {
		otmena()
		return nil, fmt.Errorf("конфиг ядра не принят: %w", err)
	}
	b, err := box.New(box.Options{Context: ctx, Options: opcii})
	if err != nil {
		otmena()
		return nil, fmt.Errorf("ядро не собралось: %w", err)
	}
	b.Router().AppendTracker(y.schyotchik)
	return &zapushchennoe{box: b, uchet: uchet, otmena: otmena, karkas: karkas, paket: paket, yadro: p.Yadro, vhody: opisatVhody(p.Yadro)}, nil
}

// kontekstYadra это реестры sing-box, где четыре входа заменены нашими.
func kontekstYadra(ctx context.Context, uchet *vhody.Uchet) context.Context {
	reestr := include.InboundRegistry()
	vhody.Zaregistrirovat(reestr, uchet)
	return box.Context(ctx, reestr, include.OutboundRegistry(), include.EndpointRegistry(),
		include.DNSTransportRegistry(), include.ServiceRegistry(), include.CertificateProviderRegistry())
}

func (y *Yadro) podnyat(paket []byte) (*zapushchennoe, error) {
	z, err := y.sobrat(paket)
	if err != nil {
		return nil, err
	}
	if err := z.box.Start(); err != nil {
		if errZ := z.zakryt(); errZ != nil {
			y.zhurnal.Warn("недоподнятое ядро закрылось с ошибкой", "oshibka", errZ)
		}
		return nil, fmt.Errorf("ядро не запустилось: %w", err)
	}
	z.s = time.Now()
	return z, nil
}

// Zapustit поднимает ядро из последнего принятого состояния при старте
// программы. Нет состояния - ядро ждёт пакета от центра. Не поднялось -
// ошибка видна в здоровье, повтор делает Podderzhivat.
//
// Откатываться на ещё более старое состояние нельзя: в нём могут быть люди,
// которых уже отозвали.
func (y *Yadro) Zapustit() error {
	y.mu.Lock()
	defer y.mu.Unlock()
	// Проба портов на диск не ложится, а её правила могли остаться от
	// упавшей программы.
	if err := y.brandmauer.Ubrat(); err != nil {
		y.zhurnal.Warn("правила пробы портов от прошлого запуска не снялись", "oshibka", err)
	}
	paket, err := os.ReadFile(filepath.Join(y.katalog, imyaSostoyaniya))
	if errors.Is(err, os.ErrNotExist) {
		y.zhurnal.Info("принятого состояния нет, жду пакета от центра")
		return nil
	}
	if err != nil {
		return fmt.Errorf("состояние не читается: %w", err)
	}
	y.paket = paket
	return y.podnyatPrinyatoe()
}

// podnyatPrinyatoe поднимает ядро из последнего принятого пакета: при старте
// программы и после пробы портов, которую не сменил окончательный пакет.
func (y *Yadro) podnyatPrinyatoe() error {
	y.hesh = Hesh(y.paket)
	p, err := razobratPaket(y.paket)
	if err != nil {
		y.oshibka = err.Error()
		return err
	}
	// Подписка не ждёт ядра: люди должны получать ссылки, даже пока ядро
	// не встало, иначе клиент решит, что подписки больше нет.
	if err := y.zapisatSerty(p); err != nil {
		y.zhurnal.Error("сертификат из состояния не лёг на диск", "oshibka", err)
	}
	if err := y.nastroitVypusk(p); err != nil {
		y.zhurnal.Error("выпуск своего сертификата не настроился", "oshibka", err)
	}
	y.zapomnitSerty(p)
	y.postavitPodpisku(p)
	y.schyotchik.BezAdresov(p.Versiya >= 3)
	return y.podnyatSohranennoe()
}

func (y *Yadro) podnyatSohranennoe() error {
	z, err := y.podnyat(y.paket)
	if err != nil {
		y.oshibka = err.Error()
		return err
	}
	y.rabotaet, y.oshibka = z, ""
	y.zhurnal.Info("ядро поднято", "hesh", korotko(y.hesh))
	return nil
}

// Podderzhivat повторяет подъём, пока ядро лежит, а состояние есть. Ядро
// может не встать при загрузке из-за того, чего ещё нет: сертификата после
// продления, занятого порта.
func (y *Yadro) Podderzhivat(ctx context.Context) {
	t := time.NewTicker(PovtorPodnyatiya)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		y.mu.Lock()
		if y.rabotaet == nil && y.paket != nil {
			osvobodit, err := y.zanyatVypusk()
			if err == nil {
				err = y.podnyatSohranennoe()
				osvobodit()
			}
			if err != nil {
				y.zhurnal.Error("ядро всё ещё не поднимается", "oshibka", err)
			}
		}
		y.mu.Unlock()
	}
}

// Primenit принимает пакет от центра.
func (y *Yadro) Primenit(paket []byte) Kvitanciya {
	y.mu.Lock()
	defer y.mu.Unlock()
	kak, zakryto, err := y.primenit(paket)
	k := Kvitanciya{Hesh: y.hesh, Kak: kak, Zakryto: zakryto, YadroZhivo: y.rabotaet != nil}
	if err != nil {
		k.Oshibka = err.Error()
		k.Zanyat = zanyatIz(err, paket)
		y.zhurnal.Error("пакет не применён", "oshibka", err)
	} else {
		y.zhurnal.Info("пакет применён", "kak", kak, "zakryto", zakryto, "hesh", korotko(y.hesh))
	}
	return k
}

func (y *Yadro) primenit(paket []byte) (string, int, error) {
	hesh := Hesh(paket)
	if hesh == y.hesh && y.rabotaet != nil && (y.proba == nil || !y.proba.vozvrat) {
		y.prodlitProbu(paket)
		return BezIzmeneniy, 0, nil
	}
	p, err := razobratPaket(paket)
	if err != nil {
		return "", 0, err
	}
	karkas, err := Karkas(p.Yadro)
	if err != nil {
		return "", 0, err
	}
	if p.Proba != nil {
		// Порты кандидатов открываются до ядра: проба должна быть видна
		// снаружи с первой секунды.
		if err := y.otkrytPorty(p.Yadro); err != nil {
			y.vernutPorty()
			return "", 0, err
		}
	}
	kak, zakryto, err := y.smenit(p, paket, karkas)
	if err != nil {
		y.brositProbu()
		if p.Proba != nil {
			y.vernutPorty()
		}
		return "", 0, err
	}
	y.schyotchik.BezAdresov(p.Versiya >= 3)
	if p.Proba != nil {
		y.hesh, y.oshibka = hesh, ""
		y.zapustitProbu(hesh, p.Proba.Srok)
		return kak, zakryto, nil
	}
	y.zavershitProbu()
	return kak, zakryto, y.prinyat(paket, hesh)
}

// smenit ставит сертификаты и ядро пакета: на лету, если поменялись только
// люди, иначе перезапуском.
func (y *Yadro) smenit(p Paket, paket []byte, karkas string) (string, int, error) {
	osvobodit, err := y.zanyatVypusk()
	if err != nil {
		return "", 0, err
	}
	defer osvobodit()
	// Сертификаты ложатся до ядра: новый конфиг может на них ссылаться.
	// Входы берут их из пакета на каждом рукопожатии (vhody.PostavitSerty),
	// файлы нужны подъёму ядра и присмотру.
	if err := y.zapisatSerty(p); err != nil {
		return "", 0, err
	}
	if err := y.nastroitVypusk(p); err != nil {
		return "", 0, err
	}
	y.zapomnitSerty(p)

	kak, zakryto := "", 0
	switch {
	case y.rabotaet != nil && bytes.Equal(p.Yadro, y.rabotaet.yadro):
		kak = YadroPrezhnee
	case y.rabotaet != nil && karkas == y.rabotaet.karkas:
		n, err := y.naLetu(p.Yadro)
		if err == nil {
			kak, zakryto = NaLetu, n
			y.rabotaet.yadro = p.Yadro
			y.rabotaet.vhody = opisatVhody(p.Yadro)
			break
		}
		// Проверка состава прошла до правок, так что сюда попадает только
		// чужая неожиданность. Перезапуск приведёт ядро к пакету целиком.
		y.zhurnal.Warn("смена на лету не прошла, перезапускаю ядро", "oshibka", err)
	}
	if kak == "" {
		if err := y.perezapustit(paket); err != nil {
			return "", 0, err
		}
		kak = Perezapusk
	}
	y.rabotaet.paket = paket
	y.postavitPodpisku(p)
	return kak, zakryto, nil
}

// otkrytPorty открывает в брандмауэре порты входов пробы. Правила прежней
// пробы снимаются: открыто ровно то, что слушает ядро.
func (y *Yadro) otkrytPorty(yadroKonfig []byte) error {
	if err := y.brandmauer.Ubrat(); err != nil {
		return fmt.Errorf("правила прежней пробы не снялись: %w", err)
	}
	for _, v := range opisatVhody(yadroKonfig) {
		if v.Port <= 0 {
			continue
		}
		if err := y.brandmauer.Otkryt(protoVhoda(v.Tip), int(v.Port)); err != nil {
			return fmt.Errorf("порт %d/%s пробы не открылся в брандмауэре: %w", v.Port, protoVhoda(v.Tip), err)
		}
	}
	return nil
}

// vernutPorty приводит брандмауэр к тому, что работает: проба так и осталась
// прежней или её нет.
func (y *Yadro) vernutPorty() {
	if y.proba != nil && y.rabotaet != nil {
		if err := y.otkrytPorty(y.rabotaet.yadro); err != nil {
			y.zhurnal.Error("порты идущей пробы не открылись заново", "oshibka", err)
		}
		return
	}
	if err := y.brandmauer.Ubrat(); err != nil {
		y.zhurnal.Error("правила несостоявшейся пробы не снялись", "oshibka", err)
	}
}

// zapustitProbu заводит срок пробы заново.
func (y *Yadro) zapustitProbu(hesh string, srok int) {
	y.zavestiSrok(hesh, time.Duration(srok)*edinicaSroka)
}

func (y *Yadro) zavestiSrok(hesh string, dlitelnost time.Duration) {
	if y.proba != nil {
		y.proba.tajmer.Stop()
	}
	pr := &idushchayaProba{hesh: hesh, do: time.Now().Add(dlitelnost)}
	pr.tajmer = time.AfterFunc(dlitelnost, func() { y.istekla(pr) })
	y.proba = pr
}

// brositProbu вызывается, когда ядра нет вовсе (не поднялось ни новое, ни
// прежнее): проба кончилась, здоровье показывает принятое, а поднимает его
// Podderzhivat.
func (y *Yadro) brositProbu() {
	if y.rabotaet != nil || y.proba == nil {
		return
	}
	y.zavershitProbu()
	y.hesh = ""
	if y.paket != nil {
		y.hesh = Hesh(y.paket)
	}
}

// prodlitProbu: тот же пробный пакет ещё раз продлевает пробу на её срок.
func (y *Yadro) prodlitProbu(paket []byte) {
	if y.proba == nil {
		return
	}
	p, err := razobratPaket(paket)
	if err != nil || p.Proba == nil {
		return
	}
	y.zapustitProbu(y.proba.hesh, p.Proba.Srok)
}

// zavershitProbu снимает срок и правила пробы, если она шла.
func (y *Yadro) zavershitProbu() {
	if y.proba == nil {
		return
	}
	y.proba.tajmer.Stop()
	y.proba = nil
	if err := y.brandmauer.Ubrat(); err != nil {
		y.zhurnal.Error("правила пробы портов не снялись", "oshibka", err)
	}
}

// istekla возвращает сервер к принятому пакету, когда окончательный за срок
// пробы не пришёл. Принятого нет - ядро гасится.
func (y *Yadro) istekla(pr *idushchayaProba) {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.proba != pr {
		return // проба уже кончилась или сменилась другой
	}
	y.zavershitProbu()
	y.zhurnal.Warn("проба портов истекла без окончательного пакета, возвращаю принятое")
	if y.paket == nil {
		// Принятого нет: гаснет ядро, а с ним выпуск сертификата и раздача.
		if y.rabotaet != nil {
			if err := y.rabotaet.zakryt(); err != nil {
				y.zhurnal.Warn("ядро пробы закрылось с ошибкой", "oshibka", err)
			}
			y.rabotaet = nil
		}
		if err := y.nastroitVypusk(Paket{}); err != nil {
			y.zhurnal.Warn("выпуск сертификата после пробы не выключился", "oshibka", err)
		}
		y.zapomnitSerty(Paket{})
		y.postavitPodpisku(Paket{})
		y.hesh = ""
		return
	}
	err := y.vernutPrinyatoe()
	switch {
	case err == nil:
	case y.rabotaet == nil:
		// Не поднялось ни принятое, ни проба: здоровье показывает принятое,
		// поднимать его будет Podderzhivat.
		y.hesh = Hesh(y.paket)
		y.zhurnal.Error("ни принятое, ни проба не поднялись, повтор раз в полминуты", "oshibka", err)
	default:
		// Ядро пробы откатом осталось: гасить его, не подняв принятое,
		// значит оставить сервер вовсе без VPN.
		y.zhurnal.Error("принятое не вернулось, повторю через минуту", "oshibka", err)
		y.zavestiSrok(pr.hesh, time.Minute)
		y.proba.vozvrat = true
	}
}

// vernutPrinyatoe меняет ядро пробы на принятое тем же путём, что пакет:
// перезапуск с откатом на пробу, если принятое не встало.
func (y *Yadro) vernutPrinyatoe() error {
	p, err := razobratPaket(y.paket)
	if err != nil {
		return err
	}
	karkas, err := Karkas(p.Yadro)
	if err != nil {
		return err
	}
	if _, _, err := y.smenit(p, y.paket, karkas); err != nil {
		return err
	}
	y.schyotchik.BezAdresov(p.Versiya >= 3)
	y.hesh, y.oshibka = Hesh(y.paket), ""
	return nil
}

// protoVhoda это протокол, который слушает вход: hy2, hysteria и tuic живут
// на UDP, остальные на TCP.
func protoVhoda(tip string) string {
	switch tip {
	case C.TypeHysteria2, C.TypeHysteria, C.TypeTUIC:
		return "udp"
	}
	return "tcp"
}

// zanyatIz достаёт из ошибки запуска ядра порт, который не занялся, и
// находит в пакете вход на нём.
func zanyatIz(err error, paket []byte) *Zanyat {
	var op *net.OpError
	if !errors.As(err, &op) || op.Op != "listen" || op.Err == nil {
		return nil
	}
	z := &Zanyat{Proto: strings.TrimRight(op.Net, "46"), Prichina: op.Err.Error()}
	switch a := op.Addr.(type) {
	case *net.TCPAddr:
		z.Port = a.Port
	case *net.UDPAddr:
		z.Port = a.Port
	default:
		return nil
	}
	p, errP := razobratPaket(paket)
	if errP != nil {
		return z
	}
	for _, v := range opisatVhody(p.Yadro) {
		if int(v.Port) == z.Port && protoVhoda(v.Tip) == z.Proto {
			z.Vhod = v.Teg
			break
		}
	}
	return z
}

// postavitPodpisku отдаёт раздаче подписку из пакета. Пакет версии 1 её
// гасит: значит, центр подписку этому серверу не доверяет.
func (y *Yadro) postavitPodpisku(p Paket) {
	if y.podpiska != nil {
		y.podpiska.Primenit(p.Podpiska, p.tlsSert)
	}
}

func (y *Yadro) zapisatSerty(p Paket) error {
	if err := zapisatSert(y.katalog, katalogSerta, p.Sert); err != nil {
		return err
	}
	if p.SertIPSam != nil {
		// Свой на IP ведёт выпуск, копию из пакета он берёт сам, если она
		// лучше лежащей (nastroitVypusk).
		return nil
	}
	return zapisatSert(y.katalog, katalogSertaIP, p.SertIP)
}

// nastroitVypusk включает самостоятельный выпуск для пакета третьей версии и
// выключает для прежних. Заглушка ложится до ядра: TLS-входы ссылаются на
// файл, а настоящего сертификата может ещё не быть, и без файла не поднялся
// бы и reality.
func (y *Yadro) nastroitVypusk(p Paket) error {
	if y.vypusk == nil {
		if p.SertIPSam != nil {
			return errors.New("пакет просит выпускать свой сертификат, а выпуск в программе не подключён")
		}
		return nil
	}
	if p.SertIPSam == nil {
		return y.vypusk.Nastroit(nil, nil)
	}
	if err := y.vypusk.Nastroit(p.SertIPSam, p.kopiyaSertIP()); err != nil {
		y.zhurnal.Warn("копия своего сертификата из пакета не легла", "oshibka", err)
	}
	return y.vypusk.Zaglushka(p.SertIPSam.IP)
}

// zapomnitSerty запоминает сертификаты пакета и отдаёт их входам.
func (y *Yadro) zapomnitSerty(p Paket) {
	y.sertyMu.Lock()
	y.sertyPaketa, y.sam = *p.serty(), p.SertIPSam != nil
	y.sertyMu.Unlock()
	y.postavitSerty()
}

// postavitSerty отдаёт TLS-входам сертификаты пакета, а свой на IP в третьей
// версии берёт у выпуска. Выпуск зовёт её сам после продления.
func (y *Yadro) postavitSerty() {
	y.sertyMu.Lock()
	defer y.sertyMu.Unlock()
	s := y.sertyPaketa
	if y.sam && y.vypusk != nil {
		s.Svoy = y.vypusk.Tekushchiy()
	}
	vhody.PostavitSerty(&s)
}

// zanyatVypusk не даёт выпуску начать попытку, пока ядро меняется: слушатель
// проверки на 443/tcp помешал бы ядру подняться. Идущую попытку ждёт.
func (y *Yadro) zanyatVypusk() (func(), error) {
	if y.vypusk == nil {
		return func() {}, nil
	}
	ctx, otmena := context.WithTimeout(context.Background(), zhdatVypuska)
	defer otmena()
	return y.vypusk.Zanyat(ctx)
}

// zapisatSert кладёт сертификат из пакета в каталог состояния koren.
// Одинаковый файл не переписывается: ядро следит за файлами и перечитывало
// бы их зря. Пока ключ уже новый, а сертификат ещё старый, ядро может разок
// написать в журнал, что пара не сходится, и останется на прежней.
func zapisatSert(koren, imyaKataloga string, s *podpiska.Sert) error {
	if s == nil {
		return nil
	}
	katalog := filepath.Join(koren, imyaKataloga)
	if err := os.MkdirAll(katalog, 0o700); err != nil {
		return fmt.Errorf("каталог сертификата: %w", err)
	}
	for _, f := range []struct{ imya, soderzhimoe string }{
		{imyaKlyucha, s.Klyuch},
		{imyaSerta, s.Cert},
	} {
		put := filepath.Join(katalog, f.imya)
		if bylo, err := os.ReadFile(put); err == nil && string(bylo) == f.soderzhimoe {
			continue
		}
		if err := zapisatAtomarno(put, []byte(f.soderzhimoe)); err != nil {
			return fmt.Errorf("сертификат не лёг на диск: %w", err)
		}
	}
	return nil
}

// naLetu меняет людей во входах работающего ядра. Сначала проверяет состав
// всех входов, потом меняет: остановка посреди дала бы ядро, где половина
// входов на новом составе, половина на старом.
func (y *Yadro) naLetu(yadro []byte) (int, error) {
	ctx := kontekstYadra(context.Background(), vhody.NovyyUchet(nil))
	opcii, err := sjson.UnmarshalExtendedContext[option.Options](ctx, yadro)
	if err != nil {
		return 0, fmt.Errorf("конфиг ядра не принят: %w", err)
	}
	type smena struct {
		vhod  vhody.Smennyy
		opcii any
	}
	var smeny []smena
	for _, v := range opcii.Inbounds {
		if !vhody.NashiTipy[v.Type] {
			continue
		}
		if err := vhody.Proverit(v.Tag, v.Options); err != nil {
			return 0, err
		}
		najden, est := y.rabotaet.box.Inbound().Get(v.Tag)
		if !est {
			return 0, fmt.Errorf("входа %s нет в работающем ядре", v.Tag)
		}
		sm, est := najden.(vhody.Smennyy)
		if !est {
			return 0, fmt.Errorf("вход %s в работающем ядре не наш (%T)", v.Tag, najden)
		}
		smeny = append(smeny, smena{sm, v.Options})
	}
	zakryto := 0
	for _, s := range smeny {
		n, err := s.vhod.Smenit(s.opcii)
		zakryto += n
		if err != nil {
			return zakryto, err
		}
	}
	return zakryto, nil
}

// perezapustit меняет ядро целиком. Новое сперва собирается без запуска:
// конфиг, который ядро не принимает, не должен гасить работающее.
func (y *Yadro) perezapustit(paket []byte) error {
	proba, err := y.sobrat(paket)
	if err != nil {
		return err
	}
	if err := proba.zakryt(); err != nil {
		y.zhurnal.Warn("проверочное ядро закрылось с ошибкой", "oshibka", err)
	}

	staroe := y.rabotaet
	if staroe != nil {
		y.rabotaet = nil
		if err := staroe.zakryt(); err != nil {
			y.zhurnal.Warn("прежнее ядро закрылось с ошибкой", "oshibka", err)
		}
	}
	novoe, err := y.podnyat(paket)
	if err == nil {
		y.rabotaet = novoe
		return nil
	}
	if staroe == nil {
		return err
	}
	vernuli, errV := y.podnyat(staroe.paket)
	if errV != nil {
		return fmt.Errorf("новое ядро не поднялось (%w), прежнее тоже (%v): VPN стоит", err, errV)
	}
	y.rabotaet = vernuli
	return fmt.Errorf("новое ядро не поднялось, работает прежнее: %w", err)
}

// prinyat запоминает пакет как принятый и кладёт на диск. Сбой записи не
// отменяет применения: ядро уже на новом, а после перезагрузки сервер
// поднимется на прежнем, покажет прежний хэш, и центр пришлёт пакет снова.
//
// Прежний пакет откладывается рядом копией, а не переносом: файл состояния
// не должен исчезать ни на миг, иначе сбой между двумя шагами оставил бы
// сервер после перезагрузки вовсе без VPN.
func (y *Yadro) prinyat(paket []byte, hesh string) error {
	if y.paket != nil && Hesh(y.paket) != hesh {
		if err := zapisatAtomarno(filepath.Join(y.katalog, imyaPrezhnego), y.paket); err != nil {
			y.zhurnal.Warn("прежнее состояние не отложилось", "oshibka", err)
		}
	}
	y.paket, y.hesh, y.primeneno, y.oshibka = paket, hesh, time.Now(), ""
	if err := zapisatAtomarno(filepath.Join(y.katalog, imyaSostoyaniya), paket); err != nil {
		return fmt.Errorf("применено, но на диск не легло: %w", err)
	}
	return nil
}

// Proverit собирает ядро из пакета, не запуская: тот же разбор и те же
// входы, что при применении, только без портов. Сертификаты пакета ложатся
// во временный каталог, и пути конфига ядра в каталоги сертификатов
// состояния подменяются на него: на чистой машине файлов ещё нет, их кладёт
// только применение.
func (y *Yadro) Proverit(paket []byte) error {
	p, err := razobratPaket(paket)
	if err != nil {
		return err
	}
	vremennyy, err := os.MkdirTemp("", "vpn-uzel-proverka-")
	if err != nil {
		return fmt.Errorf("временный каталог проверки: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(vremennyy); err != nil {
			y.zhurnal.Warn("временный каталог проверки не убрался", "put", vremennyy, "oshibka", err)
		}
	}()
	if err := zapisatSert(vremennyy, katalogSerta, p.Sert); err != nil {
		return err
	}
	if p.SertIPSam != nil && p.SertIP == nil {
		// Своего ещё может не быть: проверка берёт заглушку, как подъём.
		v := sertip.Novyy(filepath.Join(vremennyy, katalogVypuska), filepath.Join(vremennyy, katalogSertaIP),
			y.zhurnal, nil, sertip.Opcii{})
		if err := v.Zaglushka(p.SertIPSam.IP); err != nil {
			return fmt.Errorf("заглушка для проверки: %w", err)
		}
	} else if err := zapisatSert(vremennyy, katalogSertaIP, p.SertIP); err != nil {
		return err
	}
	if p.Yadro, err = podmenitPuti(p.Yadro, y.katalog, vremennyy); err != nil {
		return err
	}
	podmenyonnyy, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("пакет для проверки не собрался: %w", err)
	}
	z, err := y.sobrat(podmenyonnyy)
	if err != nil {
		return err
	}
	return z.zakryt()
}

// podmenitPuti меняет в конфиге ядра пути в каталоги сертификатов katalog на
// те же каталоги в drugoy.
func podmenitPuti(yadroKonfig []byte, katalog, drugoy string) (json.RawMessage, error) {
	var konfig any
	dec := json.NewDecoder(bytes.NewReader(yadroKonfig))
	dec.UseNumber()
	if err := dec.Decode(&konfig); err != nil {
		return nil, fmt.Errorf("конфиг ядра не разбирается: %w", err)
	}
	razdelitel := string(filepath.Separator)
	zamenit := func(s string) string {
		for _, k := range []string{katalogSerta, katalogSertaIP} {
			if bylo := filepath.Join(katalog, k) + razdelitel; strings.HasPrefix(s, bylo) {
				return filepath.Join(drugoy, k) + razdelitel + strings.TrimPrefix(s, bylo)
			}
		}
		return s
	}
	return json.Marshal(zamenitStroki(konfig, zamenit))
}

func zamenitStroki(x any, f func(string) string) any {
	switch v := x.(type) {
	case string:
		return f(v)
	case []any:
		for i := range v {
			v[i] = zamenitStroki(v[i], f)
		}
	case map[string]any:
		for k, e := range v {
			v[k] = zamenitStroki(e, f)
		}
	}
	return x
}

// Zdorove отдаёт состояние ядра.
func (y *Yadro) Zdorove() Zdorove {
	y.mu.Lock()
	defer y.mu.Unlock()
	z := Zdorove{SingBox: C.Version, Hesh: y.hesh, Oshibka: y.oshibka}
	if !y.primeneno.IsZero() {
		z.Primeneno = y.primeneno.Unix()
	}
	if y.proba != nil && !y.proba.vozvrat {
		z.ProbaDo = y.proba.do.Unix()
	}
	if y.rabotaet == nil {
		return z
	}
	z.YadroZhivo = true
	z.ZhivoS = y.rabotaet.s.Unix()
	zhivyh := y.rabotaet.uchet.Zhivyh()
	for _, v := range y.rabotaet.vhody {
		v.Soedineniy = zhivyh[v.Teg]
		z.Vhody = append(z.Vhody, v)
	}
	return z
}

// Ostanovit гасит ядро при выходе программы.
func (y *Yadro) Ostanovit() {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.zavershitProbu()
	if y.rabotaet == nil {
		return
	}
	if err := y.rabotaet.zakryt(); err != nil {
		y.zhurnal.Warn("ядро закрылось с ошибкой", "oshibka", err)
	}
	y.rabotaet = nil
}

func zapisatAtomarno(put string, dannye []byte) error {
	vremennyy := put + ".novyy"
	f, err := os.OpenFile(vremennyy, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(dannye)
	if err == nil {
		err = f.Sync()
	}
	if errZ := f.Close(); err == nil {
		err = errZ
	}
	if err != nil {
		_ = os.Remove(vremennyy) // недописанный файл не нужен, ошибка удаления ничего не меняет
		return err
	}
	return os.Rename(vremennyy, put)
}

// korotko это начало хэша для журнала программы.
func korotko(hesh string) string {
	if len(hesh) > 12 {
		return hesh[:12]
	}
	return hesh
}
