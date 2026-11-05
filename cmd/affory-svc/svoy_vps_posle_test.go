package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
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
	return itogSoSekretami(t, sek, kh, reality, hy2)
}

// itogSoSekretami это итог с заданными секретами: подбор портов заново
// меняет порты, а секреты оставляет.
func itogSoSekretami(t *testing.T, sek konfig.Sekrety, kh ssh.KlyuchHosta, reality, hy2 int) upravlenie.Itog {
	t.Helper()
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
	if h.Faza != fazaGotovo || h.Snyat == nil || !h.Snyat.ParolVernulsya || h.Server != id {
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

func TestSvoyVpsOtpustit(t *testing.T) {
	for _, snyat := range []bool{true, false} {
		s := sPustymNaborom(t)
		kh := klyuchHostaDlyaTesta(t)
		id, _ := postavitSvoy(t, s, kh, true)
		nash, _, err := s.klyuchUpravleniya(id)
		if err != nil {
			t.Fatal(err)
		}
		var zapros ssh.Zapros
		s.vpsOtpustit = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros) (upravlenie.OtpuskItog, error) {
			zapros = z
			return upravlenie.OtpuskItog{Snyat: snyat, Vhod: upravlenie.Svedeniya{Put: "напрямую"}}, nil
		}
		if otv := s.svoyVpsOtpustit(kadrVps(t, "svoyVpsOtpustit", map[string]string{"id": id})); otv.Oshib != nil {
			t.Fatal(otv.Oshib)
		}
		h := dozhdatsyaVps(t, s)
		if h.Vid != vidOtpusk || h.Faza != fazaGotovo || h.Otpushchen == nil || h.Otpushchen.KlyuchaNeBylo == snyat ||
			h.Server != id || h.Put != "напрямую" || len(h.Preduprezhdeniya) != 0 {
			t.Fatalf("снят %v: %+v %+v", snyat, h, h.Otpushchen)
		}
		if zapros.Adres != adresSvoego || zapros.Port != 2222 || zapros.Vhod.Podpis == nil ||
			!bytes.Equal(zapros.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
			zapros.Izvesten == nil || !bytes.Equal(zapros.Izvesten.Telo, kh.Telo) {
			t.Fatalf("вход: %+v", zapros)
		}
		n, err := s.nabor()
		if err != nil {
			t.Fatal(err)
		}
		if len(n.Servery) != 0 || len(n.SvoiServery) != 0 {
			t.Fatalf("после «забыть и снять ключ» в наборе: %+v", n)
		}
		if _, ok, _ := s.klyuchUpravleniya(id); ok {
			t.Fatal("ключ управления остался")
		}
	}
}

// Сервер не пустил или ключ - последний вход: в клиенте не меняется ничего.
func TestSvoyVpsOtpustitOtkaz(t *testing.T) {
	for _, c := range []struct {
		imya  string
		err   error
		kod   string
		tekst string
	}{
		{"последний вход", server.ErrPosledniyVhod, protokol.KodVpsPosledniyVhod, "последний вход root"},
		// Проверить не вышло: текст не утверждает, что других входов нет.
		{"входы не проверены", server.ErrVhodyNeProvereny, protokol.KodVpsPosledniyVhod, "не вышло проверить"},
		{"сервер молчит", &upravlenie.Molchanie{Pryamo: errors.New("тишина")}, protokol.KodVpsNeOtvechaet, ""},
	} {
		t.Run(c.imya, func(t *testing.T) {
			s := sPustymNaborom(t)
			id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
			s.vpsOtpustit = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.OtpuskItog, error) {
				return upravlenie.OtpuskItog{}, c.err
			}
			s.svoyVpsOtpustit(kadrVps(t, "svoyVpsOtpustit", map[string]string{"id": id}))
			if h := dozhdatsyaVps(t, s); h.Otkaz == nil || h.Otkaz.Kod != c.kod || !strings.Contains(h.Otkaz.Tekst, c.tekst) || h.Otpushchen != nil {
				t.Fatalf("отказ: %+v %+v", h, h.Otkaz)
			}
			n, err := s.nabor()
			if err != nil {
				t.Fatal(err)
			}
			if len(n.Servery) != 2 || len(n.SvoiServery) != 1 {
				t.Fatal("неудача убрала ключи из клиента")
			}
			if _, ok, _ := s.klyuchUpravleniya(id); !ok {
				t.Fatal("неудача стёрла ключ управления")
			}
		})
	}
	// Без ключа управления снимать нечем: окно ведёт к «забыть».
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), false)
	if kod := kodOtkaza(s.svoyVpsOtpustit(kadrVps(t, "svoyVpsOtpustit", map[string]string{"id": id}))); kod != protokol.KodVpsVzyat {
		t.Fatalf("без ключа управления: %q", kod)
	}
}

