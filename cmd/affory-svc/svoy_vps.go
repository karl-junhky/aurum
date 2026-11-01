package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Свой VPN (1.11.0): осмотр и установка своего сервера. Порядок шагов живёт в
// internal/svoyvps/upravlenie, здесь задание с ходом, запись о сервере и
// тексты. Каждая команда svoyVps* дольше срока канала, поэтому идёт заданием:
// ответ сразу отдаёт снимок, ход приходит событием svoy-vps-hod и командой
// svoyVpsHod. Окно можно закрыть: задание дорабатывает в службе.

// SvoyServer это запись о своём сервере в наборе (§3.6 плана). Секретов в ней
// нет: ключи VPN лежат в наборе как у всех серверов, ключ управления в своём
// файле.
type SvoyServer struct {
	Id         string `json:"id"`
	Imya       string `json:"imya"`
	Adres      string `json:"adres"`
	PortSsh    int    `json:"port_ssh"`
	Polzovatel string `json:"polzovatel"`
	// KlyuchHosta это тип и тело ключа хоста в base64: по ним следующий вход
	// сверяет, тот ли это сервер.
	KlyuchHostaTip  string `json:"klyuch_hosta_tip"`
	KlyuchHosta     string `json:"klyuch_hosta"`
	ParolVyklyuchen bool   `json:"parol_vyklyuchen,omitempty"`
	Versiya         string `json:"versiya,omitempty"`
	Maska           string `json:"maska"`
	Reality         int    `json:"reality,omitempty"`
	Hy2             int    `json:"hy2,omitempty"`
	PortyRuchnye    bool   `json:"porty_ruchnye,omitempty"`
	// Prodlenie: чем программа сервера продлевает сертификат: 80, 443 или
	// net (сертификата нет, hy2 не стоит).
	Prodlenie string `json:"prodlenie,omitempty"`
	// Servery это id ключей группы в наборе.
	Servery   []string  `json:"servery"`
	Postavlen time.Time `json:"postavlen"`
	// SertDo это конец срока сертификата hy2 (Unix-секунды), как его видела
	// программа сервера при последнем визите.
	SertDo int64 `json:"sert_do,omitempty"`
	// SertTrevoga и SertOtkaz: тревога программы о сертификате и последний
	// отказ продления. SertProveren - когда эти сведения пришли от программы
	// сервера: проверка, где она не ответила, их не обновляет.
	SertTrevoga  bool      `json:"sert_trevoga,omitempty"`
	SertOtkaz    string    `json:"sert_otkaz,omitempty"`
	SertProveren time.Time `json:"sert_proveren,omitzero"`
	// Proveren это последний визит службы: установка, захват или проверка,
	// удачная или нет. ProverkaOtkaz говорит, почему последняя проверка не
	// прошла или прошла без программы сервера и сведений о сертификате не
	// обновила.
	Proveren      time.Time `json:"proveren,omitzero"`
	ProverkaOtkaz string    `json:"proverka_otkaz,omitempty"`
}

// idSvoego выводит id своего сервера из адреса: та же машина после
// переустановки получает тот же id и тот же ключ управления.
func idSvoego(adres netip.Addr) string {
	h := sha256.Sum256([]byte(adres.Unmap().String()))
	return "svoy-" + hex.EncodeToString(h[:8])
}

func prodlenieDlyaZapisi(pr porty.Prodlenie) string {
	switch pr {
	case porty.Cherez80:
		return "80"
	case porty.Cherez443:
		return "443"
	}
	return "net"
}

// Задание.

const (
	vidOsmotr    = "osmotr"
	vidUstanovka = "ustanovka"

	fazaIdyot    = "idyot"
	fazaGotovo   = "gotovo"
	fazaOshibka  = "oshibka"
	fazaOtmeneno = "otmeneno"
)

// OtkazVps это отказ или предупреждение задания: код из §9.1 и текст службы.
type OtkazVps struct {
	Kod   string `json:"kod"`
	Tekst string `json:"tekst"`
	// KlyuchOstalsya: наш ключ root остался на сервере, а войти им нельзя.
	// Окно говорит об этом отдельной строкой.
	KlyuchOstalsya bool `json:"klyuch_ostalsya,omitempty"`
	// Podozhdat: следующий вход не раньше, чем через столько секунд.
	Podozhdat int `json:"podozhdat,omitempty"`
	// Otpechatok это ключ хоста, который предъявил сервер при смене ключа:
	// его человек сверяет до «Я переустановил систему».
	Otpechatok string `json:"otpechatok,omitempty"`
}

