package disco

import (
	"net/netip"
	"sync"
	"time"

	"github.com/miscord-dev/toxfu/disco/pktmgr"
	"github.com/miscord-dev/toxfu/disco/ticker"
)

type DiscoPeerEndpoint struct {
	recvChan      chan DiscoPacket
	closed        chan struct{}
	packetManager *pktmgr.Manager
	ticker        ticker.Ticker

	peer       *DiscoPeer
	endpointID uint32
	endpoint   netip.AddrPort

	packetID uint32

	statusLock        sync.Mutex
	priority          ticker.Priority
	status            *DiscoPeerEndpointStatus
	lastReinitialized time.Time
}

func newDiscoPeerEndpoint(ds *DiscoPeer, endpointID uint32, endpoint netip.AddrPort) *DiscoPeerEndpoint {
	ep := &DiscoPeerEndpoint{
		recvChan:   make(chan DiscoPacket, 1),
		closed:     make(chan struct{}),
		ticker:     ticker.NewTicker(),
		peer:       ds,
		endpointID: endpointID,
		endpoint:   endpoint,
		priority:   ticker.Primary,

		status: &DiscoPeerEndpointStatus{
			cond: sync.NewCond(&sync.Mutex{}),
		},
	}
	ep.packetManager = pktmgr.New(2*time.Second, ep.dropCallback)
	ep.run()

	return ep
}

func (pe *DiscoPeerEndpoint) dropCallback() {
	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	pe.ticker.SetState(ticker.Connecting, pe.priority, false)
	pe.status.setStatus(ticker.Connecting, 0)
}

func (pe *DiscoPeerEndpoint) sendDiscoPing() {
	pkt := DiscoPacket{
		Header:            PingMessage,
		SrcPublicDiscoKey: pe.peer.disco.publicKey,
		EndpointID:        pe.endpointID,
		ID:                pe.packetID,

		Endpoint:  pe.endpoint,
		SharedKey: pe.peer.sharedKey,
	}
	pe.packetID++

	encrypted, ok := pkt.Encrypt()

	if !ok {
		return
	}

	pe.peer.disco.sendChan <- encrypted
	pe.packetManager.AddPacket(pkt.ID)
}

func (pe *DiscoPeerEndpoint) handlePong(pkt DiscoPacket) {
	rtt, ok := pe.packetManager.RecvAck(pkt.ID)

	if !ok {
		return
	}

	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	pe.ticker.SetState(ticker.Connected, pe.priority, false)
	pe.status.setStatus(ticker.Connected, rtt)
}

func (pe *DiscoPeerEndpoint) run() {
	go func() {
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
			case <-pe.peer.closed:
				pe.Close()
				return
			case <-pe.peer.disco.closed:
				pe.Close()
				return
			}
		}
	}()
}

func (pe *DiscoPeerEndpoint) enqueueReceivedPacket(pkt DiscoPacket) {
	pe.recvChan <- pkt
}

func (pe *DiscoPeerEndpoint) Close() error {
	close(pe.closed)
	pe.Status().close()

	return nil
}

func (pe *DiscoPeerEndpoint) Status() *DiscoPeerEndpointStatus {
	return pe.status
}

func (pe *DiscoPeerEndpoint) SetPriority(priority ticker.Priority) {
	pe.statusLock.Lock()
	defer pe.statusLock.Unlock()

	state := pe.status.Get().State

	pe.priority = priority

	pe.ticker.SetState(state, priority, false)
}

func (pe *DiscoPeerEndpoint) ReceivePing() {
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

type DiscoPeerEndpointStatus struct {
	cond *sync.Cond

	closed bool
	state  ticker.State
	rtt    time.Duration
}

type DiscoPeerEndpointStatusReadOnly struct {
	State ticker.State
	RTT   time.Duration
}

func (s *DiscoPeerEndpointStatus) NotifyStatus(fn func(status DiscoPeerEndpointStatusReadOnly)) {
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

func (s *DiscoPeerEndpointStatus) Get() DiscoPeerEndpointStatusReadOnly {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	return s.readonly()
}

func (s *DiscoPeerEndpointStatus) setStatus(state ticker.State, rtt time.Duration) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.state = state
	s.rtt = rtt

	s.cond.Broadcast()
}

func (s *DiscoPeerEndpointStatus) close() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.closed = true

	s.cond.Broadcast()
}

func (s *DiscoPeerEndpointStatus) readonly() DiscoPeerEndpointStatusReadOnly {
	return DiscoPeerEndpointStatusReadOnly{
		State: s.state,
		RTT:   s.rtt,
	}
}

func (s *DiscoPeerEndpointStatusReadOnly) equalsTo(target DiscoPeerEndpointStatusReadOnly) bool {
	return s.State == target.State && s.RTT == target.RTT
}
