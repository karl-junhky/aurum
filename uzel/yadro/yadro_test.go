package yadro_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	sjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/zxczxczxczxczxczxc1111/affory/uzel/sayt"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/sertip"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/trafik"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/vhody"
	"github.com/zxczxczxczxczxczxc1111/affory/uzel/yadro"
)

// Протоколы, как они стоят на серверах, плюс мультиплекс и gRPC: через них
// у одного сырого соединения много потоков, и отзыв обязан рвать все.
var protokoly = []string{"vless", "vmux", "vgrpc", "trojan", "anytls", "hy2"}

type chelovek struct {
	imya, uuid, parol string
}

var (
	lyudA = chelovek{"anna", "11111111-1111-4111-8111-111111111111", "parol-anny"}
	lyudB = chelovek{"boris", "22222222-2222-4222-8222-222222222222", "parol-borisa"}
)

type stend struct {
	t       *testing.T
	porty   map[string]int
	sert    string
	klyuch  string
	eho     string
	y       *yadro.Yadro
	schyot  *trafik.Schyotchik
	katalog string
}

func novyyStend(t *testing.T) *stend {
	t.Helper()
	s := &stend{t: t, porty: map[string]int{}, katalog: t.TempDir()}
	s.sert, s.klyuch = samopodpisannyy(t)
	for _, p := range protokoly {
		s.porty[p] = svobodnyyPort(t, p == "hy2")
	}
	s.eho = zapustitEho(t)
	s.schyot = trafik.NovyySchyotchik()
	s.y = yadro.Novoe(s.katalog, s.schyot, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(s.y.Ostanovit)
	return s
}

func (s *stend) konfigServera(lyudi []chelovek, portHy2 int) map[string]any {
	vless := func() []any {
		var u []any
		for _, ch := range lyudi {
			u = append(u, map[string]any{"name": ch.imya, "uuid": ch.uuid})
		}
		return u
	}
	paroli := func() []any {
		var u []any
		for _, ch := range lyudi {
			u = append(u, map[string]any{"name": ch.imya, "password": ch.parol})
		}
		return u
	}
	tlsS := map[string]any{"enabled": true, "server_name": "localhost", "certificate_path": s.sert, "key_path": s.klyuch}
	return map[string]any{
		"log": map[string]any{"disabled": true},
		"inbounds": []any{
			map[string]any{"type": "vless", "tag": "vless", "listen": "127.0.0.1", "listen_port": s.porty["vless"], "users": vless()},
			map[string]any{"type": "vless", "tag": "vmux", "listen": "127.0.0.1", "listen_port": s.porty["vmux"], "users": vless(),
				"multiplex": map[string]any{"enabled": true}},
			map[string]any{"type": "vless", "tag": "vgrpc", "listen": "127.0.0.1", "listen_port": s.porty["vgrpc"], "users": vless(),
				"transport": map[string]any{"type": "grpc", "service_name": "proba"}},
			map[string]any{"type": "trojan", "tag": "trojan", "listen": "127.0.0.1", "listen_port": s.porty["trojan"], "users": paroli(), "tls": tlsS},
			map[string]any{"type": "anytls", "tag": "anytls", "listen": "127.0.0.1", "listen_port": s.porty["anytls"], "users": paroli(), "tls": tlsS},
			map[string]any{"type": "hysteria2", "tag": "hy2", "listen": "127.0.0.1", "listen_port": portHy2, "users": paroli(),
				"obfs": map[string]any{"type": "salamander", "password": "maska"},
				"tls":  map[string]any{"enabled": true, "server_name": "localhost", "alpn": []any{"h3"}, "certificate_path": s.sert, "key_path": s.klyuch}},
		},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "pryamo"}},
	}
}