// KlyuchHoda это ключ в ходе подбора и проверки.
type KlyuchHoda struct {
	Transport string `json:"transport"`
	Port      int    `json:"port"`
	// Vid пуст, пока ключ проверяется.
	Vid   string `json:"vid,omitempty"`
	Otkaz string `json:"otkaz,omitempty"`
}

// OtsevHoda это кандидат подбора, выпавший до проверки.
type OtsevHoda struct {
	Proto    string `json:"proto"`
	Port     int    `json:"port"`
	Prichina string `json:"prichina"`
	Process  string `json:"process,omitempty"`
}

// ZamechanieVps это пункт осмотра.
type ZamechanieVps struct {
	Kod   string `json:"kod"`
	Stop  bool   `json:"stop"`
	Tekst string `json:"tekst"`
}

// OsmotrVps это шаг «Проверка» мастера.
type OsmotrVps struct {
	Put  string `json:"put"`
	Root bool   `json:"root"`
	// Ключ хоста с осмотра: установка входит, только если сервер предъявит
	// его же.
	KlyuchHostaTip string          `json:"klyuch_hosta_tip"`
	KlyuchHosta    string          `json:"klyuch_hosta"`
	Otpechatok     string          `json:"otpechatok"`
	Sistema        string          `json:"sistema,omitempty"`
	Zamechaniya    []ZamechanieVps `json:"zamechaniya"`
	// MozhnoStavit: ни одно замечание установку не останавливает.
	MozhnoStavit bool `json:"mozhno_stavit"`
}

// UstanovkaVps это итог установки: ключи группы.
type UstanovkaVps struct {
	Server  string       `json:"server"`
	Klyuchi []KlyuchHoda `json:"klyuchi"`
	Ids     []string     `json:"ids"`
}

// HodVps это снимок задания своего сервера. Его отдаёт svoyVpsHod тому, кто
// спросил; событием всем подписчикам уходят только id, вид, фаза и шаг.
type HodVps struct {
	Id   int64  `json:"id"`
	Vid  string `json:"vid"`
	Faza string `json:"faza"`
	Shag string `json:"shag,omitempty"`
	// Server - id своего сервера, который проверяется; Planovaya - проверку
	// начала сама служба, а не человек.
	Server    string `json:"server,omitempty"`
	Planovaya bool   `json:"planovaya,omitempty"`
	// Put: напрямую или через VPN, после входа.
	Put              string        `json:"put,omitempty"`
	Klyuch           *KlyuchHoda   `json:"klyuch,omitempty"`
	Proverki         []KlyuchHoda  `json:"proverki,omitempty"`
	Otseyany         []OtsevHoda   `json:"otseyany,omitempty"`
	Krug             int           `json:"krug,omitempty"`
	Sleduyushchiy    int           `json:"sleduyushchiy,omitempty"`
	Osmotr           *OsmotrVps    `json:"osmotr,omitempty"`
	Ustanovlen       *UstanovkaVps `json:"ustanovlen,omitempty"`
	Snyat            *SnosVps      `json:"snyat,omitempty"`
	Proverka         *ProverkaVps  `json:"proverka,omitempty"`
	Otkaz            *OtkazVps     `json:"otkaz,omitempty"`
	Preduprezhdeniya []OtkazVps    `json:"preduprezhdeniya,omitempty"`
	Nachalo          time.Time     `json:"nachalo"`
	Konec            *time.Time    `json:"konec,omitempty"`
}

const (
	otkazZanyatoVps    = "со своим сервером уже идёт дело: дождись его конца или отмени"
	otkazIdyotProverka = "служба как раз сама проверяет свой сервер: дождись конца проверки или отмени её"
)

type zadanieVps struct {
	snimok HodVps
	otmena context.CancelFunc
	gotovo bool
	// fonovoe: плановая проверка. Итог прошлого дела человека она не
	// заслоняет: по её концу svoyVpsHod снова отдаёт prezhnee.
	fonovoe  bool
	prezhnee *zadanieVps
}

