package tetraengine

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"sort"

	"github.com/miscord-dev/tetrapod/disco"
	"github.com/miscord-dev/tetrapod/pkg/endpoints"
	"github.com/miscord-dev/tetrapod/pkg/hijack"
	"github.com/miscord-dev/tetrapod/pkg/splitconn"
	"github.com/miscord-dev/tetrapod/pkg/wgkey"
	"github.com/miscord-dev/tetrapod/tetraengine/wgengine"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type TetraEngine interface {
	Notify(fn func(PeerConfig))
	Reconfig(cfg *Config)
	Trigger()
	Close() error
}

type tetraEngine struct {
	wgEngine   wgengine.Engine
	disco      disco.Disco
	hijackConn *hijack.Conn
	collector  *endpoints.Collector

	discoPrivateKey       wgkey.DiscoPrivateKey
	currentConfig         atomic.Pointer[Config]
	endpoints             atomic.Pointer[[]netip.AddrPort]
	callback              atomic.Pointer[func(PeerConfig)]
	reconfigTriggerCh     chan struct{}
	latestDiscoStatusHash atomic.Pointer[string]

	logger *zap.Logger
}

func New(ifaceName, vrf string, table uint32, config *Config, logger *zap.Logger) (res TetraEngine, err error) {
	engine := &tetraEngine{
		logger:            logger,
		reconfigTriggerCh: make(chan struct{}, 1),
	}

	if err := engine.init(ifaceName, vrf, table, config); err != nil {
		return nil, fmt.Errorf("failed to init engine: %w", err)
	}

	return engine, nil
}

func (e *tetraEngine) init(ifaceName, vrf string, table uint32, config *Config) error {
	var err error

	e.wgEngine, err = wgengine.NewVRF(ifaceName, vrf, table)
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

	e.disco = disco.NewFromPacketConn(discoPrivateKey, discoConn, e.logger)
	e.disco.SetStatusCallback(e.discoStatusCallback)

	e.currentConfig.Store(config)
	e.triggerReconfig()

	go e.runReconfig()

	return nil
}

func (e *tetraEngine) endpointsCallback(addrPorts []netip.AddrPort) {
	e.endpoints.Store(&addrPorts)
	e.notify()
}

func (e *tetraEngine) notify() {
	cfg := e.currentConfig.Load()

	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		e.logger.Error("failed to parse private key", zap.Error(err))

		return
	}

	addresses := make([]string, 0, len(cfg.Addresses))
	addressSets := map[string]struct{}{}
	allowedIPs := make([]string, 0, len(cfg.Addresses))
	for _, a := range cfg.Addresses {
		addresses = append(addresses, a.String())
		addressSets[a.IP.String()] = struct{}{}

		addr, _ := netip.AddrFromSlice(a.IP)
		allowedIPs = append(allowedIPs, toAddrPrefix(addr).String())
	}

	var endpoints []string
	if ap := e.endpoints.Load(); ap != nil {
		for _, e := range *ap {
			_, ok := addressSets[e.Addr().String()]
			if ok {
				continue
			}

			endpoints = append(endpoints, e.String())
		}
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

func (e *tetraEngine) Notify(fn func(PeerConfig)) {
	e.callback.Store(&fn)
}

func (e *tetraEngine) triggerReconfig() {
	select {
	case e.reconfigTriggerCh <- struct{}{}:
	default:
	}
}

func (e *tetraEngine) runReconfig() {
	for range e.reconfigTriggerCh {
		if err := e.reconfig(); err != nil {
			e.logger.Error("reconfig failed", zap.Error(err))
		}
	}
}

func (e *tetraEngine) reconfigDisco(cfg *Config) {
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

func (e *tetraEngine) reconfig() error {
	e.hijackConn.Refresh()

	cfg := e.currentConfig.Load()

	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)

	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	e.reconfigDisco(cfg)

	discoStatuses := e.disco.GetAllStatuses()

	e.logger.Debug("disco status", zap.Any("disco", convertDiscoStatusesForJSON(discoStatuses)))

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

func (e *tetraEngine) hasDiscoStatusUpdate() bool {
	var statusPairs []struct {
		PubKey   string
		Endpoint string
	}
	for k, v := range e.disco.GetAllStatuses() {
		statusPairs = append(statusPairs, struct {
			PubKey   string
			Endpoint string
		}{
			PubKey:   k.String(),
			Endpoint: v.ActiveEndpoint.String(),
		})
	}

	sort.Slice(statusPairs, func(i, j int) bool {
		return statusPairs[i].PubKey < statusPairs[j].PubKey
	})

	hash, _ := json.Marshal(statusPairs)

	latestHash := e.latestDiscoStatusHash.Load()

	if latestHash == nil || string(hash) != *latestHash {
		h := string(hash)
		e.latestDiscoStatusHash.Store(&h)

		return true
	}

	return false
}

func (e *tetraEngine) discoStatusCallback(wgkey.DiscoPublicKey, disco.DiscoPeerStatusReadOnly) {
	if e.hasDiscoStatusUpdate() {
		e.triggerReconfig()
	}
}

func (e *tetraEngine) Reconfig(cfg *Config) {
	e.currentConfig.Store(cfg)
	e.triggerReconfig()
}

func (e *tetraEngine) Trigger() {
	e.collector.Trigger()
	e.triggerReconfig()
}

func (e *tetraEngine) Close() error {
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
