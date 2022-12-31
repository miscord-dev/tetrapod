package disco

import (
	"math"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miscord-dev/toxfu/toxfucni/disco/ticker"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/sets"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/syncmap"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/wgkey"
	"go.uber.org/zap"
)

//go:generate mockgen -source=$GOFILE -package=mock_$GOPACKAGE -destination=./mock/mock_$GOFILE

type DiscoPeer interface {
	EnqueueReceivedPacket(pkt EncryptedDiscoPacket)
	SetEndpoints(endpoints []netip.AddrPort)
	Status() DiscoPeerStatus
	Close() error
}

type DiscoPeerStatus interface {
	Get() DiscoPeerStatusReadOnly
	NotifyStatus(fn func(status DiscoPeerStatusReadOnly))
}

type discoPeer struct {
	closed chan struct{}

	recvChan chan EncryptedDiscoPacket

	sender              Sender
	peerPublicDiscoKey  wgkey.DiscoPublicKey
	localPublicDiscoKey wgkey.DiscoPublicKey
	sharedKey           wgkey.DiscoSharedKey
	onClose             func()
	newEndpoint         NewDiscoPeerEndpointFunc

	endpointIDCounter    uint32
	endpointToEndpointID syncmap.Map[netip.AddrPort, uint32]
	endpoints            syncmap.Map[uint32, DiscoPeerEndpoint]

	endpointStatusMap syncmap.Map[uint32, DiscoPeerEndpointStatusReadOnly]
	status            *discoPeerStatus

	logger *zap.Logger
}

type NewDiscoPeerFunc func(
	d Sender,
	privateKey wgkey.DiscoPrivateKey,
	pubKey wgkey.DiscoPublicKey,
	onClose func(),
	logger *zap.Logger,
	newEndpoint NewDiscoPeerEndpointFunc,
) DiscoPeer

var _ NewDiscoPeerFunc = NewDiscoPeer

