// Копия protocol/trojan/inbound.go из sing-box v1.14.2 (GPL-3.0-or-later).
// Отличия помечены «наше:». При обновлении sing-box сверять с исходником.

package vhody

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/common/mux"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/trojan"
	"github.com/sagernet/sing-box/transport/v2ray"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var (
	_ adapter.TCPInjectableInbound = (*Trojan)(nil)
	_ Smennyy                      = (*Trojan)(nil)
)

type Trojan struct {
	inbound.Adapter
	router   adapter.ConnectionRouterEx
	logger   log.ContextLogger
	listener *listener.Listener
	// наше: служба меняется целиком при смене состава, как у vless.
	sluzhba                  atomic.Pointer[trojan.Service[string]]
	fallbackHandler          N.TCPConnectionHandlerEx
	uchet                    *Uchet
	tlsConfig                tls.ServerConfig
	fallbackAddr             M.Socksaddr
	fallbackAddrTLSNextProto map[string]M.Socksaddr
	transport                adapter.V2RayServerTransport
}

func NovyyTrojan(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanInboundOptions, uchet *Uchet) (*Trojan, error) {
	h := &Trojan{
		Adapter: inbound.NewAdapter(C.TypeTrojan, tag),
		router:  router,
		logger:  logger,
		uchet:   uchet,
	}
	if options.TLS != nil {
		tlsConfig, err := tls.NewServerWithOptions(tls.ServerOptions{
			Context: ctx,
			Logger:  logger,
			Options: common.PtrValueOrDefault(options.TLS),
			KTLSCompatible: common.PtrValueOrDefault(options.Transport).Type == "" &&
				!common.PtrValueOrDefault(options.Multiplex).Enabled,
		})
		if err != nil {
			return nil, err
		}
		if err := podklyuchitSerty(tlsConfig, options.TLS); err != nil {
			return nil, err
		}
		h.tlsConfig = tlsConfig
	}
	if options.Fallback != nil && options.Fallback.Server != "" || len(options.FallbackForALPN) > 0 {
		if options.Fallback != nil && options.Fallback.Server != "" {
			h.fallbackAddr = options.Fallback.Build()
			if !h.fallbackAddr.IsValid() {
				return nil, E.New("invalid fallback address: ", h.fallbackAddr)
			}
		}
		if len(options.FallbackForALPN) > 0 {
			if h.tlsConfig == nil {
				return nil, E.New("fallback for ALPN is not supported without TLS")
			}
			fallbackAddrNextProto := make(map[string]M.Socksaddr)
			for nextProto, destination := range options.FallbackForALPN {
				fallbackAddr := destination.Build()
				if !fallbackAddr.IsValid() {
					return nil, E.New("invalid fallback address for ALPN ", nextProto, ": ", fallbackAddr)
				}
				fallbackAddrNextProto[nextProto] = fallbackAddr
			}
			h.fallbackAddrTLSNextProto = fallbackAddrNextProto
		}
		h.fallbackHandler = adapter.NewUpstreamContextHandler(h.fallbackConnection, nil)
	}
	// наше: начальный состав ставится тем же путём, что и смена на лету.
	if _, err := h.Smenit(&options); err != nil {
		return nil, err
	}
	var err error
	if options.Transport != nil {
		h.transport, err = v2ray.NewServerTransport(ctx, logger, common.PtrValueOrDefault(options.Transport), h.tlsConfig, (*trojanTransportHandler)(h))
		if err != nil {
			return nil, E.Cause(err, "create server transport: ", options.Transport.Type)
		}
	}
	h.router, err = mux.NewRouterWithOptions(h.router, logger, common.PtrValueOrDefault(options.Multiplex))
	if err != nil {
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
func (h *Trojan) Smenit(opcii any) (int, error) {
	o, ok := opcii.(*option.TrojanInboundOptions)
	if !ok {
		return 0, fmt.Errorf("вход %s: ждали опции trojan, пришли %T", h.Tag(), opcii)
	}
	paroli := make([]string, 0, len(o.Users))
	for _, u := range o.Users {
		paroli = append(paroli, u.Password)
	}
	klyuchi, err := sostav(h.Tag(), lyudiTrojan(o))
	if err != nil {
		return 0, err
	}
	sluzhba := trojan.NewService[string](adapter.NewUpstreamContextHandler(h.newConnection, h.newPacketConnection), h.fallbackHandler, h.logger)
	if err := sluzhba.UpdateUsers(klyuchi, paroli); err != nil {
		return 0, fmt.Errorf("вход %s: %w", h.Tag(), err)
	}
	otozvat := h.uchet.Dopustit(h.Tag(), klyuchi)
	h.sluzhba.Store(sluzhba)
	return zakrytOtozvannyh(otozvat), nil
}

func (h *Trojan) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return E.Cause(err, "create TLS config")
		}
	}
	if h.transport == nil {
		return h.listener.Start()
	}
	if common.Contains(h.transport.Network(), N.NetworkTCP) {
		tcpListener, err := h.listener.ListenTCP()
		if err != nil {
			return err
		}
		go func() {
			// наше: сокеты под транспортом с ссылкой для отзыва.
			sErr := h.transport.Serve(slushatelSoSsylkoy{Listener: tcpListener, uchet: h.uchet, vhod: h.Tag()})
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	if common.Contains(h.transport.Network(), N.NetworkUDP) {
		udpConn, err := h.listener.ListenUDP()
		if err != nil {
			return err
		}
		go func() {
			sErr := h.transport.ServePacket(udpConn)
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	return nil
}

func (h *Trojan) Close() error {
	return common.Close(
		h.listener,
		h.tlsConfig,
		h.transport,
	)
}

func (h *Trojan) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	if h.tlsConfig != nil && h.transport == nil {
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
	// наше: запись на сырое соединение, её закроет отзыв. У транспорта под
	// потоком ещё и сокет.
	var zakryt io.Closer = conn
	if h.transport != nil {
		zakryt = zapisDlyaTransporta(ctx, conn)
	}
	z := h.uchet.Novaya(zakryt, h.Tag(), adresKlienta(ctx, metadata.Source))
	defer h.uchet.Otpustit(z)
	err := h.sluzhba.Load().NewConnection(adapter.WithContext(sZapisyu(ctx, z), &metadata), conn, metadata.Source, onClose)
	if err != nil {
		N.CloseOnHandshakeFailure(conn, onClose, err)
		h.logger.ErrorContext(ctx, E.Cause(err, "process connection from ", metadata.Source))
	}
}

func (h *Trojan) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	// наше: человек из ключа, поток в учёт.
	user, otpustit, ok := h.uchet.vpustit(ctx, zapisIz(ctx), h.Tag())
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	h.router.RouteConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}

func (h *Trojan) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	// наше: человек из ключа, поток в учёт.
	user, otpustit, ok := h.uchet.vpustit(ctx, zapisIz(ctx), h.Tag())
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	h.router.RoutePacketConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}

func (h *Trojan) fallbackConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	var fallbackAddr M.Socksaddr
	if len(h.fallbackAddrTLSNextProto) > 0 {
		if tlsConn, loaded := common.Cast[tls.Conn](conn); loaded {
			connectionState := tlsConn.ConnectionState()
			if connectionState.NegotiatedProtocol != "" {
				if fallbackAddr, loaded = h.fallbackAddrTLSNextProto[connectionState.NegotiatedProtocol]; !loaded {
					h.logger.DebugContext(ctx, "process connection from ", metadata.Source, ": fallback disabled for ALPN: ", connectionState.NegotiatedProtocol)
					N.CloseOnHandshakeFailure(conn, onClose, os.ErrInvalid)
					return
				}
			}
		}
	}
	if !fallbackAddr.IsValid() {
		if !h.fallbackAddr.IsValid() {
			h.logger.DebugContext(ctx, "process connection from ", metadata.Source, ": fallback disabled by default")
			N.CloseOnHandshakeFailure(conn, onClose, os.ErrInvalid)
			return
		}
		fallbackAddr = h.fallbackAddr
	}
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	metadata.Destination = fallbackAddr
	h.logger.InfoContext(ctx, "fallback connection to ", fallbackAddr)
	h.router.RouteConnectionEx(ctx, conn, metadata, onClose)
}

var _ adapter.V2RayServerTransportHandler = (*trojanTransportHandler)(nil)

type trojanTransportHandler Trojan

func (h *trojanTransportHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Source = source
	metadata.Destination = destination
	//nolint:staticcheck
	metadata.InboundDetour = h.listener.ListenOptions().Detour
	//nolint:staticcheck
	h.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	(*Trojan)(h).NewConnection(ctx, conn, metadata, onClose)
}
