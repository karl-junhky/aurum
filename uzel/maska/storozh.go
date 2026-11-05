package maska

import (
	"context"
	"sync"
	"time"
)

// Storozh раз в час проверяет маску работающего ядра теми же условиями, что
// подбор (МА3 плана 1.12.0), и три провала подряд считает тревогой. Маску сам
// не меняет: ключи на телефоне со старой маской перестали бы работать молча,
// а сменить её может только клиент, который тут же отдаст новые ключи.
type Storozh struct {
	// maska это маска reality работающего ядра; пусто - reality нет или ядро
	// не поднято.
	maska    func() string
	proverit func(ctx context.Context, imya string) Proverka
	seychas  func() time.Time
	// shag это как часто смотреть, не сменилась ли маска; period - как часто
	// проверять ту же.
	shag, period time.Duration

	mu sync.Mutex
	s  SostoyanieStorozha
	// sled это когда проверять маску s.Imya снова.
	sled time.Time
}

// ProvalovDoTrevogi: столько провалов подряд, прежде чем сказать человеку.
// Один провал бывает от сбоя сети сервера или сайта на минуту.
const ProvalovDoTrevogi = 3

// SostoyanieStorozha это маска глазами сторожа. Время в Unix-секундах.
type SostoyanieStorozha struct {
	Imya string `json:"imya"`
	// Provereno: когда маска проверялась последний раз; ноль - эту маску
	// ещё не проверяли.
	Provereno int64 `json:"provereno,omitempty"`
	Godna     bool  `json:"godna"`
	// Provalov это провалы подряд, ProvalS - время первого из них, Prichina -
	// чем кончился последний.
	Provalov int    `json:"provalov,omitempty"`
	ProvalS  int64  `json:"proval_s,omitempty"`
	Prichina string `json:"prichina,omitempty"`
	Trevoga  bool   `json:"trevoga,omitempty"`
}

// NovyyStorozh: maska отдаёт маску работающего ядра.
func NovyyStorozh(maska func() string) *Storozh {
	return &Storozh{
		maska:    maska,
		proverit: func(ctx context.Context, imya string) Proverka { return Proverit(ctx, imya, Opcii{}) },
		seychas:  time.Now,
		shag:     time.Minute,
		period:   time.Hour,
	}
}

// Rabotat смотрит на маску раз в shag до отмены ctx. Новая маска
// проверяется на ближайшем шаге, та же - раз в period.
func (s *Storozh) Rabotat(ctx context.Context) {
	t := time.NewTicker(s.shag)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.krug(ctx)
		}
	}
}

func (s *Storozh) krug(ctx context.Context) {
	imya := s.maska()
	s.mu.Lock()
	// Без reality (проба портов, ядро упало на минуту) счёт той же маски
	// ждёт её возвращения, а другая маска начинает свой.
	if imya != "" && imya != s.s.Imya {
		s.s, s.sled = SostoyanieStorozha{Imya: imya}, time.Time{}
	}
	pora := imya != "" && !s.seychas().Before(s.sled)
	s.mu.Unlock()
	if !pora {
		return
	}
	p := s.proverit(ctx, imya)
	// Остановленная программа проверку не досмотрела: это не провал маски.
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Маску сменили, пока шла проверка: итог про прежнюю.
	if s.s.Imya != imya {
		return
	}
	seychas := s.seychas()
	s.s.Provereno, s.sled = seychas.Unix(), seychas.Add(s.period)
	if p.Godna {
		s.s.Godna, s.s.Provalov, s.s.ProvalS, s.s.Prichina = true, 0, 0, ""
	} else {
		if s.s.Provalov == 0 {
			s.s.ProvalS = seychas.Unix()
		}
		s.s.Godna, s.s.Prichina = false, p.Prichina
		s.s.Provalov++
	}
	s.s.Trevoga = s.s.Provalov >= ProvalovDoTrevogi
}

// Sostoyanie отдаёт то, что сторож знает о маске работающего ядра; nil -
// reality нет. Маска, которую ещё не проверяли, приходит без Provereno.
func (s *Storozh) Sostoyanie() *SostoyanieStorozha {
	imya := s.maska()
	if imya == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if imya != s.s.Imya {
		return &SostoyanieStorozha{Imya: imya}
	}
	kopiya := s.s
	return &kopiya
}
