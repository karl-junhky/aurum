package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/blok"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/proverka"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

const adresSvoego = "203.0.113.10"

func klyuchHostaDlyaTesta(t *testing.T) ssh.KlyuchHosta {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return ssh.KlyuchHosta{Tip: pk.Type(), Telo: pk.Marshal()}
}

func kadrVps(t *testing.T, imya string, telo any) protokol.Kadr {
	t.Helper()
	b, err := json.Marshal(telo)
	if err != nil {
		t.Fatal(err)
	}
	return protokol.Kadr{Id: 1, Imya: imya, Telo: b}
}

// dozhdatsyaVps ждёт конца задания и отдаёт последний снимок.
func dozhdatsyaVps(t *testing.T, s *Sluzhba) HodVps {
	t.Helper()
	srok := time.Now().Add(5 * time.Second)
	for time.Now().Before(srok) {
		s.muVps.Lock()
		if s.vps != nil && s.vps.gotovo {
			h := s.vps.snimok
			s.muVps.Unlock()
			return h
		}
		s.muVps.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("задание своего сервера не кончилось за 5 с")
	return HodVps{}
}

func kodOtkaza(k protokol.Kadr) string {
	if k.Oshib == nil {
		return ""
	}
	return k.Oshib.Kod
}

// sPustymNaborom читает и пишет набор в памяти: фикстура отдаёт свою
// заготовку мимо хранилища, а установка пишет в него.
func sPustymNaborom(t *testing.T) *Sluzhba {
	t.Helper()
	s := sPamyatyu(t, []byte(`{"servery":[]}`))
	s.nabor = s.naborIzHranilishcha
	return s
}

func osmotrChistyy() *server.Osmotr {
	return &server.Osmotr{
		OS: "ubuntu", OSVersiya: "24.04", OSImya: "Ubuntu 24.04.1 LTS", Arch: "x86_64", PID1: "systemd",
		PamyatMB: 1024, DiskMB: 20000,
		IPv4: []server.Adres{{Interfeys: "eth0", Set: netip.MustParsePrefix(adresSvoego + "/24")}},
	}
}

func TestKlyuchUpravleniya(t *testing.T) {
	s := podstavnaya(t, nil)
	id := idSvoego(netip.MustParseAddr(adresSvoego))
	if _, ok, err := s.klyuchUpravleniya(id); err != nil || ok || s.estKlyuchUpravleniya(id) {
		t.Fatalf("ключа ещё нет, а ответ ok=%v err=%v", ok, err)
	}
	k1, err := s.zavestiKlyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	if !s.estKlyuchUpravleniya(id) {
		t.Fatal("заведённого ключа статус не видит")
	}
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("PRIVATE KEY")) {
		t.Fatal("ключ управления лежит на диске открытым")
	}
	// Повтор установки входит тем же ключом: он уже мог лечь в authorized_keys.
	k2, err := s.zavestiKlyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	k3, ok, err := s.klyuchUpravleniya(id)
	if err != nil || !ok {
		t.Fatalf("заведённый ключ не прочитан: ok=%v err=%v", ok, err)
	}
	for _, k := range []gossh.Signer{k2, k3} {
		if !bytes.Equal(k.PublicKey().Marshal(), k1.PublicKey().Marshal()) {
			t.Fatal("второе обращение отдало другой ключ")
		}
	}
	if err := s.ubratKlyuchUpravleniya(id); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.klyuchUpravleniya(id); err != nil || ok || s.estKlyuchUpravleniya(id) {
		t.Fatalf("стёртый ключ читается: ok=%v err=%v", ok, err)
	}
	if err := s.ubratKlyuchUpravleniya(id); err != nil {
		t.Fatalf("стирать нечего, а отказ: %v", err)
	}
	for _, plohoy := range []string{"", "svoy-zz", "../svoy-0123456789abcdef", "svoy-0123456789abcdef.bak"} {
		if _, err := s.zavestiKlyuchUpravleniya(plohoy); err == nil {
			t.Errorf("id %q принят", plohoy)
		}
	}
}

func TestKommentariyKlyucha(t *testing.T) {
	den := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for vhod, nado := range map[string]string{
		"DESKTOP-1A":            "affory DESKTOP-1A 2026-11-01",
		"Мой ПК":                "affory pc 2026-11-01",
		"ПК_Офис-2":             "affory _-2 2026-11-01",
		"a b\nc":                "affory abc 2026-11-01",
		strings.Repeat("x", 80): "affory " + strings.Repeat("x", 63) + " 2026-11-01",
	} {
		if got := kommentariyKlyucha(vhod, den); got != nado {
			t.Errorf("%q: %q, а нужно %q", vhod, got, nado)
		}
	}
}

