package sertip

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
)

const ip = "127.0.0.1"

// zapisBrandmauera помнит, какие порты открыты сейчас и сколько раз
// открывались.
type zapisBrandmauera struct {
	mu      sync.Mutex
	otkryto map[int]int
	vsego   int
}

func (z *zapisBrandmauera) Otkryt(_ net.IP, port int) (func() error, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.otkryto[port]++
	z.vsego++
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
	proveryayushchiy := va.New(tiho, portHTTP, portTLS, false, "", baza)
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
		KatalogACME: srv.URL + wfe.DirectoryPath, HTTPKlient: srv.Client(),
	})
	return s
}

func (s *stend) novyyV(t *testing.T, katalog string) *Vypusk {
	t.Helper()
	zhurnal := slog.New(slog.NewTextHandler(io.Discard, nil))
	return Novyy(filepath.Join(katalog, "acme"), filepath.Join(katalog, "sert-ip"), zhurnal,
		func() { s.obnovleno.Add(1) }, s.opcii)
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
