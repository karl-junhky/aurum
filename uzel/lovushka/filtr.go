package lovushka

import (
	"errors"
	"fmt"

	"golang.org/x/net/bpf"
)

// snaplen: из пойманного пакета нужны заголовки до флагов TCP.
const snaplen = 96

// sobratFiltr это классический BPF для пакета с сетевого заголовка (сокет
// SOCK_DGRAM): IPv4 или IPv6 без расширений, нужный протокол, порт
// назначения, у TCP только SYN без ACK, у IPv4 не фрагмент. Отдаёт snaplen
// для подходящего пакета и 0 для остальных.
func sobratFiltr(proto, port int) ([]bpf.RawInstruction, error) {
	flagi := func(zagruzit bpf.Instruction) []shag {
		if proto != protoTCP {
			return []shag{{i: bpf.Jump{}, da: "vzyat"}}
		}
		return []shag{
			{i: zagruzit},
			{i: bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0x12}},
			{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x02}, da: "vzyat", net: "mimo"},
		}
	}
	shagi := []shag{
		{i: bpf.LoadAbsolute{Off: 0, Size: 1}},
		{i: bpf.ALUOpConstant{Op: bpf.ALUOpShiftRight, Val: 4}},
		{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: 4}, net: "v6"},
		{i: bpf.LoadAbsolute{Off: 9, Size: 1}},
		{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(proto)}, net: "mimo"},
		{i: bpf.LoadAbsolute{Off: 6, Size: 2}},
		{i: bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff}, da: "mimo"},
		{i: bpf.LoadMemShift{Off: 0}},
		{i: bpf.LoadIndirect{Off: 2, Size: 2}},
		{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(port)}, net: "mimo"},
	}
	shagi = append(shagi, flagi(bpf.LoadIndirect{Off: 13, Size: 1})...)
	shagi = append(shagi,
		shag{metka: "v6", i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: 6}, net: "mimo"},
		shag{i: bpf.LoadAbsolute{Off: 6, Size: 1}},
		shag{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(proto)}, net: "mimo"},
		shag{i: bpf.LoadAbsolute{Off: 42, Size: 2}},
		shag{i: bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(port)}, net: "mimo"},
	)
	shagi = append(shagi, flagi(bpf.LoadAbsolute{Off: 53, Size: 1})...)
	shagi = append(shagi,
		shag{metka: "vzyat", i: bpf.RetConstant{Val: snaplen}},
		shag{metka: "mimo", i: bpf.RetConstant{Val: 0}},
	)
	programma, err := razmetit(shagi)
	if err != nil {
		return nil, err
	}
	syroe, err := bpf.Assemble(programma)
	if err != nil {
		return nil, fmt.Errorf("ловушка: фильтр не собрался: %w", err)
	}
	return syroe, nil
}

// shag это инструкция фильтра с метками вместо счёта прыжков: da и net -
// куда при истине и при лжи, пусто - на следующую.
type shag struct {
	metka   string
	i       bpf.Instruction
	da, net string
}

func razmetit(shagi []shag) ([]bpf.Instruction, error) {
	gde := map[string]int{}
	for n, s := range shagi {
		if s.metka != "" {
			gde[s.metka] = n
		}
	}
	pryzhok := func(n int, metka string) (uint8, error) {
		if metka == "" {
			return 0, nil
		}
		k, est := gde[metka]
		if !est || k <= n || k-n-1 > 255 {
			return 0, fmt.Errorf("ловушка: прыжок %d -> %q некуда", n, metka)
		}
		return uint8(k - n - 1), nil
	}
	itog := make([]bpf.Instruction, len(shagi))
	for n, s := range shagi {
		switch i := s.i.(type) {
		case bpf.JumpIf:
			var errDa, errNet error
			i.SkipTrue, errDa = pryzhok(n, s.da)
			i.SkipFalse, errNet = pryzhok(n, s.net)
			if err := errors.Join(errDa, errNet); err != nil {
				return nil, err
			}
			itog[n] = i
		case bpf.Jump:
			k, err := pryzhok(n, s.da)
			if err != nil {
				return nil, err
			}
			i.Skip = uint32(k)
			itog[n] = i
		default:
			itog[n] = s.i
		}
	}
	return itog, nil
}