func TestIdSvoego(t *testing.T) {
	a := idSvoego(netip.MustParseAddr(adresSvoego))
	if a != idSvoego(netip.MustParseAddr("::ffff:"+adresSvoego)) {
		t.Fatal("тот же IPv4 в записи IPv6 дал другой id")
	}
	if !reIdSvoego.MatchString(a) {
		t.Fatalf("id %q не годится в имя файла ключа", a)
	}
	if a == idSvoego(netip.MustParseAddr("203.0.113.11")) {
		t.Fatal("два адреса дали один id")
	}
}

func TestSvoyVpsOsmotr(t *testing.T) {
	s := podstavnaya(t, nil)
	kh := klyuchHostaDlyaTesta(t)
	var zapros ssh.Zapros
	s.vpsOsmotret = func(_ context.Context, p upravlenie.Puti, z ssh.Zapros) (upravlenie.Otchet, error) {
		zapros = z
		if _, est := p.Tunnel(); est {
			t.Error("путь через VPN при выключенном VPN")
		}
		o := osmotrChistyy()
		o.Chuzhoe = []string{"nginx"}
		return upravlenie.Otchet{Vhod: upravlenie.Svedeniya{KlyuchHosta: kh, Root: true, Put: "напрямую"},
			Osmotr: o, Zamechaniya: o.Zamechaniya(time.Now())}, nil
	}
	id, sobytiya := s.Podpisatsya()
	defer s.Otpisatsya(id)

	otv := s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret",
		dostupVps{Adres: adresSvoego, Port: 2222, Polzovatel: "root", Parol: "пароль-входа"}))
	if otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Osmotr == nil {
		t.Fatalf("осмотр не дошёл: %+v", h)
	}
	if zapros.Adres != adresSvoego || zapros.Port != 2222 || zapros.Vhod.Parol != "пароль-входа" {
		t.Fatalf("вход ушёл не тот: %+v", zapros)
	}
	o := h.Osmotr
	if o.MozhnoStavit {
		t.Fatal("на непустую машину установка разрешена")
	}
	if o.KlyuchHosta != kh.TeloBase64() || o.KlyuchHostaTip != kh.Tip || o.Otpechatok != kh.Otpechatok() {
		t.Fatal("ключ хоста с осмотра не тот")
	}
	nashli := false
	for _, z := range o.Zamechaniya {
		if z.Kod == string(server.KodNePustoy) {
			nashli = z.Stop && strings.Contains(z.Tekst, "nginx")
		}
	}
	if !nashli {
		t.Fatalf("замечание о чужом без стопа или без найденного: %+v", o.Zamechaniya)
	}
	// Событие уходит всем окнам: в нём нет ни адреса, ни пароля.
	bylo := 0
	for {
		select {
		case k := <-sobytiya:
			if k.Imya != "svoy-vps-hod" {
				continue
			}
			bylo++
			if bytes.Contains(k.Telo, []byte("203.0.113")) || bytes.Contains(k.Telo, []byte("пароль")) {
				t.Fatalf("в событии адрес или пароль: %s", k.Telo)
			}
			continue
		default:
		}
		break
	}
	if bylo == 0 {
		t.Fatal("ход не пришёл событием")
	}
}

func TestSvoyVpsOsmotrNastraivaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	kh := klyuchHostaDlyaTesta(t)
	s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
		o := osmotrChistyy()
		o.CloudInit = "running"
		return upravlenie.Otchet{Vhod: upravlenie.Svedeniya{KlyuchHosta: kh}, Osmotr: o, Zamechaniya: o.Zamechaniya(time.Now())}, nil
	}
	s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
	o := dozhdatsyaVps(t, s).Osmotr
	if o == nil || !o.MozhnoStavit {
		t.Fatalf("машина, которая настраивается, заперла установку: %+v", o)
	}
}