// Запись пришла импортом без ключа управления; на сервере hy2 уже на другом
// порту. Захват берёт ключи с сервера, прежние уходят.
func TestSvoyVpsVzyat(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, starye := postavitSvoy(t, s, kh, false)
	if err := s.pravitNabor(func(n *Nabor) error { n.SvoiServery[0].Postavlen = postavlenDlyaTesta; return nil }); err != nil {
		t.Fatal(err)
	}
	novyy := itogSvoego(t, kh, 443, 9443)
	novyy.ParolVyklyuchen = true
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
	if h.Faza != fazaGotovo || h.Ustanovlen == nil || len(h.Ustanovlen.Ids) != 2 || h.Server != idSvoego(netip.MustParseAddr(adresSvoego)) {
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
	if n.SvoiServery[0].Hy2 != 9443 || !n.SvoiServery[0].ParolVyklyuchen || !n.SvoiServery[0].Postavlen.Equal(postavlenDlyaTesta) {
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

func TestSvoyVpsPereimenovat(t *testing.T) {
	s := sPustymNaborom(t)
	id, ids := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	chuzhoy := protokol.Server{Id: "chuzhoy", Imya: "Чужой", Transport: "hy2", Host: "198.51.100.7", Port: 443, Parol: "p", Sni: "198.51.100.7"}
	if err := s.pravitNabor(func(n *Nabor) error { n.Servery = append(n.Servery, chuzhoy); return nil }); err != nil {
		t.Fatal(err)
	}
	pereimenovat := func(id, imya string) protokol.Kadr {
		return s.svoyVpsPereimenovat(kadrVps(t, "svoyVpsPereimenovat", map[string]string{"id": id, "imya": imya}))
	}
	// Ядро поднято на ключе reality этого сервера.
	s.mu.Lock()
	s.serveryYadra = []protokol.Server{{Id: ids[0], Imya: "Дача, reality"}, {Id: "chuzhoy", Imya: "Чужой"}}
	s.nesushchiyId, s.nesushchiyImya = ids[0], "Дача, reality"
	s.mu.Unlock()

	// Знаки, которые ссылка кодирует, доезжают до ключа на телефоне как есть.
	if otv := pereimenovat(id, "  Офис #1 %20 & ?‮ "); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	s.mu.Lock()
	nesushchiy, yadro := s.nesushchiyImya, slices.Clone(s.serveryYadra)
	s.mu.Unlock()
	if nesushchiy != "Офис #1 %20 & ?, reality" || yadro[0].Imya != nesushchiy || yadro[1].Imya != "Чужой" {
		t.Fatalf("имя несущего %q, снимок ядра %+v", nesushchiy, yadro)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.SvoiServery) != 1 || n.SvoiServery[0].Imya != "Офис #1 %20 & ?" {
		t.Fatalf("запись после переименования: %+v", n.SvoiServery)
	}
	imena := map[string]string{}
	for _, srv := range n.Servery {
		imena[srv.Id] = srv.Imya
		if !slices.Contains(ids, srv.Id) {
			continue
		}
		ssylka, err := ssylki.Sobrat(srv)
		if err != nil {
			t.Fatal(err)
		}
		r, err := ssylki.Razobrat(ssylka)
		if err != nil || r.Imya != srv.Imya {
			t.Fatalf("имя %q в ссылке %q вернулось %q: %v", srv.Imya, ssylka, r.Imya, err)
		}
	}
	want := map[string]string{ids[0]: "Офис #1 %20 & ?, reality", ids[1]: "Офис #1 %20 & ?, hysteria2", "chuzhoy": "Чужой"}
	if !reflect.DeepEqual(imena, want) {
		t.Fatalf("имена ключей: %v", imena)
	}
	// Запор снят за собой.
	otperet, _ := s.zaperetVps(time.Now().Add(time.Minute), "x")
	if otperet == nil {
		t.Fatal("переименование оставило запор")
	}

	if kod := kodOtkaza(pereimenovat(id, "Дача")); kod != protokol.KodVpsZanyato {
		t.Fatalf("переименование посреди запора: %q", kod)
	}
	otperet()
	if otv := pereimenovat(id, strings.Repeat("я", 30)); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	if n, err := s.nabor(); err != nil || n.SvoiServery[0].Imya != strings.Repeat("я", dlinaImeni) {
		t.Fatalf("длинное название не обрезано: %v", err)
	}
	for _, plohoe := range []struct{ id, imya string }{{id, " ​ "}, {"svoy-net", "Дача"}} {
		if kod := kodOtkaza(pereimenovat(plohoe.id, plohoe.imya)); kod != protokol.KodTeloNegodno {
			t.Fatalf("%q, %q: %q", plohoe.id, plohoe.imya, kod)
		}
	}
}

// Переименованию мешают только задания, что пишут запись этого сервера
// целиком: осмотр и прочие идут дальше.
func TestSvoyVpsPereimenovatPosredDela(t *testing.T) {
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	pereimenovat := func(imya string) protokol.Kadr {
		return s.svoyVpsPereimenovat(kadrVps(t, "svoyVpsPereimenovat", map[string]string{"id": id, "imya": imya}))
	}

	pusk := make(chan struct{})
	s.vpsOsmotret = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros) (upravlenie.Otchet, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.Otchet{}, ctx.Err()
	}
	s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
	<-pusk
	if otv := pereimenovat("Офис"); otv.Oshib != nil {
		t.Fatalf("переименование посреди осмотра: %v", otv.Oshib)
	}
	s.svoyVpsOtmenit(protokol.Kadr{Imya: "svoyVpsOtmenit"})
	dozhdatsyaVps(t, s)

	pusk = make(chan struct{})
	s.vpsPodobratPorty = func(ctx context.Context, _ upravlenie.Podbor) (upravlenie.Itog, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.Itog{}, ctx.Err()
	}
	if otv := s.svoyVpsPodobratPorty(kadrVps(t, "svoyVpsPodobratPorty", zaprosPodboraVps{Id: id})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	<-pusk
	if kod := kodOtkaza(pereimenovat("Дача")); kod != protokol.KodVpsZanyato {
		t.Fatalf("переименование посреди подбора: %q", kod)
	}
	s.svoyVpsOtmenit(protokol.Kadr{Imya: "svoyVpsOtmenit"})
	dozhdatsyaVps(t, s)
	if z := zapisPoId(t, s, id); z.Imya != "Офис" {
		t.Fatalf("название после подбора: %q", z.Imya)
	}
}

// Правила, отставшие после записи набора, не делают переименование отказом:
// названия уже сменились, а отставание приходит предупреждением.
func TestSvoyVpsPereimenovatPravilaOtstali(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	s.sobratAdresa = adresaIzNabora(s)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	id, ids := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), false)
	// Тот же список адресов поверх стоящего замка не пересобирается. Отстать
	// правила могут, только если прошлая пересборка оборвалась: она стирает
	// отпечаток замка, и следующая запись набора пробует снова.
	s.muZaslon.Lock()
	s.otpechatokZamka = ""
	s.muZaslon.Unlock()
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error {
		return fmt.Errorf("%w: профиль Domain", set.ErrBrandmauerVyklyuchen)
	}
	otv := s.svoyVpsPereimenovat(kadrVps(t, "svoyVpsPereimenovat", map[string]string{"id": id, "imya": "Офис"}))
	if otv.Oshib != nil {
		t.Fatalf("отставшие правила дали отказ: %v", otv.Oshib)
	}
	var telo struct {
		Preduprezhdenie *OtkazVps `json:"preduprezhdenie"`
	}
	if err := json.Unmarshal(otv.Telo, &telo); err != nil || telo.Preduprezhdenie == nil || telo.Preduprezhdenie.Kod != protokol.KodFirewallDisabled {
		t.Fatalf("предупреждение %+v: %v", telo.Preduprezhdenie, err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	for _, srv := range n.Servery {
		if slices.Contains(ids, srv.Id) && !strings.HasPrefix(srv.Imya, "Офис, ") {
			t.Fatalf("ключ %s после переименования: %q", srv.Id, srv.Imya)
		}
	}
}

func TestOtkazVpsZapisi(t *testing.T) {
	o := otkazVps(&oshibkaZapisi{err: ErrKandidatZanyat})
	if o.Kod != protokol.KodKandidatZanyat {
		t.Fatalf("отказ записи набора: %+v", o)
	}
	if o := otkazVps(server.ErrNetUstanovki); o.Kod != protokol.KodVpsPostavitZanovo || o.Tekst == "" {
		t.Fatalf("нет установки: %+v", o)
	}
	if o := otkazVps(fmt.Errorf("подбор: %w", upravlenie.ErrNeDoshla)); o.Kod != protokol.KodVpsPostavitZanovo || o.Tekst == "" {
		t.Fatalf("не дошла: %+v", o)
	}
	if !errors.Is(&oshibkaZapisi{err: ErrKandidatZanyat}, ErrKandidatZanyat) {
		t.Fatal("отказ записи не разворачивается")
	}
}

// parolNeZhdali подставляет переключение пароля, которого тест не ждёт.
func parolNeZhdali(t *testing.T, chto string) func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
	return func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		t.Errorf("%s не ждали", chto)
		return upravlenie.ParolItog{}, errors.New("не ждали")
	}
}

func TestSvoyVpsParol(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	nash, _, err := s.klyuchUpravleniya(id)
	if err != nil {
		t.Fatal(err)
	}
	var zapros ssh.Zapros
	s.vpsVyklyuchitParol = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros) (upravlenie.ParolItog, error) {
		zapros = z
		return upravlenie.ParolItog{Vhod: upravlenie.Svedeniya{Put: "напрямую"}}, nil
	}
	s.vpsVklyuchitParol = parolNeZhdali(t, "включение")
	if otv := s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": true})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h := dozhdatsyaVps(t, s)
	if h.Vid != vidParol || h.Faza != fazaGotovo || h.Parol == nil || !h.Parol.Vyklyuchen || h.Parol.Snaruzhi != "" ||
		h.Server != id || h.Put != "напрямую" || len(h.Preduprezhdeniya) != 0 {
		t.Fatalf("выключение: %+v %+v", h, h.Parol)
	}
	if zapros.Adres != adresSvoego || zapros.Port != 2222 || zapros.Vhod.Podpis == nil ||
		!bytes.Equal(zapros.Vhod.Podpis.PublicKey().Marshal(), nash.PublicKey().Marshal()) ||
		zapros.Izvesten == nil || !bytes.Equal(zapros.Izvesten.Telo, kh.Telo) {
		t.Fatalf("вход: %+v", zapros)
	}
	if !zapisPoId(t, s, id).ParolVyklyuchen {
		t.Fatal("выключение не отмечено в записи")
	}

	// Включение: пароль всё равно выключен другой настройкой.
	fayl := "/etc/ssh/sshd_config.d/50-cloud-init.conf"
	s.vpsVyklyuchitParol = parolNeZhdali(t, "выключение")
	s.vpsVklyuchitParol = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		return upravlenie.ParolItog{Vhod: upravlenie.Svedeniya{Put: "через VPN"},
			Posle: server.Vklyuchenie{Parol: "vyklyuchen_chuzhim", Fayly: []string{fayl}}}, nil
	}
	if otv := s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": false})); otv.Oshib != nil {
		t.Fatal(otv.Oshib)
	}
	h = dozhdatsyaVps(t, s)
	if h.Faza != fazaGotovo || h.Parol == nil || h.Parol.Vyklyuchen || h.Parol.Snaruzhi != "vyklyuchen_chuzhim" ||
		len(h.Parol.Fayly) != 1 || h.Parol.Fayly[0] != fayl || h.Put != "через VPN" {
		t.Fatalf("включение: %+v %+v", h, h.Parol)
	}
	if zapisPoId(t, s, id).ParolVyklyuchen {
		t.Fatal("включение не снято с записи")
	}
}

