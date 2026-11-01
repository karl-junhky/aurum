// Копия protocol/anytls/inbound.go из sing-box v1.14.2 (GPL-3.0-or-later).
// Отличия помечены «наше:». При обновлении sing-box сверять с исходником.

package vhody

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/uot"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	anytls "github.com/anytls/sing-anytls"
	"github.com/anytls/sing-anytls/padding"
)

var _ Smennyy = (*AnyTLS)(nil)

type AnyTLS struct {
	inbound.Adapter
	tlsConfig tls.ServerConfig
	router    adapter.ConnectionRouterEx
	logger    logger.ContextLogger
	listener  *listener.Listener
	// наше: служба хранит только состав и схему набивки, поэтому при смене
	// состава ставится новая. Сессия anytls живёт внутри вызова старой службы
	// и доживает там, новые идут в новую.
	sluzhba       atomic.Pointer[anytls.Service]
	paddingScheme []byte
	uchet         *Uchet
}

func NovyyAnyTLS(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.AnyTLSInboundOptions, uchet *Uchet) (*AnyTLS, error) {
	h := &AnyTLS{
		Adapter: inbound.NewAdapter(C.TypeAnyTLS, tag),
		router:  uot.NewRouter(router, logger),
		logger:  logger,
		uchet:   uchet,
	}

	if options.TLS != nil && options.TLS.Enabled {
		tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
		if err := podklyuchitSerty(tlsConfig, options.TLS); err != nil {
			return nil, err
		}
		h.tlsConfig = tlsConfig
	}

	h.paddingScheme = padding.DefaultPaddingScheme
	if len(options.PaddingScheme) > 0 {
		h.paddingScheme = []byte(strings.Join(options.PaddingScheme, "\n"))
	}

	// наше: начальный состав ставится тем же путём, что и смена на лету.
	if _, err := h.Smenit(&options); err != nil {
		return nil, err
	}
	h.listener = listener.New(listener.Options{
		Context:           ctx,
		Logger:            logger,
		Network:           []string{N.NetworkTCP},
		Listen:            options.ListenOptions,
		ConnectionHandler: h,
	})
	return h, nil
}

// Smenit наше: ставит новый состав и закрывает соединения выбывших.
//
// Схема набивки берётся та, что была при создании входа: её смена меняет
// каркас конфига и идёт перезапуском, а не сюда.
func (h *AnyTLS) Smenit(opcii any) (int, error) {
	o, ok := opcii.(*option.AnyTLSInboundOptions)
	if !ok {
		return 0, fmt.Errorf("вход %s: ждали опции anytls, пришли %T", h.Tag(), opcii)
	}
	lyudi := lyudiAnyTLS(o)
	klyuchi, err := sostav(h.Tag(), lyudi)
	if err != nil {
		return 0, err
	}
	polzovateli := make([]anytls.User, 0, len(lyudi))
	for _, ch := range lyudi {
		polzovateli = append(polzovateli, anytls.User{Name: ch.Klyuch, Password: ch.Sekret})
	}
	sluzhba, err := anytls.NewService(anytls.ServiceConfig{
		Users:         polzovateli,
		PaddingScheme: h.paddingScheme,
		Handler:       (*anytlsObrabotchik)(h),
		Logger:        h.logger,
	})
	if err != nil {
		return 0, err
	}
	otozvat := h.uchet.Dopustit(h.Tag(), klyuchi)
	h.sluzhba.Store(sluzhba)
	return zakrytOtozvannyh(otozvat), nil
}

func (h *AnyTLS) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return err
		}
	}
	return h.listener.Start()
}

func (h *AnyTLS) Close() error {
	return common.Close(h.listener, h.tlsConfig)
}

func (h *AnyTLS) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	if h.tlsConfig != nil {
		tlsConn, err := tls.ServerHandshake(ctx, conn, h.tlsConfig)
		if err != nil {
			N.CloseOnHandshakeFailure(conn, onClose, err)
			// наше: отказ TLS в отметки входа.
			h.uchet.Otmetit(h.Tag(), adresKlienta(ctx, metadata.Source), "", vidTLS(err))
			h.logger.ErrorContext(ctx, E.Cause(err, "process connection from ", metadata.Source, ": TLS handshake"))
			return
		}
		conn = tlsConn
	}
	// наше: запись на сырое соединение. Служба держит вызов всю жизнь
	// сессии, поэтому ссылка снимается, когда сессия кончилась.
	z := h.uchet.Novaya(conn, h.Tag(), adresKlienta(ctx, metadata.Source))
	defer h.uchet.Otpustit(z)
	err := h.sluzhba.Load().NewConnection(adapter.WithContext(sZapisyu(ctx, z), &metadata), conn, metadata.Source, onClose)
	if err != nil {
		N.CloseOnHandshakeFailure(conn, onClose, err)
		h.logger.ErrorContext(ctx, E.Cause(err, "process connection from ", metadata.Source))
	}
}

type anytlsObrabotchik AnyTLS

func (h *anytlsObrabotchik) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	//nolint:staticcheck
	metadata.InboundDetour = h.listener.ListenOptions().Detour
	//nolint:staticcheck
	metadata.Source = source
	metadata.Destination = destination.Unwrap()
	// наше: человек из ключа обязателен, поток в учёт.
	user, otpustit, ok := h.uchet.vpustit(ctx, zapisIz(ctx), h.Tag())
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	h.router.RouteConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}
