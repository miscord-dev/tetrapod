package rawsocksend

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/routing"
	"github.com/miscord-dev/tetrapod/pkg/hijack/sender"
	"go.uber.org/zap"
)

type Sender struct {
	fdv4, fdv6 int
	srcPort    int

	routerLock sync.RWMutex
	router     routing.Router

	logger *zap.Logger
}

var _ sender.Sender = &Sender{}

func NewSender(port int) (res *Sender, err error) {
	sender := &Sender{
		srcPort: port,
	}

	defer func() {
		if err != nil {
			sender.Close()
			res = nil
		}
	}()

	fdv4, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)

	if err != nil {
		return nil, fmt.Errorf("failed to open raw socket for v4: %w", err)
	}
	sender.fdv4 = fdv4

	fdv6, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_RAW, syscall.IPPROTO_RAW)

	if err != nil {
		return nil, fmt.Errorf("failed to open raw socket for v6: %w", err)
	}
	sender.fdv6 = fdv6

	if err := sender.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to refresh router: %w", err)
	}

	return sender, nil
}

func (s *Sender) Close() error {
	if s.fdv4 != 0 {
		syscall.Close(s.fdv4)
	}
	if s.fdv6 != 0 {
		syscall.Close(s.fdv6)
	}

	return nil
}

func (s *Sender) Refresh() error {
	var router routing.Router
	var err error

	for i := 0; i < 5; i++ {
		router, err = routing.New()

		if err != nil {
			if s.logger != nil {
				s.logger.Error("failed to refresh router", zap.Error(err))
			}

			time.Sleep(100 * time.Millisecond)
			continue
		}
	}

	if err != nil {
		return fmt.Errorf("failed to refresh router: %w", err)
	}

	s.routerLock.Lock()
	defer s.routerLock.Unlock()

	s.router = router

	return nil
}

func (s *Sender) Send(payload []byte, dst netip.AddrPort) error {
	dst = netip.AddrPortFrom(dst.Addr().Unmap(), dst.Port())

	if dst.Addr().Is4() {
		return s.sendIPv4(dst, payload)
	}

	return s.sendIPv6(dst, payload)
}

func (s *Sender) sendIPv4(dst netip.AddrPort, payload []byte) error {
	s.routerLock.RLock()
	router := s.router
	s.routerLock.RUnlock()

	_, _, src, err := router.Route(net.IP(dst.Addr().AsSlice()))

	if err != nil {
		return fmt.Errorf("failed to get routing for %v: %w", dst, err)
	}

	ip := &layers.IPv4{
		Version:  4,
		Flags:    layers.IPv4DontFragment,
		SrcIP:    src,
		DstIP:    net.IP(dst.Addr().AsSlice()),
		Protocol: layers.IPProtocolUDP,
		TTL:      128,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(s.srcPort),
		DstPort: layers.UDPPort(dst.Port()),
	}
	udp.SetNetworkLayerForChecksum(ip)
	pl := gopacket.Payload(payload)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(
		buf, opts,
		ip, udp, pl,
	)

	if err != nil {
		return fmt.Errorf("failed to serialize packet: %w", err)
	}

	addr := syscall.SockaddrInet4{
		Addr: dst.Addr().As4(),
		Port: int(dst.Port()),
	}

	err = syscall.Sendto(s.fdv4, buf.Bytes(), 0, &addr)

	if err != nil {
		return fmt.Errorf("sendto() to %v failed: %w", dst, err)
	}

	return nil
}

func (s *Sender) sendIPv6(dst netip.AddrPort, payload []byte) error {
	s.routerLock.RLock()
	router := s.router
	s.routerLock.RUnlock()

	_, _, src, err := router.Route(net.IP(dst.Addr().AsSlice()))

	if err != nil {
		return fmt.Errorf("failed to get routing for %v: %w", dst, err)
	}

	ip := &layers.IPv6{
		Version:    6,
		SrcIP:      src,
		DstIP:      net.IP(dst.Addr().AsSlice()),
		NextHeader: layers.IPProtocolUDP,
		HopLimit:   128,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(s.srcPort),
		DstPort: layers.UDPPort(dst.Port()),
	}
	udp.SetNetworkLayerForChecksum(ip)
	pl := gopacket.Payload(payload)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(
		buf, opts,
		ip, udp, pl,
	)

	if err != nil {
		return fmt.Errorf("failed to serialize packet: %w", err)
	}

	addr := syscall.SockaddrInet6{
		Addr: dst.Addr().As16(),
		Port: 0,
	}

	err = syscall.Sendto(s.fdv6, buf.Bytes(), 0, &addr)

	if err != nil {
		return fmt.Errorf("sendto() to %v failed: %w", dst, err)
	}

	return nil
}
