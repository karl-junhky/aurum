package sertip

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
	"golang.org/x/net/dns/dnsmessage"
)

const ip = "127.0.0.1"

// zapisBrandmauera помнит, какие порты открыты сейчас и сколько раз
// открывались.
type zapisBrandmauera struct {
	mu      sync.Mutex
	otkryto map[int]int
	vsego   int
	// priOtkrytii зовётся посреди попытки, когда открывается порт проверки.
	priOtkrytii func()
}

func (z *zapisBrandmauera) Otkryt(_ net.IP, port int) (func() error, error) {
	z.mu.Lock()
	z.otkryto[port]++
	z.vsego++
	pri := z.priOtkrytii
	z.mu.Unlock()
	if pri != nil {
		pri()
	}
	return func() error {
		z.mu.Lock()
		defer z.mu.Unlock()
		z.otkryto[port]--
		return nil
	}, nil
}

func (z *zapisBrandmauera) Ubrat() error { return nil }

func (z *zapisBrandmauera) vseZakryty(t *testing.T) {
	t.Helper()
	z.mu.Lock()
	defer z.mu.Unlock()
	for port, n := range z.otkryto {
		if n != 0 {
			t.Fatalf("после попытки на %d/tcp осталось открытых правил: %d", port, n)
		}
	}
}

type chasy struct{ sdvig atomic.Int64 }

func (c *chasy) seychas() time.Time         { return time.Now().Add(time.Duration(c.sdvig.Load())) }
func (c *chasy) vperyod(d time.Duration)    { c.sdvig.Add(int64(d)) }
func (c *chasy) opcii(o Opcii) Opcii        { o.Chasy = c.seychas; return o }
func (s *stend) novyy(t *testing.T) *Vypusk { t.Helper(); return s.novyyV(t, s.katalog) }

type stend struct {
	katalog    string
	opcii      Opcii
	brandmauer *zapisBrandmauera
	chasy      *chasy
	obnovleno  atomic.Int32
}

// novyyStend поднимает Pebble, тестовый ACME-сервер Let's Encrypt, в этом
// же процессе: проверку адреса он делает по-настоящему, запросом на
// 127.0.0.1 в порты слушателей.
func novyyStend(t *testing.T) *stend {
	t.Helper()
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	portHTTP, portTLS := svobodnyyPort(t), svobodnyyPort(t)
	tiho := log.New(io.Discard, "", 0)
	baza := db.NewMemoryStore()
	profili := map[string]ca.Profile{
		"default":    {Description: "обычный", ValidityPeriod: 90 * 24 * 3600},
		"shortlived": {Description: "как у Let's Encrypt на IP", ValidityPeriod: 160 * 3600},
	}
	c := ca.New(tiho, baza, "", "ecdsa", 0, 1, profili)
	proveryayushchiy := va.New(tiho, portHTTP, portTLS, false, dnsNaPetlyu(t), baza)
	w := wfe.New(tiho, baza, proveryayushchiy, c, []string{"pebble.letsencrypt.org"}, false, false, 0, 0)
	// Let's Encrypt в ответе на finalize даёт адрес заказа в Location, и
	// x/crypto по нему ждёт сертификат. Pebble заголовок не ставит, здесь он
	// добавляется так же, как у Let's Encrypt.
	obrabotchik := w.Handler()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(otvet http.ResponseWriter, r *http.Request) {
		if id, est := strings.CutPrefix(r.URL.Path, "/finalize-order/"); est {
			otvet.Header().Set("Location", "https://"+r.Host+"/my-order/"+id)
		}
		obrabotchik.ServeHTTP(otvet, r)
	}))
	t.Cleanup(srv.Close)
	s := &stend{katalog: t.TempDir(), brandmauer: &zapisBrandmauera{otkryto: map[int]int{}}, chasy: &chasy{}}
	s.opcii = s.chasy.opcii(Opcii{
		PortHTTP: portHTTP, PortTLS: portTLS, Brandmauer: s.brandmauer,
		KatalogACME: srv.URL + wfe.DirectoryPath, HTTPKlient: srv.Client(), ProfilDomena: "default",
	})
	return s
}