// deloVpsIdyot говорит, какое задание своего сервера идёт; пусто - никакое.
// Под muVps.
func (s *Sluzhba) deloVpsIdyot() string {
	switch {
	case s.vps == nil || s.vps.gotovo:
		return ""
	case s.vps.fonovoe:
		return otkazIdyotProverka
	}
	return otkazZanyatoVps
}

// svoyVpsIdyot: задание своего сервера идёт, текст - какое. Установка
// обновления посреди него опустила бы службу вместе с заданием.
func (s *Sluzhba) svoyVpsIdyot() string {
	s.muVps.Lock()
	defer s.muVps.Unlock()
	return s.deloVpsIdyot()
}

// zaperetVps запирает задания своего сервера до срока do одним взятием
// замка с проверкой: между проверкой и делом новое задание не начнётся.
// otperet снимает только этот запор: истёкший и занятый другим он не
// трогает. Отказ - otperet nil, а второе значение говорит почему.
func (s *Sluzhba) zaperetVps(do time.Time, tekst string) (otperet func(), otkaz string) {
	s.muVps.Lock()
	defer s.muVps.Unlock()
	if tekst := s.deloVpsIdyot(); tekst != "" {
		return nil, tekst
	}
	if time.Now().Before(s.vpsZaporDo) {
		return nil, s.vpsZaporTekst
	}
	s.vpsZaporNomer++
	nomer := s.vpsZaporNomer
	s.vpsZaporDo, s.vpsZaporTekst = do, tekst
	return func() {
		s.muVps.Lock()
		defer s.muVps.Unlock()
		if s.vpsZaporNomer == nomer {
			s.vpsZaporDo, s.vpsZaporTekst = time.Time{}, ""
		}
	}, ""
}

// pravitHod меняет снимок под замком и рассылает событие.
func (s *Sluzhba) pravitHod(z *zadanieVps, fn func(*HodVps)) {
	s.muVps.Lock()
	fn(&z.snimok)
	sobytie := map[string]any{"id": z.snimok.Id, "vid": z.snimok.Vid, "faza": z.snimok.Faza, "shag": z.snimok.Shag,
		"server": z.snimok.Server, "planovaya": z.snimok.Planovaya}
	s.muVps.Unlock()
	s.izvestit("svoy-vps-hod", sobytie)
}

// zapustitVps заводит задание и отдаёт начальный снимок. Одно задание на
// службу: два входа на сервер разом мешали бы друг другу, а заслон SSH-слоя
// посчитал бы их неудачи вместе.
func (s *Sluzhba) zapustitVps(k protokol.Kadr, vid string, delo func(ctx context.Context, z *zadanieVps) error) protokol.Kadr {
	nachalo, tekst := s.nachatVps(vid, false, delo)
	if tekst != "" {
		return otkaz(k.Id, k.Imya, protokol.KodVpsZanyato, tekst)
	}
	return otvet(k.Id, k.Imya, nachalo)
}

// nachatVps заводит задание без кадра: его зовут и команды, и плановая
// проверка (fonovoe). Отказ - непустой текст, почему занято.
func (s *Sluzhba) nachatVps(vid string, fonovoe bool, delo func(ctx context.Context, z *zadanieVps) error) (HodVps, string) {
	s.muVps.Lock()
	if tekst := s.deloVpsIdyot(); tekst != "" {
		s.muVps.Unlock()
		return HodVps{}, tekst
	}
	if time.Now().Before(s.vpsZaporDo) {
		tekst := s.vpsZaporTekst
		s.muVps.Unlock()
		return HodVps{}, tekst
	}
	if !s.zavestiFonovuyu() {
		s.muVps.Unlock()
		return HodVps{}, "служба останавливается"
	}
	ctx, otmena := context.WithCancel(s.fonCtx)
	z := &zadanieVps{snimok: HodVps{Id: time.Now().UnixMilli(), Vid: vid, Faza: fazaIdyot, Nachalo: time.Now()}, otmena: otmena}
	if fonovoe {
		z.fonovoe, z.prezhnee, z.snimok.Planovaya = true, s.vps, true
		// Прошлая плановая могла не успеть вернуть своё прежнее.
		if s.vps != nil && s.vps.fonovoe {
			z.prezhnee = s.vps.prezhnee
		}
	}
	s.vps = z
	nachalo := z.snimok
	s.muVps.Unlock()

	fon.Zapustit("задании своего сервера", func() {
		defer s.fon.Done()
		defer otmena()
		var err error
		if fon.Vypolnit("задании своего сервера", func() { err = delo(ctx, z) }) {
			err = errors.New("задание прервано внутренней ошибкой службы")
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("свой сервер, %s: %v", vid, err)
		}
		// Конец ставится тем же взятием замка, что и последний снимок: окно,
		// получив событие конца, сразу может завести следующее дело.
		s.pravitHod(z, func(h *HodVps) {
			konec := time.Now()
			h.Konec = &konec
			h.Klyuch = nil
			// Дело, дошедшее до конца, готово, даже если отмена пришла после.
			switch {
			case err == nil:
				h.Faza = fazaGotovo
			case ctx.Err() != nil:
				h.Faza = fazaOtmeneno
			default:
				o := otkazVps(err)
				h.Faza, h.Otkaz = fazaOshibka, &o
			}
			z.gotovo = true
		})
		if fonovoe {
			s.muVps.Lock()
			if s.vps == z {
				s.vps = z.prezhnee
			}
			s.muVps.Unlock()
		}
	})
	return nachalo, ""
}

