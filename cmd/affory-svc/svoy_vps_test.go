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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
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
	if got := obrezatImya(strings.Repeat("я", 50)); got != strings.Repeat("я", dlinaImeni) {
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
	// Обрезка по пределу не оставляет пробела на конце.
	if got := obrezatImya("  " + strings.Repeat("я", dlinaImeni-1) + " ж"); got != strings.Repeat("я", dlinaImeni-1) {
		t.Errorf("пробел на месте обрезки: %q", got)
	}
	// Флаг, разрезанный обрезкой, уходит целиком, а влезший остаётся.
	flag := "\U0001F1F7\U0001F1FA"
	if got := obrezatImya(strings.Repeat("я", dlinaImeni-1) + flag); got != strings.Repeat("я", dlinaImeni-1) {
		t.Errorf("половина флага на краю: %q", got)
	}
	if got := obrezatImya(strings.Repeat("я", dlinaImeni-2) + flag + "ж"); got != strings.Repeat("я", dlinaImeni-2)+flag {
		t.Errorf("целый флаг на краю: %q", got)
	}
}

func TestImyaPoUmolchaniyu(t *testing.T) {
	servery := []SvoyServer{{Id: "a", Imya: "Мой сервер"}, {Id: "b", Imya: "Мой сервер 2"}, {Id: "c", Imya: "Дача"}}
	if got := imyaPoUmolchaniyu(servery, "novyy"); got != "Мой сервер 3" {
		t.Errorf("третий сервер: %q", got)
	}
	// Сам сервер своё имя не занимает.
	if got := imyaPoUmolchaniyu(servery, "a"); got != "Мой сервер" {
		t.Errorf("поставить заново: %q", got)
	}
	if got := imyaPoUmolchaniyu(nil, "novyy"); got != "Мой сервер" {
		t.Errorf("первый сервер: %q", got)
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

// Ключи ложатся в набор до перепроверки из сети (И7): человек может её не
// ждать. Итог детектора и проверки дописывается в ту же запись после.
func TestSvoyVpsUstanovitPoHodu(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader([]byte("vpn-uzel"))}, nil
	}
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	vhody := konfig.Vhody{Imya: "Мой сервер", Adres: adresSvoego, Reality: []int{443}, Hy2: []int{8443}}
	klyuchi, err := konfig.Klyuchi(sek, vhody)
	if err != nil {
		t.Fatal(err)
	}
	id := idSvoego(netip.MustParseAddr(adresSvoego))
	snimok := func() HodVps {
		s.muVps.Lock()
		defer s.muVps.Unlock()
		return s.vps.snimok
	}
	d := &blok.Itog{Vid: blok.NeDohodit}
	s.vpsUstanovit = func(_ context.Context, x upravlenie.Ustanovka) (upravlenie.Itog, error) {
		rannyy := upravlenie.Itog{
			Adres: adresSvoego, Sekrety: sek, Vhody: vhody, Klyuchi: klyuchi, Prodlenie: porty.Cherez80,
			Versiya: "0.6.0", Vhod: upravlenie.Svedeniya{KlyuchHosta: kh, Put: "напрямую"},
			NeProshli: []string{"reality-tcp", "hy2"},
		}
		if err := x.Klyuchi(rannyy); err != nil {
			return upravlenie.Itog{}, err
		}
		// Запись и ключи уже в наборе, а снимок их называет.
		n, err := s.nabor()
		if err != nil || len(n.Servery) != 2 || len(n.SvoiServery) != 1 ||
			!slices.Equal(n.SvoiServery[0].NeIzSeti, []string{"reality-tcp", "hy2"}) || n.SvoiServery[0].Versiya != "0.6.0" {
			t.Errorf("набор до детектора: %+v, %v", n.SvoiServery, err)
		}
		if h := snimok(); h.Faza != fazaIdyot || h.Ustanovlen == nil || len(h.Ustanovlen.Ids) != 2 || len(h.Ustanovlen.NeIzSeti) != 2 {
			t.Errorf("снимок до детектора: %+v", h)
		}
		povtor := time.Now().Add(2 * time.Minute)
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagDetektor, Krug: &blok.Krug{Nomer: 1, Vid: blok.NeDohodit, Povtory: []time.Time{povtor}}})
		if h := snimok(); h.KrugTekst == "" || len(h.Povtory) != 1 || !h.Povtory[0].Equal(povtor) {
			t.Errorf("снимок после круга: %+v", h)
		}
		x.Hod(upravlenie.Hod{Shag: upravlenie.ShagProverka})
		if h := snimok(); h.KrugTekst != "" || h.Povtory != nil {
			t.Errorf("повторы после детектора: %+v", h)
		}
		itog := rannyy
		itog.Preduprezhdeniya = []upravlenie.Preduprezhdenie{
			{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443, Detektor: d},
			{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: d},
		}
		itog.Proverki = []proverka.Itog{{Server: klyuchi[0].Server, Vid: proverka.NetOtveta}, {Server: klyuchi[1].Server, Vid: proverka.NetOtveta}}
		return itog, nil
	}
	z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64()}
	if otv := s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z)); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Ustanovlen == nil || h.Ustanovlen.Server != id {
		t.Fatalf("установка: %+v", h)
	}
	// Один вердикт на оба вида это одна строка, и проверки ключей в снимке.
	if len(h.Preduprezhdeniya) != 1 || h.Preduprezhdeniya[0].Kod != protokol.KodVpsAdresZakryt || !h.Preduprezhdeniya[0].IzSeti {
		t.Errorf("предупреждения: %+v", h.Preduprezhdeniya)
	}
	if h.Ustanovlen.Klyuchi[0].Vid != string(proverka.NetOtveta) {
		t.Errorf("ключи: %+v", h.Ustanovlen.Klyuchi)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
		t.Fatalf("набор: %d ключей, %d своих", len(n.Servery), len(n.SvoiServery))
	}
	if zap := n.SvoiServery[0]; !slices.Equal(zap.NeIzSeti, []string{"reality-tcp", "hy2"}) || !strings.Contains(zap.SetTekst, "Проверь их с телефона") ||
		!strings.Contains(zap.SetPoVidam["hy2"], "Проверь hysteria2 с телефона") {
		t.Errorf("запись: %+v", zap)
	}
}