func (s *stend) novyyV(t *testing.T, katalog string) *Vypusk {
	t.Helper()
	zhurnal := slog.New(slog.NewTextHandler(io.Discard, nil))
	return Novyy(filepath.Join(katalog, "acme"), filepath.Join(katalog, "sert-ip"), zhurnal,
		func() { s.obnovleno.Add(1) }, s.opcii)
}

// dnsNaPetlyu поднимает DNS по TCP (так спрашивает Pebble), который на любое
// имя отвечает A 127.0.0.1 и пустым AAAA: проверка домена идёт запросом на
// слушатели теста.
func dnsNaPetlyu(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go otvetitDNS(c)
		}
	}()
	return ln.Addr().String()
}

func otvetitDNS(c net.Conn) {
	defer c.Close()
	for {
		var dlina [2]byte
		if _, err := io.ReadFull(c, dlina[:]); err != nil {
			return
		}
		vopros := make([]byte, int(dlina[0])<<8|int(dlina[1]))
		if _, err := io.ReadFull(c, vopros); err != nil {
			return
		}
		var p dnsmessage.Parser
		zag, err := p.Start(vopros)
		if err != nil {
			return
		}
		v, err := p.Question()
		if err != nil {
			return
		}
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: zag.ID, Response: true, Authoritative: true})
		_ = b.StartQuestions()
		_ = b.Question(v)
		_ = b.StartAnswers()
		if v.Type == dnsmessage.TypeA {
			_ = b.AResource(dnsmessage.ResourceHeader{Name: v.Name, Class: dnsmessage.ClassINET, TTL: 60},
				dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}})
		}
		otvet, err := b.Finish()
		if err != nil {
			return
		}
		if _, err := c.Write(append([]byte{byte(len(otvet) >> 8), byte(len(otvet))}, otvet...)); err != nil {
			return
		}
	}
}

func svobodnyyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// zanyat держит порт чужим слушателем до конца теста.
func zanyat(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort("", itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

func itoa(n int) string { return big.NewInt(int64(n)).String() }

func TestVypuskHTTP01(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if v.Sostoyanie() != nil {
		t.Fatal("без настройки выпуск должен молчать")
	}
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())

	sost := v.Sostoyanie()
	if !sost.Est || sost.Sposob != SposobHTTP || sost.Otkaz != "" || sost.Trevoga {
		t.Fatalf("после выпуска: %+v", sost)
	}
	if srok := time.Unix(sost.Do, 0).Sub(time.Unix(sost.Ot, 0)); srok < 159*time.Hour || srok > 161*time.Hour {
		t.Fatalf("срок %v, ждали профиль shortlived на 160 ч", srok)
	}
	if v.Tekushchiy() == nil || s.obnovleno.Load() != 1 {
		t.Fatalf("сертификат не отдан входам: текущий %v, обновлений %d", v.Tekushchiy(), s.obnovleno.Load())
	}
	if _, err := prochitatSert(filepath.Join(s.katalog, "sert-ip")); err != nil {
		t.Fatalf("на диске нет пары: %v", err)
	}
	if s.brandmauer.vsego != 1 {
		t.Fatalf("брандмауэр открывался %d раз, ждали один (80)", s.brandmauer.vsego)
	}
	s.brandmauer.vseZakryty(t)

	// Половина срока не прошла: новой попытки нет.
	v.krug(context.Background())
	if n := len(v.uchet.Popytki); n != 1 {
		t.Fatalf("попыток %d, ждали одну", n)
	}
	// С диска после перезапуска программы тот же сертификат.
	if v2 := s.novyy(t); v2.Nastroit(&Nastroyka{IP: ip}, nil) != nil || v2.Tekushchiy() == nil {
		t.Fatal("после перезапуска сертификат с диска не поднялся")
	}
}

func TestVypuskTLSALPNKogda80Zanyat(t *testing.T) {
	s := novyyStend(t)
	zanyat(t, s.opcii.PortHTTP)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if sost := v.Sostoyanie(); !sost.Est || sost.Sposob != SposobTLS {
		t.Fatalf("при занятом 80 ждали выпуск через 443: %+v", sost)
	}
	s.brandmauer.vseZakryty(t)
}

// Оба порта заняты: отказ называет оба, попыток не больше двух в час, и
// счёт переживает перезапуск программы.
func TestOtkazIPredelPopytok(t *testing.T) {
	s := novyyStend(t)
	zanyat(t, s.opcii.PortHTTP)
	zanyat(t, s.opcii.PortTLS)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	sost := v.Sostoyanie()
	if sost.Est || !sost.Trevoga || sost.OtkazS == 0 ||
		!strings.Contains(sost.Otkaz, itoa(s.opcii.PortHTTP)+"/tcp") || !strings.Contains(sost.Otkaz, itoa(s.opcii.PortTLS)+"/tcp") {
		t.Fatalf("отказ: %+v", sost)
	}
	v.krug(context.Background())
	v.krug(context.Background())
	if n := len(v.uchet.Popytki); n != popytokVOkne {
		t.Fatalf("попыток за час %d, ждали %d", n, popytokVOkne)
	}
	if sled := v.Sostoyanie().Sleduyushchaya; sled < s.chasy.seychas().Add(50*time.Minute).Unix() {
		t.Fatalf("следующая попытка %v, ждали через час после первой", time.Unix(sled, 0))
	}
	if sost := v.Sostoyanie(); sost.Predel <= 0 || sost.Predel > int64(oknoPopytok/time.Second)+1 {
		t.Fatalf("попытки часа кончились, а предел в состоянии %d с: %+v", sost.Predel, sost)
	}
	// Круг просыпается, когда самая старая попытка выходит из окна, а не на
	// тике десять минут спустя.
	osvob := time.Unix(v.uchet.Popytki[0], 0).Add(oknoPopytok)
	if d, ok := v.doPredela(); !ok || !s.chasy.seychas().Add(d).After(osvob) || s.chasy.seychas().Add(d).After(osvob.Add(2*time.Second)) {
		t.Fatalf("будильник предела через %v (%v), окно освобождается в %v", d, ok, osvob)
	}

	v2 := s.novyy(t)
	if err := v2.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v2.krug(context.Background())
	if n := len(v2.uchet.Popytki); n != popytokVOkne {
		t.Fatalf("после перезапуска попыток %d: лимит не пережил перезапуск", n)
	}
	s.chasy.vperyod(61 * time.Minute)
	v2.krug(context.Background())
	if n := len(v2.uchet.Popytki); n != 1 {
		t.Fatalf("через час попыток в окне %d, ждали одну новую", n)
	}
	if _, ok := v2.doPredela(); ok || v2.Sostoyanie().Predel != 0 {
		t.Fatal("в окне одна попытка, а выпуск считает, что упёрся в предел")
	}
}

// Попытки часа кончились: Rabotat начинает следующую, как только окно
// освободилось, а не на тике десять минут спустя.
func TestRabotatProsypaetsyaPoPredelu(t *testing.T) {
	s := novyyStend(t)
	zanyat(t, s.opcii.PortHTTP)
	zanyat(t, s.opcii.PortTLS)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	v.krug(context.Background())
	if n := len(v.uchet.Popytki); n != popytokVOkne {
		t.Fatalf("попыток %d, ждали %d", n, popytokVOkne)
	}
	byla := v.Sostoyanie().Popytka
	// Окно освободится через полторы секунды по часам выпуска: первый круг
	// Rabotat ещё упирается в предел, попытку начинает только будильник.
	s.chasy.vperyod(v.osvobozhdenie().Sub(s.chasy.seychas()) - 1500*time.Millisecond)
	if d, ok := v.doPredela(); !ok || d < time.Second || d > 3*time.Second {
		t.Fatalf("будильник через %v (%v)", d, ok)
	}
	ctx, otmena := context.WithCancel(context.Background())
	konchil := make(chan struct{})
	go func() { defer close(konchil); v.Rabotat(ctx) }()
	defer func() { otmena(); <-konchil }()
	for do := time.Now().Add(10 * time.Second); v.Sostoyanie().Popytka <= byla; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(do) {
			t.Fatal("окно освободилось, а новой попытки нет: круг ждёт тика")
		}
	}
}

func TestProdlenieSPolovinySroka(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	pervyy := v.Tekushchiy()
	if pervyy == nil {
		t.Fatal(v.Sostoyanie().Otkaz)
	}
	s.chasy.vperyod(79 * time.Hour)
	if v.krug(context.Background()); v.Tekushchiy() != pervyy {
		t.Fatal("продлён раньше половины срока")
	}
	s.chasy.vperyod(2 * time.Hour)
	v.krug(context.Background())
	vtoroy := v.Tekushchiy()
	if vtoroy == nil || vtoroy.Leaf.SerialNumber.Cmp(pervyy.Leaf.SerialNumber) == 0 {
		t.Fatalf("после половины срока не продлён: %+v", v.Sostoyanie())
	}
	if s.obnovleno.Load() != 2 {
		t.Fatalf("входам отдано %d сертификатов, ждали два", s.obnovleno.Load())
	}
}