func paket(t *testing.T, yadroKonfig map[string]any) []byte {
	t.Helper()
	p, err := json.Marshal(map[string]any{"versiya": 1, "yadro": yadroKonfig})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (s *stend) primenit(lyudi []chelovek, portHy2 int) yadro.Kvitanciya {
	s.t.Helper()
	return s.y.Primenit(paket(s.t, s.konfigServera(lyudi, portHy2)))
}

// klient поднимает sing-box с исходящими на каждый протокол для каждого
// человека: тег <протокол>-<имя>.
func (s *stend) klient(lyudi []chelovek, portHy2 int) *box.Box {
	s.t.Helper()
	return s.klientTLS(lyudi, portHy2, map[string]any{"enabled": true, "server_name": "localhost", "insecure": true})
}

// klientTLS как klient, но со своими настройками TLS у trojan, anytls и hy2.
func (s *stend) klientTLS(lyudi []chelovek, portHy2 int, tlsK map[string]any) *box.Box {
	t := s.t
	t.Helper()
	tlsHy2 := map[string]any{"alpn": []any{"h3"}}
	for k, v := range tlsK {
		tlsHy2[k] = v
	}
	var ishodyashchie []any
	for _, ch := range lyudi {
		ishodyashchie = append(ishodyashchie,
			map[string]any{"type": "vless", "tag": "vless-" + ch.imya, "server": "127.0.0.1", "server_port": s.porty["vless"], "uuid": ch.uuid},
			map[string]any{"type": "vless", "tag": "vmux-" + ch.imya, "server": "127.0.0.1", "server_port": s.porty["vmux"], "uuid": ch.uuid,
				// smux, а не h2mux: клиент h2mux на x/net 0.57 изредка виснет
				// на своём же замке (примерно раз в 5 прогонов), сервер тут ни при чём.
				"multiplex": map[string]any{"enabled": true, "protocol": "smux", "max_connections": 1, "min_streams": 8}},
			map[string]any{"type": "vless", "tag": "vgrpc-" + ch.imya, "server": "127.0.0.1", "server_port": s.porty["vgrpc"], "uuid": ch.uuid,
				"transport": map[string]any{"type": "grpc", "service_name": "proba"}},
			map[string]any{"type": "trojan", "tag": "trojan-" + ch.imya, "server": "127.0.0.1", "server_port": s.porty["trojan"], "password": ch.parol, "tls": tlsK},
			map[string]any{"type": "anytls", "tag": "anytls-" + ch.imya, "server": "127.0.0.1", "server_port": s.porty["anytls"], "password": ch.parol, "tls": tlsK},
			map[string]any{"type": "hysteria2", "tag": "hy2-" + ch.imya, "server": "127.0.0.1", "server_port": portHy2, "password": ch.parol,
				"obfs": map[string]any{"type": "salamander", "password": "maska"},
				"tls":  tlsHy2},
		)
	}
	syroe, err := json.Marshal(map[string]any{"log": map[string]any{"disabled": true}, "outbounds": ishodyashchie})
	if err != nil {
		t.Fatal(err)
	}
	ctx := include.Context(context.Background())
	opcii, err := sjson.UnmarshalExtendedContext[option.Options](ctx, syroe)
	if err != nil {
		t.Fatal(err)
	}
	b, err := box.New(box.Options{Context: ctx, Options: opcii})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

// soedinit открывает соединение до эха через исходящий teg.
func (s *stend) soedinit(k *box.Box, teg string) (net.Conn, error) {
	out, est := k.Outbound().Outbound(teg)
	if !est {
		s.t.Fatalf("нет исходящего %s", teg)
	}
	ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
	defer otmena()
	return out.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(s.eho))
}

// pereklichka шлёт строку и ждёт её обратно. Срок держит таймер, а не
// SetDeadline: потоки h2mux на стороне клиента сроков не умеют.
func pereklichka(c net.Conn, stroka string, srok time.Duration) error {
	itog := make(chan error, 1)
	go func() {
		if _, err := io.WriteString(c, stroka+"\n"); err != nil {
			itog <- err
			return
		}
		otvet, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			itog <- err
			return
		}
		if otvet != stroka+"\n" {
			itog <- fmt.Errorf("эхо вернуло %q вместо %q", otvet, stroka)
			return
		}
		itog <- nil
	}()
	select {
	case err := <-itog:
		return err
	case <-time.After(srok):
		c.Close()
		return fmt.Errorf("эхо не ответило за %v", srok)
	}
}

// mozhet проверяет, что человек пользуется протоколом с нуля.
func (s *stend) mozhet(k *box.Box, teg string) error {
	c, err := s.soedinit(k, teg)
	if err != nil {
		return err
	}
	defer c.Close()
	return pereklichka(c, "proba-"+teg, 5*time.Second)
}

func TestSostavNaLetu(t *testing.T) {
	s := novyyStend(t)
	k := s.primenit([]chelovek{lyudA}, s.porty["hy2"])
	if k.Oshibka != "" || k.Kak != yadro.Perezapusk || !k.YadroZhivo {
		t.Fatalf("первый пакет: %+v", k)
	}
	klient := s.klient([]chelovek{lyudA, lyudB}, s.porty["hy2"])

	// Анна держит по живому соединению на каждом протоколе.
	unnaSoed := map[string]net.Conn{}
	for _, p := range protokoly {
		c, err := s.soedinit(klient, p+"-anna")
		if err != nil {
			t.Fatalf("%s: Анна не соединилась: %v", p, err)
		}
		defer c.Close()
		if err := pereklichka(c, "do-borisa", 5*time.Second); err != nil {
			t.Fatalf("%s: эхо Анны: %v", p, err)
		}
		unnaSoed[p] = c
	}
	for _, p := range protokoly {
		if err := s.mozhet(klient, p+"-boris"); err == nil {
			t.Fatalf("%s: Борис прошёл до выдачи", p)
		}
	}

	// Выдача Борису: на лету, у Анны ничего не рвётся.
	k = s.primenit([]chelovek{lyudA, lyudB}, s.porty["hy2"])
	if k.Oshibka != "" || k.Kak != yadro.NaLetu || k.Zakryto != 0 {
		t.Fatalf("выдача: %+v", k)
	}
	borisSoed := map[string]net.Conn{}
	for _, p := range protokoly {
		if err := pereklichka(unnaSoed[p], "posle-vydachi", 5*time.Second); err != nil {
			t.Fatalf("%s: соединение Анны порвалось при выдаче Борису: %v", p, err)
		}
		c, err := s.soedinit(klient, p+"-boris")
		if err != nil {
			t.Fatalf("%s: Борис не соединился после выдачи: %v", p, err)
		}
		defer c.Close()
		if err := pereklichka(c, "boris", 5*time.Second); err != nil {
			t.Fatalf("%s: эхо Бориса после выдачи: %v", p, err)
		}
		borisSoed[p] = c
	}

	// Отзыв Анны: её соединения рвутся сразу, у Бориса ничего.
	k = s.primenit([]chelovek{lyudB}, s.porty["hy2"])
	if k.Oshibka != "" || k.Kak != yadro.NaLetu {
		t.Fatalf("отзыв: %+v", k)
	}
	if k.Zakryto < len(protokoly) {
		t.Fatalf("отзыв закрыл %d соединений, а у Анны их было не меньше %d", k.Zakryto, len(protokoly))
	}
	for _, p := range protokoly {
		nachalo := time.Now()
		err := pereklichka(unnaSoed[p], "posle-otzyva", 5*time.Second)
		if err == nil {
			t.Fatalf("%s: соединение Анны живо после отзыва", p)
		}
		if proshlo := time.Since(nachalo); proshlo > 3*time.Second {
			t.Fatalf("%s: соединение Анны умерло только через %v (%v), а должно сразу", p, proshlo, err)
		}
		if err := pereklichka(borisSoed[p], "posle-otzyva-anny", 5*time.Second); err != nil {
			t.Fatalf("%s: соединение Бориса порвалось при отзыве Анны: %v", p, err)
		}
		if err := s.mozhet(klient, p+"-anna"); err == nil {
			t.Fatalf("%s: Анна снова прошла после отзыва", p)
		}
	}

	snyato := s.schyot.Snyat()
	for _, imya := range []string{"anna", "boris"} {
		if d := snyato.Lyudi[imya]; d[0] == 0 || d[1] == 0 {
			t.Fatalf("трафик %s не посчитан: %v", imya, snyato.Lyudi)
		}
		// Трафик разложен по всем входам, и разбивка сходится с суммой.
		var summa [2]int64
		for _, d := range snyato.PoVhodam[imya] {
			summa[0] += d[0]
			summa[1] += d[1]
		}
		if len(snyato.PoVhodam[imya]) != len(protokoly) || summa != snyato.Lyudi[imya] {
			t.Fatalf("трафик %s по входам %v, сумма %v", imya, snyato.PoVhodam[imya], snyato.Lyudi[imya])
		}
	}
	// Отметки входов: оба вошли на каждом входе, соединения Анны отмечены
	// отзывом, у gRPC виден и приход сокета.
	est := map[string]bool{}
	for _, o := range snyato.Otmetki {
		est[o.Vhod+" "+o.Adres+" "+o.Imya+" "+o.Vid] = true
	}
	for _, p := range protokoly {
		for _, zhdu := range []string{p + " 127.0.0.1 anna vhod", p + " 127.0.0.1 boris vhod", p + " 127.0.0.1 anna otozvan"} {
			if !est[zhdu] {
				t.Errorf("нет отметки %q", zhdu)
			}
		}
	}
	if !est["vgrpc 127.0.0.1  priem"] {
		t.Error("нет отметки прихода сокета gRPC")
	}
	if t.Failed() {
		t.Fatalf("отметки: %+v", snyato.Otmetki)
	}

	// Всё закрыто, учёт пустеет: записи не текут.
	for _, c := range borisSoed {
		c.Close()
	}
	klient.Close()
	zhdat(t, 10*time.Second, "учёт не опустел после закрытия клиента", func() bool {
		for _, v := range s.y.Zdorove().Vhody {
			if v.Soedineniy != 0 {
				return false
			}
		}
		return true
	})
}