func NewDiscoPeer(
	d Sender,
	privateKey wgkey.DiscoPrivateKey,
	pubKey wgkey.DiscoPublicKey,
	onClose func(),
	logger *zap.Logger,
	newEndpoint NewDiscoPeerEndpointFunc,
) DiscoPeer {
	dp := &discoPeer{
		closed:               make(chan struct{}),
		recvChan:             make(chan EncryptedDiscoPacket),
		sender:               d,
		peerPublicDiscoKey:   pubKey,
		localPublicDiscoKey:  privateKey.Public(),
		sharedKey:            privateKey.Shared(pubKey),
		onClose:              onClose,
		endpointToEndpointID: syncmap.Map[netip.AddrPort, uint32]{},
		endpoints:            syncmap.Map[uint32, DiscoPeerEndpoint]{},
		endpointStatusMap:    syncmap.Map[uint32, DiscoPeerEndpointStatusReadOnly]{},
		newEndpoint:          newEndpoint,
		status: &discoPeerStatus{
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

func (p *discoPeer) SetEndpoints(
	endpoints []netip.AddrPort,
) {
	select {
	case <-p.closed:
		p.closeAllEndpoints()
		return
	default:
	}

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

		pe := p.newEndpoint(
			endpointID,
			ep,
			p.localPublicDiscoKey,
			p.sharedKey,
			p.sender,
			p.logger,
		)

		pe.Status().NotifyStatus(func(status DiscoPeerEndpointStatusReadOnly) {
			p.endpointStatusMap.Store(endpointID, status)
			p.updateStatus()
		})

		p.endpoints.Store(endpointID, pe)
	}

	select {
	case <-p.closed:
		p.closeAllEndpoints()
		return
	default:
	}

	endpointSet := sets.FromSlice(endpoints)

	p.endpoints.Range(func(key uint32, value DiscoPeerEndpoint) bool {
		ep := value.Endpoint()

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

func (p *discoPeer) Status() DiscoPeerStatus {
	return p.status
}

func (p *discoPeer) updateStatus() {
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
		minEndpoint = dpe.Endpoint()
		minID = key

		return true
	})

	p.endpoints.Range(func(key uint32, value DiscoPeerEndpoint) bool {
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

func (p *discoPeer) EnqueueReceivedPacket(pkt EncryptedDiscoPacket) {
	p.recvChan <- pkt
}

func (p *discoPeer) handlePing(pkt DiscoPacket) {
	resp := DiscoPacket{
		Header:            PongMessage,
		SrcPublicDiscoKey: p.localPublicDiscoKey,
		EndpointID:        pkt.EndpointID,
		ID:                pkt.ID,

		Endpoint:  pkt.Endpoint,
		SharedKey: p.sharedKey,
	}

	encrypted, ok := resp.Encrypt()

	if !ok {
		return
	}

	p.sender.Send(encrypted)

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

func (p *discoPeer) run() {
	for {
		var pkt EncryptedDiscoPacket
		select {
		case <-p.closed:
			return
		case pkt = <-p.recvChan:
		}

		decrypted := DiscoPacket{
			SharedKey: p.sharedKey,
		}
		if !decrypted.Decrypt(&pkt) {
			p.logger.Debug("decryption failed", zap.String("endpoint", pkt.Endpoint.String()))

			continue
		}

		p.logger.Debug("disco peer message received", zap.String("endpoint", decrypted.Endpoint.String()), zap.String("header", decrypted.Header.String()))

		if decrypted.Header == PingMessage {
			p.handlePing(decrypted)

			continue
		}

		ep, ok := p.endpoints.Load(decrypted.EndpointID)

		if !ok {
			continue
		}

		ep.EnqueueReceivedPacket(decrypted)
	}
}

func (p *discoPeer) closeAllEndpoints() {
	p.endpoints.Range(func(key uint32, value DiscoPeerEndpoint) bool {
		ep := value.Endpoint()

		p.endpointToEndpointID.Delete(ep)
		endpoint, ok := p.endpoints.LoadAndDelete(key)
		if !ok {
			return true
		}
		endpoint.Close()

		return true
	})
}

func (p *discoPeer) Close() error {
	defer func() {
		recover()
	}()
	close(p.closed)
	p.closeAllEndpoints()
	p.status.close()

	if p.onClose != nil {
		p.onClose()
	}

	return nil
}

type discoPeerStatus struct {
	cond *sync.Cond

	closed         bool
	activeEndpoint netip.AddrPort
	activeRTT      time.Duration
}

type DiscoPeerStatusReadOnly struct {
	ActiveEndpoint netip.AddrPort
	ActiveRTT      time.Duration
}

func (s *discoPeerStatus) NotifyStatus(fn func(status DiscoPeerStatusReadOnly)) {
	go s.notifyStatus(fn)
}

func (s *discoPeerStatus) notifyStatus(fn func(status DiscoPeerStatusReadOnly)) {
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
		if !curr.equalTo(prev) {
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

		if !curr.equalTo(prev) {
			fn(curr)
			prev = curr
		}
	}
}

func (s *discoPeerStatus) Get() DiscoPeerStatusReadOnly {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	return s.readonly()
}

func (s *discoPeerStatus) setStatus(activeEndpoint netip.AddrPort, activeRTT time.Duration) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.activeEndpoint = activeEndpoint
	s.activeRTT = activeRTT

	s.cond.Broadcast()
}

func (s *discoPeerStatus) close() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.closed = true

	s.cond.Broadcast()
}

func (s *discoPeerStatus) readonly() DiscoPeerStatusReadOnly {
	return DiscoPeerStatusReadOnly{
		ActiveEndpoint: s.activeEndpoint,
		ActiveRTT:      s.activeRTT,
	}
}

func (s *DiscoPeerStatusReadOnly) equalTo(target DiscoPeerStatusReadOnly) bool {
	return s.ActiveEndpoint == target.ActiveEndpoint && s.ActiveRTT == target.ActiveRTT
}
