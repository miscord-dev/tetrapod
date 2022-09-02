package toxfuengine

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/miscord-dev/toxfu/disco"
	"github.com/miscord-dev/toxfu/pkg/endpoints"
	"github.com/miscord-dev/toxfu/pkg/hijack"
	"github.com/miscord-dev/toxfu/pkg/splitconn"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"github.com/miscord-dev/toxfu/toxfuengine/wgengine"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ToxfuEngine interface {
	Notify(fn func(PeerConfig))
	Reconfig(cfg *Config)
	Trigger()
	Close() error
}

type toxfuEngine struct {
	wgEngine   wgengine.Engine
	disco      *disco.Disco
	hijackConn *hijack.Conn
	collector  *endpoints.Collector

	discoPrivateKey   wgkey.DiscoPrivateKey
	currentConfig     atomic.Pointer[Config]
	endpoints         atomic.Pointer[[]netip.AddrPort]
	callback          atomic.Pointer[func(PeerConfig)]
	reconfigTriggerCh chan struct{}

	logger *zap.Logger
}

func New(ifaceName string, config *Config, logger *zap.Logger) (res ToxfuEngine, err error) {
	engine := &toxfuEngine{
		logger:            logger,
		reconfigTriggerCh: make(chan struct{}, 1),
	}

	if err := engine.init(ifaceName, config); err != nil {
		return nil, fmt.Errorf("failed to init engine: %w", err)
	}

	return engine, nil
}

func (e *toxfuEngine) init(ifaceName string, config *Config) error {
	var err error

	e.wgEngine, err = wgengine.New(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to set up wgengine: %w", err)
	}

	discoPrivateKey, err := wgkey.New()
	if err != nil {
		return fmt.Errorf("failed to generate disco private key: %w", err)
	}
	e.discoPrivateKey = discoPrivateKey

	e.hijackConn, err = hijack.NewConnWithLogger(config.ListenPort, e.logger.With(zap.String("service", "hijack")))
	if err != nil {
		return fmt.Errorf("failed to initialize hijack conn: %w", err)
	}

	splitter := splitconn.NewBundler(e.hijackConn)
	discoConn := splitter.Add(func(b []byte, addr netip.AddrPort) bool {
		return len(b) != 0 && (b[0]&0x80 != 0)
	})
	stunConn := splitter.Add(func(b []byte, addr netip.AddrPort) bool {
		return true
	})

	collector, err := endpoints.NewCollector(
		stunConn,
		config.ListenPort,
		config.STUNEndpoint,
		e.logger.With(zap.String("service", "endpoints_collector")),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize collector: %w", err)
	}
	e.collector = collector
	collector.Notify(e.endpointsCallback)

	e.disco, err = disco.NewFromPacketConn(discoPrivateKey, discoConn)
	if err != nil {
		return fmt.Errorf("failed to initialize disco: %w", err)
	}
	e.disco.Start()
	e.disco.SetStatusCallback(e.discoStatusCallback)

	e.currentConfig.Store(config)
	e.triggerReconfig()

	go e.runReconfig()

	return nil
}

func (e *toxfuEngine) endpointsCallback(addrPorts []netip.AddrPort) {
	e.endpoints.Store(&addrPorts)
	e.notify()
}

func (e *toxfuEngine) notify() {
	var endpoints []string
	if ap := e.endpoints.Load(); ap != nil {
		for _, e := range *ap {
			endpoints = append(endpoints, e.String())
		}
	}

	cfg := e.currentConfig.Load()

	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		e.logger.Error("failed to parse private key", zap.Error(err))

		return
	}

	addresses := make([]string, 0, len(cfg.Addresses))
	allowedIPs := make([]string, 0, len(cfg.Addresses))
	for _, a := range cfg.Addresses {
		addresses = append(addresses, a.String())

		addr, _ := netip.AddrFromSlice(a.IP)
		addr = addr.Unmap()
		bits := 32

		if addr.Is6() {
			bits = 128
		}

		allowedIPs = append(addresses, netip.PrefixFrom(addr, bits).String())
	}

	peerConfig := PeerConfig{
		Endpoints:      endpoints,
		PublicKey:      privKey.PublicKey().String(),
		PublicDiscoKey: e.discoPrivateKey.Public().String(),
		Addresses:      addresses,
		AllowedIPs:     allowedIPs,
	}

	fn := e.callback.Load()
	if fn == nil || *fn == nil {
		return
	}

	(*fn)(peerConfig)
}