func TestSvoyVpsParolOtkaz(t *testing.T) {
	for _, c := range []struct {
		imya       string
		vyklyuchit bool
		err        error
		kod        string
		tekst      []string
		// Отметка в записи после отказа, если до него она стояла.
		otmetka bool
	}{
		{"чужая настройка", true, &server.ParolChuzhoy{Fayly: []string{"/etc/ssh/sshd_config"}}, protokol.KodVpsParolChuzhoy,
			[]string{"в /etc/ssh/sshd_config", "осталось как было"}, false},
		{"чужая настройка без файла", true, &server.ParolChuzhoy{}, protokol.KodVpsParolChuzhoy,
			[]string{"найти не вышло"}, false},
		{"ключ не пустил", true, &upravlenie.KlyuchNePustil{Vhod: &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchNePrinyat}},
			protokol.KodVpsParol, []string{"сервер не принял ключ SSH", "включён обратно"}, false},
		{"ключ не пустил, пароль не вернулся", true,
			&upravlenie.KlyuchNePustil{Vhod: errors.New("обрыв"), Vozvrat: errors.New("обрыв")},
			protokol.KodVpsParol, []string{"таймер на сервере через 5 минут"}, false},
		{"скрипт выключения", true, &server.Oshibka{Shag: "parol", Kod: 1, Tekst: "sshd -t не принял настройку"},
			protokol.KodVpsParol, []string{"вход по паролю не переключился: sshd -t не принял"}, false},
		{"сервер молчит при выключении", true, &upravlenie.Molchanie{Pryamo: errors.New("тишина")},
			protokol.KodVpsNeOtvechaet, nil, false},
		// Неудачное включение оставляет наш файл, как был.
		{"скрипт включения", false, &server.Oshibka{Shag: "parol", Kod: 1}, protokol.KodVpsParol,
			[]string{"вход по паролю не переключился"}, true},
	} {
		t.Run(c.imya, func(t *testing.T) {
			s := sPustymNaborom(t)
			id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
			if err := s.otmetitProverku(id, func(x *SvoyServer) { x.ParolVyklyuchen = true }); err != nil {
				t.Fatal(err)
			}
			otkazParolya := func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
				return upravlenie.ParolItog{}, c.err
			}
			if c.vyklyuchit {
				s.vpsVyklyuchitParol, s.vpsVklyuchitParol = otkazParolya, parolNeZhdali(t, "включение")
			} else {
				s.vpsVyklyuchitParol, s.vpsVklyuchitParol = parolNeZhdali(t, "выключение"), otkazParolya
			}
			s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": c.vyklyuchit}))
			h := dozhdatsyaVps(t, s)
			if h.Otkaz == nil || h.Otkaz.Kod != c.kod || h.Parol != nil {
				t.Fatalf("отказ: %+v %+v", h, h.Otkaz)
			}
			for _, tk := range c.tekst {
				if !strings.Contains(h.Otkaz.Tekst, tk) {
					t.Fatalf("в тексте нет %q: %s", tk, h.Otkaz.Tekst)
				}
			}
			if z := zapisPoId(t, s, id); z.ParolVyklyuchen != c.otmetka {
				t.Fatalf("отметка пароля после отказа: %v", z.ParolVyklyuchen)
			}
		})
	}
	// Срок ожидания вложенного отказа доходит до окна, строка о нашем ключе
	// входа - нет: она про другое дело.
	o := otkazVps(&upravlenie.KlyuchNePustil{Vhod: &ssh.Otkaz{Prichina: ssh.PrichinaPodozhdi, Podozhdat: 30}})
	if o.Kod != protokol.KodVpsParol || o.Podozhdat != 30 || !strings.HasPrefix(o.Tekst, "пароль не выключен") {
		t.Fatalf("срок ожидания: %+v", o)
	}
	o = otkazVps(&upravlenie.KlyuchNePustil{Vhod: &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchNePrinyat, KlyuchOstalsya: true}})
	if o.Kod != protokol.KodVpsParol || o.KlyuchOstalsya {
		t.Fatalf("строка о нашем ключе: %+v", o)
	}
	s := sPustymNaborom(t)
	id, _ := postavitSvoy(t, s, klyuchHostaDlyaTesta(t), true)
	for _, telo := range []any{map[string]any{"id": id}, map[string]any{"id": id, "vyklyuchit": "da"}} {
		if kod := kodOtkaza(s.svoyVpsParol(kadrVps(t, "svoyVpsParol", telo))); kod != protokol.KodTeloNegodno {
			t.Fatalf("тело %v: %q", telo, kod)
		}
	}
	// Без ключа управления входить нечем.
	s = sPustymNaborom(t)
	id, _ = postavitSvoy(t, s, klyuchHostaDlyaTesta(t), false)
	if kod := kodOtkaza(s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": true}))); kod != protokol.KodVpsVzyat {
		t.Fatalf("без ключа управления: %q", kod)
	}
}

