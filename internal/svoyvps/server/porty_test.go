package server

import (
	"path/filepath"
	"slices"
	"testing"
)

// podstavnoyUFW это ufw в виде функции оболочки: правила лежат в файле $F
// строками `ufw show added` без «ufw », вызовы правок пишутся в $LOG. $UFW:
// active, inactive, active-warn (предупреждение перед состоянием) или sboy
// (status упал, как при занятом iptables).
const podstavnoyUFW = `ufw() {
    case "$1" in
    status)
        case "$UFW" in
        active) echo "Status: active" ;;
        active-warn) echo "WARN: /etc is world writable!" >&2; echo "Status: active" ;;
        sboy) echo "ERROR: problem running iptables: Another app is currently holding the xtables lock" >&2; return 1 ;;
        *) echo "Status: inactive" ;;
        esac ;;
    show)
        echo "Added user rules (see 'ufw status' for running firewall):"
        if [ -s "$F" ]; then sed 's/^/ufw /' "$F"; else echo "(None)"; fi ;;
    allow)
        echo "allow $2 comment '$4'" >> "$F"
        echo "allow $2 $4" >> "$LOG" ;;
    delete)
        grep -v "^$2 $3\( \|$\)" "$F" > "$F.t" || true
        mv "$F.t" "$F"
        echo "delete $2 $3" >> "$LOG" ;;
    *) echo "ufw: $*" >&2; return 1 ;;
    esac
}
`

// prognatPorty гоняет настоящий porty.sh с подставным ufw.
func prognatPorty(t *testing.T, ufw string, pravila []string, tcp, udp string) (vyvod, pravilaPosle, vyzovy []string, kod int) {
	t.Helper()
	k := t.TempDir()
	f, log := filepath.Join(k, "pravila"), filepath.Join(k, "log")
	zapisat(t, f, pravila...)
	out, kod := bashSkript(t, podstavnoyUFW+skriptPortov,
		[]string{"F=" + filepath.ToSlash(f), "LOG=" + filepath.ToSlash(log), "UFW=" + ufw}, tcp, udp)
	return razbit(out), chitat(t, f), chitat(t, log), kod
}

func TestPortySkript(t *testing.T) {
	nash := func(p string) string { return "allow " + p + " comment 'affory-vhod'" }
	sluchai := []struct {
		imya             string
		ufw              string
		pravila          []string
		tcp, udp         string
		vyvod, posle, vy []string
		kod              int
	}{
		{"пустой включённый", "active", nil, "443", "443",
			[]string{"ufw active", "otkryt 443/tcp", "otkryt 443/udp"},
			[]string{nash("443/tcp"), nash("443/udp")}, []string{"allow 443/tcp affory-vhod", "allow 443/udp affory-vhod"}, 0},
		// Выключенный: не открывается ничего, лишние наши снимаются.
		{"выключенный", "inactive", []string{nash("443/tcp"), nash("8443/tcp")}, "443", "",
			[]string{"ufw inactive", "zakryt 8443/tcp"}, []string{nash("443/tcp")}, []string{"delete allow 8443/tcp"}, 0},
		{"снос из выключенного", "inactive", []string{"allow 22/tcp", nash("443/tcp")}, "", "",
			[]string{"ufw inactive", "zakryt 443/tcp"}, []string{"allow 22/tcp"}, []string{"delete allow 443/tcp"}, 0},
		// Упавший status это отказ до любой правки, а не «выключен».
		{"сбой ufw status", "sboy", []string{nash("8443/tcp")}, "443", "",
			nil, []string{nash("8443/tcp")}, nil, 1},
		{"предупреждение перед состоянием", "active-warn", nil, "443", "",
			[]string{"ufw active", "otkryt 443/tcp"}, []string{nash("443/tcp")}, []string{"allow 443/tcp affory-vhod"}, 0},
		// Правило человека без протокола открывает порт; наш прежний 443 снимается.
		{"открыт человеком", "active", []string{"allow 2053", nash("443/tcp")}, "2053", "",
			[]string{"ufw active", "uzhe_otkryt 2053/tcp", "zakryt 443/tcp"}, []string{"allow 2053"}, []string{"delete allow 443/tcp"}, 0},
		{"открыт списком", "active", []string{"allow 80,443/tcp"}, "443", "",
			[]string{"ufw active", "uzhe_otkryt 443/tcp"}, []string{"allow 80,443/tcp"}, nil, 0},
		{"пометка человека с пробелом", "active", []string{"allow 443/tcp comment 'my rule'"}, "443", "",
			[]string{"ufw active", "uzhe_otkryt 443/tcp"}, []string{"allow 443/tcp comment 'my rule'"}, nil, 0},
		{"allow in", "active", []string{"allow in 443/tcp"}, "443", "",
			[]string{"ufw active", "uzhe_otkryt 443/tcp"}, []string{"allow in 443/tcp"}, nil, 0},
		// Правила с адресом порт для всех не открывают, limit на чужом порту и
		// запрет диапазона другого протокола не мешают.
		{"с адресом, limit и чужой протокол", "active",
			[]string{"allow from 203.0.113.5 to any port 443 proto tcp", "limit 22/tcp", "deny 2000:3000/udp"}, "443,2053", "",
			[]string{"ufw active", "otkryt 443/tcp", "otkryt 2053/tcp"},
			[]string{"allow from 203.0.113.5 to any port 443 proto tcp", "limit 22/tcp", "deny 2000:3000/udp", nash("443/tcp"), nash("2053/tcp")},
			[]string{"allow 443/tcp affory-vhod", "allow 2053/tcp affory-vhod"}, 0},
		{"своё уже стоит", "active", []string{nash("443/tcp")}, "443", "",
			[]string{"ufw active", "otkryt 443/tcp"}, []string{nash("443/tcp")}, nil, 0},
		// Запрет человека: отказ до любой правки, прежнее наше правило на месте.
		{"запрет человека", "active", []string{"deny 9443/tcp", nash("2053/tcp")}, "443,9443", "",
			[]string{"ufw active"}, []string{"deny 9443/tcp", nash("2053/tcp")}, nil, 1},
		{"запрет диапазоном", "active", []string{"deny 2000:3000/tcp"}, "443,2053", "",
			[]string{"ufw active"}, []string{"deny 2000:3000/tcp"}, nil, 1},
		{"limit на нужном порту", "active", []string{"limit 443"}, "", "443",
			[]string{"ufw active"}, []string{"limit 443"}, nil, 1},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			vyvod, posle, vy, kod := prognatPorty(t, s.ufw, s.pravila, s.tcp, s.udp)
			if kod != s.kod || !slices.Equal(vyvod, s.vyvod) || !slices.Equal(posle, s.posle) || !slices.Equal(vy, s.vy) {
				t.Fatalf("код %d, вывод %q\nправила %q\nвызовы %q", kod, vyvod, posle, vy)
			}
		})
	}
}