func (e *toxfuEngine) Notify(fn func(PeerConfig)) {
	e.callback.Store(&fn)
}

func (e *toxfuEngine) triggerReconfig() {
	select {
	case e.reconfigTriggerCh <- struct{}{}:
	default:
	}
}

func (e *toxfuEngine) runReconfig() {
	for range e.reconfigTriggerCh {
		if err := e.reconfig(); err != nil {
			e.logger.Error("reconfig failed", zap.Error(err))
		}
	}
}

func (e *toxfuEngine) reconfigDisco(cfg *Config) {
	peers := make(map[wgkey.DiscoPublicKey][]netip.AddrPort)
	for _, peer := range cfg.Peers {
		logger := e.logger.With(
			zap.String("pubkey", peer.PublicKey),
			zap.String("pubDiscoKey", peer.PublicKey),
		)

		pubKey, err := wgkey.Parse(peer.PublicDiscoKey)

		if err != nil {
			logger.Error("failed to parse disco pubkey", zap.Error(err))

			continue
		}

		eps := peer.Endpoints
		endpoints := make([]netip.AddrPort, 0, len(eps))

		for i := range eps {
			addrPort, err := netip.ParseAddrPort(eps[i])

			if err != nil {
				logger.Error("failed to parse endpoint", zap.Error(err), zap.String("endpoint", eps[i]))

				continue
			}

			endpoints = append(endpoints, addrPort)
		}

		peers[pubKey] = endpoints
	}

	e.disco.SetPeers(peers)
}

func (e *toxfuEngine) reconfig() error {
	cfg := e.currentConfig.Load()

	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)

	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	e.reconfigDisco(cfg)

	discoStatuses := e.disco.GetAllStatuses()

	e.logger.Info("disco status", zap.Any("disco", convertDiscoStatusesForJSON(discoStatuses)))

	getDiscoStatus := func(pubKey string) (disco.DiscoPeerStatusReadOnly, bool) {
		discoPubKey, err := wgkey.Parse(pubKey)

		if err != nil {
			return disco.DiscoPeerStatusReadOnly{}, false
		}

		status, ok := discoStatuses[discoPubKey]
		if !ok {
			return disco.DiscoPeerStatusReadOnly{}, false
		}

		return status, true
	}

	wgPeerConfigs := make([]wgtypes.PeerConfig, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		logger := e.logger.With(
			zap.String("pubkey", peer.PublicKey),
			zap.String("pubDiscoKey", peer.PublicKey),
		)

		wcfg, err := peer.toWGConfig()

		if err != nil {
			logger.Error("failed to convert peer config", zap.Error(err))

			continue
		}

		status, ok := getDiscoStatus(peer.PublicDiscoKey)
		if ok && status.ActiveEndpoint.IsValid() {
			wcfg.Endpoint = net.UDPAddrFromAddrPort(status.ActiveEndpoint)
		}

		wgPeerConfigs = append(wgPeerConfigs, *wcfg)
	}

	wgConfig := wgtypes.Config{
		ListenPort:   ptr(cfg.ListenPort),
		PrivateKey:   &privKey,
		ReplacePeers: true,
		Peers:        wgPeerConfigs,
	}

	err = e.wgEngine.Reconfig(wgConfig, cfg.Addresses)

	if err != nil {
		return fmt.Errorf("failed to reconfig wgengine: %w", err)
	}

	return nil
}

func (e *toxfuEngine) discoStatusCallback(wgkey.DiscoPublicKey, disco.DiscoPeerStatusReadOnly) {
	e.triggerReconfig()
}

func (e *toxfuEngine) Reconfig(cfg *Config) {
	e.currentConfig.Store(cfg)
	e.triggerReconfig()
}

func (e *toxfuEngine) Trigger() {
	e.collector.Trigger()
	e.triggerReconfig()
}

func (e *toxfuEngine) Close() error {
	if e.disco != nil {
		e.disco.Close()
	}
	if e.wgEngine != nil {
		e.wgEngine.Close()
	}
	if e.collector != nil {
		e.collector.Close()
	}
	if e.hijackConn != nil {
		e.hijackConn.Close()
	}
	close(e.reconfigTriggerCh)

	return nil
}

func convertDiscoStatusesForJSON(r map[wgkey.DiscoPublicKey]disco.DiscoPeerStatusReadOnly) map[string]interface{} {
	m := map[string]interface{}{}

	for k, v := range r {
		m[k.String()] = v
	}

	return m
}
