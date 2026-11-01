// Пакет trafik считает байты по людям и пишет их в журнал, который центр
// забирает и подтверждает.
package trafik

import (
	"context"
	"net"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
)

// Schyotchik подключается к маршрутизатору sing-box так же, как его
// собственная статистика v2ray_api, и оборачивает каждое соединение
// человека счётчиком. Один на всю жизнь программы: перезапуск ядра его не
// обнуляет.
//
// Кроме байтов он копит отметки входов: кто и с какого адреса вошёл, кто не
// вошёл и почему. По ним видно, доходили ли попытки человека до сервера
// вообще, когда у него «ничего не подключается».
type Schyotchik struct {
	mu      sync.Mutex
	lyudi   map[klyuchPary]*para
	otmetki map[KlyuchOtmetki]int64
	// bezAdresov: в своём VPN адрес подключившегося не пишется. Хозяину
	// сервера он не нужен, а в журнале лежал бы месяцами.
	bezAdresov atomic.Bool
}

type klyuchPary struct{ imya, vhod string }

type para struct {
	vverh atomic.Int64 // от человека в интернет
	vniz  atomic.Int64 // из интернета человеку
}

// KlyuchOtmetki это одна строка отметок за отрезок.
type KlyuchOtmetki struct {
	Vhod  string // тег входа
	Adres string // адрес клиента без порта
	Imya  string // человек, если вход удался
	Vid   string // что случилось, см. vhody
}

// PredelOtmetok это сколько разных строк отметок держится за отрезок. Сверх
// него отметки складываются в строку с адресом «*»: сканер с тысяч адресов
// не должен раздувать память и журнал.
const PredelOtmetok = 2000

// AdresMnogo это адрес строки, в которую уходят отметки сверх предела.
const AdresMnogo = "*"

var _ adapter.ConnectionTracker = (*Schyotchik)(nil)

func NovyySchyotchik() *Schyotchik {
	return &Schyotchik{lyudi: make(map[klyuchPary]*para), otmetki: make(map[KlyuchOtmetki]int64)}
}

// para отдаёт счётчики человека на входе. Записи не удаляются никогда:
// открытое соединение держит указатель, и удалённая запись тихо съела бы его
// байты. Людей десятки, входов пять, места это не стоит.
func (s *Schyotchik) para(imya, vhod string) *para {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := klyuchPary{imya, vhod}
	p := s.lyudi[k]
	if p == nil {
		p = new(para)
		s.lyudi[k] = p
	}
	return p
}

func (s *Schyotchik) RoutedConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, matchedRule adapter.Rule, matchOutbound adapter.Outbound) net.Conn {
	if metadata.User == "" {
		return conn
	}
	p := s.para(metadata.User, metadata.Inbound)
	return bufio.NewInt64CounterConn(conn, []*atomic.Int64{&p.vverh}, []*atomic.Int64{&p.vniz})
}

func (s *Schyotchik) RoutedPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, matchedRule adapter.Rule, matchOutbound adapter.Outbound) N.PacketConn {
	if metadata.User == "" {
		return conn
	}
	p := s.para(metadata.User, metadata.Inbound)
	return bufio.NewInt64CounterPacketConn(conn, []*atomic.Int64{&p.vverh}, nil, []*atomic.Int64{&p.vniz}, nil)
}

// RoutedFlow бывает только у входа tun, на сервере его нет.
func (s *Schyotchik) RoutedFlow(ctx context.Context, metadata adapter.InboundContext, matchedRule adapter.Rule, matchOutbound adapter.Outbound) tun.FlowTracker {
	return nil
}

// BezAdresov включает и выключает отметки без адреса клиента.
func (s *Schyotchik) BezAdresov(da bool) { s.bezAdresov.Store(da) }

// Otmetit добавляет одну отметку входа.
func (s *Schyotchik) Otmetit(vhod, adres, imya, vid string) {
	if s.bezAdresov.Load() {
		adres = ""
	}
	k := KlyuchOtmetki{Vhod: vhod, Adres: adres, Imya: imya, Vid: vid}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, est := s.otmetki[k]; !est && len(s.otmetki) >= PredelOtmetok {
		k.Adres, k.Imya = AdresMnogo, ""
	}
	s.otmetki[k]++
}

// Snimok это всё накопленное за отрезок.
type Snimok struct {
	Lyudi    map[string][2]int64            // имя -> [вверх, вниз]
	PoVhodam map[string]map[string][2]int64 // имя -> вход -> [вверх, вниз]
	Otmetki  []Otmetka
}

// Pust отвечает, нечего ли писать в журнал.
func (s Snimok) Pust() bool { return len(s.Lyudi) == 0 && len(s.Otmetki) == 0 }

// Snyat забирает накопленное и обнуляет. Люди без трафика в итог не попадают.
func (s *Schyotchik) Snyat() Snimok {
	s.mu.Lock()
	defer s.mu.Unlock()
	itog := Snimok{Lyudi: make(map[string][2]int64), PoVhodam: make(map[string]map[string][2]int64)}
	for k, p := range s.lyudi {
		vverh, vniz := p.vverh.Swap(0), p.vniz.Swap(0)
		if vverh == 0 && vniz == 0 {
			continue
		}
		v := itog.Lyudi[k.imya]
		itog.Lyudi[k.imya] = [2]int64{v[0] + vverh, v[1] + vniz}
		poVhodam := itog.PoVhodam[k.imya]
		if poVhodam == nil {
			poVhodam = make(map[string][2]int64)
			itog.PoVhodam[k.imya] = poVhodam
		}
		poVhodam[k.vhod] = [2]int64{vverh, vniz}
	}
	for k, n := range s.otmetki {
		itog.Otmetki = append(itog.Otmetki, Otmetka{Vhod: k.Vhod, Adres: k.Adres, Imya: k.Imya, Vid: k.Vid, Chislo: n})
	}
	sort.Slice(itog.Otmetki, func(i, j int) bool { return itog.Otmetki[i].menshe(itog.Otmetki[j]) })
	s.otmetki = make(map[KlyuchOtmetki]int64)
	return itog
}

// Vernut кладёт снятое обратно, если записать его не вышло: цифры не должны
// пропасть из-за сбоя диска.
func (s *Schyotchik) Vernut(snyato Snimok) {
	for imya, poVhodam := range snyato.PoVhodam {
		for vhod, d := range poVhodam {
			p := s.para(imya, vhod)
			p.vverh.Add(d[0])
			p.vniz.Add(d[1])
		}
	}
	// Записи старого вида, без разбивки по входам, возвращаются целиком.
	for imya, d := range snyato.Lyudi {
		if _, est := snyato.PoVhodam[imya]; est {
			continue
		}
		p := s.para(imya, "")
		p.vverh.Add(d[0])
		p.vniz.Add(d[1])
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range snyato.Otmetki {
		s.otmetki[KlyuchOtmetki{Vhod: o.Vhod, Adres: o.Adres, Imya: o.Imya, Vid: o.Vid}] += o.Chislo
	}
}