func (s *Sluzhba) svoyVpsHod(k protokol.Kadr) protokol.Kadr {
	s.muVps.Lock()
	defer s.muVps.Unlock()
	if s.vps == nil {
		return otvet(k.Id, k.Imya, map[string]string{"faza": "net"})
	}
	return otvet(k.Id, k.Imya, s.vps.snimok)
}

func (s *Sluzhba) svoyVpsOtmenit(k protokol.Kadr) protokol.Kadr {
	s.muVps.Lock()
	if s.vps != nil && !s.vps.gotovo {
		s.vps.otmena()
	}
	s.muVps.Unlock()
	return s.svoyVpsHod(k)
}

// Вход.

// dostupVps это данные входа от окна. Ключ человека приходит содержимым:
// путь служба SYSTEM не принимает, иначе прочла бы любой файл по просьбе
// процесса сеанса. Пароль живёт только в теле команды и в памяти задания.
type dostupVps struct {
	Adres        string `json:"adres"`
	Port         int    `json:"port"`
	Polzovatel   string `json:"polzovatel"`
	Parol        string `json:"parol,omitempty"`
	Klyuch       []byte `json:"klyuch,omitempty"`
	ParolKlyucha string `json:"parol_klyucha,omitempty"`
}

// razobrat отдаёт адрес и запрос входа. Адрес только IP: на него выпускается
// сертификат и смотрят ключи, а имя хоста могло бы сменить адрес между
// осмотром и установкой.
func (d dostupVps) razobrat() (netip.Addr, ssh.Zapros, error) {
	a, err := netip.ParseAddr(d.Adres)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, ssh.Zapros{}, errors.New("нужен IP-адрес сервера из письма хостера")
	}
	if d.Port < 0 || d.Port > 65535 {
		return netip.Addr{}, ssh.Zapros{}, errors.New("порт SSH вне 1-65535")
	}
	a = a.Unmap()
	return a, ssh.Zapros{Adres: a.String(), Port: d.Port, Vhod: ssh.Vhod{
		Polzovatel: d.Polzovatel, Parol: d.Parol, Klyuch: d.Klyuch, ParolKlyucha: d.ParolKlyucha,
	}}, nil
}

// putiVps это дороги до сервера. Через туннель только тогда, когда ядро на
// входе подняла сама служба: слой SSH не отличает наше ядро от чужого
// прокси, а через чужой пароль и SSH человека ушли бы в чужой выход (§3.5).
func (s *Sluzhba) putiVps() upravlenie.Puti {
	return upravlenie.Puti{
		Pryamo: func() (ssh.Put, error) {
			a, err := s.fizicheskiyAdapter()
			if err != nil {
				return ssh.Put{}, err
			}
			return ssh.PutPryamo(a.Indeks, a.Imya), nil
		},
		Tunnel: func() (ssh.Put, bool) {
			s.mu.Lock()
			ok, port := s.sost == protokol.SostPodnyat && s.portProksiNash > 0, s.portProksiNash
			s.mu.Unlock()
			if !ok {
				return ssh.Put{}, false
			}
			return ssh.PutCherezTunnel(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))), true
		},
	}
}