func prinyatKlyuch(t *testing.T, s *Sluzhba, id, otpechatok string) protokol.Kadr {
	t.Helper()
	return s.svoyVpsPrinyatKlyuchHosta(kadrVps(t, "svoyVpsPrinyatKlyuchHosta", map[string]string{"id": id, "otpechatok": otpechatok}))
}

// Сервер предъявил другой ключ: запись помнит его, карточка видит отпечаток,
// «Систему переустановили» принимает ровно этот ключ.
func TestSvoyVpsPrinyatKlyuchHosta(t *testing.T) {
	s := sPustymNaborom(t)
	kh, novyy := klyuchHostaDlyaTesta(t), klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.ParolVyklyuchen = true }); err != nil {
		t.Fatal(err)
	}
	var zapros ssh.Zapros
	s.vpsProverit = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		zapros = z
		return upravlenie.ZdorovyeServera{}, fmt.Errorf("вход: %w", &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &novyy})
	}
	// Новый ключ записан до конца задания: окно перечитывает список по
	// событию конца. Каждая запись набора смотрит, идёт ли ещё задание.
	pisat := s.sekretyPisat
	var poKoncu atomic.Bool
	s.sekretyPisat = func(b []byte) error {
		if s.svoyVpsIdyot() == "" {
			poKoncu.Store(true)
		}
		return pisat(b)
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	h := dozhdatsyaVps(t, s)
	s.sekretyPisat = pisat
	if h.Faza != fazaOshibka || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsKlyuchHosta || h.Otkaz.Otpechatok != novyy.Otpechatok() {
		t.Fatalf("проверка с чужим ключом: %+v", h)
	}
	if poKoncu.Load() {
		t.Fatal("набор писался после конца задания")
	}
	z := zapisPoId(t, s, id)
	if z.KlyuchHosta != kh.TeloBase64() || z.NovyyKlyuchHostaTip != novyy.Tip || z.NovyyKlyuchHosta != novyy.TeloBase64() || z.ProverkaOtkaz == "" {
		t.Fatalf("запись после чужого ключа: %+v", z)
	}

	otv := s.svoyVpsSpisok(protokol.Kadr{Imya: "svoyVpsSpisok"})
	if bytes.Contains(otv.Telo, []byte(novyy.TeloBase64())) {
		t.Fatalf("тело нового ключа ушло в список: %s", otv.Telo)
	}
	var spisok struct {
		Servery []SvoyServerDlyaEkrana `json:"servery"`
	}
	if err := json.Unmarshal(otv.Telo, &spisok); err != nil {
		t.Fatal(err)
	}
	if len(spisok.Servery) != 1 || spisok.Servery[0].Otpechatok != kh.Otpechatok() || spisok.Servery[0].NovyyOtpechatok != novyy.Otpechatok() {
		t.Fatalf("список: %+v", spisok.Servery)
	}

	// Не тот отпечаток, чужой id, пустое тело: запись не тронута. На чужом
	// отпечатке окно перечитает список и покажет новый.
	for _, c := range []struct {
		imya string
		otv  protokol.Kadr
		kod  string
	}{
		{"прежний отпечаток", prinyatKlyuch(t, s, id, kh.Otpechatok()), protokol.KodVpsKlyuchHosta},
		{"нет сервера", prinyatKlyuch(t, s, "net-takogo", novyy.Otpechatok()), protokol.KodTeloNegodno},
		{"без отпечатка", prinyatKlyuch(t, s, id, ""), protokol.KodTeloNegodno},
	} {
		if kod := kodOtkaza(c.otv); kod != c.kod {
			t.Fatalf("%s: %q", c.imya, kod)
		}
	}
	if b := zapisPoId(t, s, id); !reflect.DeepEqual(b, z) {
		t.Fatalf("запись после отказов: %+v", b)
	}

	// Посреди задания ключ не принимается: оно вошло бы с прежним.
	pusk := make(chan struct{})
	s.vpsProverit = func(ctx context.Context, _ upravlenie.Puti, _ ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		close(pusk)
		<-ctx.Done()
		return upravlenie.ZdorovyeServera{}, ctx.Err()
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	<-pusk
	if kod := kodOtkaza(prinyatKlyuch(t, s, id, novyy.Otpechatok())); kod != protokol.KodVpsZanyato {
		t.Fatalf("приём посреди задания: %q", kod)
	}
	s.svoyVpsOtmenit(protokol.Kadr{Imya: "svoyVpsOtmenit"})
	dozhdatsyaVps(t, s)

	// Запись не легла: ключ остаётся прежним, ожидание тоже.
	s.sekretyPisat = func([]byte) error { return errors.New("тест: диск полон") }
	if otv := prinyatKlyuch(t, s, id, novyy.Otpechatok()); otv.Oshib == nil {
		t.Fatal("приём без записи прошёл")
	}
	s.sekretyPisat = pisat
	if b := zapisPoId(t, s, id); !reflect.DeepEqual(b, z) {
		t.Fatalf("запись после незаписанного приёма: %+v", b)
	}

	otv = prinyatKlyuch(t, s, id, novyy.Otpechatok())
	if otv.Oshib != nil || !bytes.Contains(otv.Telo, []byte(id)) {
		t.Fatalf("приём: %+v", otv)
	}
	z = zapisPoId(t, s, id)
	// Отказ прошлой проверки был про прежний ключ: он снят, и сервер ждёт
	// плановую первым.
	if z.KlyuchHostaTip != novyy.Tip || z.KlyuchHosta != novyy.TeloBase64() || z.NovyyKlyuchHosta != "" || z.NovyyKlyuchHostaTip != "" || z.ParolVyklyuchen ||
		!z.Proveren.IsZero() || z.ProverkaOtkaz != "" {
		t.Fatalf("запись после приёма: %+v", z)
	}
	if kod := kodOtkaza(prinyatKlyuch(t, s, id, novyy.Otpechatok())); kod != protokol.KodTeloNegodno {
		t.Fatalf("повторный приём: %q", kod)
	}

	// Следующий вход ждёт уже новый ключ.
	s.vpsProverit = func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros, _ string) (upravlenie.ZdorovyeServera, error) {
		zapros = z
		return upravlenie.ZdorovyeServera{Osmotr: osmotrChistyy(), Uzel: server.SostoyanieUzla{Versiya: "0.6.1"}}, nil
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Faza != fazaGotovo || zapros.Izvesten == nil || !bytes.Equal(zapros.Izvesten.Telo, novyy.Telo) {
		t.Fatalf("вход после приёма: %+v, ключ %+v", h, zapros.Izvesten)
	}
}

// Ожидающий ключ снимается любым удачным входом с записанным и не
// заводится отказом без предъявленного ключа.
func TestSvoyVpsNovyyKlyuchHostaSnimaetsya(t *testing.T) {
	s := sPustymNaborom(t)
	kh, novyy := klyuchHostaDlyaTesta(t), klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	ozhidaet := func() {
		t.Helper()
		if err := s.otmetitProverku(id, func(x *SvoyServer) {
			x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta = novyy.Tip, novyy.TeloBase64()
		}); err != nil {
			t.Fatal(err)
		}
	}
	snyat := func(chto string) {
		t.Helper()
		if z := zapisPoId(t, s, id); z.NovyyKlyuchHosta != "" || z.NovyyKlyuchHostaTip != "" {
			t.Fatalf("%s: ожидающий ключ остался: %+v", chto, z)
		}
	}

	ozhidaet()
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		return upravlenie.ZdorovyeServera{Osmotr: osmotrChistyy(), Uzel: server.SostoyanieUzla{Versiya: "0.6.1"}}, nil
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	dozhdatsyaVps(t, s)
	snyat("удачная проверка")

	ozhidaet()
	s.vpsVyklyuchitParol = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.ParolItog, error) {
		return upravlenie.ParolItog{}, nil
	}
	s.svoyVpsParol(kadrVps(t, "svoyVpsParol", map[string]any{"id": id, "vyklyuchit": true}))
	dozhdatsyaVps(t, s)
	snyat("удачное выключение пароля")

	// Предъявлен тот же ключ, что записан: ждать нечего.
	ozhidaet()
	s.otmetitKlyuchHosta(id, &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &kh})
	snyat("предъявлен записанный ключ")

	// Отказ без предъявленного ключа или с другой причиной ожидание не
	// трогает, тот же предъявленный ключ - тоже, и набор тогда не пишется.
	ozhidaet()
	byla := zapisPoId(t, s, id)
	pisat := s.sekretyPisat
	zapisey := 0
	s.sekretyPisat = func(b []byte) error { zapisey++; return pisat(b) }
	for _, err := range []error{
		&ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya},
		&ssh.Otkaz{Prichina: ssh.PrichinaTishina, Predyavlen: &novyy},
		errors.New("что-то"),
		&ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &novyy},
	} {
		s.otmetitKlyuchHosta(id, err)
	}
	s.otmetitKlyuchHosta("", nil)
	s.sekretyPisat = pisat
	if z := zapisPoId(t, s, id); !reflect.DeepEqual(z, byla) || zapisey != 0 {
		t.Fatalf("запись после посторонних отказов (%d записей): %+v", zapisey, z)
	}
}