// Своя отметка установки продолжается, чужая берётся под управление: решает
// ключ управления этого компьютера.
func TestSvoyVpsOsmotrOtmetka(t *testing.T) {
	for _, svoya := range []bool{true, false} {
		t.Run(strconv.FormatBool(svoya), func(t *testing.T) {
			s := podstavnaya(t, nil)
			kh := klyuchHostaDlyaTesta(t)
			nash, err := s.zavestiKlyuchUpravleniya(idSvoego(netip.MustParseAddr(adresSvoego)))
			if err != nil {
				t.Fatal(err)
			}
			klyuch := base64.StdEncoding.EncodeToString(nash.PublicKey().Marshal())
			if !svoya {
				drugoy := klyuchHostaDlyaTesta(t)
				klyuch = base64.StdEncoding.EncodeToString(drugoy.Telo)
			}
			s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
				o := osmotrChistyy()
				o.Otmetka = &server.Sostav{Format: 1, Sostoyanie: "gotovo", Klyuchi: []string{klyuch}}
				return upravlenie.Otchet{Vhod: upravlenie.Svedeniya{KlyuchHosta: kh}, Osmotr: o, Zamechaniya: o.Zamechaniya(time.Now())}, nil
			}
			s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
			o := dozhdatsyaVps(t, s).Osmotr
			if o == nil || o.MozhnoStavit != svoya {
				t.Fatalf("своя=%v, а осмотр %+v", svoya, o)
			}
			for _, z := range o.Zamechaniya {
				if z.Kod == string(server.KodUzheStoit) &&
					(z.Stop == svoya || strings.Contains(z.Tekst, "ключа управления им на этом компьютере нет") == svoya) {
					t.Fatalf("своя=%v, а замечание %+v", svoya, z)
				}
			}
		})
	}
}

// Установка, дошедшая до конца, готова и записана, даже если отмена пришла
// после: сервер уже настроен, ключи на нём живые.
func TestSvoyVpsOtmenaPosleKonca(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader(nil)}, nil
	}
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	klyuchi, err := konfig.Klyuchi(sek, konfig.Vhody{Imya: "x", Adres: adresSvoego, Reality: []int{443}})
	if err != nil {
		t.Fatal(err)
	}
	pusk := make(chan struct{})
	s.vpsUstanovit = func(ctx context.Context, _ upravlenie.Ustanovka) (upravlenie.Itog, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.Itog{Adres: adresSvoego, Sekrety: sek, Klyuchi: klyuchi, Vhod: upravlenie.Svedeniya{KlyuchHosta: kh}}, nil
	}
	z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64()}
	s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))
	<-pusk
	s.svoyVpsOtmenit(protokol.Kadr{Imya: "svoyVpsOtmenit"})
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || h.Ustanovlen == nil {
		t.Fatalf("законченная установка: %+v", h)
	}
	if n, err := s.nabor(); err != nil || len(n.SvoiServery) != 1 {
		t.Fatalf("законченная установка не записана: %v", err)
	}
}

func TestObrezatImya(t *testing.T) {
	if got := obrezatImya(strings.Repeat("я", 50)); got != strings.Repeat("я", 40) {
		t.Errorf("длинное имя: %q", got)
	}
	if got := obrezatImya("Да\x00ча\x1b\x7f\u0085  ‮​"); got != "Дача" {
		t.Errorf("управляющие и невидимые знаки: %q", got)
	}
	for _, pustoe := range []string{"   ", "​​", " ‮ "} {
		if got := obrezatImya(pustoe); got != "" {
			t.Errorf("пустое на вид имя %q: %q", pustoe, got)
		}
	}
	// Обрезка по 40 знакам не оставляет пробела на конце.
	if got := obrezatImya("  " + strings.Repeat("я", 39) + " ж"); got != strings.Repeat("я", 39) {
		t.Errorf("пробел на месте обрезки: %q", got)
	}
}

func TestSvoyVpsZanyato(t *testing.T) {
	s := podstavnaya(t, nil)
	pusk := make(chan struct{})
	s.vpsOsmotret = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros) (upravlenie.Otchet, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.Otchet{}, ctx.Err()
	}
	k := kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"})
	if otv := s.svoyVpsOsmotret(k); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	<-pusk
	if kod := kodOtkaza(s.svoyVpsOsmotret(k)); kod != protokol.KodVpsZanyato {
		t.Fatalf("второе дело разом: %q", kod)
	}
	if kod := kodOtkaza(s.ustanovitArhiv(protokol.Kadr{Id: 2, Imya: "installUpdate"}, "nichego.zip")); kod != protokol.KodVpsZanyato {
		t.Fatalf("обновление посреди дела: %q", kod)
	}
	if kod := kodOtkaza(s.downloadUpdate(context.Background(), protokol.Kadr{Id: 3, Imya: "downloadUpdate"})); kod != protokol.KodVpsZanyato {
		t.Fatalf("загрузка обновления посреди дела: %q", kod)
	}
	s.svoyVpsOtmenit(protokol.Kadr{Id: 4, Imya: "svoyVpsOtmenit"})
	if h := dozhdatsyaVps(t, s); h.Faza != fazaOtmeneno || h.Otkaz != nil {
		t.Fatalf("отмена: %+v", h)
	}
	s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
		return upravlenie.Otchet{}, &upravlenie.Molchanie{Pryamo: errors.New("тишина")}
	}
	if otv := s.svoyVpsOsmotret(k); otv.Oshib != nil {
		t.Fatalf("после отмены новое дело не пускают: %v", otv.Oshib)
	}
	if h := dozhdatsyaVps(t, s); h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsNeOtvechaet {
		t.Fatalf("отказ: %+v", h)
	}
}