// Осмотр.

func (s *Sluzhba) svoyVpsOsmotret(k protokol.Kadr) protokol.Kadr {
	var d dostupVps
	if err := json.Unmarshal(k.Telo, &d); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "данные входа не разобраны")
	}
	adres, z, err := d.razobrat()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	}
	return s.zapustitVps(k, vidOsmotr, func(ctx context.Context, zad *zadanieVps) error {
		s.pravitHod(zad, func(h *HodVps) { h.Shag = string(upravlenie.ShagVhod) })
		o, err := s.vpsOsmotret(ctx, s.putiVps(), z)
		if err != nil {
			return err
		}
		// Своя ли отметка установки, решает ключ управления этого компьютера.
		var klyuch gossh.PublicKey
		if k, ok, err := s.klyuchUpravleniya(idSvoego(adres)); err != nil {
			return err
		} else if ok {
			klyuch = k.PublicKey()
		}
		otch := osmotrVps(o, upravlenie.Prepyatstviya(o.Osmotr, klyuch))
		s.pravitHod(zad, func(h *HodVps) { h.Put, h.Osmotr = o.Vhod.Put, &otch })
		return nil
	})
}

// osmotrVps собирает шаг «Проверка» из отчёта: тексты замечаний пишет
// служба, а установку останавливают те, что назвал upravlenie.Prepyatstviya.
// Машину, которая ещё настраивается после создания, установка ждёт сама,
// поэтому кнопку это замечание не запирает.
func osmotrVps(o upravlenie.Otchet, p *upravlenie.Ostanovka) OsmotrVps {
	v := OsmotrVps{
		Put: o.Vhod.Put, Root: o.Vhod.Root,
		KlyuchHostaTip: o.Vhod.KlyuchHosta.Tip, KlyuchHosta: o.Vhod.KlyuchHosta.TeloBase64(),
		Otpechatok: o.Vhod.KlyuchHosta.Otpechatok(), Sistema: ssh.Obrezat(o.Osmotr.OSImya, 60),
		Zamechaniya: []ZamechanieVps{}, MozhnoStavit: true,
	}
	stop := map[server.Kod]bool{}
	if p != nil {
		for _, z := range p.Zamechaniya {
			stop[z.Kod] = true
			if z.Kod != server.KodNastraivaetsya {
				v.MozhnoStavit = false
			}
		}
	}
	for _, z := range o.Zamechaniya {
		z.Stop = stop[z.Kod]
		v.Zamechaniya = append(v.Zamechaniya, ZamechanieVps{Kod: string(z.Kod), Stop: z.Stop, Tekst: tekstZamechaniya(z)})
	}
	return v
}

// Установка.

type zaprosUstanovkiVps struct {
	dostupVps
	// Pustoy это галочка «Сервер пустой».
	Pustoy         bool   `json:"pustoy"`
	KlyuchHostaTip string `json:"klyuch_hosta_tip"`
	KlyuchHosta    string `json:"klyuch_hosta"`
	Imya           string `json:"imya"`
	// Reality и Hy2 это порты из «Дополнительно», 0 - подобрать.
	Reality int `json:"reality,omitempty"`
	Hy2     int `json:"hy2,omitempty"`
}

const imyaSvoegoPoUmolchaniyu = "Мой сервер"

func (s *Sluzhba) svoyVpsUstanovit(k protokol.Kadr) protokol.Kadr {
	var z zaprosUstanovkiVps
	if err := json.Unmarshal(k.Telo, &z); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "данные установки не разобраны")
	}
	adres, zapros, err := z.razobrat()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, err.Error())
	}
	if !z.Pustoy {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "без галочки «Сервер пустой» на машине ничего не меняется")
	}
	kh, err := ssh.KlyuchHostaIzZapisi(z.KlyuchHostaTip, z.KlyuchHosta)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "нет ключа сервера с проверки: проверь сервер ещё раз")
	}
	zapros.Izvesten = &kh
	if z.Imya = obrezatImya(z.Imya); z.Imya == "" {
		z.Imya = imyaSvoegoPoUmolchaniyu
	}
	id := idSvoego(adres)
	n, err := s.nabor()
	if err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	if slices.ContainsFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id }) {
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "этот сервер уже в разделе «Свой VPN»: действия в его карточке")
	}
	return s.zapustitVps(k, vidUstanovka, func(ctx context.Context, zad *zadanieVps) error {
		return s.ustanovitVps(ctx, zad, id, z, zapros)
	})
}

