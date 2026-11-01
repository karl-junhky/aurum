//go:build !linux

package lovushka

import (
	"context"
	"errors"
)

func lovit(context.Context, int, int, int, *Itog) error {
	return errors.New("ловушка есть только на Linux")
}
