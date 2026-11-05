package maska

import (
	"context"
	"testing"
	"time"
)

// storozhDlyaTesta: маска и ответы проверки задаёт тест, часы идут руками.
func storozhDlyaTesta(t *testing.T) (*Storozh, *string, *[]Proverka, *time.Time, *int) {
	t.Helper()
	maska := "sosed.example.net"
	var otvety []Proverka
	seychas := time.Unix(1_800_000_000, 0)
	sprosheno := 0
	s := NovyyStorozh(func() string { return maska })
	s.seychas = func() time.Time { return seychas }
	s.proverit = func(_ context.Context, imya string) Proverka {
		sprosheno++
		if len(otvety) == 0 {
			t.Fatalf("проверка %q без ответа", imya)
		}
		p := otvety[0]
		otvety = otvety[1:]
		p.Imya = imya
		return p
	}
	return s, &maska, &otvety, &seychas, &sprosheno
}

func TestStorozhTriProvala(t *testing.T) {
	s, _, otvety, seychas, sprosheno := storozhDlyaTesta(t)
	ctx := context.Background()
	if st := s.Sostoyanie(); st == nil || st.Imya != "sosed.example.net" || st.Provereno != 0 {
		t.Fatalf("до проверки %+v", st)
	}
	*otvety = []Proverka{{Godna: true}, {Prichina: "сайт не отвечает"}, {Prichina: "сайт не отвечает"}, {Prichina: "сертификат истёк"}, {Godna: true}}

	s.krug(ctx)
	if st := s.Sostoyanie(); !st.Godna || st.Provereno != seychas.Unix() || st.Trevoga {
		t.Fatalf("годна: %+v", st)
	}
	// Та же маска раньше часа не проверяется.
	*seychas = seychas.Add(59 * time.Minute)
	s.krug(ctx)
	if *sprosheno != 1 {
		t.Fatalf("проверок %d", *sprosheno)
	}
	nachalo := seychas.Add(time.Hour)
	for i := 1; i <= 3; i++ {
		*seychas = seychas.Add(time.Hour)
		s.krug(ctx)
		st := s.Sostoyanie()
		if st.Godna || st.Provalov != i || st.ProvalS != nachalo.Unix() || st.Trevoga != (i == 3) {
			t.Fatalf("провал %d: %+v", i, st)
		}
	}
	if st := s.Sostoyanie(); st.Prichina != "сертификат истёк" {
		t.Fatalf("причина %q", st.Prichina)
	}
	// Ответила снова - тревога и счёт уходят.
	*seychas = seychas.Add(time.Hour)
	s.krug(ctx)
	if st := s.Sostoyanie(); !st.Godna || st.Provalov != 0 || st.ProvalS != 0 || st.Prichina != "" || st.Trevoga {
		t.Fatalf("после провалов годна: %+v", st)
	}
}

// Новая маска проверяется на ближайшем шаге, и провалы прежней ей не
// достаются.
func TestStorozhNovayaMaska(t *testing.T) {
	s, maska, otvety, seychas, sprosheno := storozhDlyaTesta(t)
	ctx := context.Background()
	*otvety = []Proverka{{Prichina: "нет"}, {Prichina: "нет"}, {Prichina: "нет"}, {Godna: true}}
	for range 3 {
		s.krug(ctx)
		*seychas = seychas.Add(time.Hour)
	}
	if !s.Sostoyanie().Trevoga {
		t.Fatal("три провала без тревоги")
	}
	*maska = "drugoy.example.org"
	if st := s.Sostoyanie(); st.Imya != "drugoy.example.org" || st.Provereno != 0 || st.Trevoga || st.Provalov != 0 {
		t.Fatalf("новая маска до проверки: %+v", st)
	}
	*seychas = seychas.Add(time.Minute)
	s.krug(ctx)
	if st := s.Sostoyanie(); *sprosheno != 4 || !st.Godna || st.Imya != "drugoy.example.org" {
		t.Fatalf("проверок %d, %+v", *sprosheno, st)
	}
	*maska = ""
	if st := s.Sostoyanie(); st != nil {
		t.Fatalf("без reality %+v", st)
	}
	s.krug(ctx)
	if *sprosheno != 4 {
		t.Fatal("без reality проверка пошла")
	}
}

// reality пропал на время (проба портов, ядро упало): провалы той же маски
// не теряются.
func TestStorozhRealityNaVremyaPropal(t *testing.T) {
	s, maska, otvety, seychas, _ := storozhDlyaTesta(t)
	ctx := context.Background()
	*otvety = []Proverka{{Prichina: "нет"}, {Prichina: "нет"}, {Prichina: "нет"}}
	for range 2 {
		s.krug(ctx)
		*seychas = seychas.Add(time.Hour)
	}
	*maska = ""
	s.krug(ctx)
	*maska = "sosed.example.net"
	if st := s.Sostoyanie(); st.Provalov != 2 {
		t.Fatalf("после пропажи reality %+v", st)
	}
	s.krug(ctx)
	if st := s.Sostoyanie(); !st.Trevoga || st.Provalov != 3 {
		t.Fatalf("третий провал без тревоги: %+v", st)
	}
}

// Маска сменилась посреди проверки или программу остановили: итог не
// пишется.
func TestStorozhPrervannayaProverka(t *testing.T) {
	s, maska, _, _, _ := storozhDlyaTesta(t)
	s.proverit = func(context.Context, string) Proverka {
		*maska = "drugoy.example.org"
		return Proverka{Prichina: "нет"}
	}
	s.krug(context.Background())
	if st := s.Sostoyanie(); st.Imya != "drugoy.example.org" || st.Provereno != 0 || st.Provalov != 0 {
		t.Fatalf("итог прежней маски записан: %+v", st)
	}

	s, _, _, _, _ = storozhDlyaTesta(t)
	ctx, otmena := context.WithCancel(context.Background())
	s.proverit = func(context.Context, string) Proverka {
		otmena()
		return Proverka{Prichina: "проверка прервана"}
	}
	s.krug(ctx)
	if st := s.Sostoyanie(); st.Provereno != 0 || st.Provalov != 0 {
		t.Fatalf("остановка записана провалом: %+v", st)
	}
}

func TestStorozhRabotat(t *testing.T) {
	s, _, otvety, _, sprosheno := storozhDlyaTesta(t)
	*otvety = []Proverka{{Godna: true}}
	s.shag = time.Millisecond
	ctx, otmena := context.WithCancel(context.Background())
	gotovo := make(chan struct{})
	go func() {
		s.Rabotat(ctx)
		close(gotovo)
	}()
	srok := time.After(5 * time.Second)
	for s.Sostoyanie().Provereno == 0 {
		select {
		case <-srok:
			t.Fatal("сторож не проверил маску за 5 с")
		case <-time.After(time.Millisecond):
		}
	}
	otmena()
	<-gotovo
	if *sprosheno != 1 {
		t.Fatalf("проверок %d", *sprosheno)
	}
}