// Сервер уже в разделе: установка идёт поверх записи, сюда ведут отказы «нет
// установки» и «не дошла». Ключ хоста и имя из записи, ключ управления
// прежний, ключи группы заменяются, выбор переходит на новый ключ того же вида.
func TestSvoyVpsUstanovitPoverhZapisi(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, ids := postavitSvoy(t, s, kh, true)
	if err := s.pravitNabor(func(n *Nabor) error { n.Vybran = ids[0]; return nil }); err != nil {
		t.Fatal(err)
	}
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	s.programmaUzla = func(context.Context) (io.ReadSeekCloser, error) {
		return &programmaDlyaTesta{Reader: bytes.NewReader(nil)}, nil
	}
	var u upravlenie.Ustanovka
	s.vpsUstanovit = func(_ context.Context, x upravlenie.Ustanovka) (upravlenie.Itog, error) {
		u = x
		return itogSvoego(t, kh, 2087, 5443), nil
	}
	// С проверки пришёл другой ключ хоста и пустое имя.
	drugoy := klyuchHostaDlyaTesta(t)
	z := zaprosUstanovkiVps{dostupVps: dostupVps{Adres: adresSvoego, Port: 2222, Polzovatel: "root", Parol: "x"},
		Pustoy: true, KlyuchHostaTip: drugoy.Tip, KlyuchHosta: drugoy.TeloBase64()}
	// Без «Поставить заново» или с адресом другого сервера запись не
	// заменяется: «Поставить ещё» с адресом рабочего сменило бы все ключи.
	for _, poverh := range []string{"", "203.0.113.99", "не адрес"} {
		z.Poverh = poverh
		if kod := kodOtkaza(s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))); kod != protokol.KodTeloNegodno {
			t.Errorf("поверх записи без пометки %q: %q", poverh, kod)
		}
	}
	z.Poverh = adresSvoego
	if otv := s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z)); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Ustanovlen == nil || h.Server != id {
		t.Fatalf("установка поверх записи: %+v", h)
	}
	if u.Vhod.Izvesten == nil || !bytes.Equal(u.Vhod.Izvesten.Telo, kh.Telo) {
		t.Error("установка поверх записи входит не с записанным ключом хоста")
	}
	if u.Imya != "Дача" {
		t.Errorf("имя %q, а не из записи", u.Imya)
	}
	if !bytes.Equal(u.Upravlenie.PublicKey().Marshal(), nash.PublicKey().Marshal()) {
		t.Error("установка поверх записи завела новый ключ управления")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.SvoiServery) != 1 || len(n.Servery) != 2 || n.SvoiServery[0].Reality != 2087 || n.SvoiServery[0].Hy2 != 5443 {
		t.Fatalf("набор: %d ключей, записи %+v", len(n.Servery), n.SvoiServery)
	}
	for _, srv := range n.Servery {
		if slices.Contains(ids, srv.Id) {
			t.Errorf("прежний ключ %s остался", srv.Id)
		}
	}
	if !slices.Contains(n.SvoiServery[0].Servery, n.Vybran) {
		t.Errorf("выбор %q не перешёл на новый ключ", n.Vybran)
	}

	// В записи нет ключа хоста: идёт ключ с проверки. Неразборный - порча
	// хранилища, установка не начинается.
	for imya, sluchay := range map[string]struct {
		klyuch string
		kod    string
	}{
		"без ключа":   {"", ""},
		"неразборный": {"!!!", protokol.KodSecretsUnreadable},
	} {
		if err := s.pravitNabor(func(n *Nabor) error { n.SvoiServery[0].KlyuchHosta = sluchay.klyuch; return nil }); err != nil {
			t.Fatal(err)
		}
		u = upravlenie.Ustanovka{}
		if kod := kodOtkaza(s.svoyVpsUstanovit(kadrVps(t, "svoyVpsUstanovit", z))); kod != sluchay.kod {
			t.Fatalf("%s: %q", imya, kod)
		}
		if sluchay.kod != "" {
			continue
		}
		if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo {
			t.Fatalf("%s: %+v", imya, h)
		}
		if u.Vhod.Izvesten == nil || !bytes.Equal(u.Vhod.Izvesten.Telo, drugoy.Telo) {
			t.Errorf("%s: установка не взяла ключ хоста с проверки", imya)
		}
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
		"закрыт и в VPN":  {&upravlenie.Molchanie{Pryamo: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}, Tunnel: &ssh.Otkaz{Prichina: ssh.PrichinaPortZakryt}}, protokol.KodVpsNeOtvechaet, "не принимает вход: проверь порт", nil},
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
		o := preduprezhdenieVps([]upravlenie.Preduprezhdenie{c.p}, nil)
		iz := c.p.Vid == upravlenie.NeProshyol || c.p.Vid == upravlenie.NeVezyot
		if o.Kod != c.kod || !strings.Contains(o.Tekst, c.est) || o.Tekst == "" || o.IzSeti != iz {
			t.Errorf("%s: %+v", imya, o)
		}
	}
}

