package brandmauer

import (
	"slices"
	"testing"
)

func TestSPometkoy(t *testing.T) {
	vyvod := "-P INPUT DROP\r\n" +
		"-A INPUT -p tcp -m tcp --dport 80 -m comment --comment affory-acme -j ACCEPT\n" +
		"-A INPUT -p udp -m udp --dport 8443 -m comment --comment affory-proba -j ACCEPT\n" +
		"-A INPUT -p tcp -m tcp --dport 443 -m comment --comment affory-acme-drugoy -j ACCEPT\n" +
		"-A INPUT -m comment --comment affory-acme\n" +
		"-N ufw-before-input\n" +
		"-A INPUT -j ufw-before-input\n"
	nashli := sPometkoy(vyvod, "affory-acme")
	zhdali := [][]string{
		{"INPUT", "-p", "tcp", "-m", "tcp", "--dport", "80", "-m", "comment", "--comment", "affory-acme", "-j", "ACCEPT"},
		{"INPUT", "-m", "comment", "--comment", "affory-acme"},
	}
	if !slices.EqualFunc(nashli, zhdali, slices.Equal) {
		t.Fatalf("нашли %q", nashli)
	}
	if p := sPometkoy(vyvod, "affory-proba"); len(p) != 1 || p[0][6] != "8443" {
		t.Fatalf("проба: %q", p)
	}
	if p := sPometkoy("", "affory-acme"); len(p) != 0 {
		t.Fatalf("пустой вывод: %q", p)
	}
}

func TestOtkrytChuzhoyProto(t *testing.T) {
	if err := Otkryt(false, "icmp", 1, "affory-proba"); err == nil {
		t.Fatal("icmp прошёл")
	}
}
