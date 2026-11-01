package vhody

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/auth"
	N "github.com/sagernet/sing/common/network"
)

// Smennyy это вход, которому можно сменить состав без перезапуска.
type Smennyy interface {
	adapter.Inbound
	// Smenit ставит состав из опций того же типа, что у входа (указатель,
	// как их отдаёт разбор конфига), и закрывает соединения выбывших.
	// Отдаёт, сколько соединений закрыто.
	Smenit(opcii any) (int, error)
}

// NashiTipy это типы входов, у которых состав меняется на лету.
var NashiTipy = map[string]bool{
	C.TypeVLESS:     true,
	C.TypeTrojan:    true,
	C.TypeAnyTLS:    true,
	C.TypeHysteria2: true,
}

// Zaregistrirovat ставит наши входы вместо входов sing-box в реестр. Реестр
// при повторной регистрации типа перезаписывает его, так что звать после
// include.InboundRegistry().
func Zaregistrirovat(reestr *inbound.Registry, uchet *Uchet) {
	inbound.Register[option.VLESSInboundOptions](reestr, C.TypeVLESS,
		func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, opcii option.VLESSInboundOptions) (adapter.Inbound, error) {
			return NovyyVLESS(ctx, router, logger, tag, opcii, uchet)
		})
	inbound.Register[option.TrojanInboundOptions](reestr, C.TypeTrojan,
		func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, opcii option.TrojanInboundOptions) (adapter.Inbound, error) {
			return NovyyTrojan(ctx, router, logger, tag, opcii, uchet)
		})
	inbound.Register[option.AnyTLSInboundOptions](reestr, C.TypeAnyTLS,
		func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, opcii option.AnyTLSInboundOptions) (adapter.Inbound, error) {
			return NovyyAnyTLS(ctx, router, logger, tag, opcii, uchet)
		})
	inbound.Register[option.Hysteria2InboundOptions](reestr, C.TypeHysteria2,
		func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, opcii option.Hysteria2InboundOptions) (adapter.Inbound, error) {
			return NovyyHysteria2(ctx, router, logger, tag, opcii, uchet)
		})
}

// Chelovek это человек глазами учёта: имя для журнала и счёта, ключ для
// службы протокола, секрет для её проверки.
type Chelovek struct {
	Imya   string
	Klyuch string
	Sekret string
	Flow   string // только у vless
}

// sostav проверяет список людей входа и строит ключи.
//
// Пустое имя, повтор имени или повтор секрета это отказ, а не молчаливое
// «последний победил»: при повторе секрета служба приписала бы трафик и отзыв
// не тому человеку.
func sostav(teg string, lyudi []Chelovek) ([]string, error) {
	imena := make(map[string]struct{}, len(lyudi))
	sekrety := make(map[string]string, len(lyudi))
	klyuchi := make([]string, 0, len(lyudi))
	for i, ch := range lyudi {
		if ch.Imya == "" {
			return nil, fmt.Errorf("вход %s: у человека №%d нет имени", teg, i+1)
		}
		if strings.IndexByte(ch.Imya, 0) >= 0 {
			return nil, fmt.Errorf("вход %s: в имени %q нулевой байт", teg, ch.Imya)
		}
		if ch.Sekret == "" {
			return nil, fmt.Errorf("вход %s: у %s пустой секрет", teg, ch.Imya)
		}
		if _, est := imena[ch.Imya]; est {
			return nil, fmt.Errorf("вход %s: имя %s повторяется", teg, ch.Imya)
		}
		if drugoy, est := sekrety[ch.Sekret]; est {
			return nil, fmt.Errorf("вход %s: у %s и %s один секрет", teg, drugoy, ch.Imya)
		}
		imena[ch.Imya] = struct{}{}
		sekrety[ch.Sekret] = ch.Imya
		klyuchi = append(klyuchi, ch.Klyuch)
	}
	return klyuchi, nil
}

func lyudiVLESS(o *option.VLESSInboundOptions) []Chelovek {
	lyudi := make([]Chelovek, 0, len(o.Users))
	for _, u := range o.Users {
		// Регистр uuid служба не различает, значит и повтор надо ловить без него.
		sekret := strings.ToLower(u.UUID)
		lyudi = append(lyudi, Chelovek{
			Imya:   u.Name,
			Klyuch: Klyuch(u.Name, sekret+"\x00"+u.Flow),
			Sekret: sekret,
			Flow:   u.Flow,
		})
	}
	return lyudi
}

