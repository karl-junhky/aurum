package vhody

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

type zakryvashka struct{ raz atomic.Int32 }

func (z *zakryvashka) Close() error { z.raz.Add(1); return nil }

func TestUchetOtzyvIOtpusk(t *testing.T) {
	u := NovyyUchet(nil)
	kA, kB := Klyuch("anna", "p1"), Klyuch("boris", "p2")
	u.Dopustit("v", []string{kA, kB})

	cA := &zakryvashka{}
	zA := u.Novaya(cA, "v", "198.51.100.1")
	otpA, ok := u.Privyazat(zA, "v", kA)
	if !ok {
		t.Fatal("допущенную не впустили")
	}
	// Второй поток того же сырого соединения.
	otpA2, ok := u.Privyazat(zA, "v", kA)
	if !ok {
		t.Fatal("второй поток не впустили")
	}
	if _, ok := u.Privyazat(zA, "v", kB); ok {
		t.Fatal("у одного сырого соединения два хозяина")
	}
	if _, ok := u.Privyazat(u.Novaya(&zakryvashka{}, "v", "198.51.100.1"), "v", Klyuch("anna", "staryy")); ok {
		t.Fatal("старый пароль впустили")
	}

	cB := &zakryvashka{}
	zB := u.Novaya(cB, "v", "198.51.100.1")
	otpB, _ := u.Privyazat(zB, "v", kB)
	if z := u.Zhivyh()["v"]; z != 2 {
		t.Fatalf("живых %d", z)
	}

	// Отзыв Анны: закрывается её соединение, Бориса нет.
	zakryt := u.Dopustit("v", []string{kB})
	if len(zakryt) != 1 || Zakryt(zakryt) != 1 || cA.raz.Load() != 1 || cB.raz.Load() != 0 {
		t.Fatalf("закрыто %d, у Анны %d, у Бориса %d", len(zakryt), cA.raz.Load(), cB.raz.Load())
	}
	if _, ok := u.Privyazat(zA, "v", kA); ok {
		t.Fatal("отозванную запись впустили снова")
	}
	// Ссылки отозванной записи снимаются без вреда.
	otpA()
	otpA2()
	otpA2()
	u.Otpustit(zA)

	// Ссылки Бориса: поток и само сырое соединение.
	otpB()
	if z := u.Zhivyh()["v"]; z != 1 {
		t.Fatalf("сырое соединение Бориса ушло из учёта раньше времени: %d", z)
	}
	u.Otpustit(zB)
	if len(u.Zhivyh()) != 0 {
		t.Fatalf("учёт не опустел: %v", u.Zhivyh())
	}
}

// Отзыв и привязка из многих потоков: после отзыва ни одна запись выбывшего
// не остаётся в учёте открытой.
func TestUchetGonkaOtzyvaIPrivyazki(t *testing.T) {
	for krug := 0; krug < 50; krug++ {
		u := NovyyUchet(nil)
		k := Klyuch("anna", "p")
		u.Dopustit("v", []string{k})
		var wg sync.WaitGroup
		zapisi := make([]*Zapis, 64)
		closers := make([]*zakryvashka, 64)
		for i := range zapisi {
			closers[i] = &zakryvashka{}
			zapisi[i] = u.Novaya(closers[i], "v", "198.51.100.1")
		}
		var vpushcheno atomic.Int32
		for i := range zapisi {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, ok := u.Privyazat(zapisi[i], "v", k); ok {
					vpushcheno.Add(1)
				}
			}()
		}
		var zakryto []int
		wg.Add(1)
		go func() {
			defer wg.Done()
			zakryto = append(zakryto, Zakryt(u.Dopustit("v", nil)))
		}()
		wg.Wait()
		if len(u.Zhivyh()) != 0 {
			t.Fatalf("круг %d: после отзыва в учёте остались записи: %v", krug, u.Zhivyh())
		}
		if int(vpushcheno.Load()) != zakryto[0] {
			t.Fatalf("круг %d: впущено %d, закрыто отзывом %d", krug, vpushcheno.Load(), zakryto[0])
		}
	}
}

type sborOtmetok struct {
	mu   sync.Mutex
	vidy []string
}

func (s *sborOtmetok) Otmetit(vhod, adres, imya, vid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vidy = append(s.vidy, vhod+" "+adres+" "+imya+" "+vid)
}

// Каждое сырое соединение даёт ровно одну отметку о том, чем кончился вход:
// вошло, не вошло или отозвано. Потоки внутри вошедшего отметок не множат.
func TestUchetOtmetki(t *testing.T) {
	sbor := &sborOtmetok{}
	u := NovyyUchet(sbor)
	kA := Klyuch("anna", "p1")
	u.Dopustit("v", []string{kA})

	zA := u.Novaya(&zakryvashka{}, "v", "198.51.100.1")
	otp1, _ := u.Privyazat(zA, "v", kA)
	otp2, _ := u.Privyazat(zA, "v", kA)
	otp1()
	otp2()
	u.Otpustit(zA)

	zChuzhoy := u.Novaya(&zakryvashka{}, "v", "198.51.100.2")
	u.Otpustit(zChuzhoy)

	zStaryy := u.Novaya(&zakryvashka{}, "v", "198.51.100.3")
	if _, ok := u.Privyazat(zStaryy, "v", Klyuch("anna", "staryy")); ok {
		t.Fatal("старый пароль впустили")
	}
	if _, ok := u.Privyazat(zStaryy, "v", Klyuch("anna", "staryy")); ok {
		t.Fatal("старый пароль впустили со второго потока")
	}
	u.Otpustit(zStaryy)

	zB := u.Novaya(&zakryvashka{}, "v", "198.51.100.4")
	if _, ok := u.Privyazat(zB, "v", kA); !ok {
		t.Fatal("допущенную не впустили")
	}
	u.Dopustit("v", nil)
	u.Otpustit(zB)

	zhdali := []string{
		"v 198.51.100.1 anna vhod",
		"v 198.51.100.2  bez-vhoda",
		"v 198.51.100.3 anna otozvan",
		"v 198.51.100.4 anna vhod",
		"v 198.51.100.4 anna otozvan",
	}
	if strings.Join(sbor.vidy, "|") != strings.Join(zhdali, "|") {
		t.Fatalf("отметки:\n%s\nждали:\n%s", strings.Join(sbor.vidy, "\n"), strings.Join(zhdali, "\n"))
	}
}

func TestVidTLS(t *testing.T) {
	sluchai := []struct {
		err error
		vid string
	}{
		{context.DeadlineExceeded, VidTLSSrok},
		{fmt.Errorf("рукопожатие: %w", os.ErrDeadlineExceeded), VidTLSSrok},
		{io.EOF, VidTLSObryv},
		{&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, VidTLSObryv},
		{errors.New("tls: client requested unsupported application protocols"), VidTLS},
	}
	for _, s := range sluchai {
		if v := vidTLS(s.err); v != s.vid {
			t.Errorf("%v: %s, ждали %s", s.err, v, s.vid)
		}
	}
}