// Сервер уже в разделе: осмотр и захват входят с записанным ключом хоста, а
// присланный с осмотра не в счёт. Их отказ со сменой ключа ложится в запись
// ожиданием приёма.
func TestSvoyVpsZapisannyyKlyuchHosta(t *testing.T) {
	s := sPustymNaborom(t)
	kh, novyy := klyuchHostaDlyaTesta(t), klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, false)
	smena := &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &novyy}
	var vhod ssh.Zapros
	osmotr := func(_ context.Context, _ upravlenie.Puti, z ssh.Zapros) (upravlenie.Otchet, error) {
		vhod = z
		return upravlenie.Otchet{}, smena
	}
	s.vpsOsmotret = osmotr
	s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
	if h := dozhdatsyaVps(t, s); h.Server != id || h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsKlyuchHosta {
		t.Fatalf("осмотр сервера из раздела: %+v", h)
	}
	if vhod.Izvesten == nil || !bytes.Equal(vhod.Izvesten.Telo, kh.Telo) {
		t.Fatalf("осмотр вошёл не с записанным ключом: %+v", vhod.Izvesten)
	}
	if z := zapisPoId(t, s, id); z.NovyyKlyuchHosta != novyy.TeloBase64() {
		t.Fatalf("осмотр не оставил ожидания: %+v", z)
	}

	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta = "", "" }); err != nil {
		t.Fatal(err)
	}
	var v upravlenie.Vzyatie
	s.vpsVzyat = func(_ context.Context, x upravlenie.Vzyatie) (upravlenie.Itog, error) {
		v = x
		return upravlenie.Itog{}, smena
	}
	telo := zaprosVzyatiya{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"},
		KlyuchHostaTip: novyy.Tip, KlyuchHosta: novyy.TeloBase64()}
	s.svoyVpsVzyat(kadrVps(t, "svoyVpsVzyat", telo))
	if h := dozhdatsyaVps(t, s); h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsKlyuchHosta {
		t.Fatalf("захват при сменившемся ключе: %+v", h)
	}
	if v.Vhod.Izvesten == nil || !bytes.Equal(v.Vhod.Izvesten.Telo, kh.Telo) {
		t.Fatalf("захват вошёл не с записанным ключом: %+v", v.Vhod.Izvesten)
	}
	if z := zapisPoId(t, s, id); z.NovyyKlyuchHosta != novyy.TeloBase64() || z.KlyuchHosta != kh.TeloBase64() {
		t.Fatalf("захват не оставил ожидания: %+v", z)
	}

	// Адрес без записи осмотр встречает впервые.
	s = sPustymNaborom(t)
	s.vpsOsmotret = osmotr
	vhod = ssh.Zapros{}
	s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))
	if h := dozhdatsyaVps(t, s); h.Server != "" || vhod.Izvesten != nil || vhod.Adres != adresSvoego {
		t.Fatalf("первая встреча: %+v, вход %+v", h, vhod)
	}
}

