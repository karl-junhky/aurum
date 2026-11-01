package yadro

import "time"

// UkorotitSrokProby делает секунду срока пробы короче на время теста.
func UkorotitSrokProby(edinica time.Duration) (vernut func()) {
	bylo := edinicaSroka
	edinicaSroka = edinica
	return func() { edinicaSroka = bylo }
}
