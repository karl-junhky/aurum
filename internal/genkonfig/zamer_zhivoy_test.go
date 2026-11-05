package genkonfig

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Пинг по каждому серверу на живом ядре (01.10.2026), целиком на петле.
//
// check подтверждает имена полей, но не маршрут: что логин входа замеров
// действительно уводит запрос в исходящий своего сервера, знает только
// запущенное ядро. Сервер здесь тоже sing-box, shadowsocks на петле, а цель
// замера локальная: тест не ходит в интернет и не трогает сеть машины.
func TestZamerNaZhivomYadre(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX: маршрут входа замеров проверяется только живым ядром")
	}
	const klyuch = "MTIzNDU2Nzg5MDEyMzQ1Ng=="
	portServera, portZamera, portKlash, portMyortvyy := svobodnyyPort(t), svobodnyyPort(t), svobodnyyPort(t), svobodnyyPort(t)

	cel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(cel.Close)

	zapustit(t, yadro, "server.json", fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"shadowsocks","listen":"127.0.0.1","listen_port":%d,"method":"2022-blake3-aes-128-gcm","password":%q}],`+
		`"outbounds":[{"type":"direct"}]}`, portServera, klyuch))

	zhivoy := protokol.Server{Id: "ss:1", Transport: "ss", Host: "127.0.0.1", Port: portServera, Metod: "2022-blake3-aes-128-gcm", Parol: klyuch}
	myortvyy := protokol.Server{Id: "ss-2", Transport: "ss", Host: "127.0.0.1", Port: portMyortvyy, Metod: "2022-blake3-aes-128-gcm", Parol: klyuch}
	v := Vhod{
		BezTun:         true,
		Server:         zhivoy,
		Servery:        []protokol.Server{zhivoy, myortvyy},
		Kandidaty:      []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Resolver:       netip.MustParseAddr("127.0.0.1"),
		PutiProtsessov: []string{`C:\a\sing-box.exe`, `C:\a\affory-svc.exe`},
		ClashApi:       ClashApi{Adres: "127.0.0.1", Port: portKlash, Sekret: "s"},
		Zamer:          &VhodZamera{Port: portZamera, Parol: "p"},
	}
	b, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	zapustit(t, yadro, "klient.json", string(b))
	zhdatPort(t, portZamera)

	proksi := func(login string) *url.URL {
		return &url.URL{Scheme: "http", User: url.UserPassword(login, "p"), Host: fmt.Sprintf("127.0.0.1:%d", portZamera)}
	}
	ctx := context.Background()
	if d, err := set.OtklikCherez(ctx, cel.URL+"/generate_204", proksi(PolzovatelZamera(zhivoy.Id))); err != nil {
		t.Fatalf("живой сервер не измерен: %v", err)
	} else if d <= 0 {
		t.Fatalf("пинг живого сервера %v", d)
	}
	if _, err := set.OtklikCherez(ctx, cel.URL+"/generate_204", proksi(PolzovatelZamera(myortvyy.Id))); err == nil {
		t.Fatal("мёртвый сервер измерен: логин ушёл не в свой исходящий")
	}
	if _, err := set.OtklikCherez(ctx, cel.URL+"/generate_204", proksi("zchuzhoy")); err == nil {
		t.Fatal("чужой логин прошёл через вход замеров")
	}
}

// Таблица адресов службы на живом ядре (02.10.2026). Имя сервера .invalid не
// разрешается нигде, а местный резолвер смотрит на петлю, где DNS нет: дойти
// до сервера ядро может только по адресу из таблицы. Контроль: сервер с тем
// же портом без строки в таблице обязан не измериться, иначе тест проверял
// бы DNS машины, а не таблицу.
func TestTablicaAdresovNaZhivomYadre(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX: путь ядра к серверу проверяется только живым ядром")
	}
	const klyuch = "MTIzNDU2Nzg5MDEyMzQ1Ng=="
	portServera, portZamera, portKlash := svobodnyyPort(t), svobodnyyPort(t), svobodnyyPort(t)

	cel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(cel.Close)

	zapustit(t, yadro, "server.json", fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"shadowsocks","listen":"127.0.0.1","listen_port":%d,"method":"2022-blake3-aes-128-gcm","password":%q}],`+
		`"outbounds":[{"type":"direct"}]}`, portServera, klyuch))

	vTablice := protokol.Server{Id: "ss-tablica", Transport: "ss", Host: "zablokirovan.invalid", Port: portServera, Metod: "2022-blake3-aes-128-gcm", Parol: klyuch}
	bezTablicy := protokol.Server{Id: "ss-bez", Transport: "ss", Host: "bez-tablicy.invalid", Port: portServera, Metod: "2022-blake3-aes-128-gcm", Parol: klyuch}
	v := Vhod{
		BezTun:         true,
		Server:         vTablice,
		Servery:        []protokol.Server{vTablice, bezTablicy},
		Kandidaty:      []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		AdresaServerov: map[string][]netip.Addr{"zablokirovan.invalid": {netip.MustParseAddr("127.0.0.1")}},
		Resolver:       netip.MustParseAddr("127.0.0.1"),
		PutiProtsessov: []string{`C:\a\sing-box.exe`, `C:\a\affory-svc.exe`},
		ClashApi:       ClashApi{Adres: "127.0.0.1", Port: portKlash, Sekret: "s"},
		Zamer:          &VhodZamera{Port: portZamera, Parol: "p"},
	}
	b, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	zapustit(t, yadro, "klient.json", string(b))
	zhdatPort(t, portZamera)

	proksi := func(login string) *url.URL {
		return &url.URL{Scheme: "http", User: url.UserPassword(login, "p"), Host: fmt.Sprintf("127.0.0.1:%d", portZamera)}
	}
	ctx := context.Background()
	if _, err := set.OtklikCherez(ctx, cel.URL+"/generate_204", proksi(PolzovatelZamera(vTablice.Id))); err != nil {
		t.Fatalf("сервер из таблицы не измерен: ядро не взяло адрес из таблицы: %v", err)
	}
	if _, err := set.OtklikCherez(ctx, cel.URL+"/generate_204", proksi(PolzovatelZamera(bezTablicy.Id))); err == nil {
		t.Fatal("сервер без строки в таблице измерен: имя .invalid разрешилось мимо таблицы, контроль негоден")
	}
}