// Испорченный ключ хоста в записи это порча хранилища: осмотр, захват и
// приём отвечают о ней, а не зовут перепроверить сервер.
func TestSvoyVpsKlyuchHostaIsporchen(t *testing.T) {
	s := sPustymNaborom(t)
	kh := klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.NovyyKlyuchHostaTip, x.NovyyKlyuchHosta = kh.Tip, "!!!" }); err != nil {
		t.Fatal(err)
	}
	var spisok struct {
		Servery []SvoyServerDlyaEkrana `json:"servery"`
	}
	if err := json.Unmarshal(s.svoyVpsSpisok(protokol.Kadr{Imya: "svoyVpsSpisok"}).Telo, &spisok); err != nil {
		t.Fatal(err)
	}
	if len(spisok.Servery) != 1 || spisok.Servery[0].NovyyOtpechatok != "" || spisok.Servery[0].Otpechatok != kh.Otpechatok() {
		t.Fatalf("список с испорченным новым ключом: %+v", spisok.Servery)
	}
	otv := prinyatKlyuch(t, s, id, kh.Otpechatok())
	if otv.Oshib == nil || otv.Oshib.Kod != protokol.KodTeloNegodno || !strings.Contains(otv.Oshib.Tekst, "не разобран") {
		t.Fatalf("приём испорченного ключа: %+v", otv.Oshib)
	}
	if z := zapisPoId(t, s, id); z.KlyuchHosta != kh.TeloBase64() {
		t.Fatalf("приём испорченного ключа тронул запись: %+v", z)
	}

	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.KlyuchHosta = "!!!" }); err != nil {
		t.Fatal(err)
	}
	s.vpsOsmotret = func(context.Context, upravlenie.Puti, ssh.Zapros) (upravlenie.Otchet, error) {
		t.Error("осмотр пошёл на сервер с испорченной записью")
		return upravlenie.Otchet{}, nil
	}
	if kod := kodOtkaza(s.svoyVpsOsmotret(kadrVps(t, "svoyVpsOsmotret", dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}))); kod != protokol.KodSecretsUnreadable {
		t.Fatalf("осмотр с испорченной записью: %q", kod)
	}
	telo := zaprosVzyatiya{dostupVps: dostupVps{Adres: adresSvoego, Polzovatel: "root", Parol: "x"}, KlyuchHostaTip: kh.Tip, KlyuchHosta: kh.TeloBase64()}
	if kod := kodOtkaza(s.svoyVpsVzyat(kadrVps(t, "svoyVpsVzyat", telo))); kod != protokol.KodSecretsUnreadable {
		t.Fatalf("захват с испорченной записью: %q", kod)
	}
	// Ключа хоста в записи нет вовсе: ключом управления не входят вслепую.
	if err := s.otmetitProverku(id, func(x *SvoyServer) { x.KlyuchHostaTip, x.KlyuchHosta = "", "" }); err != nil {
		t.Fatal(err)
	}
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		t.Error("проверка пошла на сервер без ключа хоста в записи")
		return upravlenie.ZdorovyeServera{}, nil
	}
	if kod := kodOtkaza(s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))); kod != protokol.KodSecretsUnreadable {
		t.Fatalf("проверка без ключа хоста в записи: %q", kod)
	}
}