func lyudiPoParolyu[T option.TrojanUser | option.AnyTLSUser | option.Hysteria2User](spisok []T, imya func(T) string, parol func(T) string) []Chelovek {
	lyudi := make([]Chelovek, 0, len(spisok))
	for _, u := range spisok {
		lyudi = append(lyudi, Chelovek{Imya: imya(u), Klyuch: Klyuch(imya(u), parol(u)), Sekret: parol(u)})
	}
	return lyudi
}

func lyudiTrojan(o *option.TrojanInboundOptions) []Chelovek {
	return lyudiPoParolyu(o.Users, func(u option.TrojanUser) string { return u.Name }, func(u option.TrojanUser) string { return u.Password })
}

func lyudiAnyTLS(o *option.AnyTLSInboundOptions) []Chelovek {
	return lyudiPoParolyu(o.Users, func(u option.AnyTLSUser) string { return u.Name }, func(u option.AnyTLSUser) string { return u.Password })
}

func lyudiHysteria2(o *option.Hysteria2InboundOptions) []Chelovek {
	return lyudiPoParolyu(o.Users, func(u option.Hysteria2User) string { return u.Name }, func(u option.Hysteria2User) string { return u.Password })
}

// Proverit проверяет состав входа, ничего не меняя. Нужна, чтобы смена на
// лету не останавливалась на полпути: сначала проверяются все входы, потом
// меняются.
func Proverit(teg string, opcii any) error {
	var lyudi []Chelovek
	switch o := opcii.(type) {
	case *option.VLESSInboundOptions:
		lyudi = lyudiVLESS(o)
	case *option.TrojanInboundOptions:
		lyudi = lyudiTrojan(o)
	case *option.AnyTLSInboundOptions:
		lyudi = lyudiAnyTLS(o)
	case *option.Hysteria2InboundOptions:
		lyudi = lyudiHysteria2(o)
	default:
		return fmt.Errorf("вход %s: у опций %T состав на лету не меняется", teg, opcii)
	}
	_, err := sostav(teg, lyudi)
	return err
}

type klyuchZapisi struct{}

// sZapisyu кладёт запись сырого соединения в контекст, с которым служба
// протокола потом зовёт обработчик. Так обработчик потока узнаёт, какое сырое
// соединение закрывать при отзыве: у anytls и мультиплекса потоков много, а
// сырое соединение одно.
func sZapisyu(ctx context.Context, z *Zapis) context.Context {
	return context.WithValue(ctx, klyuchZapisi{}, z)
}

func zapisIz(ctx context.Context) *Zapis {
	z, _ := ctx.Value(klyuchZapisi{}).(*Zapis)
	return z
}

// vpustit сверяет поток с учётом: чей он, допущен ли ещё, и берёт ссылку на
// его сырое соединение z. Ключ человека служба протокола положила в контекст
// после проверки секрета.
//
// При отказе закрывается всё сырое соединение, а не один поток: иначе сессия
// мультиплекса или anytls, прошедшая проверку старым ключом, жила бы дальше и
// отказывала бы клиенту поток за потоком, не давая ему переподключиться.
func (u *Uchet) vpustit(ctx context.Context, z *Zapis, teg string) (imya string, otpustit func(), ok bool) {
	klyuch, _ := auth.UserFromContext[string](ctx)
	otpustit, ok = u.Privyazat(z, teg, klyuch)
	if !ok && z != nil {
		zakrytSoSbrosom(z.zakryt)
	}
	return ImyaIz(klyuch), otpustit, ok
}

// sOtpuskom снимает ссылку на сырое соединение, когда поток закончился.
// Маршрутизатор sing-box зовёт onClose ровно один раз на любом исходе, в том
// числе при отказе правила, поэтому учёт не течёт.
func sOtpuskom(otpustit func(), onClose N.CloseHandlerFunc) N.CloseHandlerFunc {
	return func(err error) {
		otpustit()
		if onClose != nil {
			onClose(err)
		}
	}
}

// zakrytOtozvannyh закрывает соединения выбывших и считает их.
func zakrytOtozvannyh(spisok []io.Closer) int {
	if len(spisok) == 0 {
		return 0
	}
	return Zakryt(spisok)
}