// obrezatImya чистит имя от управляющих и невидимых знаков, разрывов строки, пробелов
// по краям и режет до 40 знаков. Пустое имя заменяет вызывающий.
func obrezatImya(s string) string {
	// Cf это невидимые знаки и переворот направления письма (U+202E).
	r := slices.DeleteFunc([]rune(s), func(c rune) bool {
		return unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == ' ' || c == ' '
	})
	r = []rune(strings.TrimSpace(string(r)))
	if len(r) > 40 {
		r = r[:40]
	}
	return strings.TrimSpace(string(r))
}

func (s *Sluzhba) ustanovitVps(ctx context.Context, zad *zadanieVps, id string, z zaprosUstanovkiVps, zapros ssh.Zapros) error {
	podpis, err := s.zavestiKlyuchUpravleniya(id)
	if err != nil {
		return err
	}
	prog, err := s.programmaUzla(ctx)
	if err != nil {
		return err
	}
	defer prog.Close()
	a, err := s.fizicheskiyAdapter()
	if err != nil {
		return err
	}
	imyaPK, _ := os.Hostname()
	u := upravlenie.Ustanovka{
		Puti: s.putiVps(), Vhod: zapros, Pustoy: z.Pustoy, Imya: z.Imya,
		Upravlenie: podpis, Kommentariy: kommentariyKlyucha(imyaPK, time.Now()),
		Programma: prog, Ruchnye: porty.Ruchnye{Reality: z.Reality, Hy2: z.Hy2},
		Opcii:    konfig.Opcii{Staging: s.vpsStaging},
		Proverka: proverka.Opcii{Adapter: a.Imya},
		Zapusk:   s.zapuskProverki,
		Pryamo:   ssh.PutPryamo(a.Indeks, a.Imya).Dial,
		Hod:      func(h upravlenie.Hod) { s.pravitHod(zad, func(x *HodVps) { primenitHod(x, h) }) },
	}
	itog, err := s.vpsUstanovit(ctx, u)
	if err != nil {
		return err
	}
	return s.zapisatIPokazat(zad, id, osnovaZapisi{Imya: z.Imya, Port: z.Port, Polzovatel: z.Polzovatel,
		PortyRuchnye: z.Reality != 0 || z.Hy2 != 0}, itog)
}

// oshibkaZapisi: ключи на сервере стоят, а набор не записан. Код тот же,
// что у прочих отказов записи набора.
type oshibkaZapisi struct{ err error }

func (o *oshibkaZapisi) Error() string {
	return "ключи своего сервера не записаны: " + o.err.Error()
}
func (o *oshibkaZapisi) Unwrap() error { return o.err }

// zapisatIPokazat кладёт ключи в набор и итог в снимок. Набор записан, а
// правила брандмауэра отстали - это предупреждение, а не отказ: ключи уже
// в списке, и повтор установки их только задвоил бы.
func (s *Sluzhba) zapisatIPokazat(zad *zadanieVps, id string, osn osnovaZapisi, itog upravlenie.Itog) error {
	ids, err := s.zapisatSvoy(id, osn, itog)
	var otstali *OtkazVps
	switch {
	case errors.Is(err, errPravilaOtstali):
		log.Printf("свой сервер %s: %v", id, err)
		otstali = &OtkazVps{Kod: kodSohraneniya(err), Tekst: sboi.DlyaCheloveka(err)}
	case err != nil:
		return &oshibkaZapisi{err}
	}
	klyuchi := make([]KlyuchHoda, len(itog.Klyuchi))
	for i, kl := range itog.Klyuchi {
		klyuchi[i] = KlyuchHoda{Transport: kl.Server.Transport, Port: kl.Server.Port}
		for _, p := range itog.Proverki {
			if p.Server.Id == kl.Server.Id {
				klyuchi[i].Vid, klyuchi[i].Otkaz = string(p.Vid), p.Otkaz
			}
		}
	}
	s.pravitHod(zad, func(h *HodVps) {
		h.Put = itog.Vhod.Put
		h.Ustanovlen = &UstanovkaVps{Server: id, Klyuchi: klyuchi, Ids: ids}
		for _, p := range itog.Preduprezhdeniya {
			h.Preduprezhdeniya = append(h.Preduprezhdeniya, preduprezhdenieVps(p))
		}
		if otstali != nil {
			h.Preduprezhdeniya = append(h.Preduprezhdeniya, *otstali)
		}
	})
	return nil
}