// Установка обновления запирает новые дела до остановки службы, а отказ
// обновления запор снимает.
func TestSvoyVpsZaporObnovleniya(t *testing.T) {
	s := podstavnaya(t, nil)
	k := kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"})
	otperet, _ := s.zaperetVps(time.Now().Add(time.Minute), otkazObnovlenieIdyot)
	if otperet == nil {
		t.Fatal("запор не встал")
	}
	otv := s.svoyVpsOsmotret(k)
	if otv.Oshib == nil || otv.Oshib.Kod != protokol.KodVpsZanyato || otv.Oshib.Tekst != otkazObnovlenieIdyot {
		t.Fatalf("дело посреди обновления: %+v", otv.Oshib)
	}
	if kod := kodOtkaza(s.ustanovitArhiv(protokol.Kadr{Id: 2, Imya: "installUpdate"}, "nichego.zip")); kod != protokol.KodVpsZanyato {
		t.Fatalf("второе обновление разом: %q", kod)
	}
	otperet()

	// Истёкший запор, взятый заново другим, прежний хозяин не снимает.
	staryy, _ := s.zaperetVps(time.Now().Add(-time.Second), "старый")
	novyy, _ := s.zaperetVps(time.Now().Add(time.Minute), otkazObnovlenieIdyot)
	if staryy == nil || novyy == nil {
		t.Fatal("запор не встал")
	}
	staryy()
	if kod := kodOtkaza(s.svoyVpsOsmotret(k)); kod != protokol.KodVpsZanyato {
		t.Fatalf("прежний хозяин снял чужой запор: %q", kod)
	}
	novyy()

	// Архива нет: обновление отказывает и запор снимает.
	if kod := kodOtkaza(s.ustanovitArhiv(protokol.Kadr{Id: 3, Imya: "installUpdate"}, "nichego.zip")); kod == protokol.KodVpsZanyato || kod == "" {
		t.Fatalf("обновление без архива: %q", kod)
	}
	s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
		return upravlenie.Otchet{}, &upravlenie.Molchanie{Pryamo: errors.New("тишина")}
	}
	if otv := s.svoyVpsOsmotret(k); otv.Oshib != nil {
		t.Fatalf("после отказа обновления дело не пускают: %v", otv.Oshib)
	}
	dozhdatsyaVps(t, s)
	// Просроченный запор не держит.
	if otperet, _ := s.zaperetVps(time.Now().Add(-time.Second), otkazObnovlenieIdyot); otperet == nil {
		t.Fatal("запор не встал")
	}
	if otv := s.svoyVpsOsmotret(k); otv.Oshib != nil {
		t.Fatalf("просроченный запор держит: %v", otv.Oshib)
	}
	dozhdatsyaVps(t, s)

	// Подменщик запущен: запор остаётся до остановки службы.
	s.zapustitPodmenshchika = func(string, string, bool) error { return nil }
	put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, true)
	if otv := s.ustanovitArhiv(protokol.Kadr{Id: 4, Imya: "installUpdate"}, put); otv.Oshib != nil {
		t.Fatalf("обновление: %+v", otv.Oshib)
	}
	if otv := s.svoyVpsOsmotret(k); otv.Oshib == nil || otv.Oshib.Tekst != otkazObnovlenieIdyot {
		t.Fatalf("после запуска подменщика дело пускают: %+v", otv.Oshib)
	}
}

func TestSvoyVpsNeprinyatoeTelo(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	horoshiy := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64()}
	for imya, izmenit := range map[string]func(*zaprosUstanovkiVps){
		"имя хоста":   func(z *zaprosUstanovkiVps) { z.Adres = "vps.example.net" },
		"зона":        func(z *zaprosUstanovkiVps) { z.Adres = "fe80::1%eth0" },
		"порт":        func(z *zaprosUstanovkiVps) { z.Port = 70000 },
		"без галочки": func(z *zaprosUstanovkiVps) { z.Pustoy = false },
		"без ключа":   func(z *zaprosUstanovkiVps) { z.KlyuchHosta = "" },
		"не тот тип":  func(z *zaprosUstanovkiVps) { z.KlyuchHostaTip = "ssh-rsa" },
	} {
		z := horoshiy
		izmenit(&z)
		if kod := kodOtkaza(s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))); kod != protokol.KodTeloNegodno {
			t.Errorf("%s: %q", imya, kod)
		}
	}
	if kod := kodOtkaza(s.svoyVpsOsmotret(protokol.Kadr{Imya: "svoyVpsOsmotret", Telo: []byte("{")})); kod != protokol.KodTeloNegodno {
		t.Errorf("битое тело: %q", kod)
	}
}

