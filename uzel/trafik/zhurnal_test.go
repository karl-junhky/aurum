package trafik

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func otkryt(t *testing.T, katalog string) *Zhurnal {
	t.Helper()
	z, preduprezhdeniya, err := Otkryt(katalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range preduprezhdeniya {
		t.Log(p)
	}
	return z
}

func nomera(zapisi []Zapis) []uint64 {
	itog := make([]uint64, 0, len(zapisi))
	for _, zp := range zapisi {
		itog = append(itog, zp.N)
	}
	return itog
}

func TestZhurnalNumeraciyaIPodtverzhdenie(t *testing.T) {
	katalog := t.TempDir()
	z := otkryt(t, katalog)
	for i := 1; i <= 5; i++ {
		if err := z.Dopisat(int64(1000+i), Snimok{Lyudi: map[string][2]int64{"anna": {int64(i), int64(10 * i)}}}); err != nil {
			t.Fatal(err)
		}
	}
	zapisi, err := z.Posle(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := nomera(zapisi); len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Fatalf("номера %v", got)
	}
	if zapisi[2].L["anna"] != [2]int64{3, 30} {
		t.Fatalf("третья запись %+v", zapisi[2])
	}

	// Центр забрал по третью включительно.
	if err := z.Podtverdit(3); err != nil {
		t.Fatal(err)
	}
	if got, _ := z.Posle(0, 100); len(got) != 2 || got[0].N != 4 {
		t.Fatalf("после подтверждения %v", nomera(got))
	}
	// Повтор и откат подтверждения ничего не ломают.
	if err := z.Podtverdit(2); err != nil {
		t.Fatal(err)
	}
	if err := z.Podtverdit(3); err != nil {
		t.Fatal(err)
	}
	// Номер больше записанного режется до последнего.
	if err := z.Podtverdit(99); err != nil {
		t.Fatal(err)
	}
	if s := z.Sostoyanie(); s.Podtverzhdeno != 5 || s.Zhdut != 0 || s.Posledniy != 5 {
		t.Fatalf("состояние %+v", s)
	}

	// После перезапуска номера продолжаются, а не начинаются заново: центр
	// отбрасывает виденные номера, и повтор номера съел бы новые цифры.
	z2 := otkryt(t, katalog)
	if err := z2.Dopisat(2000, Snimok{Lyudi: map[string][2]int64{"boris": {1, 1}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := z2.Posle(0, 100); len(got) != 1 || got[0].N != 6 {
		t.Fatalf("после перезапуска %v", nomera(got))
	}
}

func TestZhurnalNePodtverzhdennoePerezhivaetPerezapusk(t *testing.T) {
	katalog := t.TempDir()
	z := otkryt(t, katalog)
	for i := 0; i < 3; i++ {
		if err := z.Dopisat(int64(i), Snimok{Lyudi: map[string][2]int64{"anna": {1, 2}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Podtverdit(1); err != nil {
		t.Fatal(err)
	}
	z2 := otkryt(t, katalog)
	got, err := z2.Posle(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n := nomera(got); len(n) != 2 || n[0] != 2 || n[1] != 3 {
		t.Fatalf("после перезапуска ждут %v", n)
	}
	if s := z2.Sostoyanie(); s.Zhdut != 2 || s.Podtverzhdeno != 1 {
		t.Fatalf("состояние %+v", s)
	}
}

func TestZhurnalObryvHvosta(t *testing.T) {
	katalog := t.TempDir()
	z := otkryt(t, katalog)
	if err := z.Dopisat(1, Snimok{Lyudi: map[string][2]int64{"anna": {1, 2}}}); err != nil {
		t.Fatal(err)
	}
	// Сбой посреди записи: строка без конца.
	f, err := os.OpenFile(filepath.Join(katalog, "trafik.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"n":2,"t":2,"l":{"bor`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	z2, preduprezhdeniya, err := Otkryt(katalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(preduprezhdeniya) == 0 {
		t.Fatal("оборванный хвост прошёл молча")
	}
	if err := z2.Dopisat(3, Snimok{Lyudi: map[string][2]int64{"boris": {5, 6}}}); err != nil {
		t.Fatal(err)
	}
	got, err := z2.Posle(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n := nomera(got); len(n) != 2 || n[0] != 1 || n[1] != 2 || got[1].L["boris"] != [2]int64{5, 6} {
		t.Fatalf("после обрыва %+v", got)
	}
}

func TestZhurnalPredel(t *testing.T) {
	z := otkryt(t, t.TempDir())
	if got, _ := z.Posle(0, 2); len(got) != 0 {
		t.Fatal("пустой журнал что-то отдал")
	}
	for i := 0; i < 5; i++ {
		if err := z.Dopisat(int64(i), Snimok{Lyudi: map[string][2]int64{"anna": {1, 1}}}); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := z.Posle(1, 2); len(got) != 2 || got[0].N != 2 || got[1].N != 3 {
		t.Fatalf("предел выдачи %v", nomera(got))
	}
}

func TestSchyotchikSnyatIVernut(t *testing.T) {
	s := NovyySchyotchik()
	p := s.para("anna", "hy2")
	p.vverh.Add(5)
	p.vniz.Add(7)
	p = s.para("anna", "trojan")
	p.vverh.Add(1)
	p.vniz.Add(2)
	s.para("boris", "hy2") // без трафика в итог не попадает
	s.Otmetit("hy2", "198.51.100.7", "anna", "vhod")
	s.Otmetit("hy2", "198.51.100.7", "anna", "vhod")
	s.Otmetit("trojan", "198.51.100.9", "", "tls-srok")

	snyato := s.Snyat()
	if len(snyato.Lyudi) != 1 || snyato.Lyudi["anna"] != [2]int64{6, 9} {
		t.Fatalf("снято по людям %v", snyato.Lyudi)
	}
	if v := snyato.PoVhodam["anna"]; len(v) != 2 || v["hy2"] != [2]int64{5, 7} || v["trojan"] != [2]int64{1, 2} {
		t.Fatalf("снято по входам %v", snyato.PoVhodam)
	}
	zhdali := []Otmetka{
		{Vhod: "hy2", Adres: "198.51.100.7", Imya: "anna", Vid: "vhod", Chislo: 2},
		{Vhod: "trojan", Adres: "198.51.100.9", Vid: "tls-srok", Chislo: 1},
	}
	if !reflect.DeepEqual(snyato.Otmetki, zhdali) {
		t.Fatalf("отметки %+v", snyato.Otmetki)
	}
	if again := s.Snyat(); !again.Pust() {
		t.Fatalf("второе снятие %+v", again)
	}

	s.Vernut(snyato)
	back := s.Snyat()
	if back.Lyudi["anna"] != [2]int64{6, 9} || back.PoVhodam["anna"]["hy2"] != [2]int64{5, 7} || !reflect.DeepEqual(back.Otmetki, zhdali) {
		t.Fatalf("возврат %+v", back)
	}
}

// Сканер с тысяч адресов не раздувает память: сверх предела отметки уходят
// в одну строку с адресом «*», и ни одна не теряется.
func TestSchyotchikPredelOtmetok(t *testing.T) {
	s := NovyySchyotchik()
	for i := 0; i < PredelOtmetok+10; i++ {
		s.Otmetit("trojan", fmt.Sprintf("198.51.%d.%d", i/250, i%250), "", "tls")
	}
	snyato := s.Snyat()
	if len(snyato.Otmetki) != PredelOtmetok+1 {
		t.Fatalf("строк %d", len(snyato.Otmetki))
	}
	var vsego, mnogo int64
	for _, o := range snyato.Otmetki {
		vsego += o.Chislo
		if o.Adres == AdresMnogo {
			mnogo = o.Chislo
		}
	}
	if vsego != PredelOtmetok+10 || mnogo != 10 {
		t.Fatalf("всего %d, сверх предела %d", vsego, mnogo)
	}
}

// В своём VPN отметки идут без адреса и складываются в одну строку на вход,
// человека и вид.
func TestSchyotchikBezAdresov(t *testing.T) {
	s := NovyySchyotchik()
	s.BezAdresov(true)
	s.Otmetit("hy2", "198.51.100.7", "anna", "vhod")
	s.Otmetit("hy2", "198.51.100.9", "anna", "vhod")
	snyato := s.Snyat()
	if len(snyato.Otmetki) != 1 || snyato.Otmetki[0].Adres != "" || snyato.Otmetki[0].Chislo != 2 {
		t.Fatalf("без адресов: %+v", snyato.Otmetki)
	}
	s.BezAdresov(false)
	s.Otmetit("hy2", "198.51.100.7", "anna", "vhod")
	if o := s.Snyat().Otmetki; len(o) != 1 || o[0].Adres != "198.51.100.7" {
		t.Fatalf("с адресами: %+v", o)
	}
}

// Запись с одними отметками ложится в журнал и читается обратно целиком.
func TestZhurnalOtmetki(t *testing.T) {
	z, _, err := Otkryt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snimok := Snimok{Otmetki: []Otmetka{{Vhod: "hy2", Adres: "198.51.100.7", Vid: "bez-vhoda", Chislo: 3}}}
	if err := z.Dopisat(100, snimok); err != nil {
		t.Fatal(err)
	}
	snimok = Snimok{
		Lyudi:    map[string][2]int64{"anna": {1, 2}},
		PoVhodam: map[string]map[string][2]int64{"anna": {"hy2": {1, 2}}},
	}
	if err := z.Dopisat(160, snimok); err != nil {
		t.Fatal(err)
	}
	zapisi, err := z.Posle(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(zapisi) != 2 || zapisi[0].L == nil || len(zapisi[0].L) != 0 || len(zapisi[0].O) != 1 || zapisi[0].O[0].Chislo != 3 {
		t.Fatalf("первая запись %+v", zapisi[0])
	}
	if zapisi[1].LV["anna"]["hy2"] != [2]int64{1, 2} || zapisi[1].O != nil {
		t.Fatalf("вторая запись %+v", zapisi[1])
	}
}