func TestKopiya(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	seychas := time.Now()
	dolgaya := pemNa(t, ip, seychas.Add(-time.Hour), seychas.Add(100*time.Hour))
	if err := v.Nastroit(&Nastroyka{IP: ip}, &dolgaya); err != nil {
		t.Fatal(err)
	}
	if sost := v.Sostoyanie(); !sost.Est || sost.Sposob != SposobKopiya || s.obnovleno.Load() != 1 {
		t.Fatalf("годная копия не легла: %+v", sost)
	}
	// Копия живёт дольше половины срока: выпуска нет, лимит Let's Encrypt цел.
	v.krug(context.Background())
	if len(v.uchet.Popytki) != 0 {
		t.Fatal("при свежей копии пошёл выпуск")
	}
	korotkaya := pemNa(t, ip, seychas.Add(-time.Hour), seychas.Add(50*time.Hour))
	if err := v.Nastroit(&Nastroyka{IP: ip}, &korotkaya); err != nil || s.obnovleno.Load() != 1 {
		t.Fatalf("копия хуже лежащей заменила её: %v", err)
	}
	istekshaya := pemNa(t, ip, seychas.Add(-200*time.Hour), seychas.Add(-time.Hour))
	if err := v.Nastroit(&Nastroyka{IP: ip}, &istekshaya); err != nil || s.obnovleno.Load() != 1 {
		t.Fatalf("истёкшая копия: %v", err)
	}
	chuzhaya := pemNa(t, "198.51.100.7", seychas.Add(-time.Hour), seychas.Add(150*time.Hour))
	if err := v.Nastroit(&Nastroyka{IP: ip}, &chuzhaya); err == nil {
		t.Fatal("копия на чужой адрес принята")
	}
}

func TestZaglushka(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if err := v.Zaglushka(ip); err != nil {
		t.Fatal(err)
	}
	put := filepath.Join(s.katalog, "sert-ip", imyaSerta)
	bylo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	if v.Tekushchiy() != nil || v.Sostoyanie().Est {
		t.Fatal("заглушку приняли за настоящий сертификат")
	}
	if err := v.Zaglushka(ip); err != nil {
		t.Fatal(err)
	}
	if stalo, _ := os.ReadFile(put); string(stalo) != string(bylo) {
		t.Fatal("вторая заглушка переписала первую")
	}
	// Заглушка не мешает выпуску и уходит первой же удачной попыткой.
	v.krug(context.Background())
	if v.Tekushchiy() == nil {
		t.Fatalf("при заглушке на диске не выпущен: %+v", v.Sostoyanie())
	}
	if v2 := s.novyy(t); v2.Nastroit(&Nastroyka{IP: ip}, nil) != nil || v2.Tekushchiy() == nil {
		t.Fatal("после выпуска поверх заглушки с диска читается заглушка")
	}
}