// primenitHod переносит весть установки в снимок.
func primenitHod(x *HodVps, h upravlenie.Hod) {
	x.Shag = string(h.Shag)
	switch {
	case h.Klyuch != nil && h.Klyuch.Vid == "":
		x.Klyuch = &KlyuchHoda{Transport: h.Klyuch.Server.Transport, Port: h.Klyuch.Server.Port}
	case h.Klyuch != nil:
		x.Klyuch = nil
		x.Proverki = append(x.Proverki, KlyuchHoda{Transport: h.Klyuch.Server.Transport, Port: h.Klyuch.Server.Port,
			Vid: string(h.Klyuch.Vid), Otkaz: h.Klyuch.Otkaz})
	case h.Otsev != nil:
		x.Otseyany = append(x.Otseyany, OtsevHoda{Proto: h.Otsev.Proto, Port: h.Otsev.Port,
			Prichina: string(h.Otsev.Prichina), Process: h.Otsev.Process})
	case h.Krug != nil:
		x.Krug, x.Sleduyushchiy = h.Krug.Nomer, int(h.Krug.Sleduyushchiy.Seconds())
	}
}

// osnovaZapisi это то, что запись о сервере берёт от человека, а не от
// сервера.
type osnovaZapisi struct {
	Imya         string
	Port         int
	Polzovatel   string
	PortyRuchnye bool
}

// zapisatSvoy кладёт ключи группы и запись о сервере одной правкой набора
// (Е6): дальше их берёт авто. Та же ссылка, вставленная руками раньше,
// остаётся одной записью и получает пометку своего сервера. Ключи прежней
// записи того же сервера, которых нет среди новых, уходят из набора.
func (s *Sluzhba) zapisatSvoy(id string, osn osnovaZapisi, it upravlenie.Itog) (ids []string, err error) {
	port := osn.Port
	if port == 0 {
		port = 22
	}
	zapis := SvoyServer{
		Id: id, Imya: osn.Imya, Adres: it.Adres, PortSsh: port, Polzovatel: osn.Polzovatel,
		KlyuchHostaTip: it.Vhod.KlyuchHosta.Tip, KlyuchHosta: it.Vhod.KlyuchHosta.TeloBase64(),
		Versiya: it.Versiya, Maska: it.Sekrety.Maska, PortyRuchnye: osn.PortyRuchnye,
		Prodlenie: prodlenieDlyaZapisi(it.Prodlenie), Postavlen: time.Now().UTC(),
	}
	if len(it.Vhody.Reality) > 0 {
		zapis.Reality = it.Vhody.Reality[0]
	}
	if len(it.Vhody.Hy2) > 0 {
		zapis.Hy2 = it.Vhody.Hy2[0]
	}
	if it.Sert != nil {
		if it.Sert.Est {
			zapis.SertDo = it.Sert.Do
		}
		zapis.SertTrevoga, zapis.SertOtkaz = it.Sert.Trevoga, it.Sert.Otkaz
	}
	zapis.Proveren, zapis.SertProveren = zapis.Postavlen, zapis.Postavlen
	err = s.pravitNabor(func(n *Nabor) error {
		ids = nil
		for _, k := range it.Klyuchi {
			var itog protokol.Server
			n.Servery, itog = ssylki.DobavitProfil(n.Servery, k.Server)
			for i := range n.Servery {
				if n.Servery[i].Id == itog.Id {
					n.Servery[i].Svoy = true
				}
			}
			ids = append(ids, itog.Id)
		}
		zapis.Servery = slices.Clone(ids)
		if i := slices.IndexFunc(n.SvoiServery, func(x SvoyServer) bool { return x.Id == id }); i >= 0 {
			ubratServery(n, slices.DeleteFunc(slices.Clone(n.SvoiServery[i].Servery), func(x string) bool {
				return slices.Contains(ids, x)
			}))
			n.SvoiServery = slices.Delete(n.SvoiServery, i, i+1)
		}
		n.SvoiServery = append(n.SvoiServery, zapis)
		return nil
	})
	return ids, err
}

