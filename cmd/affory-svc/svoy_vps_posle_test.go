package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/konfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/porty"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/server"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/ssh"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/svoyvps/upravlenie"
)

// itogSvoego собирает итог установки на порты reality и hy2.
func itogSvoego(t *testing.T, kh ssh.KlyuchHosta, reality, hy2 int) upravlenie.Itog {
	t.Helper()
	sek, err := konfig.NovyeSekrety("dl.google.com")
	if err != nil {
		t.Fatal(err)
	}
	v := konfig.Vhody{Imya: "Дача", Adres: adresSvoego, Reality: []int{reality}, Hy2: []int{hy2}}
	klyuchi, err := konfig.Klyuchi(sek, v)
	if err != nil {
		t.Fatal(err)
	}
	return upravlenie.Itog{Adres: adresSvoego, Sekrety: sek, Vhody: v, Klyuchi: klyuchi,
		Prodlenie: porty.Cherez80, Versiya: "0.6.0", Vhod: upravlenie.Svedeniya{KlyuchHosta: kh}}
}

// postavitSvoy кладёт в набор свой сервер, как его оставила бы установка.
func postavitSvoy(t *testing.T, s *Sluzhba, kh ssh.KlyuchHosta, sKlyuchom bool) (string, []string) {
	t.Helper()
	id := idSvoego(netip.MustParseAddr(adresSvoego))
	ids, err := s.zapisatSvoy(id, osnovaZapisi{Imya: "Дача", Port: 2222, Polzovatel: "root"}, itogSvoego(t, kh, 443, 8443))
	if err != nil {
		t.Fatal(err)
	}
	if sKlyuchom {
		if _, err := s.zavestiKlyuchUpravleniya(id); err != nil {
			t.Fatal(err)
		}
	}
	return id, ids
}