// Сертификат не вышел из-за предела попыток программы сервера: тексты
// называют предел и время следующей попытки, а не закрытые порты и не запись
// домена.
func TestTekstyPredelaSerta(t *testing.T) {
	sled := time.Date(2026, 11, 6, 22, 14, 0, 0, time.Local).Unix()
	for imya, o := range map[string]OtkazVps{
		"на адрес": preduprezhdenieVps([]upravlenie.Preduprezhdenie{{Vid: upravlenie.NetSerta, Predel: sled}}, nil),
		"домен при установке": preduprezhdenieVps([]upravlenie.Preduprezhdenie{
			{Vid: upravlenie.DomenNeVyshel, Domen: "kot.example.net", Predel: sled}}, nil),
		"переход на домен": otkazVps(&upravlenie.SertDomenaNeVyshel{Domen: "kot.example.net", Otkaz: "попытки кончились", Predel: sled}),
		"возврат с домена": preduprezhdenieVps([]upravlenie.Preduprezhdenie{{Vid: upravlenie.NetSerta, Predel: sled, Hy2Zhdyot: true}}, nil),
	} {
		if !strings.Contains(o.Tekst, "попытки сервера на этот час кончились") || !strings.Contains(o.Tekst, "22:14") ||
			strings.Contains(o.Tekst, "A-запись") || strings.Contains(o.Tekst, "закрыты порты") {
			t.Errorf("%s: %+v", imya, o)
		}
	}
	// После возврата с домена hy2 в пакете, и сертификат сервер получит сам:
	// подбирать порты незачем, и «только reality» неправда.
	for _, predel := range []int64{0, sled} {
		o := preduprezhdenieVps([]upravlenie.Preduprezhdenie{{Vid: upravlenie.NetSerta, Predel: predel, Hy2Zhdyot: true}}, nil)
		if strings.Contains(o.Tekst, "подбери порты") || strings.Contains(o.Tekst, "только reality") || !strings.Contains(o.Tekst, "сам") {
			t.Errorf("возврат с домена, предел %d: %+v", predel, o)
		}
	}
}

