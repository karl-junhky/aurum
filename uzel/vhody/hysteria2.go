// Копия protocol/hysteria2/inbound.go из sing-box v1.14.2 (GPL-3.0-or-later).
// Отличия помечены «наше:». При обновлении sing-box сверять с исходником.

package vhody

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	qtls "github.com/sagernet/sing-quic"
	"github.com/sagernet/sing-quic/hysteria"
	"github.com/sagernet/sing-quic/hysteria2"
	"github.com/sagernet/sing-quic/hysteria2/realm"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"
)

// razmerBufera это приёмный и передающий буфер сокета hy2, как
// rmem_default на 153 после разбора отбросов 26.09.
const razmerBufera = 16 << 20

var _ Smennyy = (*Hysteria2)(nil)

type Hysteria2 struct {
	inbound.Adapter
	router    adapter.Router
	logger    log.ContextLogger
	listener  *listener.Listener
	tlsConfig tls.ServerConfig
	// наше: служба одна на всю жизнь входа, она держит QUIC-слушатель.
	// Состав меняется внутри неё (UpdateUsers), соединения отозванных
	// закрывает слежка.
	service *hysteria2.Service[string]
	uchet   *Uchet
	slezhka *slezhkaQUIC
}

func NovyyHysteria2(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.Hysteria2InboundOptions, uchet *Uchet) (*Hysteria2, error) {
	options.UDPFragmentDefault = true
	if options.TLS == nil || !options.TLS.Enabled {
		return nil, C.ErrTLSRequired
	}
	tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
	if err != nil {
		return nil, err
	}
	if err := podklyuchitSerty(tlsConfig, options.TLS); err != nil {
		return nil, err
	}
	var salamanderPassword string
	var geckoPassword string
	var geckoMinPacketSize, geckoMaxPacketSize int
	if options.Obfs != nil {
		if options.Obfs.Password == "" {
			return nil, E.New("missing obfs password")
		}
		switch options.Obfs.Type {
		case hysteria2.ObfsTypeSalamander:
			salamanderPassword = options.Obfs.Password
		case hysteria2.ObfsTypeGecko:
			geckoPassword = options.Obfs.Password
			geckoMinPacketSize = options.Obfs.GeckoOptions.MinPacketSize
			geckoMaxPacketSize = options.Obfs.GeckoOptions.MaxPacketSize
		default:
			return nil, E.New("unknown obfs type: ", options.Obfs.Type)
		}
	}
	var masqueradeHandler http.Handler
	if options.Masquerade != nil && options.Masquerade.Type != "" {
		switch options.Masquerade.Type {
		case C.Hysterai2MasqueradeTypeFile:
			masqueradeDirectory := filemanager.BasePath(ctx, os.ExpandEnv(options.Masquerade.FileOptions.Directory))
			_, err = filemanager.ReadDir(ctx, masqueradeDirectory)
			if err != nil && !os.IsNotExist(err) {
				return nil, E.Cause(err, "read masquerade directory")
			}
			masqueradeHandler = http.FileServer(http.Dir(masqueradeDirectory))
		case C.Hysterai2MasqueradeTypeProxy:
			masqueradeURL, err := url.Parse(options.Masquerade.ProxyOptions.URL)
			if err != nil {
				return nil, E.Cause(err, "parse masquerade URL")
			}
			masqueradeHandler = &httputil.ReverseProxy{
				Rewrite: func(r *httputil.ProxyRequest) {
					r.SetURL(masqueradeURL)
					if !options.Masquerade.ProxyOptions.RewriteHost {
						r.Out.Host = r.In.Host
					}
				},
				ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
					w.WriteHeader(http.StatusBadGateway)
				},
			}
		case C.Hysterai2MasqueradeTypeString:
			masqueradeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if options.Masquerade.StringOptions.StatusCode != 0 {
					w.WriteHeader(options.Masquerade.StringOptions.StatusCode)
				}
				for key, values := range options.Masquerade.StringOptions.Headers {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.Write([]byte(options.Masquerade.StringOptions.Content))
			})
		default:
			return nil, E.New("unknown masquerade type: ", options.Masquerade.Type)
		}
	}
	h := &Hysteria2{
		Adapter: inbound.NewAdapter(C.TypeHysteria2, tag),
		router:  router,
		logger:  logger,
		listener: listener.New(listener.Options{
			Context: ctx,
			Logger:  logger,
			Listen:  options.ListenOptions,
		}),
		tlsConfig: tlsConfig,
		uchet:     uchet,
		slezhka:   novayaSlezhkaQUIC(uchet, tag),
	}
	var udpTimeout time.Duration
	if options.UDPTimeout != 0 {
		udpTimeout = time.Duration(options.UDPTimeout)
	} else {
		udpTimeout = C.UDPTimeout
	}
	var realmOptions *realm.Options
	if options.Realm != nil {
		if options.Realm.IPVersion != 0 && options.ListenOptions.Listen != nil {
			listenAddr := netip.Addr(*options.ListenOptions.Listen).Unmap()
			if options.Realm.IPVersion == 6 && listenAddr.Is4() {
				return nil, E.New("realm.ip_version 6 conflicts with listen address ", listenAddr)
			}
			if options.Realm.IPVersion == 4 && listenAddr.Is6() && !listenAddr.IsUnspecified() {
				return nil, E.New("realm.ip_version 4 conflicts with listen address ", listenAddr)
			}
		}
		var queryOptions adapter.DNSQueryOptions
		if options.Realm.STUNServersIsDomain() {
			queryOptions, err = dialer.NewDNSQueryOptions(ctx, options.Realm.STUNDomainResolver, true)
			if err != nil {
				return nil, E.Cause(err, "create realm STUN domain resolver")
			}
		}
		var httpClientTransport adapter.HTTPTransport
		httpClientTransport, err = service.FromContext[adapter.HTTPClientManager](ctx).ResolveTransport(ctx, logger, common.PtrValueOrDefault(options.Realm.HTTPClient))
		if err != nil {
			return nil, E.Cause(err, "create realm http client")
		}
		dnsRouter := service.FromContext[adapter.DNSRouter](ctx)
		realmOptions = &realm.Options{
			ServerURL:   options.Realm.ServerURL,
			Token:       options.Realm.Token,
			RealmID:     options.Realm.RealmID,
			STUNServers: options.Realm.STUNServers,
			HTTPClient:  &http.Client{Transport: httpClientTransport},
			Resolver: func(ctx context.Context, host string, ipv4, ipv6 bool) ([]netip.Addr, error) {
				dnsOptions := queryOptions
				switch {
				case ipv4 && !ipv6:
					dnsOptions.Strategy = C.DomainStrategyIPv4Only
				case !ipv4 && ipv6:
					dnsOptions.Strategy = C.DomainStrategyIPv6Only
				}
				return dnsRouter.Lookup(ctx, host, dnsOptions)
			},
			Logger:    logger,
			IPVersion: options.Realm.IPVersion,
		}
		if options.Realm.PortMapping != nil && options.Realm.PortMapping.Enabled {
			realmOptions.PortMapping = &realm.PortMappingOptions{
				Timeout:  time.Duration(options.Realm.PortMapping.Timeout),
				Lifetime: time.Duration(options.Realm.PortMapping.Lifetime),
			}
		}
	}
	hysteriaService, err := hysteria2.NewService[string](hysteria2.ServiceOptions{
		Context:            ctx,
		Logger:             logger,
		BrutalDebug:        options.BrutalDebug,
		SendBPS:            uint64(options.UpMbps * hysteria.MbpsToBps),
		ReceiveBPS:         uint64(options.DownMbps * hysteria.MbpsToBps),
		SalamanderPassword: salamanderPassword,
		GeckoPassword:      geckoPassword,
		GeckoMinPacketSize: geckoMinPacketSize,
		GeckoMaxPacketSize: geckoMaxPacketSize,
		// наше: TLS в обёртке, которая сама поднимает QUIC и следит за
		// соединениями.
		TLSConfig: &tlsSoSlezhkoy{
			ServerConfig: tlsConfig,
			slezhka:      h.slezhka,
			obfs:         salamanderPassword != "" || geckoPassword != "",
		},
		QUICOptions: qtls.QUICOptions{
			IdleTimeout:             options.IdleTimeout.Build(),
			KeepAlivePeriod:         options.KeepAlivePeriod.Build(),
			StreamReceiveWindow:     options.StreamReceiveWindow.Value(),
			ConnectionReceiveWindow: options.ConnectionReceiveWindow.Value(),
			MaxConcurrentStreams:    options.MaxConcurrentStreams,
			InitialPacketSize:       options.InitialPacketSize,
			DisablePathMTUDiscovery: options.DisablePathMTUDiscovery,
		},
		IgnoreClientBandwidth: options.IgnoreClientBandwidth,
		UDPTimeout:            udpTimeout,
		Handler:               h,
		MasqueradeHandler:     masqueradeHandler,
		BBRProfile:            options.BBRProfile,
		RealmOptions:          realmOptions,
	})
	if err != nil {
		return nil, err
	}
	h.service = hysteriaService
	// наше: начальный состав ставится тем же путём, что и смена на лету.
	if _, err := h.Smenit(&options); err != nil {
		return nil, err
	}
	return h, nil
}