type programmaDlyaTesta struct {
	*bytes.Reader
	zakryta bool
}

func (p *programmaDlyaTesta) Close() error { p.zakryta = true; return nil }

func TestSvoyVpsUstanovit(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	prog := &programmaDlyaTesta{Reader: bytes.NewReader([]byte("vpn-uzel"))}
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) { return prog, nil }

	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	vhody := konfig.Vhody{Imya: "Дача", Adres: adresSvoego, Reality: []int{443}, Hy2: []int{8443}}
	klyuchi, err := konfig.Klyuchi(sek, vhody)
	if err != nil {
		t.Fatal(err)
	}
	var u upravlenie.Ustanovka
	s.vpsUstanovit = func(_ context.Context, x upravlenie.Ustanovka) (upravlenie.Itog, error) {
		u = x
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagPodbor, Otsev: &porty.Otsev{Proto: "tcp", Port: 443, Prichina: porty.PrichinaZanyat, Process: "nginx"}})
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagProverka, Klyuch: &proverka.Itog{Server: klyuchi[0].Server}})
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagProverka, Klyuch: &proverka.Itog{Server: klyuchi[0].Server, Vid: proverka.Vezyot}})
		return upravlenie.Itog{
			Adres: adresSvoego, Sekrety: sek, Vhody: vhody, Klyuchi: klyuchi,
			Proverki: []proverka.Itog{
				{Server: klyuchi[0].Server, Vid: proverka.Vezyot},
				{Server: klyuchi[1].Server, Vid: proverka.NetOtveta, Otkaz: "молчит"},
			},
			Prodlenie: porty.Cherez80, Versiya: "0.6.0",
			Vhod: upravlenie.Svedeniya{KlyuchHosta: kh, Put: "напрямую"},
			Sert: &server.SertUzla{Est: true, Do: 1790000000},
			Preduprezhdeniya: []upravlenie.Preduprezhdenie{
				{Vid: upravlenie.NeVezyot, Transport: "hy2", Port: 8443, Otkaz: "молчит"},
			},
		}, nil
	}
	z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Port: 2222, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64(), Imya: "Дача\x07"}
	if otv := s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z)); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Ustanovlen == nil || h.Server != idSvoego(netip.MustParseAddr(adresSvoego)) {
		t.Fatalf("установка: %+v", h)
	}
	if h.Adres != adresSvoego {
		t.Errorf("адрес в снимке установки %q, а не %q: окно, открытое посреди неё, не узнает сервер", h.Adres, adresSvoego)
	}
	if !prog.zakryta {
		t.Error("программа сервера не закрыта")
	}

	// Что ушло в установку.
	if u.Vhod.Izvesten == nil || !bytes.Equal(u.Vhod.Izvesten.Telo, kh.Telo) {
		t.Error("установка входит без ключа хоста с осмотра")
	}
	if !u.Pustoy || u.Imya != "Дача" || !strings.HasPrefix(u.Kommentariy, "affory ") {
		t.Errorf("установка: Pustoy=%v Imya=%q Kommentariy=%q", u.Pustoy, u.Imya, u.Kommentariy)
	}
	nash, ok, err := s.klyuchUpravleniya(idSvoego(netip.MustParseAddr(adresSvoego)))
	if err != nil || !ok || !bytes.Equal(nash.PublicKey().Marshal(), u.Upravlenie.PublicKey().Marshal()) {
		t.Error("ключ управления не тот, что лежит на диске")
	}
	if u.Proverka.Adapter != "Ethernet" || u.Pryamo == nil || u.Zapusk == nil {
		t.Errorf("проверка без адаптера или запуска: %+v", u.Proverka)
	}

	// Ход.
	if len(h.Otseyany) != 1 || h.Otseyany[0].Process != "nginx" || len(h.Proverki) != 1 || h.Klyuch != nil {
		t.Errorf("ход подбора и проверки: %+v", h)
	}
	if len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Kod != protokol.KodVpsPorty {
		t.Errorf("предупреждения: %+v", h.Preduprezhdeniya)
	}
	if len(h.Ustanovlen.Klyuchi) != 2 || h.Ustanovlen.Klyuchi[1].Vid != string(proverka.NetOtveta) {
		t.Errorf("ключи: %+v", h.Ustanovlen.Klyuchi)
	}

	// Набор: два ключа своим сервером и запись о нём, одной правкой.
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
		t.Fatalf("набор: %d ключей, %d своих", len(n.Servery), len(n.SvoiServery))
	}
	for _, srv := range n.Servery {
		if !srv.Svoy {
			t.Errorf("ключ %s без пометки своего", srv.Id)
		}
	}
	zap := n.SvoiServery[0]
	if zap.Id != h.Ustanovlen.Server || zap.PortSsh != 2222 || zap.Reality != 443 || zap.Hy2 != 8443 ||
		zap.Prodlenie != "80" || zap.SertDo != 1790000000 || zap.Maska != "dl.google.com" || zap.PortyRuchnye ||
		zap.KlyuchHosta != kh.TeloBase64() || len(zap.Servery) != 2 || zap.Servery[0] != n.Servery[0].Id || zap.Postavlen.IsZero() {
		t.Errorf("запись: %+v", zap)
	}

	// Второй раз тот же сервер не ставится: действия в карточке.
	if kod := kodOtkaza(s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))); kod != protokol.KodTeloNegodno {
		t.Errorf("повтор установки: %q", kod)
	}

	// Список для карточки.
	var spisok struct {
		Servery []SvoyServerDlyaEkrana `json:"servery"`
	}
	otv := s.svoyVpsSpisok(protokol.Kadr{Imya: "svoyVpsSpisok"})
	if err := json.Unmarshal(otv.Telo, &spisok); err != nil {
		t.Fatal(err)
	}
	if len(spisok.Servery) != 1 || !spisok.Servery[0].KlyuchUpravleniya || spisok.Servery[0].Otpechatok != kh.Otpechatok() ||
		spisok.Servery[0].KlyuchHosta != "" {
		t.Errorf("список: %+v", spisok.Servery)
	}
}

func TestSvoyVpsUstanovitOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader(nil)}, nil
	}
	s.vpsUstanovit = func(context.Context, upravlenie.Ustanovka) (upravlenie.Itog, error) {
		return upravlenie.Itog{}, fmt.Errorf("установка: %w", &server.Oshibka{Shag: "ustanovka", Kod: 3, Tekst: "apt\x1b[31m сломан"})
	}
	z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64()}
	s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsUstanovka || strings.ContainsRune(h.Otkaz.Tekst, 0x1b) {
		t.Fatalf("отказ установки: %+v", h.Otkaz)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || len(n.SvoiServery) != 0 {
		t.Fatal("неудачная установка оставила записи в наборе")
	}
	// Ключ управления остаётся: он мог уже лечь на сервер.
	if _, ok, _ := s.klyuchUpravleniya(idSvoego(netip.MustParseAddr(adresSvoego))); !ok {
		t.Fatal("ключ управления стёрт после неудачи")
	}
}

func TestPutiVps(t *testing.T) {
	s := podstavnaya(t, nil)
	p := s.putiVps()
	if _, est := p.Tunnel(); est {
		t.Fatal("путь через VPN, когда VPN выключен")
	}
	s.mu.Lock()
	s.sost, s.portProksiNash = protokol.SostPodnyat, 0
	s.mu.Unlock()
	if _, est := p.Tunnel(); est {
		t.Fatal("путь через VPN без своего входа")
	}
	s.mu.Lock()
	s.portProksiNash = 10809
	s.mu.Unlock()
	if _, est := p.Tunnel(); !est {
		t.Fatal("поднятый VPN со своим входом не взят")
	}
	if _, err := p.Pryamo(); err != nil {
		t.Fatal(err)
	}
	s.fizicheskiyAdapter = func() (set.Adapter, error) { return set.Adapter{}, errors.New("адаптера нет") }
	if _, err := s.putiVps().Pryamo(); err == nil {
		t.Fatal("прямой путь без адаптера")
	}
}

func TestZapuskProverkiUbiraetKonfig(t *testing.T) {
	s := podstavnaya(t, nil)
	put := ""
	s.proveritKonfig = func(p string) error {
		put = p
		if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
			t.Errorf("конфиг не лёг до проверки: %v", err)
		}
		return errors.New("ядро не приняло")
	}
	if _, err := s.zapuskProverki(context.Background(), []byte(`{"log":{}}`)); err == nil {
		t.Fatal("отказ судьи не дошёл")
	}
	if _, err := os.Stat(put); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("конфиг с ключами остался на диске: %v", err)
	}
}