// svobodnyyPort берёт порт, свободный и для TCP, и для UDP: shadowsocks слушает
// оба. Свободный по одному протоколу номер у Windows бывает в
// зарезервированном диапазоне другого (их двигает Hyper-V), и ядро-сервер
// падало на bind (02.10.2026). Брать номер у системы нельзя: и TCP, и UDP она
// раздаёт подряд (замерено 06.11.2026: 61220, 61221, 61222…), и двадцать
// попыток целиком ложились в одну дыру резерва в сотню портов. Номер поэтому
// случайный и ниже раздаваемых системой.
func svobodnyyPort(t *testing.T) int {
	t.Helper()
	var posled error
	for range 200 {
		port := 20000 + rand.IntN(20000)
		// Проба порт отпускает: без памяти о выданных один тест мог бы
		// получить один номер дважды, и второе ядро не заняло бы его.
		if _, uzhe := vydannyePorty.LoadOrStore(port, struct{}{}); uzhe {
			continue
		}
		u, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			posled = err
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		u.Close()
		if err == nil {
			l.Close()
			return port
		}
		posled = err
	}
	t.Fatalf("за 200 попыток не нашлось порта, свободного и для TCP, и для UDP: %v", posled)
	return 0
}

// vydannyePorty это номера, уже отданные тестам пакета, в том числе отвергнутые пробой.
var vydannyePorty sync.Map

func zapustit(t *testing.T, yadro, imya, konfig string) {
	t.Helper()
	put := filepath.Join(t.TempDir(), imya)
	if err := os.WriteFile(put, []byte(konfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(yadro, "run", "-c", put)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("ядро %s не остановлено: %v", imya, err)
		}
		_ = cmd.Wait()
	})
}

func zhdatPort(t *testing.T, port int) {
	t.Helper()
	do := time.Now().Add(10 * time.Second)
	for time.Now().Before(do) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("вход замеров на порту %d не поднялся за 10 секунд", port)
}