// Smenit наше: ставит новый состав и закрывает соединения выбывших.
//
// Здесь служба не подменяется: она держит слушатель. Карта паролей внутри неё
// заменяется целиком одной записью указателя без замка, как у самого
// sing-box при перезагрузке и у s-ui. На amd64 читатель видит либо старую
// карту, либо новую, собранную до записи указателя.
func (h *Hysteria2) Smenit(opcii any) (int, error) {
	o, ok := opcii.(*option.Hysteria2InboundOptions)
	if !ok {
		return 0, fmt.Errorf("вход %s: ждали опции hysteria2, пришли %T", h.Tag(), opcii)
	}
	paroli := make([]string, 0, len(o.Users))
	for _, u := range o.Users {
		paroli = append(paroli, u.Password)
	}
	klyuchi, err := sostav(h.Tag(), lyudiHysteria2(o))
	if err != nil {
		return 0, err
	}
	otozvat := h.uchet.Dopustit(h.Tag(), klyuchi)
	h.service.UpdateUsers(klyuchi, paroli)
	return zakrytOtozvannyh(otozvat), nil
}

// vpustitPotok наше: поток hy2 приходит без своего QUIC-соединения, только с
// адресом клиента; соединение находит слежка.
func (h *Hysteria2) vpustitPotok(ctx context.Context, source M.Socksaddr) (string, func(), bool) {
	return h.uchet.vpustit(ctx, h.slezhka.zapis(source), h.Tag())
}