// Avast на этом компьютере перехватывает почтовые порты (П6): предупреждение
// о reality на них называет его, а не дорогу. Не называет, когда отказ
// Avast не объясняет: упали и обычные кандидаты, адрес закрыт, сервер
// молчит, ключ ушёл не тем выходом; hy2 идёт по UDP.
func TestPreduprezhdenieVpsAvast(t *testing.T) {
	avast := pomehiKompyutera{{Imya: "Avast", Vid: pomehi.VidAntivirus, Gruppa: pomehi.Meshaet}}
	s := func(v blok.Vid) *blok.Itog { return &blok.Itog{Vid: v} }
	neProshyol := func(port int, d *blok.Itog, kandidaty ...int) upravlenie.Preduprezhdenie {
		return upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: port, Detektor: d, Kandidaty: kandidaty}
	}
	for imya, c := range map[string]struct {
		p     upravlenie.Preduprezhdenie
		avast bool
	}{
		"руками":                 {neProshyol(993, s(blok.Glushat), 993), true},
		"подбор по почтовым":     {neProshyol(465, s(blok.Dostupen), 465, 993), true},
		"без детектора":          {upravlenie.Preduprezhdenie{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 465, Otkaz: "детектор не дошёл", Kandidaty: []int{465}}, true},
		"не везёт":               {upravlenie.Preduprezhdenie{Vid: upravlenie.NeVezyot, Transport: "reality-tcp", Port: 995, Proba: proverka.NetOtveta}, true},
		"упали и обычные":        {neProshyol(993, s(blok.Glushat), 443, 993, 2053), false},
		"адрес закрыт":           {neProshyol(993, s(blok.NeDohodit), 993), false},
		"сервер молчит":          {neProshyol(993, s(blok.ServerMolchit), 993), false},
		"молчит напрямую":        {neProshyol(993, s(blok.MolchitNapryamuyu), 993), false},
		"не почтовый":            {neProshyol(443, s(blok.Glushat), 443), false},
		"не везёт, не тот выход": {upravlenie.Preduprezhdenie{Vid: upravlenie.NeVezyot, Transport: "reality-tcp", Port: 995, Proba: proverka.ChuzhoyVyhod}, false},
		"hy2 по UDP":             {upravlenie.Preduprezhdenie{Vid: upravlenie.NeVezyot, Transport: "hy2", Port: 993, Proba: proverka.NetOtveta}, false},
	} {
		o := preduprezhdenieVps([]upravlenie.Preduprezhdenie{c.p}, avast)
		if strings.Contains(o.Tekst, "Avast") != c.avast || !o.IzSeti {
			t.Errorf("%s: %+v", imya, o)
		}
		if bez := preduprezhdenieVps([]upravlenie.Preduprezhdenie{c.p}, nil); strings.Contains(bez.Tekst, "Avast") {
			t.Errorf("%s без Avast: %+v", imya, bez)
		}
	}
	// Общий вердикт не склеивает reality под Avast с hy2: у reality своя
	// причина, и она же идёт в запись о сервере.
	d := s(blok.Glushat)
	pr := []upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 993, Detektor: d, Kandidaty: []int{993}},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 443, Detektor: d, Kandidaty: []int{443}},
	}
	if o := preduprezhdeniyaVps(pr, avast); len(o) != 2 || !strings.Contains(o[0].Tekst, "Avast") || strings.Contains(o[1].Tekst, "Avast") {
		t.Fatalf("раздельно: %+v", o)
	}
	obshchaya, poVidam := prichinySeti(pr, []string{"reality-tcp", "hy2"}, avast)
	if !strings.Contains(obshchaya, "Avast") || !strings.Contains(poVidam["reality-tcp"], "Avast") || strings.Contains(poVidam["hy2"], "Avast") {
		t.Fatalf("причины %q %+v", obshchaya, poVidam)
	}
}

