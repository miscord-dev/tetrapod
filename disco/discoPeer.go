package disco

import (
	"math"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miscord-dev/toxfu/disco/ticker"
	"github.com/miscord-dev/toxfu/pkg/sets"
	"github.com/miscord-dev/toxfu/pkg/syncmap"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap"
)

type DiscoPeer struct {
	closed chan struct{}

	disco    *Disco
	recvChan chan EncryptedDiscoPacket

	srcPublicDiscoKey wgkey.DiscoPublicKey
	sharedKey         wgkey.DiscoSharedKey

	endpointIDCounter    uint32
	endpointToEndpointID syncmap.Map[netip.AddrPort, uint32]
	endpoints            syncmap.Map[uint32, *DiscoPeerEndpoint]

	endpointStatusMap syncmap.Map[uint32, DiscoPeerEndpointStatusReadOnly]
	status            *DiscoPeerStatus

	logger *zap.Logger
}

func newDiscoPeer(d *Disco, pubKey wgkey.DiscoPublicKey, logger *zap.Logger) *DiscoPeer {
	dp := &DiscoPeer{
		closed:               make(chan struct{}),
		disco:                d,
		recvChan:             make(chan EncryptedDiscoPacket),
		srcPublicDiscoKey:    pubKey,
		sharedKey:            d.privateKey.Shared(pubKey),
		endpointToEndpointID: syncmap.Map[netip.AddrPort, uint32]{},
		endpoints:            syncmap.Map[uint32, *DiscoPeerEndpoint]{},
		endpointStatusMap:    syncmap.Map[uint32, DiscoPeerEndpointStatusReadOnly]{},
		status: &DiscoPeerStatus{
			cond: sync.NewCond(&sync.Mutex{}),
		},
		logger: logger.With(
			zap.String("service", "disco_peer"),
			zap.String("public_disco_key", pubKey.String()),
		),
	}

	go dp.run()

	return dp
}

func (p *DiscoPeer) SetEndpoints(
	endpoints []netip.AddrPort,
) {
	id := atomic.AddUint32(&p.endpointIDCounter, 1)

	// Assign next DESID
	renewID := func() {
		id = atomic.AddUint32(&p.endpointIDCounter, 1)
	}

	for _, ep := range endpoints {
		endpointID, loaded := p.endpointToEndpointID.LoadOrStore(ep, id)

		if loaded {
			continue
		}

		renewID()

		pe := newDiscoPeerEndpoint(p, endpointID, ep, p.logger)

		pe.Status().NotifyStatus(func(status DiscoPeerEndpointStatusReadOnly) {
			p.endpointStatusMap.Store(endpointID, status)
			p.updateStatus()
		})

		p.endpoints.Store(endpointID, pe)
	}

	endpointSet := sets.FromSlice(endpoints)

	p.endpoints.Range(func(key uint32, value *DiscoPeerEndpoint) bool {
		ep := value.endpoint

		if endpointSet.Contains(ep) {
			return true
		}

		p.endpointToEndpointID.Delete(ep)
		endpoint, ok := p.endpoints.LoadAndDelete(key)
		if !ok {
			return true
		}
		endpoint.Close()

		return true
	})
}

func (p *DiscoPeer) Status() *DiscoPeerStatus {
	return p.status
}

func (p *DiscoPeer) updateStatus() {
	minRTT := time.Duration(math.MaxInt64)
	var minEndpoint netip.AddrPort
	var minID uint32
	p.endpointStatusMap.Range(func(key uint32, value DiscoPeerEndpointStatusReadOnly) bool {
		if value.State != ticker.Connected || minRTT < value.RTT {
			return true
		}

		dpe, ok := p.endpoints.Load(key)

		if !ok {
			return true
		}

		minRTT = value.RTT
		minEndpoint = dpe.endpoint
		minID = key

		return true
	})

	p.endpoints.Range(func(key uint32, value *DiscoPeerEndpoint) bool {
		if minRTT == math.MaxInt64 {
			value.SetPriority(ticker.Primary)

			return true
		}

		priority := ticker.Sub
		if key == minID {
			priority = ticker.Primary
		}

		value.SetPriority(priority)

		return true
	})

	if minRTT == math.MaxInt64 {
		return
	}

	p.status.setStatus(minEndpoint, minRTT)
}

