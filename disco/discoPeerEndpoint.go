package disco

import (
	"net/netip"
	"sync"
	"time"

	"github.com/miscord-dev/toxfu/pkg/pktmgr"
	"github.com/miscord-dev/toxfu/pkg/ticker"
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

	status *DiscoPeerEndpointStatus
}

func newDiscoPeerEndpoint(ds *DiscoPeer, endpointID uint32, endpoint netip.AddrPort) *DiscoPeerEndpoint {
	ep := &DiscoPeerEndpoint{
		recvChan:   make(chan DiscoPacket, 1),
		closed:     make(chan struct{}),
		ticker:     ticker.NewTicker(),
		peer:       ds,
		endpointID: endpointID,
		endpoint:   endpoint,

		status: &DiscoPeerEndpointStatus{
			cond: sync.NewCond(&sync.Mutex{}),
		},
	}
	ep.packetManager = pktmgr.New(2*time.Second, ep.dropCallback)
	ep.run()

	return ep
}

func (pe *DiscoPeerEndpoint) dropCallback() {
	// TODO: handle priority
	pe.ticker.SetState(ticker.Connecting, ticker.Primary)
	pe.status.setStatus(false, 0)
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

	pe.ticker.SetState(ticker.Connected, ticker.Primary)
	pe.status.setStatus(true, rtt)
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

type DiscoPeerEndpointStatus struct {
	cond *sync.Cond

	closed    bool
	connected bool
	rtt       time.Duration
}

type DiscoPeerEndpointStatusReadOnly struct {
	Connected bool
	RTT       time.Duration
}

func (s *DiscoPeerEndpointStatus) NotifyStatus(fn func(status DiscoPeerEndpointStatusReadOnly)) {
	s.cond.L.Lock()
	prev := s.readonly()
	fn(prev)
	s.cond.L.Unlock()
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
		s.cond.L.Unlock()

		if s.closed {
			return
		}
		if !curr.equalsTo(prev) {
			fn(curr)
			prev = curr
		}
	}
}

func (s *DiscoPeerEndpointStatus) setStatus(connected bool, rtt time.Duration) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	s.connected = connected
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
		Connected: s.connected,
		RTT:       s.rtt,
	}
}

func (s *DiscoPeerEndpointStatusReadOnly) equalsTo(target DiscoPeerEndpointStatusReadOnly) bool {
	return s.Connected == target.Connected && s.RTT == target.RTT
}
