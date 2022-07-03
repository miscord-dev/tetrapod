package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/miscord-dev/toxfu/proto"
	"github.com/miscord-dev/toxfu/proto/nodeattr"
	"github.com/miscord-dev/toxfu/signal/signalclient"
	"github.com/samber/lo"
	"inet.af/netaddr"
	"tailscale.com/net/dns"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/logger"
	"tailscale.com/types/netmap"
	"tailscale.com/types/preftype"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/router"
	"tailscale.com/wgengine/wgcfg/nmcfg"
)

type Backend interface {
	Start()

	Close() error
}

type backend struct {
	signalClient  signalclient.Client
	engine        wgengine.Engine
	cfg           *Config
	nodeAttribute *proto.NodeAttribute

	currentEngineStatus     *wgengine.Status
	currentEngineStatusLock sync.RWMutex
	triggerStatusUpdateChan chan struct{}
}

type Config struct {
	SignalClient signalclient.Client
	Engine       wgengine.Engine
	NodePrivate  key.NodePrivate
	Logger       logger.Logf
}

func New(cfg *Config) Backend {
	return &backend{
		cfg:          cfg,
		signalClient: cfg.SignalClient,
		engine:       cfg.Engine,
	}
}

func (b *backend) Start() {
	b.nodeAttribute = nodeattr.NewNodeAttribute(b.cfg.Logger)

	b.signalClient.RegisterRecvCallback(b.recvCallbck)
	b.engine.SetStatusCallback(b.statusCallback)
	b.engine.RequestStatus()
	go b.run()
}

var (
	expiryTime = time.Date(2222, time.January, 1, 0, 0, 0, 0, time.UTC)
)

func (b *backend) composeNetworkMap(resp *proto.NodeRefreshResponse) (*netmap.NetworkMap, error) {
	selfNode, err := resp.SelfNode.TailcfgNode()

	if err != nil {
		return nil, fmt.Errorf("failed to parse tailcfg node: %+v", err)
	}

	peers := make([]*tailcfg.Node, 0, len(resp.Peers))
	for _, p := range resp.Peers {
		peer, err := p.TailcfgNode()

		if err != nil {
			b.cfg.Logger("failed to parse tailcfg peer %d: %+v", p.Id, err)

			continue
		}

		peers = append(peers, peer)
	}

	nm := &netmap.NetworkMap{
		SelfNode:      selfNode,
		NodeKey:       b.cfg.NodePrivate.Public(),
		PrivateKey:    b.cfg.NodePrivate,
		Expiry:        expiryTime,
		Addresses:     selfNode.Addresses,
		MachineStatus: tailcfg.MachineAuthorized,
		Peers:         peers,
	}

	func() {
		fp, err := os.Create("/tmp/toxfu_network_map.json")

		if err != nil {
			panic(err)
		}

		defer fp.Close()

		json.NewEncoder(fp).Encode(nm)
	}()

	return nm, nil
}

func (b *backend) composeDERPMap(resp *proto.NodeRefreshResponse) (*tailcfg.DERPMap, error) {
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionName: "def",
				RegionCode: "Default",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:     "def",
						RegionID: 1,
						HostName: resp.GetStunServer(),
						STUNOnly: true,
						STUNPort: 19302,
					},
				},
			},
		},
		OmitDefaultRegions: true,
	}

	return derpMap, nil
}