func (p *DiscoPeer) enqueueReceivedPacket(pkt EncryptedDiscoPacket) {
	p.recvChan <- pkt
}

func (p *DiscoPeer) handlePing(pkt DiscoPacket) {
	resp := DiscoPacket{
		Header:            PongMessage,
		SrcPublicDiscoKey: p.disco.publicKey,
		EndpointID:        pkt.EndpointID,
		ID:                pkt.ID,

		Endpoint:  pkt.Endpoint,
		SharedKey: p.sharedKey,
	}

	encrypted, ok := resp.Encrypt()

	if !ok {
		return
	}

	p.disco.sendChan <- encrypted

	id, ok := p.endpointToEndpointID.Load(pkt.Endpoint)
	if !ok {
		return
	}

	ep, ok := p.endpoints.Load(id)
	if !ok {
		return
	}

	ep.ReceivePing()
}

func (p *DiscoPeer) run() {
	for {
		var pkt EncryptedDiscoPacket
		select {
		case <-p.closed:
			return
		case <-p.disco.closed:
			return
		case pkt = <-p.recvChan:
		}

		decrypted := DiscoPacket{
			SharedKey: p.sharedKey,
		}
		if !decrypted.Decrypt(&pkt) {
			continue
		}

		if decrypted.Header == PingMessage {
			p.handlePing(decrypted)

			continue
		}

		ep, ok := p.endpoints.Load(decrypted.EndpointID)

		if !ok {
			continue
		}

		ep.enqueueReceivedPacket(decrypted)
	}
}

func (p *DiscoPeer) Close() error {
	defer func() {
		recover()
	}()
	close(p.closed)
	p.status.close()

	p.disco.peers.Delete(p.srcPublicDiscoKey)

	return nil
}

type DiscoPeerStatus struct {
	cond *sync.Cond

	closed         bool
	activeEndpoint netip.AddrPort
	activeRTT      time.Duration
}

type DiscoPeerStatusReadOnly struct {
	ActiveEndpoint netip.AddrPort
	ActiveRTT      time.Duration
}

func (s *DiscoPeerStatus) NotifyStatus(fn func(status DiscoPeerStatusReadOnly)) {
	go s.notifyStatus(fn)
}

func (s *DiscoPeerStatus) notifyStatus(fn func(status DiscoPeerStatusReadOnly)) {
	s.cond.L.Lock()
	prev := s.readonly()
	s.cond.L.Unlock()
	fn(prev)
	for {
		s.cond.L.Lock()
		if s.closed {
			s.cond.L.Unlock()
			return
		}

		curr := s.readonly()
		if !curr.equalsTo(prev) {
			s.cond.L.Unlock()
			fn(curr)
			prev = curr

			continue
		}

		s.cond.Wait()
		curr = s.readonly()

		if s.closed {
			s.cond.L.Unlock()
			return
		}

		s.cond.L.Unlock()

		if !curr.equalsTo(prev) {
			fn(curr)
			prev = curr
		}
	}
}

func (s *DiscoPeerStatus) Get() DiscoPeerStatusReadOnly {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	return s.readonly()
}

func (s *DiscoPeerStatus) setStatus(activeEndpoint netip.AddrPort, activeRTT time.Duration) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.activeEndpoint = activeEndpoint
	s.activeRTT = activeRTT

	s.cond.Broadcast()
}

func (s *DiscoPeerStatus) close() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.closed = true

	s.cond.Broadcast()
}

func (s *DiscoPeerStatus) readonly() DiscoPeerStatusReadOnly {
	return DiscoPeerStatusReadOnly{
		ActiveEndpoint: s.activeEndpoint,
		ActiveRTT:      s.activeRTT,
	}
}

func (s *DiscoPeerStatusReadOnly) equalsTo(target DiscoPeerStatusReadOnly) bool {
	return s.ActiveEndpoint == target.ActiveEndpoint && s.ActiveRTT == target.ActiveRTT
}