// Ключ хоста в записи сменили, пока проверка шла со старым: её отказ про
// прежний ключ в запись не ложится.
func TestSvoyVpsProveritSoStarymKlyuchom(t *testing.T) {
	s := sPustymNaborom(t)
	kh, novyy := klyuchHostaDlyaTesta(t), klyuchHostaDlyaTesta(t)
	id, _ := postavitSvoy(t, s, kh, true)
	byla := zapisPoId(t, s, id)
	s.vpsProverit = func(context.Context, upravlenie.Puti, ssh.Zapros, string) (upravlenie.ZdorovyeServera, error) {
		if err := s.otmetitProverku(id, func(x *SvoyServer) { x.KlyuchHostaTip, x.KlyuchHosta = novyy.Tip, novyy.TeloBase64() }); err != nil {
			return upravlenie.ZdorovyeServera{}, err
		}
		return upravlenie.ZdorovyeServera{}, &ssh.Otkaz{Prichina: ssh.PrichinaKlyuchHostaSmenilsya, Predyavlen: &novyy}
	}
	s.svoyVpsProverit(kadrVps(t, "svoyVpsProverit", map[string]string{"id": id}))
	if h := dozhdatsyaVps(t, s); h.Otkaz == nil || h.Otkaz.Kod != protokol.KodVpsKlyuchHosta {
		t.Fatalf("проверка со старым ключом: %+v", h)
	}
	z := zapisPoId(t, s, id)
	if !z.Proveren.Equal(byla.Proveren) || z.ProverkaOtkaz != "" || z.KlyuchHosta != novyy.TeloBase64() || z.NovyyKlyuchHosta != "" {
		t.Fatalf("запись после проверки со старым ключом: %+v", z)
	}
}