func (h *Hysteria2) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	ctx = log.ContextWithNewID(ctx)
	var metadata adapter.InboundContext
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	//nolint:staticcheck
	metadata.InboundDetour = h.listener.ListenOptions().Detour
	//nolint:staticcheck
	metadata.OriginDestination = h.listener.UDPAddr()
	metadata.Source = source
	metadata.Destination = destination
	h.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	// наше: человек из ключа, поток в учёт.
	user, otpustit, ok := h.vpustitPotok(ctx, source)
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	h.router.RouteConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}

func (h *Hysteria2) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	ctx = log.ContextWithNewID(ctx)
	var metadata adapter.InboundContext
	metadata.Inbound = h.Tag()
	metadata.InboundType = h.Type()
	//nolint:staticcheck
	metadata.InboundDetour = h.listener.ListenOptions().Detour
	//nolint:staticcheck
	metadata.OriginDestination = h.listener.UDPAddr()
	metadata.Source = source
	metadata.Destination = destination
	h.logger.InfoContext(ctx, "inbound packet connection from ", metadata.Source)
	// наше: человек из ключа, поток в учёт.
	user, otpustit, ok := h.vpustitPotok(ctx, source)
	if !ok {
		N.CloseOnHandshakeFailure(conn, onClose, errOtozvan)
		return
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	h.router.RoutePacketConnectionEx(ctx, conn, metadata, sOtpuskom(otpustit, onClose))
}

func (h *Hysteria2) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return err
		}
	}
	packetConn, err := h.listener.ListenUDP()
	if err != nil {
		return err
	}
	// наше: буфер сокета до маскировки, пока это ещё голый UDP.
	if err := nastroitBufer(packetConn); err != nil {
		h.logger.Warn("буфер сокета hy2: ", err)
	}
	return h.service.Start(packetConn)
}

func (h *Hysteria2) InterfaceUpdated(ctx context.Context) {
	h.service.Reset()
}

func (h *Hysteria2) Close() error {
	// наше: сначала служба, она шлёт клиентам закрытие через ещё живой
	// сокет; в исходнике сокет закрывался первым, и закрытие не уходило.
	return common.Close(
		common.PtrOrNil(h.service),
		h.listener,
		h.tlsConfig,
	)
}