func TestSvoyVpsSnesti(t *testing.T) {
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
	var zapros ssh.Zapros
	s.vpsSnesti = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros) (upravlenie.SnosItog, error) {
		zapros = z
		return upravlenie.SnosItog{Snos: server.Snos{ParolVernulsya: true}}, nil
	}
	if otv := s.svoyVpsSnesti(kadrVps(t, "svoyVpsSnesti", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Snyat == nil || !h.Snyat.ParolVernulsya {
		t.Fatalf("снос: %+v", h)
	}
	if zapros.Adres != adresSvoego || zapros.Port != 2222 || zapros.Vhod.Polzovatel != "root" ||
		zapros.Vhod.Podpis == nil || !bytes.Equal(zapros.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
		zapros.Izvesten == nil || !bytes.Equal(zapros.Izvesten.Telo, kh.Telo) {
		t.Fatalf("вход сноса: %+v", zapros)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || len(n.SvoiServery) != 0 || n.Vybran != "" || n.Rezhim != protokol.RezhimRuchnoy {
		t.Fatalf("после сноса в наборе: %+v", n)
	}
	if _, ok, _ := s.klyuchUpravleniya(id); ok {
		t.Fatal("ключ управления пережил снос")
	}
}

// Сервер снесён и запись ушла, а файл ключа не стёрся: строки ключа на
// сервере уже нет, снос удачный и без предупреждений.
func TestSvoyVpsSnestiKlyuchOstalsya(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	s.vpsSnesti = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.SnosItog, error) {
		// Запасная копия ключа каталогом с файлом: os.Remove на ней падает.
		if err := os.MkdirAll(filepath.Join(put+sostoyanie.RasshirenieZapasa, "x"), 0o700); err != nil {
			t.Fatal(err)
		}
		return upravlenie.SnosItog{}, nil
	}
	s.svoyVpsSnesti(kadrVps(t, "svoyVpsSnesti", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Snyat == nil || len(h.Preduprezhdeniya) != 0 {
		t.Fatalf("снос с оставшимся ключом: %+v", h)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || len(n.SvoiServery) != 0 {
		t.Fatal("после сноса в наборе остались ключи или запись")
	}
}

// Нечитаемый ключ управления в списке это не «ключа нет».
func TestSvoyVpsSpisokKlyuchNeChitaetsya(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte("не DPAPI"), 0o600); err != nil {
		t.Fatal(err)
	}
	var spisok struct {
		Servery []SvoyServerDlyaEkrana `json:"servery"`
	}
	if err := json.Unmarshal(s.svoyVpsSpisok(protokol.Kadr{Imya: "svoyVpsSpisok"}).Telo, &spisok); err != nil {
		t.Fatal(err)
	}
	if len(spisok.Servery) != 1 || spisok.Servery[0].KlyuchUpravleniya || spisok.Servery[0].KlyuchNeChitaetsya == "" {
		t.Fatalf("список: %+v", spisok.Servery)
	}
}

func TestSvoyVpsSnestiOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	s.vpsSnesti = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.SnosItog, error) {
		return upravlenie.SnosItog{}, &upravlenie.NeChisto{Chto: []string{"программа vpn-uzel"}}
	}
	s.svoyVpsSnesti(kadrVps(t, "svoyVpsSnesti", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	if h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsUstanovka || !strings.Contains(h.Otkaz.Tekst, "vpn-uzel") {
		t.Fatalf("отказ сноса: %+v", h.Otkaz)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
		t.Fatal("неудачный снос убрал ключи из клиента")
	}
	if _, ok, _ := s.klyuchUpravleniya(id); !ok {
		t.Fatal("неудачный снос стёр ключ управления")
	}
}

func TestSvoyVpsSnestiBezKlyucha(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), false)
	if kod := kodOtkaza(s.svoyVpsSnesti(kadrVps(t, "svoyVpsSnesti", map[string]string{"id": id}))); kod != protokol.KodVpsVzyat {
		t.Fatalf("снос без ключа управления: %q", kod)
	}
	if kod := kodOtkaza(s.svoyVpsSnesti(kadrVps(t, "svoyVpsSnesti", map[string]string{"id": "svoy-0000000000000000"}))); kod != protokol.KodTeloNegodno {
		t.Fatalf("снос чужого id: %q", kod)
	}
}

// Запись пришла импортом без ключа управления; на сервере hy2 уже на другом
// порту. Захват берёт ключи с сервера, прежние уходят.
func TestSvoyVpsVzyat(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye := postavitSvoy(t, s, kh, false)
	novyy := itogSvoego(t, kh, 443, 9443)
	var v upravlenie.Vzyatie
	s.vpsVzyat = func(_ context.Context, x upravlenie.Vzyatie) (upravlenie.Itog, error) {
		v = x
		return novyy, nil
	}
	telo := zaprosVzyatiya{dostupVps: dostupVps{Adres: adresSvoego, Port: 2222, Polzovatel: "root", Parol: "x"}}
	if otv := s.svoyVpsVzyat(kadrVps(t, "svoyVpsVzyat", telo)); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Ustanovlen == nil || len(h.Ustanovlen.Ids) != 2 {
		t.Fatalf("захват: %+v", h)
	}
	if v.Vhod.Izvesten == nil || !bytes.Equal(v.Vhod.Izvesten.Telo, kh.Telo) || v.Imya != "Дача" ||
		v.Vhod.Vhod.Parol != "x" || !strings.HasPrefix(v.Kommentariy, "affory ") {
		t.Fatalf("захват ушёл с %+v", v)
	}
	nash, ok, err := s.klyuchUpravleniya(id)
	if err != nil || !ok || !bytes.Equal(nash.PublicKey().Marshal(), v.Upravlenie.PublicKey().Marshal()) {
		t.Fatal("ключ управления не заведён или не тот")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
		t.Fatalf("набор: %d ключей, %d своих", len(n.Servery), len(n.SvoiServery))
	}
	for _, srv := range n.Servery {
		if srv.Id == starye[1] {
			t.Fatal("прежний ключ hy2 остался рядом с новым")
		}
	}
	if n.SvoiServery[0].Hy2 != 9443 {
		t.Fatalf("запись: %+v", n.SvoiServery[0])
	}
}

func TestSvoyVpsVzyatOtkaz(t *testing.T) {
	s := sPustymNaborom(t)
	s.vpsVzyat = func(context.Context, upravlenie.Vzyatie) (upravlenie.Itog, error) {
		return upravlenie.Itog{}, upravlenie.ErrNeDoshla
	}
	telo := zaprosVzyatiya{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}}
	s.svoyVpsVzyat(kadrVps(t, "svoyVpsVzyat", telo))
	if h := dozhdatsyaVps(t, s); h.Otkaz == nil || !strings.Contains(h.Otkaz.Tekst, "заново") {
		t.Fatalf("отказ захвата: %+v", h.Otkaz)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.SvoiServery) != 0 {
		t.Fatal("неудачный захват записал сервер")
	}
	telo.KlyuchHosta = "!"
	if kod := kodOtkaza(s.svoyVpsVzyat(kadrVps(t, "svoyVpsVzyat", telo))); kod != protokol.KodTeloNegodno {
		t.Fatalf("битый ключ хоста: %q", kod)
	}
}

func TestSvoyVpsZabyt(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)

	pusk := make(chan struct{})
	s.vpsOsmotret = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros) (upravlenie.Otchet, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.Otchet{}, ctx.Err()
	}
	s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
	<-pusk
	if kod := kodOtkaza(s.svoyVpsZabyt(kadrVps(t, "svoyVpsZabyt", map[string]string{"id": id}))); kod != protokol.KodVpsZanyato {
		t.Fatalf("«забыть» посреди дела: %q", kod)
	}
	s.svoyVpsOtmenit(protokol.Kadr{Imya: "svoyVpsOtmenit"})
	dozhdatsyaVps(t, s)
	otperet, _ := s.zaperetVps(time.Now().Add(time.Minute), otkazObnovlenieIdyot)
	if otperet == nil {
		t.Fatal("запор не встал")
	}
	if kod := kodOtkaza(s.svoyVpsZabyt(kadrVps(t, "svoyVpsZabyt", map[string]string{"id": id}))); kod != protokol.KodVpsZanyato {
		t.Fatalf("«забыть» посреди обновления: %q", kod)
	}
	otperet()

	// Ключ не стирается: не меняется ничего, и повтор доделает.
	put, err := s.putKlyuchaUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	zapas := put + sostoyanie.RasshirenieZapasa
	if err := os.MkdirAll(filepath.Join(zapas, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if otv := s.svoyVpsZabyt(kadrVps(t, "svoyVpsZabyt", map[string]string{"id": id})); otv.Oshib == nil {
		t.Fatal("«забыть» с нестираемым ключом прошло")
	}
	if n, err := s.nabor(); err != nil || len(n.SvoiServery) != 1 || len(n.Servery) != 2 {
		t.Fatalf("неудачное «забыть» тронуло набор: %v", err)
	}
	if _, ok, err := s.klyuchUpravleniya(id); err != nil || !ok {
		t.Fatalf("неудачное «забыть» стёрло ключ управления: %v", err)
	}
	if err := os.RemoveAll(zapas); err != nil {
		t.Fatal(err)
	}

	if otv := s.svoyVpsZabyt(kadrVps(t, "svoyVpsZabyt", map[string]string{"id": id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	// «Забыть» снимает свой запор за собой.
	otperet, _ = s.zaperetVps(time.Now().Add(time.Minute), "x")
	if otperet == nil {
		t.Fatal("«забыть» оставило запор")
	}
	otperet()
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || len(n.SvoiServery) != 0 {
		t.Fatal("«забыть» оставило ключи или запись")
	}
	if _, ok, _ := s.klyuchUpravleniya(id); ok {
		t.Fatal("«забыть» оставило ключ управления")
	}
	if kod := kodOtkaza(s.svoyVpsZabyt(kadrVps(t, "svoyVpsZabyt", map[string]string{"id": id}))); kod != protokol.KodTeloNegodno {
		t.Fatalf("повторное «забыть»: %q", kod)
	}
}

func TestOtkazVpsZapisi(t *testing.T) {
	o := otkazVps(&oshibkaZapisi{err: ErrKandidatZanyat})
	if o.Kod != protokol.KodKandidatZanyat {
		t.Fatalf("отказ записи набора: %+v", o)
	}
	if o := otkazVps(server.ErrNetUstanovki); o.Kod != protokol.KodVpsUstanovka || o.Tekst == "" {
		t.Fatalf("нет установки: %+v", o)
	}
	if !errors.Is(&oshibkaZapisi{err: ErrKandidatZanyat}, ErrKandidatZanyat) {
		t.Fatal("отказ записи не разворачивается")
	}
}