// ubratServery убирает ключи ids из набора так же, как removeServer: выбор,
// стоявший на одном из них, снимается, режим остаётся ручным.
func ubratServery(n *Nabor, ids []string) {
	n.Servery = slices.DeleteFunc(n.Servery, func(x protokol.Server) bool { return slices.Contains(ids, x.Id) })
	if n.Vybran != "" && slices.Contains(ids, n.Vybran) {
		if n.Rezhim == "" {
			n.Rezhim = protokol.RezhimRuchnoy
		}
		n.Vybran = ""
	}
}

// Список.

// SvoyServerDlyaEkrana это запись о сервере для карточки: без ключа хоста
// целиком, с отметкой, есть ли на этом компьютере ключ управления.
type SvoyServerDlyaEkrana struct {
	SvoyServer
	KlyuchUpravleniya bool `json:"klyuch_upravleniya"`
	// KlyuchNeChitaetsya: файл ключа управления есть, а прочесть его нельзя.
	// Это не «ключа нет»: захват упал бы на нём же, выход - «Забыть».
	KlyuchNeChitaetsya string `json:"klyuch_ne_chitaetsya,omitempty"`
	Otpechatok         string `json:"otpechatok,omitempty"`
}

func (s *Sluzhba) svoyVpsSpisok(k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	spisok := make([]SvoyServerDlyaEkrana, 0, len(n.SvoiServery))
	for _, x := range n.SvoiServery {
		e := SvoyServerDlyaEkrana{SvoyServer: x}
		_, e.KlyuchUpravleniya, err = s.klyuchUpravleniya(x.Id)
		if err != nil {
			log.Printf("свой сервер %s: %v", x.Id, err)
			e.KlyuchNeChitaetsya = sboi.DlyaCheloveka(err)
		}
		if kh, err := ssh.KlyuchHostaIzZapisi(x.KlyuchHostaTip, x.KlyuchHosta); err == nil {
			e.Otpechatok = kh.Otpechatok()
		}
		e.KlyuchHosta = ""
		spisok = append(spisok, e)
	}
	return otvet(k.Id, k.Imya, map[string]any{"servery": spisok})
}

// Проверка ключей: ядро вне набора на своём конфиге.

const imyaKonfigaProverki = "sing-box.svoy-vps.json"

// zapuskProverki поднимает ядро проверки ключей своего сервера. Задание одно
// на службу, поэтому и файл конфига один.
func (s *Sluzhba) zapuskProverki(_ context.Context, telo []byte) (func(), error) {
	put := filepath.Join(s.dirDannyh, imyaKonfigaProverki)
	ubrat := func() {
		if err := os.Remove(put); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("конфиг проверки своего сервера не удалён: %v", err)
		}
	}
	// 0600: в конфиге ключи.
	if err := os.WriteFile(put, telo, 0o600); err != nil {
		return nil, fmt.Errorf("конфиг проверки не записан: %w", err)
	}
	if s.proveritKonfig != nil {
		if err := s.proveritKonfig(put); err != nil {
			ubrat()
			return nil, fmt.Errorf("ядро не приняло конфиг проверки: %w", err)
		}
	}
	y, err := yadra.Zapustit(imyaYadraTun, put)
	if err != nil {
		ubrat()
		return nil, err
	}
	var odin sync.Once
	return func() {
		odin.Do(func() {
			if err := y.Ostanovit(); err != nil {
				log.Printf("ядро проверки своего сервера не остановлено: %v", err)
			}
			ubrat()
		})
	}, nil
}

// programmaIzKataloga отдаёт программу сервера, лежащую рядом со службой.
// Доставку с выпуска и сверку отпечатка делает У5.
func (s *Sluzhba) programmaIzKataloga(context.Context) (io.ReadSeekCloser, error) {
	f, err := os.Open(filepath.Join(s.dirProgrammy, imyaProgrammyUzla))
	if err != nil {
		return nil, fmt.Errorf("программа сервера не найдена: %w", err)
	}
	return f, nil
}

const imyaProgrammyUzla = "vpn-uzel"
