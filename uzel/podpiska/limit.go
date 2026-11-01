package podpiska

import (
	"sync"
	"time"
)

// limiter это счётчик запросов в скользящем окне, по одному на адрес.
//
// Своя реализация вместо зависимости: правило здесь одно, состояние живёт в
// памяти одного процесса, и весь код умещается в полсотни строк. Тянуть ради
// этого пакет в службу, которая должна годами стоять без обновлений, незачем.
type limiter struct {
	mu      sync.Mutex
	potolok int
	okno    time.Duration
	metki   map[string][]time.Time
}

// potolokAdresov это верхняя граница числа адресов в памяти лимитера.
//
// Несколько десятков людей с их телефонами и машинами в это не упрутся никогда:
// потолок рассчитан на то, чтобы служба пережила сканирование, а не на живую
// нагрузку.
const potolokAdresov = 4096

func novyyLimiter(potolok int, okno time.Duration) *limiter {
	return &limiter{potolok: potolok, okno: okno, metki: map[string][]time.Time{}}
}

// Mozhno отвечает, пускать ли запрос, и запоминает его.
func (l *limiter) Mozhno(klyuch string, seychas time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	granica := seychas.Add(-l.okno)
	svezhie := l.metki[klyuch][:0]
	for _, t := range l.metki[klyuch] {
		if t.After(granica) {
			svezhie = append(svezhie, t)
		}
	}

	// Карта чужих адресов не имеет права расти вечно: адрес, замолчавший
	// навсегда, обязан из неё уйти сам, иначе служба течёт памятью ровно
	// столько, сколько её сканируют.
	//
	// Одной чистки по возрасту МАЛО. Служба слушает и по IPv6, где сменить
	// адрес внутри своей /64 ничего не стоит: миллион свежих адресов подряд -
	// и ни одна запись не стара настолько, чтобы уйти. Память съедалась бы до
	// `MemoryMax`, а задолго до того каждый запрос начинал бы обходить всю
	// карту под общим замком, то есть подписка клала бы сама себя.
	//
	// Поэтому у карты есть жёсткий потолок: когда чистка по возрасту не
	// помогла, она сбрасывается целиком. Для тех, кто в этот миг качал лимит,
	// это подарок в одно окно; для службы - гарантия, что памяти она съест
	// ограниченно.
	if len(l.metki) > potolokAdresov {
		for k, v := range l.metki {
			if len(v) == 0 || v[len(v)-1].Before(granica) {
				delete(l.metki, k)
			}
		}
		if len(l.metki) > potolokAdresov {
			l.metki = make(map[string][]time.Time, potolokAdresov)
			svezhie = nil
		}
	}

	if len(svezhie) >= l.potolok {
		l.metki[klyuch] = svezhie
		return false
	}
	l.metki[klyuch] = append(svezhie, seychas)
	return true
}