func (b *backend) recvCallbck(resp *proto.NodeRefreshResponse) {
	func() {
		fp, err := os.Create("/tmp/toxfu_current_config.json")

		if err != nil {
			panic(err)
		}

		defer fp.Close()

		json.NewEncoder(fp).Encode(resp)
	}()

	nm, err := b.composeNetworkMap(resp)

	if err != nil {
		b.cfg.Logger("failed to compose network map: %+v", err)

		return
	}

	derpMap, err := b.composeDERPMap(resp)

	if err != nil {
		b.cfg.Logger("failed to compose DERP map: %+v", err)

		return
	}

	nm.DERPMap = derpMap

	var exitNode tailcfg.StableNodeID
	for _, p := range nm.Peers {
		for _, allowedIP := range p.AllowedIPs {
			if allowedIP.Bits() == 0 {
				exitNode = p.StableID
			}
		}
	}

	wgCfg, err := nmcfg.WGCfg(nm, b.cfg.Logger, netmap.AllowSubnetRoutes, exitNode)

	if err != nil {
		b.cfg.Logger("failed to generate WireGuard config: %+v", err)

		return
	}

	rcfg, err := b.routerConfig(resp)

	if err != nil {
		b.cfg.Logger("failed to generate router config: %+v", err)

		return
	}

	err = b.engine.Reconfig(wgCfg, rcfg, &dns.Config{}, &tailcfg.Debug{})

	if err != nil && err != wgengine.ErrNoChanges {
		b.cfg.Logger("failed to reconfigure WireGuard: %+v", err)

		return
	}

	b.engine.SetNetworkMap(nm)
	b.engine.SetDERPMap(derpMap)
}

func (b *backend) routerConfig(resp *proto.NodeRefreshResponse) (*router.Config, error) {
	selfNode, err := resp.SelfNode.TailcfgNode()

	if err != nil {
		return nil, fmt.Errorf("failed to parse tailcfg node: %+v", err)
	}

	var routes []netaddr.IPPrefix

	for _, peer := range resp.Peers {
		node, err := peer.TailcfgNode()

		if err != nil {
			b.cfg.Logger("failed to parse tailcfg peer %d: %+v", peer.Id, err)

			continue
		}

		routes = append(routes, node.AllowedIPs...)
	}

	cfg := router.Config{
		LocalAddrs:       selfNode.Addresses,
		SubnetRoutes:     selfNode.PrimaryRoutes,
		SNATSubnetRoutes: false,
		NetfilterMode:    preftype.NetfilterOn,
		Routes:           routes,
	}

	return &cfg, nil
}

func (b *backend) sendRefreshRequest() error {
	b.currentEngineStatusLock.RLock()
	stat := b.currentEngineStatus
	b.currentEngineStatusLock.RUnlock()

	if stat == nil {
		return nil
	}

	req := proto.NodeRefreshRequest{}
	req.PublicKey = proto.MarshalNodePublic(b.cfg.NodePrivate.Public())
	req.PublicDiscoKey = proto.MarshalDiscoPublic(b.engine.DiscoPublicKey())
	req.Attribute = b.nodeAttribute
	req.Endpoints = lo.Map(stat.LocalAddrs, func(addr tailcfg.Endpoint, i int) string {
		return addr.Addr.String()
	})

	return b.signalClient.Send(&req)
}

func (b *backend) run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	b.cfg.Logger("running backend thread")

	exp := backoff.NewExponentialBackOff()
	exp.MaxInterval = 15 * time.Second
	for {
		select {
		case <-b.triggerStatusUpdateChan:
		case <-ticker.C:
		}
		b.cfg.Logger("sending status")

		backoff.Retry(func() error {
			if err := b.sendRefreshRequest(); err != nil {
				b.cfg.Logger("failed to send refresh request(elapsed: %v): %+v", exp.GetElapsedTime(), err)
			}

			return nil
		}, exp)
	}
}

func (b *backend) statusCallback(stat *wgengine.Status, err error) {
	if err != nil {
		b.cfg.Logger("status callback error: %+v", err)
		return
	}

	b.currentEngineStatusLock.Lock()
	b.currentEngineStatus = stat
	b.currentEngineStatusLock.Unlock()
	b.triggerStatusUpdate()
}

func (b *backend) triggerStatusUpdate() {
	select {
	case b.triggerStatusUpdateChan <- struct{}{}:
	default:
	}
}

func (b *backend) Close() error {
	err := b.signalClient.Close()

	b.engine.Close()
	b.engine.Wait()

	return err
}