// Отзыв рвёт соединение сразу, даже когда в буферах лежат мегабайты: на
// живой пробе 15.10 вежливое закрытие дало отозванному дочитывать ещё 28 с.
func TestOtzyvBezDochityvaniya(t *testing.T) {
	s := novyyStend(t)
	istochnik := zapustitFontan(t)
	if k := s.primenit([]chelovek{lyudA}, s.porty["hy2"]); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	klient := s.klient([]chelovek{lyudA}, s.porty["hy2"])
	type chtenie struct {
		konec time.Time
		err   error
	}
	itogi := map[string]chan chtenie{}
	bezHy2 := []string{"vless", "vmux", "vgrpc", "trojan", "anytls"}
	for _, p := range bezHy2 {
		out, _ := klient.Outbound().Outbound(p + "-anna")
		ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
		c, err := out.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(istochnik.adres))
		otmena()
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		defer c.Close()
		// Клиенты vless и trojan шлют заголовок вместе с первыми данными: без
		// записи соединение до сервера не доходит вовсе.
		if _, err := c.Write([]byte("start")); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		itog := make(chan chtenie, 1)
		itogi[p] = itog
		go func() {
			// Медленный читатель: буферы по дороге забиваются доверху.
			buf := make([]byte, 16<<10)
			for {
				if _, err := c.Read(buf); err != nil {
					itog <- chtenie{time.Now(), err}
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}
	time.Sleep(2 * time.Second)
	if n := istochnik.aktivnyh.Load(); n != int32(len(bezHy2)) {
		t.Fatalf("до отзыва к источнику открыто %d соединений, ждали %d", n, len(bezHy2))
	}
	otozvano := time.Now()
	if k := s.primenit(nil, s.porty["hy2"]); k.Oshibka != "" || k.Zakryto < len(bezHy2) {
		t.Fatalf("отзыв: %+v", k)
	}
	// Закрытие TLS ждёт отправки close_notify до 5 с, если канал забит; на
	// 188 так отзыв шёл 5 секунд. Сокет рвётся первым, и это мгновенно.
	if proshlo := time.Since(otozvano); proshlo > time.Second {
		t.Fatalf("отзыв применялся %v", proshlo)
	}
	// Сервер перестаёт возить сразу: ни одного соединения до источника.
	zhdat(t, time.Second, "через секунду после отзыва сервер ещё возит отозванному", func() bool {
		return istochnik.aktivnyh.Load() == 0
	})
	// Обрыв должен быть сбросом сокета, а не вежливым концом потока: при
	// вежливом сервер досылает всё, что лежит в его буферах. Отличить их можно
	// только по ошибке у клиента. По времени нельзя: мультиплекс и gRPC держат
	// у клиента своё окно на мегабайты и честно отдают его до ошибки в обоих
	// случаях, это уже доставленное. Поэтому им срок длиннее, остальным секунда.
	for p, itog := range itogi {
		srok := time.Second
		if p == "vmux" || p == "vgrpc" {
			srok = 10 * time.Second
		}
		select {
		case r := <-itog:
			if errors.Is(r.err, io.EOF) {
				t.Fatalf("%s: соединение закрыто вежливо, а не сброшено", p)
			}
			if proshlo := r.konec.Sub(otozvano); proshlo > srok {
				t.Fatalf("%s: отозванный дочитывал буферы %v", p, proshlo)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: чтение не оборвалось и за 10 с после отзыва", p)
		}
	}
}

func TestSmenaParolyaKakOtzyv(t *testing.T) {
	s := novyyStend(t)
	if k := s.primenit([]chelovek{lyudA}, s.porty["hy2"]); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	staryy := s.klient([]chelovek{lyudA}, s.porty["hy2"])
	soed := map[string]net.Conn{}
	for _, p := range []string{"trojan", "hy2", "vless"} {
		c, err := s.soedinit(staryy, p+"-anna")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := pereklichka(c, "staryy", 5*time.Second); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		soed[p] = c
	}
	novaya := chelovek{lyudA.imya, "33333333-3333-4333-8333-333333333333", "novyy-parol-anny"}
	k := s.primenit([]chelovek{novaya}, s.porty["hy2"])
	if k.Oshibka != "" || k.Kak != yadro.NaLetu || k.Zakryto < 3 {
		t.Fatalf("смена пароля: %+v", k)
	}
	for p, c := range soed {
		if err := pereklichka(c, "posle", 5*time.Second); err == nil {
			t.Fatalf("%s: соединение по старому паролю живо", p)
		}
	}
	svezhiy := s.klient([]chelovek{novaya}, s.porty["hy2"])
	for p := range soed {
		if err := s.mozhet(svezhiy, p+"-anna"); err != nil {
			t.Fatalf("%s: новый пароль не пускает: %v", p, err)
		}
	}
}

// Смена входа идёт перезапуском, и клиент hy2 узнаёт о закрытии сразу, а не
// через 30 с тишины.
func TestPerezapuskZakryvaetQUICSrazu(t *testing.T) {
	s := novyyStend(t)
	if k := s.primenit([]chelovek{lyudB}, s.porty["hy2"]); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	klient := s.klient([]chelovek{lyudB}, s.porty["hy2"])
	c, err := s.soedinit(klient, "hy2-boris")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := pereklichka(c, "do", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	novyyPort := svobodnyyPort(t, true)
	k := s.primenit([]chelovek{lyudB}, novyyPort)
	if k.Oshibka != "" || k.Kak != yadro.Perezapusk || !k.YadroZhivo {
		t.Fatalf("смена порта hy2: %+v", k)
	}
	if err := c.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	nachalo := time.Now()
	_, err = c.Read(make([]byte, 16))
	if err == nil {
		t.Fatal("поток hy2 жив после перезапуска ядра")
	}
	if proshlo := time.Since(nachalo); proshlo > 3*time.Second {
		t.Fatalf("клиент hy2 узнал о закрытии через %v (%v): закрытие не ушло", proshlo, err)
	}
	if err := s.mozhet(s.klient([]chelovek{lyudB}, novyyPort), "hy2-boris"); err != nil {
		t.Fatalf("hy2 на новом порту: %v", err)
	}
}

// Пакет, который ядро не поднимает, не гасит работающее.
func TestOtkatNaPrezhnee(t *testing.T) {
	s := novyyStend(t)
	k := s.primenit([]chelovek{lyudB}, s.porty["hy2"])
	if k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	prezhniy := k.Hesh
	klient := s.klient([]chelovek{lyudB}, s.porty["hy2"])

	// Порт занят чужим: ядро соберётся, но не запустится.
	zanyat, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer zanyat.Close()
	konfig := s.konfigServera([]chelovek{lyudB}, s.porty["hy2"])
	konfig["inbounds"].([]any)[0].(map[string]any)["listen_port"] = zanyat.Addr().(*net.TCPAddr).Port
	k = s.y.Primenit(paket(t, konfig))
	if k.Oshibka == "" || !strings.Contains(k.Oshibka, "работает прежнее") {
		t.Fatalf("ждали отката: %+v", k)
	}
	if k.Hesh != prezhniy || !k.YadroZhivo {
		t.Fatalf("после отката работает не прежнее: %+v", k)
	}
	portZanyat := zanyat.Addr().(*net.TCPAddr).Port
	if z := k.Zanyat; z == nil || z.Port != portZanyat || z.Proto != "tcp" || z.Vhod != "vless" || z.Prichina == "" {
		t.Fatalf("занятый порт в ответе: %+v", z)
	}

	// То же для UDP: порт hy2 держит чужой сокет.
	zanyatU, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer zanyatU.Close()
	portU := zanyatU.LocalAddr().(*net.UDPAddr).Port
	k = s.y.Primenit(paket(t, s.konfigServera([]chelovek{lyudB}, portU)))
	if z := k.Zanyat; k.Hesh != prezhniy || z == nil || z.Port != portU || z.Proto != "udp" || z.Vhod != "hy2" {
		t.Fatalf("занятый UDP-порт: %+v, %+v", k, z)
	}
	if err := s.mozhet(klient, "trojan-boris"); err != nil {
		t.Fatalf("прежнее ядро после отката не пускает: %v", err)
	}

	// Мусор вместо конфига отбивается до того, как трогать ядро.
	k = s.y.Primenit([]byte(`{"versiya":1,"yadro":{"inbounds":[{"type":"vless","listen_port":"ne-chislo"}]}}`))
	if k.Oshibka == "" || k.Hesh != prezhniy || !k.YadroZhivo {
		t.Fatalf("мусор: %+v", k)
	}
	k = s.y.Primenit([]byte(`{"versiya":4,"yadro":{}}`))
	if k.Oshibka == "" || k.Hesh != prezhniy {
		t.Fatalf("чужая версия пакета: %+v", k)
	}
}

// После перезапуска программы ядро поднимается из того, что лежит на диске,
// без центра.
func TestPodnyatieSDiska(t *testing.T) {
	s := novyyStend(t)
	k := s.primenit([]chelovek{lyudB}, s.porty["hy2"])
	if k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	s.y.Ostanovit()

	y2 := yadro.Novoe(s.katalog, trafik.NovyySchyotchik(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(y2.Ostanovit)
	if err := y2.Zapustit(); err != nil {
		t.Fatal(err)
	}
	if z := y2.Zdorove(); !z.YadroZhivo || z.Hesh != k.Hesh {
		t.Fatalf("с диска поднялось не то: %+v", z)
	}
	if err := s.mozhet(s.klient([]chelovek{lyudB}, s.porty["hy2"]), "anytls-boris"); err != nil {
		t.Fatal(err)
	}
	if k2 := y2.Primenit(paket(t, s.konfigServera([]chelovek{lyudB}, s.porty["hy2"]))); k2.Kak != yadro.BezIzmeneniy {
		t.Fatalf("тот же пакет после подъёма с диска: %+v", k2)
	}
}

func zhdat(t *testing.T, srok time.Duration, chto string, gotovo func() bool) {
	t.Helper()
	konec := time.Now().Add(srok)
	for time.Now().Before(konec) {
		if gotovo() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(chto)
}

func svobodnyyPort(t *testing.T, udp bool) int {
	t.Helper()
	if udp {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func zapustitEho(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c) // эхо до закрытия, ошибка конца тут норма
			}()
		}
	}()
	return l.Addr().String()
}

type fontan struct {
	adres    string
	aktivnyh atomic.Int32
}

// zapustitFontan льёт данные в каждое соединение без остановки и считает,
// сколько соединений к нему открыто.
func zapustitFontan(t *testing.T) *fontan {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f := &fontan{adres: l.Addr().String()}
	go func() {
		kusok := make([]byte, 64<<10)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			f.aktivnyh.Add(1)
			go func() {
				defer f.aktivnyh.Add(-1)
				defer c.Close()
				for {
					if _, err := c.Write(kusok); err != nil {
						return
					}
				}
			}()
		}
	}()
	return f
}

func samopodpisannyy(t *testing.T) (string, string) {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	klyuchDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		t.Fatal(err)
	}
	katalog := t.TempDir()
	putS, putK := filepath.Join(katalog, "sert.pem"), filepath.Join(katalog, "klyuch.pem")
	if err := os.WriteFile(putS, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(putK, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: klyuchDer}), 0o600); err != nil {
		t.Fatal(err)
	}
	return putS, putK
}

// sertPEM выпускает самоподписанный сертификат на адрес или имя и отдаёт
// его и ключ текстом, как они едут в пакете.
func sertPEM(t *testing.T, kuda string) (string, string) {
	t.Helper()
	klyuch, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shablon := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(kuda); ip != nil {
		shablon.IPAddresses = []net.IP{ip}
	} else {
		shablon.DNSNames = []string{kuda}
	}
	der, err := x509.CreateCertificate(rand.Reader, shablon, shablon, &klyuch.PublicKey, klyuch)
	if err != nil {
		t.Fatal(err)
	}
	klyuchDer, err := x509.MarshalECPrivateKey(klyuch)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: klyuchDer}))
}