// Проба reality на почтовом порту под Avast называет его в ходе и в ключах
// итога; провёзший ключ, чужой выход, неверный ответ цели и hy2 его не
// называют.
func TestHodNazyvaetAvast(t *testing.T) {
	avast := pomehiKompyutera{{Imya: "Avast", Vid: pomehi.VidAntivirus, Gruppa: pomehi.Meshaet}}
	proba := func(transport string, port int, vid proverka.Vid) proverka.Itog {
		return proverka.Itog{Server: protokol.Server{Id: transport + strconv.Itoa(port), Transport: transport, Port: port}, Vid: vid}
	}
	var x HodVps
	for _, it := range []proverka.Itog{
		proba("reality-tcp", 993, proverka.NetOtveta),
		proba("reality-tcp", 465, proverka.ChuzhoyVyhod),
		proba("reality-tcp", 2053, proverka.Vezyot),
		proba("hy2", 993, proverka.Zamerz),
		proba("reality-tcp", 995, proverka.Cel),
		proba("reality-tcp", 110, proverka.Zamerz),
	} {
		primenitHod(&x, upravlenie.Hod{Shag: upravlenie.ShagPodbor, Klyuch: &it}, avast)
	}
	var pomehiHoda []string
	for _, k := range x.Proverki {
		pomehiHoda = append(pomehiHoda, k.Pomeha)
	}
	if !slices.Equal(pomehiHoda, []string{"Avast", "", "", "", "", "Avast"}) {
		t.Fatalf("ход: %+v", x.Proverki)
	}
	k := proba("reality-tcp", 993, proverka.NetOtveta)
	itog := upravlenie.Itog{Klyuchi: []konfig.Klyuch{{Server: k.Server}}, Proverki: []proverka.Itog{k}}
	if kl := klyuchiHoda(itog, avast); len(kl) != 1 || kl[0].Pomeha != "Avast" {
		t.Fatalf("ключи итога: %+v", kl)
	}
	if kl := klyuchiHoda(itog, nil); kl[0].Pomeha != "" {
		t.Fatalf("ключи без Avast: %+v", kl)
	}
}

