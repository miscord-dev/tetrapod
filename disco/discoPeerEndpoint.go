package disco

import (
	"net/netip"
	"sync"
	"time"

	"github.com/miscord-dev/toxfu/disco/pktmgr"
	"github.com/miscord-dev/toxfu/disco/ticker"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap"
)

//go:generate mockgen -source=$GOFILE -package=mock_$GOPACKAGE -destination=./mock/mock_$GOFILE

type DiscoPeerEndpoint interface {
	Endpoint() netip.AddrPort
	Status() DiscoPeerEndpointStatus
	SetPriority(priority ticker.Priority)
	EnqueueReceivedPacket(pkt DiscoPacket)
	ReceivePing()
	Close() error
}

type DiscoPeerEndpointStatus interface {
	Get() DiscoPeerEndpointStatusReadOnly
	NotifyStatus(fn func(status DiscoPeerEndpointStatusReadOnly))
}

type discoPeerEndpoint struct {
	recvChan      chan DiscoPacket
	closed        chan struct{}
	packetManager *pktmgr.Manager
	ticker        ticker.Ticker

	endpointID  uint32
	endpoint    netip.AddrPort
	localPubKey wgkey.DiscoPublicKey
	sharedKey   wgkey.DiscoSharedKey
	sender      Sender

	packetID uint32

	statusLock        sync.Mutex
	priority          ticker.Priority
	status            *discoPeerEndpointStatus
	lastReinitialized time.Time

	logger *zap.Logger
}

var _ DiscoPeerEndpoint = &discoPeerEndpoint{}

type NewDiscoPeerEndpointFunc func(
	endpointID uint32,
	endpoint netip.AddrPort,
	peerPubKey wgkey.DiscoPublicKey,
	sharedKey wgkey.DiscoSharedKey,
	sender Sender,
	logger *zap.Logger,
) DiscoPeerEndpoint

func NewDiscoPeerEndpoint(
	endpointID uint32,
	endpoint netip.AddrPort,
	localPubKey wgkey.DiscoPublicKey,
	sharedKey wgkey.DiscoSharedKey,
	sender Sender,
	logger *zap.Logger,
) DiscoPeerEndpoint {
	ep := &discoPeerEndpoint{
		recvChan:    make(chan DiscoPacket, 1),
		closed:      make(chan struct{}),
		ticker:      ticker.NewTicker(),
		endpointID:  endpointID,
		endpoint:    endpoint,
		localPubKey: localPubKey,
		sharedKey:   sharedKey,
		sender:      sender,
		priority:    ticker.Primary,

		status: &discoPeerEndpointStatus{
			cond: sync.NewCond(&sync.Mutex{}),
		},

		logger: logger.With(
			zap.String("service", "disco_peer_endpoint"),
			zap.String("endpoint", endpoint.String()),
		),
	}
	ep.packetManager = pktmgr.New(400*time.Millisecond, ep.dropCallback)
	go ep.run()

	return ep
}

func (pe *discoPeerEndpoint) dropCallback() {
	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	pe.ticker.SetState(ticker.Connecting, pe.priority, false)
	pe.status.setStatus(ticker.Connecting, 0)
}

func (pe *discoPeerEndpoint) sendDiscoPing() {
	pkt := DiscoPacket{
		Header:            PingMessage,
		SrcPublicDiscoKey: pe.localPubKey,
		EndpointID:        pe.endpointID,
		ID:                pe.packetID,

		Endpoint:  pe.endpoint,
		SharedKey: pe.sharedKey,
	}
	pe.packetID++

	encrypted, ok := pkt.Encrypt()

	if !ok {
		return
	}

	pe.packetManager.AddPacket(pkt.ID)
	pe.sender.Send(encrypted)
}

func (pe *discoPeerEndpoint) handlePong(pkt DiscoPacket) {
	rtt, ok := pe.packetManager.RecvAck(pkt.ID)

	if !ok {
		return
	}

	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	pe.ticker.SetState(ticker.Connected, pe.priority, false)
	pe.status.setStatus(ticker.Connected, rtt)
}

func (pe *discoPeerEndpoint) run() {
	for {
		select {
		case <-pe.ticker.C():
			pe.sendDiscoPing()
		case pkt := <-pe.recvChan:
			switch pkt.Header {
			case PingMessage:
				// do nothing
			case PongMessage:
				pe.handlePong(pkt)
			}
		case <-pe.closed:
			return
		}
	}
}

func (pe *discoPeerEndpoint) Endpoint() netip.AddrPort {
	return pe.endpoint
}

func (pe *discoPeerEndpoint) EnqueueReceivedPacket(pkt DiscoPacket) {
	pe.recvChan <- pkt
}

func (pe *discoPeerEndpoint) Close() error {
	close(pe.closed)
	pe.status.close()

	return nil
}

func (pe *discoPeerEndpoint) Status() DiscoPeerEndpointStatus {
	return pe.status
}

func (pe *discoPeerEndpoint) SetPriority(priority ticker.Priority) {
	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	state := pe.status.Get().State

	pe.priority = priority

	pe.ticker.SetState(state, priority, false)
}

func (pe *discoPeerEndpoint) ReceivePing() {
	if pe.status.Get().State != ticker.Connecting {
		return
	}

	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	now := time.Now()
	if now.Sub(pe.lastReinitialized) <= 30*time.Second {
		return
	}
	pe.lastReinitialized = time.Now()

	pe.ticker.SetState(ticker.Connecting, pe.priority, true)
}

type discoPeerEndpointStatus struct {
	cond *sync.Cond

	closed bool
	state  ticker.State
	rtt    time.Duration
}

type DiscoPeerEndpointStatusReadOnly struct {
	State ticker.State
	RTT   time.Duration
}

func (s *discoPeerEndpointStatus) NotifyStatus(fn func(status DiscoPeerEndpointStatusReadOnly)) {
	go s.notifyStatus(fn)
}

func (s *discoPeerEndpointStatus) notifyStatus(fn func(status DiscoPeerEndpointStatusReadOnly)) {
	s.cond.L.Lock()
	prev := s.readonly()
	s.cond.L.Unlock()
	fn(prev)
	for {
		s.cond.L.Lock()

		curr := s.readonly()

		if s.closed {
			s.cond.L.Unlock()
			return
		}
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

func (s *discoPeerEndpointStatus) Get() DiscoPeerEndpointStatusReadOnly {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	return s.readonly()
}

func (s *discoPeerEndpointStatus) setStatus(state ticker.State, rtt time.Duration) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.state = state
	s.rtt = rtt

	s.cond.Broadcast()
}

func (s *discoPeerEndpointStatus) close() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.closed = true

	s.cond.Broadcast()
}

func (s *discoPeerEndpointStatus) readonly() DiscoPeerEndpointStatusReadOnly {
	return DiscoPeerEndpointStatusReadOnly{
		State: s.state,
		RTT:   s.rtt,
	}
}

func (s *DiscoPeerEndpointStatusReadOnly) equalTo(target DiscoPeerEndpointStatusReadOnly) bool {
	return s.State == target.State && s.RTT == target.RTT
}
