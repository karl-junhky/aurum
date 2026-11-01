package ssylki_test

import (
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"testing"
)

func profili(t *testing.T, lines string) []protokol.Server {
	t.Helper()
	r, err := ssylki.RazobratSpisok([]byte(lines))
	if err != nil {
		t.Fatal(err)
	}
	return r.Servery
}

func TestRaznyeProfiliOdnogoEndpointNeTeryayutsya(t *testing.T) {
	s := profili(t, "trojan://first@node.example:443?sni=a.example#One\ntrojan://second@node.example:443?sni=b.example#Two\ntrojan://first@node.example:443?sni=a.example#Duplicate")
	if len(s) != 2 || s[0].Id == s[1].Id {
		t.Fatal("distinct profiles lost or duplicate IDs")
	}
}

func TestProfiliSohranyayutIdPriPerestanovkeIRotatsii(t *testing.T) {
	old := profili(t, "trojan://first@node.example:443?sni=a.example#One\ntrojan://second@node.example:443?sni=b.example#Two")
	for _, lines := range []string{
		"trojan://second@node.example:443?sni=b.example#Renamed\ntrojan://first@node.example:443?sni=a.example#Renamed-again",
		"trojan://new-second@node.example:443?sni=b.example#New-two\ntrojan://new-first@node.example:443?sni=a.example#New-one",
	} {
		fresh := ssylki.Slit(old, profili(t, lines))
		if len(fresh) != 2 || fresh[0].Id != old[1].Id || fresh[1].Id != old[0].Id {
			t.Fatal("profile identity moved after refresh")
		}
	}
}

func TestRotatsiyaOdnogoProfilyaSohranyaetStaryyAdresnyyId(t *testing.T) {
	old := profili(t, "trojan://old@node.example:443?sni=old.example#Old")
	fresh := ssylki.Slit(old, profili(t, "trojan://new@node.example:443?sni=new.example#New"))
	if len(fresh) != 1 || fresh[0].Id != old[0].Id || fresh[0].Parol != "new" {
		t.Fatal("rotation lost ID or retained old key")
	}
}

func TestNeodnoznachnayaRotatsiyaNeUgadaetVybor(t *testing.T) {
	old := profili(t, "trojan://first@node.example:443#Same\ntrojan://second@node.example:443#Same")
	fresh := ssylki.Slit(old, profili(t, "trojan://third@node.example:443#Same\ntrojan://fourth@node.example:443#Same"))
	for _, s := range fresh {
		for _, o := range old {
			if s.Id == o.Id {
				t.Fatal("ambiguous rotation guessed an old profile")
			}
		}
	}
}

// Пометка своего сервера это происхождение, а не профиль: та же ссылка,
// вставленная руками раньше, остаётся одной записью.
func TestSvoyTotZheProfil(t *testing.T) {
	a, err := ssylki.Razobrat("trojan://first@node.example:443#One")
	if err != nil {
		t.Fatal(err)
	}
	b := a
	b.Svoy = true
	if !ssylki.TotZheProfil(a, b) {
		t.Fatal("пометка своего сервера разделила одну ссылку на два профиля")
	}
	all, _ := ssylki.DobavitProfil(nil, a)
	if all, _ = ssylki.DobavitProfil(all, b); len(all) != 1 {
		t.Fatalf("записей %d, ссылка одна", len(all))
	}
}

func TestRuchnyeProfiliNeZamenyayutDrugieKlyuchi(t *testing.T) {
	a, err := ssylki.Razobrat("trojan://first@node.example:443#One")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ssylki.Razobrat("trojan://second@node.example:443#Two")
	if err != nil {
		t.Fatal(err)
	}
	all, a := ssylki.DobavitProfil(nil, a)
	all, b = ssylki.DobavitProfil(all, b)
	if len(all) != 2 || a.Id == b.Id {
		t.Fatal("manual alternative replaced the first key")
	}
	all, again := ssylki.DobavitProfil(all, b)
	if len(all) != 2 || again.Id != b.Id {
		t.Fatal("exact manual duplicate changed ID")
	}
	fresh := ssylki.Slit(all, profili(t, "trojan://third@node.example:443#Subscription"))
	if len(fresh) != 3 || fresh[0].Id == a.Id || fresh[0].Id == b.Id {
		t.Fatal("subscription swallowed a manual profile")
	}
}