// Один вердикт детектора на два вида это одна строка с обоими ключами, а
// причина для записи о сервере собирается только из того, что про сеть.
func TestPreduprezhdeniyaVpsVmeste(t *testing.T) {
	d := &blok.Itog{Vid: blok.OtvetNeVozvrashchaetsya}
	drugoy := &blok.Itog{Vid: blok.Glushat}
	pr := preduprezhdeniyaVps([]upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NetSerta},
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 50443, Detektor: d},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 40443, Detektor: d},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: drugoy},
	}, nil)
	if len(pr) != 3 || pr[0].Kod != protokol.KodVpsNetSerta || pr[0].IzSeti {
		t.Fatalf("предупреждения %+v", pr)
	}
	if !pr[1].IzSeti || !strings.Contains(pr[1].Tekst, "порты 50443 (TCP) и 40443 (UDP)") {
		t.Fatalf("общий вердикт %+v", pr[1])
	}
	if !strings.Contains(pr[2].Tekst, "hysteria2 на порту 8443 не прошёл") {
		t.Fatalf("другой вердикт %+v", pr[2])
	}
	g := preduprezhdenieVps([]upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443, Detektor: drugoy},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: drugoy},
	}, nil)
	if !strings.HasSuffix(g.Tekst, "reality на порту 443 и hysteria2 на порту 8443 не прошли") {
		t.Fatalf("два ключа: %q", g.Tekst)
	}
	// Адрес закрыт для одного вида: проверять с телефона только его.
	zakryt := &blok.Itog{Vid: blok.NeDohodit}
	odin := preduprezhdenieVps([]upravlenie.Preduprezhdenie{{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: zakryt}}, nil)
	oba := preduprezhdenieVps([]upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443, Detektor: zakryt},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: zakryt},
	}, nil)
	if !strings.Contains(odin.Tekst, "Проверь hysteria2 с телефона") || !strings.Contains(odin.Tekst, "не откроется,") ||
		!strings.Contains(oba.Tekst, "Проверь их с телефона") || !strings.Contains(oba.Tekst, "не откроются,") {
		t.Fatalf("адрес закрыт: %q / %q", odin.Tekst, oba.Tekst)
	}
	tekst := tekstSeti(pr)
	if strings.Contains(tekst, "сертификат") || !strings.Contains(tekst, ". По дороге") {
		t.Fatalf("причина для записи %q", tekst)
	}
	if tekstSeti(pr[:1]) != "" {
		t.Fatal("причина без предупреждений о сети")
	}
	// Своя причина вида называет только его ключи: она останется, когда
	// проверка снимет другой вид.
	obshchaya, poVidam := prichinySeti([]upravlenie.Preduprezhdenie{
		{Vid: upravlenie.NeProshyol, Transport: "reality-tcp", Port: 443, Detektor: d},
		{Vid: upravlenie.NeProshyol, Transport: "hy2", Port: 8443, Detektor: d},
	}, []string{"reality-tcp", "hy2"}, nil)
	if !strings.Contains(obshchaya, "порты 443 (TCP) и 8443 (UDP)") || !strings.HasSuffix(poVidam["reality-tcp"], "порт 443 (TCP)") ||
		!strings.HasSuffix(poVidam["hy2"], "порт 8443 (UDP)") {
		t.Fatalf("причины %q %+v", obshchaya, poVidam)
	}
	if _, poVidam := prichinySeti(nil, []string{"hy2"}, nil); poVidam != nil {
		t.Fatalf("свои причины одного вида: %+v", poVidam)
	}
	if tekst := tekstVidov([]string{"reality-tcp", "hy2"}, map[string]string{"reality-tcp": "a", "hy2": "a"}); tekst != "a" {
		t.Fatalf("одинаковые причины: %q", tekst)
	}
	if tekst := tekstVidov([]string{"reality-tcp", "hy2"}, map[string]string{"reality-tcp": "a", "hy2": "b"}); tekst != "a. B" {
		t.Fatalf("две причины: %q", tekst)
	}
}

