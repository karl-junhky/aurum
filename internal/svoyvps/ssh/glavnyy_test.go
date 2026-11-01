package ssh

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

// Пределы сжаты: повтор после обрыва ждал бы 30 секунд, пульс 15.
// Горутина, пережившая набор, это пульс или сессия, которую слой не закрыл.
func TestMain(m *testing.M) {
	pauzyPovtora = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	srokPopytki = 3 * time.Second
	intervalPulsa = 300 * time.Millisecond
	srokProverok = 3 * time.Second
	goleak.VerifyTestMain(m)
}