func TestOtkazVps(t *testing.T) {
	kh := klyuchHostaDlyaTesta(t)
	for imya, c := range map[string]struct {
		err  error
		kod  string
		est  string
		test func(OtkazVps) bool
	}{
		"молчит без VPN":  {&upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaTishina}}, protokol.KodVpsNeOtvechaet, "Включи VPN", nil},
		"молчит оба":      {&upravlenie.Molchanie{Pryamo: errors.New("x"), Tunnel: errors.New("y")}, protokol.KodVpsNeOtvechaet, "ни напрямую, ни через VPN", nil},
		"порт закрыт":     {&upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}}, protokol.KodVpsNeOtvechaet, "включи VPN", nil},
		"закрыт и в VPN":  {&upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}, Tunnel: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}}, protokol.KodVpsNeOtvechaet, "никто не слушает: проверь порт", nil},
		"закрыт, VPN тих": {&upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}, Tunnel: &ssh.Otkaz{Prichina: ssh.PrichinaTishina}}, protokol.KodVpsNeOtvechaet, "ни напрямую, ни через VPN", nil},
		"обёрнутое":       {fmt.Errorf("осмотр: %w", &upravlenie.Molchanie{Pryamo: errors.New("x")}), protokol.KodVpsNeOtvechaet, "", nil},
		"ключ хоста":      {&ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &kh}, protokol.KodVpsKlyuchHosta, "другой ключ", func(o OtkazVps) bool { return o.Otpechatok == kh.Otpechatok() }},
		"через него":      {&ssh.Otkaz{Prichina: ssh.PrichinaTunnelCherezNego}, protokol.KodVpsTunnelCherezNego, "", nil},
		"пароль":          {&ssh.Otkaz{Prichina: ssh.PrichinaParolNePrinyat}, protokol.KodVpsVhodOtvergnut, "пароль", nil},
		"root запрещён":   {&ssh.Otkaz{Prichina: ssh.PrichinaRootZapreshchyon, KlyuchOstalsya: true}, protokol.KodVpsVhodOtvergnut, "root", func(o OtkazVps) bool { return o.KlyuchOstalsya }},
		"шум входа":       {&ssh.Otkaz{Prichina: ssh.PrichinaShumVhoda}, protokol.KodVpsVhodOtvergnut, ".bashrc", nil},
		"подожди":         {&ssh.Otkaz{Prichina: ssh.PrichinaPodozhdi}, protokol.KodVpsNeOtvechaet, "", func(o OtkazVps) bool { return o.Podozhdat == 60 }},
		"рано":            {&ssh.Otkaz{Prichina: ssh.PrichinaRanoPovtoryat, Podozhdat: 42}, protokol.KodVpsNeOtvechaet, "42", func(o OtkazVps) bool { return o.Podozhdat == 42 }},
		"сбой":            {&ssh.Otkaz{Prichina: ssh.PrichinaSboy}, protokol.KodVpsUstanovka, "", nil},
		"чужое":           {&upravlenie.Ostanovka{Zamechaniya: []server.Zamechanie{{Kod: server.KodNePustoy, Stop: true, Chto: []string{"docker"}}}}, protokol.KodVpsNePustoy, "docker", nil},
		"чужая отметка":   {&upravlenie.Ostanovka{Zamechaniya: []server.Zamechanie{{Kod: server.KodUzheStoit, Stop: true}}}, protokol.KodVpsVzyat, "под управление", nil},
		"не ubuntu":       {&upravlenie.Ostanovka{Zamechaniya: []server.Zamechanie{{Kod: server.KodNeUbuntu, Stop: true, Chto: []string{"Debian 12"}}}}, protokol.KodVpsNeGoditsya, "Debian 12", nil},
		"скрипт":          {&server.Oshibka{Shag: "ustanovka", Kod: 1, Tekst: "нет места", KlyuchOstalsya: true}, protokol.KodVpsUstanovka, "нет места", func(o OtkazVps) bool { return o.KlyuchOstalsya }},
		"порт руками":     {porty.Otsev{Proto: "tcp", Port: 22, Prichina: porty.PrichinaSsh}, protokol.KodVpsPorty, "22", nil},
		"нет кандидатов":  {fmt.Errorf("подбор: %w", porty.ErrNetKandidatov), protokol.KodVpsPorty, "руками", nil},
		"не решил":        {porty.ErrNeReshil, protokol.KodVpsPorty, "", nil},
		"нет маски":       {upravlenie.ErrNetMaski, protokol.KodVpsUstanovka, "интернет", nil},
		"прочее":          {errors.New("что-то"), protokol.KodVpsUstanovka, "", nil},
	} {
		o := otkazVps(c.err)
		if o.Kod != c.kod || !strings.Contains(o.Tekst, c.est) || o.Tekst == "" || (c.test != nil && !c.test(o)) {
			t.Errorf("%s: %+v", imya, o)
		}
	}
}