func TestTekstKruga(t *testing.T) {
	for _, v := range []blok.Vid{blok.NeDohodit, blok.ServerMolchit, blok.MolchitNapryamuyu, blok.Glushat, blok.Podmena, blok.OtvetNeVozvrashchaetsya} {
		if tekstKruga(v) == "" {
			t.Errorf("%s без текста", v)
		}
	}
	for _, v := range []blok.Vid{blok.Dostupen, blok.BelyeSpiski, blok.NeSostoyalas} {
		if tekstKruga(v) != "" {
			t.Errorf("%s: круг не отрицательный, а текст есть", v)
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

// tekstNePustoy гонит находки через настоящий осмотр: метки портов и
// vpn-uzel пишет Osmotr.chuzhoe, и смена их вида здесь покраснеет.
func tekstNePustoy(t *testing.T, o *server.Osmotr) string {
	t.Helper()
	for _, z := range o.Zamechaniya(time.Now()) {
		if z.Kod == server.KodNePustoy {
			return tekstZamechaniya(z)
		}
	}
	t.Fatal("осмотр не нашёл чужого")
	return ""
}

// Чужое на сервере называется словами, а не метками осмотра; программа и её
// служба называются один раз; слушатель без имени процесса не пишется «?».
func TestTekstZamechaniyaNePustoy(t *testing.T) {
	o := osmotrChistyy()
	o.Chuzhoe = []string{"programma nginx", "yunit nginx.service", "yunit xray.service", "katalog /etc/xray"}
	o.Slushayut = []server.Slushatel{
		{Proto: "tcp", Adres: "0.0.0.0", Port: 80, Process: "nginx"},
		{Proto: "tcp", Adres: "0.0.0.0", Port: 443},
	}
	o.VpnUzel = "vpn-uzel 0.6.0"
	tekst := tekstNePustoy(t, o)
	zhdu := "сервер не пустой: nginx, xray, папка /etc/xray, порт 80 занят nginx, порт 443 занят, " +
		"остатки прежней установки Affory. Ставь на чистую систему"
	if tekst != zhdu {
		t.Fatalf("\nесть  %q\nждали %q", tekst, zhdu)
	}
	for _, metka := range []string{"programma", "yunit", "katalog", "port tcp", ".service", "?", "vpn-uzel"} {
		if strings.Contains(tekst, metka) {
			t.Errorf("в тексте осталась метка %q: %q", metka, tekst)
		}
	}
	// TCP и UDP одного порта не склеиваются в один пункт.
	if s := chuzhoeDlyaCheloveka([]string{"port tcp/443 x", "port udp/443 x"}); s != "порт 443 занят x, порт 443/udp занят x" {
		t.Errorf("TCP и UDP одного порта: %q", s)
	}
}

// Длинный список режется по целым пунктам с хвостом «и ещё N».
func TestTekstZamechaniyaNePustoyDlinnyy(t *testing.T) {
	o := osmotrChistyy()
	for i := range 30 {
		o.Chuzhoe = append(o.Chuzhoe, fmt.Sprintf("katalog /opt/chuzhaya-ustanovka-%02d", i))
	}
	tekst := tekstNePustoy(t, o)
	spisok, ok := strings.CutPrefix(tekst, "сервер не пустой: ")
	spisok, ok2 := strings.CutSuffix(spisok, ". Ставь на чистую систему")
	if !ok || !ok2 {
		t.Fatalf("текст не той формы: %q", tekst)
	}
	punkty, hvost, est := strings.Cut(spisok, " и ещё ")
	if !est || strings.Contains(spisok, "...") {
		t.Fatalf("список не обрезан по пунктам: %q", spisok)
	}
	vidno := strings.Split(punkty, ", ")
	if n, err := strconv.Atoi(hvost); err != nil || len(vidno)+n != 30 || len([]rune(punkty)) > dlinaSoServera {
		t.Fatalf("видно %d, в хвосте %q, длина %d: %q", len(vidno), hvost, len([]rune(punkty)), spisok)
	}
	for _, p := range vidno {
		if !strings.HasPrefix(p, "папка /opt/chuzhaya-ustanovka-") || len(p) != len("папка /opt/chuzhaya-ustanovka-00") {
			t.Fatalf("пункт разорван: %q", p)
		}
	}
}

// Своя отметка: законченная установка ставится поверх со сменой ключей,
// незаконченная продолжается, чужая отправляет брать под управление.
func TestTekstZamechaniyaUzheStoit(t *testing.T) {
	for _, sluchay := range []struct {
		chto []string
		stop bool
		est  string
	}{
		{[]string{"gotovo", "0.6.0"}, false, "сменит ключи VPN"},
		{[]string{"ustanovka", ""}, false, "она продолжится"},
		{[]string{"gotovo", "0.6.0"}, true, "возьми его под управление"},
	} {
		tekst := tekstZamechaniya(server.Zamechanie{Kod: server.KodUzheStoit, Chto: sluchay.chto, Stop: sluchay.stop})
		if !strings.Contains(tekst, sluchay.est) {
			t.Errorf("%v stop=%v: %q", sluchay.chto, sluchay.stop, tekst)
		}
	}
}