// Пока держат Zanyat, попытка не начинается, а после освобождения выпуск
// будят сразу.
func TestZanyat(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	<-v.budit // Nastroit разбудил, этот сигнал тест забирает себе
	osvobodit, err := v.Zanyat(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if len(v.uchet.Popytki) != 0 {
		t.Fatal("попытка началась при занятом выпуске")
	}
	osvobodit()
	osvobodit()
	select {
	case <-v.budit:
	default:
		t.Fatal("после освобождения выпуск не разбужен")
	}
	v.krug(context.Background())
	if v.Tekushchiy() == nil {
		t.Fatalf("после освобождения не выпущен: %+v", v.Sostoyanie())
	}
}

const domen = "kot.example.net"

// Режим своего домена: сертификат на имя обычным профилем на 90 дней,
// продление с двух третей срока, тревога за неделю до конца.
func TestVypuskNaDomen(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip, Domen: "Kot.Example.NET."}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	sost := v.Sostoyanie()
	if !sost.Est || sost.Domen != domen || sost.Sposob != SposobHTTP || sost.Trevoga {
		t.Fatalf("после выпуска на домен: %+v", sost)
	}
	if srok := time.Unix(sost.Do, 0).Sub(time.Unix(sost.Ot, 0)); srok < 89*24*time.Hour {
		t.Fatalf("срок %v, ждали обычный профиль на 90 дней", srok)
	}
	pervyy := v.Tekushchiy()
	if pervyy == nil || !slices.Contains(pervyy.Leaf.DNSNames, domen) || len(pervyy.Leaf.IPAddresses) != 0 {
		t.Fatalf("сертификат не на домен: %+v", pervyy)
	}
	if sled := time.Unix(sost.Sleduyushchaya, 0); sled.Before(time.Unix(sost.Ot, 0).Add(59 * 24 * time.Hour)) {
		t.Fatalf("следующий выпуск %v, ждали после двух третей срока", sled)
	}
	s.chasy.vperyod(59 * 24 * time.Hour)
	if v.krug(context.Background()); v.Tekushchiy() != pervyy {
		t.Fatal("продлён раньше двух третей срока")
	}
	s.chasy.vperyod(24 * 24 * time.Hour)
	if sost := v.Sostoyanie(); !sost.Trevoga {
		t.Fatalf("за шесть дней до конца срока нет тревоги: %+v", sost)
	}
	v.krug(context.Background())
	if vtoroy := v.Tekushchiy(); vtoroy == nil || vtoroy.Leaf.SerialNumber.Cmp(pervyy.Leaf.SerialNumber) == 0 {
		t.Fatalf("после двух третей срока не продлён: %+v", v.Sostoyanie())
	}
}

// Смена адреса на домен: сертификат на IP домену не годится, выпуск идёт
// заново, а обратная смена снова просит IP.
func TestSmenaIPNaDomen(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if v.Tekushchiy() == nil {
		t.Fatal(v.Sostoyanie().Otkaz)
	}
	if err := v.Nastroit(&Nastroyka{IP: ip, Domen: domen}, nil); err != nil {
		t.Fatal(err)
	}
	if v.Tekushchiy() != nil || v.Sostoyanie().Est {
		t.Fatal("сертификат на IP выдан за сертификат домена")
	}
	v.krug(context.Background())
	if c := v.Tekushchiy(); c == nil || !slices.Contains(c.Leaf.DNSNames, domen) {
		t.Fatalf("на домен не выпущен: %+v", v.Sostoyanie())
	}
	if err := v.Nastroit(&Nastroyka{IP: ip}, nil); err != nil {
		t.Fatal(err)
	}
	if v.Tekushchiy() != nil {
		t.Fatal("сертификат домена выдан за сертификат на IP")
	}
}