func TestPreduprezhdenieVps(t *testing.T) {
	s := func(v blok.Vid) *blok.Itog { return &blok.Itog{Vid: v} }
	for imya, c := range map[string]struct {
		p   upravlenie.Preduprezhdenie
		kod string
		est string
	}{
		"сертификат":    {upravlenie.Preduprezhdenie{Vid: upravlenie.NetSerta}, protokol.KodVpsNetSerta, "порт 80"},
		"не везёт":      {upravlenie.Preduprezhdenie{Vid: upravlenie.NeVezyot, Transport: "hy2", Port: 8443}, protokol.KodVpsPorty, "hysteria2 на порту 8443"},
		"не проверено":  {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProvereno}, protokol.KodVpsPorty, ""},
		"порты в ufw":   {upravlenie.Preduprezhdenie{Vid: upravlenie.PortyNeOtkryty}, protokol.KodVpsPorty, "подбери порты заново"},
		"белые списки":  {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443, Detektor: s(blok.BelyeSpiski)}, protokol.KodVpsSetOgranichena, "разрешённые"},
		"сеть мертва":   {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Detektor: s(blok.NeSostoyalas)}, protokol.KodVpsSetOgranichena, ""},
		"брандмауэр":    {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Port: 8443, Detektor: s(blok.OtvetNeVozvrashchaetsya)}, protokol.KodVpsBrandmauerHostera, "8443"},
		"адрес":         {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Detektor: s(blok.NeDohodit)}, protokol.KodVpsAdresZakryt, "другой адрес"},
		"глушат":        {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Detektor: s(blok.Glushat)}, protokol.KodVpsPorty, "глушат"},
		"без детектора": {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443}, protokol.KodVpsPorty, "reality на порту 443"},
	} {
		o := preduprezhdenieVps(c.p)
		if o.Kod != c.kod || !strings.Contains(o.Tekst, c.est) || o.Tekst == "" {
			t.Errorf("%s: %+v", imya, o)
		}
	}
}

// Вердикт детектора и то, из чего он сложен, уезжают в журнал, и один
// детектор на два вида пишется раз.
func TestZhurnalPreduprezhdeniy(t *testing.T) {
	zhurnal := perehvatitZhurnal(t)
	d := &blok.Itog{Vid: blok.OtvetNeVozvrashchaetsya, Set: blok.SetZhivaya, Doshlo: -1, DoshloUDP: 4, Krugov: 2,
		OtkazLovushki: "ловушка молчит", Stuki: []blok.Stuk{
			{Port: 22, Ishod: blok.Otkryt, Otvet: blok.OtvetEst},
			{Port: 40443, Ishod: blok.Tishina, Podrobno: "i/o timeout"},
		}}
	zhurnalPreduprezhdeniy("s1", []upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 50443, Detektor: d},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 40443, Detektor: d},
		{Vid: upravlenie.NetSerta, Otkaz: "лимит выпуска"},
		{Vid: upravlenie.NeVezyot, Transport: "hy2", Port: 8443, Otkaz: "нет ответа"},
		{Vid: upravlenie.NeProvereno},
	})
	z := zhurnal()
	for _, est := range []string{
		"ne-proshyol, reality-tcp на порту 50443: детектор otvet-ne-vozvrashchaetsya",
		"ne-proshyol, hy2 на порту 40443: детектор otvet-ne-vozvrashchaetsya",
		"детектор: сеть zhivaya, кругов 2, стук дошёл -1, датаграммы 4",
		"детектор: ловушка: ловушка молчит",
		"детектор: порт 22: otkryt, ответ est\n",
		"детектор: порт 40443: tishina, i/o timeout\n",
		"net-serta: лимит выпуска",
		"ne-vezyot, hy2 на порту 8443: нет ответа",
	} {
		if !strings.Contains(z, est) {
			t.Errorf("в журнале нет %q:\n%s", est, z)
		}
	}
	if n := strings.Count(z, "детектор: сеть"); n != 1 {
		t.Errorf("детектор записан %d раз:\n%s", n, z)
	}
	if strings.Contains(z, "ne-provereno") {
		t.Errorf("предупреждение без причины в журнале:\n%s", z)
	}
}

// У каждого замечания осмотра свой текст: коды берутся разбором пакета
// server, а не списком здесь.
func TestTekstZamechaniyaKazhdyyKod(t *testing.T) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "../../internal/svoyvps/server/osmotr.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	kodov := 0
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			return true
		}
		if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Kod" {
			return true
		}
		for _, v := range vs.Values {
			lit, ok := v.(*ast.BasicLit)
			if !ok {
				continue
			}
			kod, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			kodov++
			tekst := tekstZamechaniya(server.Zamechanie{Kod: server.Kod(kod), Chto: []string{"x"}})
			if tekst == kod {
				t.Errorf("у замечания %q нет текста", kod)
			}
		}
		return true
	})
	if kodov < 20 {
		t.Fatalf("разбор нашёл %d кодов: разбор сломан", kodov)
	}
}