// Свой сертификат на IP из пакета предъявляют trojan, anytls и hy2, хотя в
// настройках входов стоит другой файл, а продлённый подхватывается без
// перезапуска ядра, в том числе QUIC-слушателем hy2.
func TestSvoySertifikatBezPerezapuska(t *testing.T) {
	s := novyyStend(t)
	t.Cleanup(func() { vhody.PostavitSerty(nil) })
	konfig := s.konfigServera([]chelovek{lyudA}, s.porty["hy2"])
	paketS := func(cert, klyuch string) []byte {
		p, err := json.Marshal(map[string]any{"versiya": 2, "yadro": konfig,
			"sert_ip": map[string]string{"cert": cert, "klyuch": klyuch}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Клиент по ссылке на IP: имени в приветствии нет, верит только cert.
	doveryaet := func(cert string) map[string]any {
		return map[string]any{"enabled": true, "server_name": "127.0.0.1", "certificate": []string{cert}}
	}
	tegi := []string{"trojan-anna", "anytls-anna", "hy2-anna"}

	c1, k1 := sertPEM(t, "127.0.0.1")
	if k := s.y.Primenit(paketS(c1, k1)); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	if b, err := os.ReadFile(filepath.Join(s.katalog, "sert-ip", "fullchain.pem")); err != nil || string(b) != c1 {
		t.Fatalf("свой сертификат не лёг в sert-ip: %v", err)
	}
	staryy := s.klientTLS([]chelovek{lyudA}, s.porty["hy2"], doveryaet(c1))
	for _, teg := range tegi {
		if err := s.mozhet(staryy, teg); err != nil {
			t.Fatalf("%s со своим сертификатом: %v", teg, err)
		}
	}

	c2, k2 := sertPEM(t, "127.0.0.1")
	if k := s.y.Primenit(paketS(c2, k2)); k.Oshibka != "" || k.Kak != yadro.YadroPrezhnee {
		t.Fatalf("продление: %+v", k)
	}
	novyy := s.klientTLS([]chelovek{lyudA}, s.porty["hy2"], doveryaet(c2))
	for _, teg := range tegi {
		if err := s.mozhet(novyy, teg); err != nil {
			t.Fatalf("%s с продлённым: %v", teg, err)
		}
	}
	// Клиент, который верит только прежнему, нового соединения trojan открыть
	// не может: значит, предъявляется уже продлённый. anytls и hy2 так не
	// проверить: их клиент пускает новые потоки по уже открытой сессии.
	if err := s.mozhet(staryy, "trojan-anna"); err == nil {
		t.Fatal("trojan всё ещё предъявляет прежний сертификат")
	}

	ci, ki := sertPEM(t, "imya.example.net")
	if k := s.y.Primenit(paketS(ci, ki)); !strings.Contains(k.Oshibka, "не на IP") {
		t.Fatalf("сертификат на имя вместо своего: %+v", k)
	}
}

// Свой VPN, пакет третьей версии: сертификат на IP ведёт выпуск самой
// программы. Пакет «только сертификат» поднимает пустое ядро; до первого
// сертификата TLS-входы стоят на заглушке и не мешают подняться остальным;
// копия из пакета доходит до входов без перезапуска; пакет центра выпуск
// выключает.
func TestSvoyVPNSertifikatVypuska(t *testing.T) {
	s := novyyStend(t)
	t.Cleanup(func() { vhody.PostavitSerty(nil) })
	if k := s.y.Primenit([]byte(`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"127.0.0.1"}}`)); !strings.Contains(k.Oshibka, "не подключён") {
		t.Fatalf("третья версия без выпуска в программе: %+v", k)
	}
	// Каталог ACME никуда не ведёт: попыток в тесте нет, Rabotat не запущен.
	v := s.y.PodklyuchitVypusk(sertip.Opcii{KatalogACME: "https://127.0.0.1:1/dir"})

	if k := s.y.Primenit([]byte(`{"versiya":3,"yadro":{},"sert_ip_sam":{"ip":"127.0.0.1"}}`)); k.Oshibka != "" || !k.YadroZhivo {
		t.Fatalf("пакет только с сертификатом: %+v", k)
	}
	if st := v.Sostoyanie(); st == nil || st.Est || st.IP != "127.0.0.1" {
		t.Fatalf("выпуск после пакета только с сертификатом: %+v", st)
	}

	konfig := s.konfigServera([]chelovek{lyudA}, s.porty["hy2"])
	naSvoySert(t, konfig, filepath.Join(s.katalog, "sert-ip"))
	paketV3 := func(kopiya map[string]string) []byte {
		p := map[string]any{"versiya": 3, "yadro": konfig, "sert_ip_sam": map[string]any{"ip": "127.0.0.1"}}
		if kopiya != nil {
			p["sert_ip"] = kopiya
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if k := s.y.Primenit(paketV3(nil)); k.Oshibka != "" || k.Kak != yadro.Perezapusk {
		t.Fatalf("входы на заглушке: %+v", k)
	}
	if v.Tekushchiy() != nil {
		t.Fatal("заглушка отдана входам как свой сертификат")
	}
	nedoverchivyy := s.klient([]chelovek{lyudA}, s.porty["hy2"])
	for _, teg := range []string{"vless-anna", "trojan-anna", "hy2-anna"} {
		if err := s.mozhet(nedoverchivyy, teg); err != nil {
			t.Fatalf("%s при заглушке: %v", teg, err)
		}
	}

	c1, k1 := sertPEM(t, "127.0.0.1")
	if k := s.y.Primenit(paketV3(map[string]string{"cert": c1, "klyuch": k1})); k.Oshibka != "" || k.Kak != yadro.YadroPrezhnee {
		t.Fatalf("копия сертификата: %+v", k)
	}
	if st := v.Sostoyanie(); !st.Est || st.Sposob != sertip.SposobKopiya {
		t.Fatalf("копия не легла: %+v", st)
	}
	doveryaet := func(cert string) map[string]any {
		return map[string]any{"enabled": true, "server_name": "127.0.0.1", "certificate": []string{cert}}
	}
	if err := s.mozhet(s.klientTLS([]chelovek{lyudA}, s.porty["hy2"], doveryaet(c1)), "trojan-anna"); err != nil {
		t.Fatalf("trojan не предъявляет копию: %v", err)
	}

	c2, k2 := sertPEM(t, "127.0.0.1")
	p2, err := json.Marshal(map[string]any{"versiya": 2, "yadro": konfig, "sert_ip": map[string]string{"cert": c2, "klyuch": k2}})
	if err != nil {
		t.Fatal(err)
	}
	if k := s.y.Primenit(p2); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	if v.Sostoyanie() != nil {
		t.Fatal("пакет центра не выключил выпуск")
	}
	if err := s.mozhet(s.klientTLS([]chelovek{lyudA}, s.porty["hy2"], doveryaet(c2)), "trojan-anna"); err != nil {
		t.Fatalf("trojan не предъявляет сертификат центра: %v", err)
	}
}

// Режим своего домена: сайт поднимается вместе с пакетом, гаснет с пакетом
// без него и встаёт с диска после перезапуска программы.
func TestSaytVPakete(t *testing.T) {
	s := novyyStend(t)
	podklyuchit := func(y *yadro.Yadro) (*sertip.Vypusk, *sayt.Sayt) {
		v := y.PodklyuchitVypusk(sertip.Opcii{KatalogACME: "https://127.0.0.1:1/dir"})
		return v, y.PodklyuchitSayt(sayt.Opcii{Port80: "127.0.0.1:0", Brandmauer: &bezBrandmauera{}})
	}
	v, st := podklyuchit(s.y)
	portSayta := svobodnyyPortTCP(t)
	nastroyka := map[string]any{"domen": "kot.example.net", "port": 443, "slushat": "127.0.0.1:" + strconv.Itoa(portSayta),
		"zerno":    "0123456789abcdef0123456789abcdef",
		"podpiska": map[string]any{"put": "fedcba9876543210fedcba9876543210", "telo": "x"}}
	sDomenom := func(saytV any) []byte {
		p := map[string]any{"versiya": 3, "yadro": map[string]any{},
			"sert_ip_sam": map[string]any{"ip": "127.0.0.1", "domen": "kot.example.net"}}
		if saytV != nil {
			p["sayt"] = saytV
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if k := s.y.Primenit(sDomenom(nastroyka)); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	if sost := st.Sostoyanie(); sost == nil || !sost.Https || !sost.Http || sost.Domen != "kot.example.net" {
		t.Fatalf("сайт после пакета: %+v", sost)
	}
	if sost := v.Sostoyanie(); sost == nil || sost.Domen != "kot.example.net" {
		t.Fatalf("выпуск не на домен: %+v", sost)
	}
	if k := s.y.Primenit(sDomenom(nil)); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	if st.Sostoyanie() != nil {
		t.Fatal("пакет без сайта его не погасил")
	}
	if k := s.y.Primenit(sDomenom(nastroyka)); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	s.y.Ostanovit()
	st.Ostanovit()

	y2 := yadro.Novoe(s.katalog, trafik.NovyySchyotchik(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, st2 := podklyuchit(y2)
	t.Cleanup(y2.Ostanovit)
	t.Cleanup(st2.Ostanovit)
	if err := y2.Zapustit(); err != nil {
		t.Fatal(err)
	}
	if sost := st2.Sostoyanie(); sost == nil || !sost.Https {
		t.Fatalf("сайт не встал с диска: %+v", sost)
	}
}

type bezBrandmauera struct{}

func (bezBrandmauera) Otkryt80() error { return nil }
func (bezBrandmauera) Ubrat() error    { return nil }

func svobodnyyPortTCP(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// naSvoySert переводит TLS-входы конфига на файлы каталога своего
// сертификата, как в пакете своего VPN.
func naSvoySert(t *testing.T, konfig map[string]any, katalog string) {
	t.Helper()
	for _, syroy := range konfig["inbounds"].([]any) {
		vhod := syroy.(map[string]any)
		tlsV, est := vhod["tls"].(map[string]any)
		if !est {
			continue
		}
		kopiya := maps.Clone(tlsV)
		kopiya["certificate_path"] = filepath.Join(katalog, "fullchain.pem")
		kopiya["key_path"] = filepath.Join(katalog, "privkey.pem")
		vhod["tls"] = kopiya
	}
}

// zapisBrandmauera помнит, какие порты пробы открыты сейчас.
type zapisBrandmauera struct {
	mu      sync.Mutex
	otkryto map[string]bool
}

func (z *zapisBrandmauera) Otkryt(proto string, port int) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.otkryto[fmt.Sprintf("%d/%s", port, proto)] = true
	return nil
}

func (z *zapisBrandmauera) Ubrat() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	clear(z.otkryto)
	return nil
}

func (z *zapisBrandmauera) skolko() int {
	z.mu.Lock()
	defer z.mu.Unlock()
	return len(z.otkryto)
}

func paketV3(t *testing.T, konfig map[string]any, srokProby int) []byte {
	t.Helper()
	p := map[string]any{"versiya": 3, "yadro": konfig}
	if srokProby > 0 {
		p["proba"] = map[string]int{"srok": srokProby}
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Проба портов: ядро с кандидатами живёт свой срок, порты открыты, на диск
// проба не ложится. Тот же пакет продлевает срок, окончательный пакет
// снимает пробу, а брошенная проба возвращает принятое и закрывает порты.
func TestProbaPortov(t *testing.T) {
	defer yadro.UkorotitSrokProby(150 * time.Millisecond)() // срок 10 = 1,5 с
	s := novyyStend(t)
	b := &zapisBrandmauera{otkryto: map[string]bool{}}
	s.y.PodklyuchitBrandmauer(b)

	prinyatyy := paketV3(t, s.konfigServera([]chelovek{lyudB}, s.porty["hy2"]), 0)
	if k := s.y.Primenit(prinyatyy); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	// Третья версия это свой VPN: адрес подключившегося не пишется.
	s.schyot.Snyat()
	s.schyot.Otmetit("vless", "198.51.100.7", "boris", "vhod")
	if o := s.schyot.Snyat().Otmetki; len(o) != 1 || o[0].Adres != "" {
		t.Fatalf("в своём VPN записан адрес: %+v", o)
	}

	portProby := svobodnyyPort(t, true)
	proba := paketV3(t, s.konfigServera([]chelovek{lyudB}, portProby), 10)
	k := s.y.Primenit(proba)
	if k.Oshibka != "" || k.Hesh != yadro.Hesh(proba) {
		t.Fatalf("проба: %+v", k)
	}
	if b.skolko() != 6 || !b.otkryto[fmt.Sprintf("%d/udp", portProby)] || !b.otkryto[fmt.Sprintf("%d/tcp", s.porty["vless"])] {
		t.Fatalf("порты пробы: %v", b.otkryto)
	}
	if na, err := os.ReadFile(filepath.Join(s.katalog, "sostoyanie.json")); err != nil || string(na) != string(prinyatyy) {
		t.Fatalf("проба легла на диск: %v", err)
	}
	klient := s.klient([]chelovek{lyudB}, portProby)
	if err := s.mozhet(klient, "hy2-boris"); err != nil {
		t.Fatalf("hy2 пробы: %v", err)
	}
	do := s.y.Zdorove().ProbaDo
	if do == 0 {
		t.Fatal("в здоровье нет срока пробы")
	}

	time.Sleep(600 * time.Millisecond)
	if k := s.y.Primenit(proba); k.Kak != yadro.BezIzmeneniy {
		t.Fatalf("повтор пробы: %+v", k)
	}
	if s.y.Zdorove().ProbaDo < do {
		t.Fatal("повтор не продлил пробу")
	}
	time.Sleep(1100 * time.Millisecond) // первый срок уже прошёл бы, продлённый ещё нет
	if z := s.y.Zdorove(); z.Hesh != yadro.Hesh(proba) || z.ProbaDo == 0 {
		t.Fatalf("продлённая проба кончилась раньше срока: %+v", z)
	}

	zhdat(t, 5*time.Second, "брошенная проба не вернула принятое", func() bool {
		z := s.y.Zdorove()
		return z.Hesh == yadro.Hesh(prinyatyy) && z.YadroZhivo && z.ProbaDo == 0
	})
	if b.skolko() != 0 {
		t.Fatalf("после пробы остались правила: %v", b.otkryto)
	}
	if err := s.mozhet(s.klient([]chelovek{lyudB}, s.porty["hy2"]), "hy2-boris"); err != nil {
		t.Fatalf("принятое после пробы: %v", err)
	}

	// Окончательный пакет снимает пробу, срок её больше ничего не трогает.
	if k := s.y.Primenit(proba); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	okonchatelnyy := paketV3(t, s.konfigServera([]chelovek{lyudA, lyudB}, portProby), 0)
	if k := s.y.Primenit(okonchatelnyy); k.Oshibka != "" || k.Hesh != yadro.Hesh(okonchatelnyy) {
		t.Fatalf("окончательный: %+v", k)
	}
	if b.skolko() != 0 || s.y.Zdorove().ProbaDo != 0 {
		t.Fatalf("окончательный не снял пробу: %v", b.otkryto)
	}
	time.Sleep(1700 * time.Millisecond)
	if z := s.y.Zdorove(); z.Hesh != yadro.Hesh(okonchatelnyy) {
		t.Fatalf("срок снятой пробы вернул прежнее: %+v", z)
	}
	if prezhnee, err := os.ReadFile(filepath.Join(s.katalog, "sostoyanie.prezhnee.json")); err != nil || string(prezhnee) != string(prinyatyy) {
		t.Fatalf("прежним отложена не принятая: %v", err)
	}

	// Остановка программы посреди пробы снимает её правила.
	if k := s.y.Primenit(proba); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	s.y.Ostanovit()
	if b.skolko() != 0 {
		t.Fatalf("остановка оставила правила: %v", b.otkryto)
	}
}

// Проба на сервере, где ещё ничего не принято: истекла - ядро гаснет.
func TestProbaBezPrinyatogo(t *testing.T) {
	defer yadro.UkorotitSrokProby(20 * time.Millisecond)()
	s := novyyStend(t)
	b := &zapisBrandmauera{otkryto: map[string]bool{}}
	s.y.PodklyuchitBrandmauer(b)
	if k := s.y.Primenit(paketV3(t, s.konfigServera([]chelovek{lyudB}, s.porty["hy2"]), 10)); k.Oshibka != "" {
		t.Fatal(k.Oshibka)
	}
	zhdat(t, 3*time.Second, "проба без принятого не погасла", func() bool {
		z := s.y.Zdorove()
		return !z.YadroZhivo && z.Hesh == "" && z.ProbaDo == 0
	})
	if b.skolko() != 0 {
		t.Fatalf("правила остались: %v", b.otkryto)
	}
	if _, err := os.Stat(filepath.Join(s.katalog, "sostoyanie.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("проба оставила состояние: %v", err)
	}
}

// Проверка без запуска на чистой машине: файлов сертификатов в каталоге
// состояния ещё нет, проверка берёт сертификаты пакета (или заглушку) из
// временного каталога и ничего не пишет в каталог состояния.
func TestProveritNaChistoyMashine(t *testing.T) {
	katalog := t.TempDir()
	y := yadro.Novoe(katalog, trafik.NovyySchyotchik(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := novyyStend(t)
	konfig := s.konfigServera([]chelovek{lyudA}, s.porty["hy2"])
	naSvoySert(t, konfig, filepath.Join(katalog, "sert-ip"))

	bezSerta, err := json.Marshal(map[string]any{"versiya": 1, "yadro": konfig})
	if err != nil {
		t.Fatal(err)
	}
	if err := y.Proverit(bezSerta); err == nil {
		t.Fatal("пакет без сертификата, на который ссылаются входы, прошёл")
	}
	c, kl := sertPEM(t, "127.0.0.1")
	sSertom, err := json.Marshal(map[string]any{"versiya": 2, "yadro": konfig, "sert_ip": map[string]string{"cert": c, "klyuch": kl}})
	if err != nil {
		t.Fatal(err)
	}
	if err := y.Proverit(sSertom); err != nil {
		t.Fatalf("пакет со своим сертификатом: %v", err)
	}
	svoy, err := json.Marshal(map[string]any{"versiya": 3, "yadro": konfig, "sert_ip_sam": map[string]any{"ip": "127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := y.Proverit(svoy); err != nil {
		t.Fatalf("пакет своего VPN без сертификата: %v", err)
	}
	if ostalos, err := os.ReadDir(katalog); err != nil || len(ostalos) != 0 {
		t.Fatalf("проверка писала в каталог состояния: %v %v", ostalos, err)
	}
}