// pereyti задаёт настройку и делает круг выпуска; сертификат на неё обязан быть.
func pereyti(t *testing.T, v *Vypusk, n Nastroyka) *tls.Certificate {
	t.Helper()
	if err := v.Nastroit(&n, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	c := v.Tekushchiy()
	if c == nil {
		t.Fatalf("на %+v нет сертификата: %+v", n, v.Sostoyanie())
	}
	return c
}

func tot(a, b *tls.Certificate) bool { return a.Leaf.SerialNumber.Cmp(b.Leaf.SerialNumber) == 0 }

// Переходы IP - домен - IP тратят только два первых выпуска: прежний
// сертификат откладывается и возвращается на место, и после перезапуска
// программы тоже. Копия хуже отложенного не ложится.
func TestOtlozhennyySertifikat(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	nIP, nDomen := Nastroyka{IP: ip}, Nastroyka{IP: ip, Domen: domen}
	sertIP := pereyti(t, v, nIP)
	sertDomena := pereyti(t, v, nDomen)
	if v.otlozhennyy == nil || !tot(v.otlozhennyy, sertIP) || v.uchet.SposobOtlozhennogo != SposobHTTP {
		t.Fatalf("сертификат на IP не отложен: %+v", v.uchet)
	}
	popytok := len(v.uchet.Popytki)
	// Обе попытки часа потрачены, но возврату они не нужны, и состояние не
	// обещает ждать час.
	if err := v.Nastroit(&nIP, nil); err != nil {
		t.Fatal(err)
	}
	if sled := v.Sostoyanie().Sleduyushchaya; sled > s.chasy.seychas().Unix() {
		t.Fatalf("до возврата отложенного состояние ждёт попытки в %v", time.Unix(sled, 0))
	}
	v.mu.Lock()
	v.uchet.OtkazS = s.chasy.seychas().Add(-time.Hour).Unix() // прошлый отказ на другом имени
	v.mu.Unlock()
	if v.Sostoyanie().Trevoga {
		t.Fatal("до возврата отложенного тревога по прошлому отказу")
	}
	if c := pereyti(t, v, nIP); !tot(c, sertIP) || len(v.uchet.Popytki) != popytok {
		t.Fatalf("возврат к IP выпустил новый: попыток %d, было %d", len(v.uchet.Popytki), popytok)
	}
	if sost := v.Sostoyanie(); !sost.Est || sost.Sposob != SposobHTTP || sost.Otkaz != "" || s.obnovleno.Load() != 3 {
		t.Fatalf("после возврата: %+v, обновлений %d", sost, s.obnovleno.Load())
	}

	// После перезапуска отложенный читается с диска вместе со способом.
	v2 := s.novyy(t)
	if c := pereyti(t, v2, nDomen); !tot(c, sertDomena) || len(v2.uchet.Popytki) != popytok {
		t.Fatalf("после перезапуска возврат к домену выпустил новый: попыток %d", len(v2.uchet.Popytki))
	}
	if sost := v2.Sostoyanie(); sost.Sposob != SposobHTTP {
		t.Fatalf("способ не вернулся с отложенным: %+v", sost)
	}

	// Отложенный пора продлевать: он всё равно встаёт на место сразу, а
	// продление идёт следующим кругом.
	s.chasy.vperyod(81 * time.Hour)
	if err := v2.Nastroit(&nIP, nil); err != nil {
		t.Fatal(err)
	}
	<-v2.budit // Nastroit разбудил, этот сигнал тест забирает себе
	v2.krug(context.Background())
	if c := v2.Tekushchiy(); c == nil || !tot(c, sertIP) {
		t.Fatalf("отложенный у порога продления не встал на место: %+v", v2.Sostoyanie())
	}
	select {
	case <-v2.budit:
	default:
		t.Fatal("вернувшийся пора продлевать, а следующий круг не разбужен")
	}
	v2.krug(context.Background())
	prodlyonnyy := v2.Tekushchiy()
	if prodlyonnyy == nil || tot(prodlyonnyy, sertIP) {
		t.Fatalf("вернувшийся не продлён: %+v", v2.Sostoyanie())
	}
	if v2.otlozhennyy == nil || !tot(v2.otlozhennyy, sertDomena) {
		t.Fatal("продление выбросило отложенный сертификат домена")
	}

	// Сертификат домена снова на месте, на IP отложен продлённый. Копия
	// короче отложенного не ложится, длиннее ложится и откладывает домен.
	pereyti(t, v2, nDomen)
	seychas := s.chasy.seychas()
	korotkaya := pemNa(t, ip, seychas.Add(-time.Hour), seychas.Add(50*time.Hour))
	obnovleniy := s.obnovleno.Load()
	if err := v2.Nastroit(&nIP, &korotkaya); err != nil || s.obnovleno.Load() != obnovleniy {
		t.Fatalf("копия хуже отложенного легла: %v", err)
	}
	v2.krug(context.Background())
	if c := v2.Tekushchiy(); c == nil || !tot(c, prodlyonnyy) {
		t.Fatalf("при копии хуже не вернулся отложенный: %+v", v2.Sostoyanie())
	}
	pereyti(t, v2, nDomen)
	dolgaya := pemNa(t, ip, seychas.Add(-time.Hour), seychas.Add(300*time.Hour))
	if err := v2.Nastroit(&nIP, &dolgaya); err != nil {
		t.Fatal(err)
	}
	if sost := v2.Sostoyanie(); !sost.Est || sost.Sposob != SposobKopiya {
		t.Fatalf("копия длиннее отложенного не легла: %+v", sost)
	}
	if v2.otlozhennyy == nil || !tot(v2.otlozhennyy, sertDomena) || v2.uchet.SposobOtlozhennogo != SposobHTTP {
		t.Fatal("копия на место сертификата домена не отложила его")
	}
}

// Истёкший отложенный удаляется и тогда, когда лежащий годен, а отложенный из
// другого каталога на место не встаёт.
func TestOtlozhennyyNegodnyy(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	nIP, nDomen := Nastroyka{IP: ip}, Nastroyka{IP: ip, Domen: domen}
	pereyti(t, v, nIP)
	sertDomena := pereyti(t, v, nDomen)
	s.chasy.vperyod(161 * time.Hour)
	v.krug(context.Background())
	katalogOtl := filepath.Join(s.katalog, "acme", katalogOtlozhennogo)
	if _, err := os.Stat(katalogOtl); v.otlozhennyy != nil || v.uchet.SposobOtlozhennogo != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("истёкший отложенный не удалён: %v", err)
	}
	pereyti(t, v, nIP)
	if v.otlozhennyy == nil || !tot(v.otlozhennyy, sertDomena) {
		t.Fatal("сертификат домена не отложен")
	}

	// Отложенный настоящий, а настройка велит staging.
	s.chasy.vperyod(time.Hour)
	if err := v.Nastroit(&Nastroyka{IP: ip, Domen: domen, Staging: true}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if c := v.Tekushchiy(); c == nil || tot(c, sertDomena) {
		t.Fatalf("отложенный из другого каталога встал на место или выпуска не было: %+v", v.Sostoyanie())
	}

	// Тестовый сертификат живой настоящий в отложенных не сменяет.
	seychas := s.chasy.seychas()
	nastoyashchiy := pemNa(t, ip, seychas.Add(-time.Hour), seychas.Add(100*time.Hour))
	n, err := tls.X509KeyPair([]byte(nastoyashchiy.Cert), []byte(nastoyashchiy.Klyuch))
	if err != nil {
		t.Fatal(err)
	}
	tst := testovyyNa(t, ip, seychas)
	if !staging(tst.Leaf) {
		t.Fatal("тестовый сертификат прибора не распознан тестовым")
	}
	v.mu.Lock()
	v.otlozhennyy = &n
	otlozhen := v.otlozhit(tst, SposobHTTP, seychas)
	v.mu.Unlock()
	if otlozhen || v.otlozhennyy != &n {
		t.Fatal("тестовый сертификат сменил в отложенных настоящий")
	}
}

// Пока шла попытка на домен, настройка вернулась на IP: выпущенный
// сертификат домена не выбрасывается, а ждёт в отложенных.
func TestOtlozhenVypushchennyyZrya(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	nIP, nDomen := Nastroyka{IP: ip}, Nastroyka{IP: ip, Domen: domen}
	sertIP := pereyti(t, v, nIP)
	s.brandmauer.priOtkrytii = func() {
		if err := v.Nastroit(&nIP, nil); err != nil {
			t.Error(err)
		}
	}
	if err := v.Nastroit(&nDomen, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	s.brandmauer.priOtkrytii = nil
	if c := v.Tekushchiy(); c == nil || !tot(c, sertIP) {
		t.Fatalf("на месте не прежний на IP: %+v", v.Sostoyanie())
	}
	if v.otlozhennyy == nil || !slices.Contains(v.otlozhennyy.Leaf.DNSNames, domen) {
		t.Fatal("выпущенный зря сертификат домена выброшен")
	}
	popytok := len(v.uchet.Popytki)
	if c := pereyti(t, v, nDomen); !slices.Contains(c.Leaf.DNSNames, domen) || len(v.uchet.Popytki) != popytok {
		t.Fatalf("переход на домен выпустил новый: попыток %d, было %d", len(v.uchet.Popytki), popytok)
	}
}

// Отказ записи: неудачный возврат не теряет отложенный, а неудачное
// откладывание не оставляет полупары. Учёт без каталога отложенного
// способа его не помнит.
func TestOtlozhennyyOtkazZapisi(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	nIP, nDomen := Nastroyka{IP: ip}, Nastroyka{IP: ip, Domen: domen}
	sertIP := pereyti(t, v, nIP)
	pereyti(t, v, nDomen)

	// Цепочку на место не положить: там каталог.
	cepochka := filepath.Join(s.katalog, "sert-ip", imyaSerta)
	pomeshat := func(put string) {
		t.Helper()
		if err := os.Remove(put); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(put, "zanyato"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pomeshat(cepochka)
	if err := v.Nastroit(&nIP, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if v.Tekushchiy() != nil || v.otlozhennyy == nil || !tot(v.otlozhennyy, sertIP) {
		t.Fatal("неудачный возврат выдан за удачный или потерял отложенный")
	}
	if err := os.RemoveAll(cepochka); err != nil {
		t.Fatal(err)
	}
	if c := pereyti(t, v, nIP); !tot(c, sertIP) {
		t.Fatal("после снятия помехи отложенный не вернулся")
	}

	// Отложить сертификат домена не выйдет: на месте цепочки отложенного каталог.
	katalogOtl := filepath.Join(s.katalog, "acme", katalogOtlozhennogo)
	pomeshat(filepath.Join(katalogOtl, imyaSerta))
	if err := v.Nastroit(&nIP, nil); err != nil {
		t.Fatal(err)
	}
	v.mu.Lock()
	otlozhen := v.otlozhit(v.tekushchiy, SposobHTTP, s.chasy.seychas())
	v.mu.Unlock()
	if _, err := os.Stat(katalogOtl); otlozhen || v.otlozhennyy != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("после неудачного откладывания остался отложенный или полупара: %v", err)
	}

	v2 := s.novyy(t)
	if v2.otlozhennyy != nil || v2.uchet.SposobOtlozhennogo != "" {
		t.Fatalf("без каталога отложенного учёт помнит способ %q", v2.uchet.SposobOtlozhennogo)
	}
}

// 80/tcp держит сайт режима домена: свой слушатель выпуск не поднимает и
// брандмауэр не трогает, ответ отдаёт сайт.
func TestProverkaCherezSayt(t *testing.T) {
	s := novyyStend(t)
	v := s.novyy(t)
	v.Derzhat80(true)
	ln, err := net.Listen("tcp", net.JoinHostPort("", itoa(s.opcii.PortHTTP)))
	if err != nil {
		t.Fatal(err)
	}
	var chuzhih atomic.Int32
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !v.OtvetitProverke(w, r) {
			chuzhih.Add(1)
			http.NotFound(w, r)
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })

	if err := v.Nastroit(&Nastroyka{IP: ip, Domen: domen}, nil); err != nil {
		t.Fatal(err)
	}
	v.krug(context.Background())
	if sost := v.Sostoyanie(); !sost.Est || sost.Sposob != SposobHTTP {
		t.Fatalf("через сайт не выпущен: %+v", sost)
	}
	if s.brandmauer.vsego != 0 {
		t.Fatalf("брандмауэр открывался %d раз, а 80 держит сайт", s.brandmauer.vsego)
	}
	if chuzhih.Load() != 0 {
		t.Fatalf("сайт не узнал проверку %d раз", chuzhih.Load())
	}
	if len(v.otvety80) != 0 {
		t.Fatal("ответ проверки остался после попытки")
	}
}

func TestNastroykaDomena(t *testing.T) {
	for _, d := range []string{"kot.example.net", "xn--80ak6aa92e.xn--p1ai", "a.b.co"} {
		if err := (Nastroyka{IP: ip, Domen: d}).Proverit(); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	for _, d := range []string{"localhost", "127.0.0.1", "Kot.example.net", "kot..net", "-kot.net", "*.kot.net", "kot.net/x"} {
		if err := (Nastroyka{IP: ip, Domen: d}).Proverit(); err == nil {
			t.Errorf("%s принят за имя сайта", d)
		}
	}
}

// testovyyNa выпускает самоподписанный сертификат на адрес, похожий на
// выданный тестовым каталогом Let's Encrypt, на 100 часов.
func testovyyNa(t *testing.T, adres string, seychas time.Time) *tls.Certificate {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "(STAGING) Pebble"},
		IPAddresses:  []net.IP{net.ParseIP(adres)},
		NotBefore:    seychas.Add(-time.Hour),
		NotAfter:     seychas.Add(100 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: klyuch, Leaf: leaf}
}

// pemNa выпускает самоподписанный сертификат на адрес с заданным сроком.
func pemNa(t *testing.T, adres string, ot, do time.Time) Pem {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		IPAddresses:  []net.IP{net.ParseIP(adres)},
		NotBefore:    ot,
		NotAfter:     do,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	kDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		t.Fatal(err)
	}
	return Pem{
		Cert:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		Klyuch: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kDer})),
	}
}
