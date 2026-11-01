// Копия protocol/vless/inbound.go из sing-box v1.14.2 (GPL-3.0-or-later).
// Отличия помечены «наше:». При обновлении sing-box сверять с исходником.

package vhody

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/common/mux"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/uot"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/v2ray"
	"github.com/sagernet/sing-vmess/packetaddr"
	"github.com/sagernet/sing-vmess/vless"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var (
	_ adapter.TCPInjectableInbound = (*VLESS)(nil)
	_ Smennyy                      = (*VLESS)(nil)
)

type VLESS struct {
	inbound.Adapter
	ctx      context.Context
	router   adapter.ConnectionRouterEx
	logger   logger.ContextLogger
	listener *listener.Listener
	// наше: служба хранит только список людей, поэтому при смене состава
	// ставится новая целиком. Старые соединения доживают в старой, новые идут
	// в новую, и гонки с её внутренними картами нет.
	sluzhba   atomic.Pointer[vless.Service[string]]
	uchet     *Uchet
	tlsConfig tls.ServerConfig
	transport adapter.V2RayServerTransport
}

func NovyyVLESS(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions, uchet *Uchet) (*VLESS, error) {
	h := &VLESS{
		Adapter: inbound.NewAdapter(C.TypeVLESS, tag),
		ctx:     ctx,
		router:  uot.NewRouter(router, logger),
		logger:  logger,
		uchet:   uchet,
	}
	var err error
	h.router, err = mux.NewRouterWithOptions(h.router, logger, common.PtrValueOrDefault(options.Multiplex))
	if err != nil {
		return nil, err
	}
	// наше: начальный состав ставится тем же путём, что и смена на лету.
	if _, err = h.Smenit(&options); err != nil {
		return nil, err
	}
	if options.TLS != nil {
		h.tlsConfig, err = tls.NewServerWithOptions(tls.ServerOptions{
			Context: ctx,
			Logger:  logger,
			Options: common.PtrValueOrDefault(options.TLS),
			KTLSCompatible: common.PtrValueOrDefault(options.Transport).Type == "" &&
				!common.PtrValueOrDefault(options.Multiplex).Enabled &&
				common.All(options.Users, func(it option.VLESSUser) bool {
					return it.Flow == ""
				}),
		})
		if err != nil {
			return nil, err
		}
	}
	if options.Transport != nil {
		h.transport, err = v2ray.NewServerTransport(ctx, logger, common.PtrValueOrDefault(options.Transport), h.tlsConfig, (*vlessTransportHandler)(h))
		if err != nil {
			return nil, E.Cause(err, "create server transport: ", options.Transport.Type)
		}
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
func (h *VLESS) Smenit(opcii any) (int, error) {
	o, ok := opcii.(*option.VLESSInboundOptions)
	if !ok {
		return 0, fmt.Errorf("вход %s: ждали опции vless, пришли %T", h.Tag(), opcii)
	}
	uuids := make([]string, 0, len(o.Users))
	flows := make([]string, 0, len(o.Users))
	for _, u := range o.Users {
		uuids = append(uuids, u.UUID)
		flows = append(flows, u.Flow)
	}
	klyuchi, err := sostav(h.Tag(), lyudiVLESS(o))
	if err != nil {
		return 0, err
	}
	sluzhba := vless.NewService[string](h.logger, adapter.NewUpstreamContextHandler(h.newConnectionEx, h.newPacketConnectionEx))
	sluzhba.UpdateUsers(klyuchi, uuids, flows)
	// Допуск раньше службы: соединение выбывшего, успевшее пройти старую
	// службу, учёт уже не впустит.
	otozvat := h.uchet.Dopustit(h.Tag(), klyuchi)
	h.sluzhba.Store(sluzhba)
	return zakrytOtozvannyh(otozvat), nil
}

func (h *VLESS) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return err
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

func (h *VLESS) Close() error {
	return common.Close(
		h.sluzhba.Load(),
		h.listener,
		h.tlsConfig,
		h.transport,
	)
}

func (h *VLESS) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
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

func (h *VLESS) newConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
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

func (h *VLESS) newPacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	// наше: человек из ключа, поток в учёт.
	user, otpustit, ok := h.uchet.vpustit(ctx, zapisIz(ctx), h.Tag())
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	if metadata.Destination.Fqdn == packetaddr.SeqPacketMagicAddress {
		metadata.Destination = M.Socksaddr{}
		conn = packetaddr.NewConn(bufio.NewNetPacketConn(conn), metadata.Destination)
		h.logger.InfoContext(ctx, "[", user, "] inbound packet addr connection")
	} else {
		h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	}
	h.router.RoutePacketConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}

var _ adapter.V2RayServerTransportHandler = (*vlessTransportHandler)(nil)

type vlessTransportHandler VLESS

func (h *vlessTransportHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Source = source
	metadata.Destination = destination
	//nolint:staticcheck
	metadata.InboundDetour = h.listener.ListenOptions().Detour
	//nolint:staticcheck
	h.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	(*VLESS)(h).NewConnection(ctx, conn, metadata, onClose)
}
