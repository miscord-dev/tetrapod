package rawsockrecv

import (
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/miscord-dev/toxfu/pkg/hijack/receiver"
	"github.com/miscord-dev/toxfu/pkg/sets"
	"github.com/miscord-dev/toxfu/pkg/sliceutil"
	"github.com/miscord-dev/toxfu/pkg/syncpool"
	"golang.org/x/net/bpf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS -strip $BPF_STRIP -type event bpf bpf.c -- -I/usr/include/x86_64-linux-gnu -Ilinux-5.18.14/tools/lib -Ixdp-tools/headers

func New(port int) (receiver.Receiver, error) {
	tPacket, err := afpacket.NewTPacket()

	if err != nil {
		return nil, fmt.Errorf("failed to init AF_PACKET: %w", err)
	}

	pool := syncpool.NewPool[message]()
	pool.New = func() message {
		return message{}
	}

	rawInstructions, err := bpf.Assemble(generateInstructions(port))

	if err != nil {
		return nil, fmt.Errorf("failed to assemble instructions: %w", err)
	}

	if err := tPacket.SetBPF(rawInstructions); err != nil {
		return nil, fmt.Errorf("setting bpf failed: %w", err)
	}

	rr := &rawsockReceiver{
		tPacket: tPacket,
		ch:      make(chan message, 10),
		pool:    pool,
		closed:  make(chan struct{}),
	}

	if err := rr.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to refresh addresses: %w", err)
	}
	go rr.run()

	return rr, nil
}

type message struct {
	src     netip.AddrPort
	payload [2048]byte
	len     int
}

type rawsockReceiver struct {
	tPacket *afpacket.TPacket

	ch   chan message
	pool *syncpool.Pool[message]

	lock  sync.RWMutex
	addrs sets.Set[netip.Addr]

	closed chan struct{}
}

func (r *rawsockReceiver) check(addr netip.Addr) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.addrs.Contains(addr)
}

func (r *rawsockReceiver) run() {
	var eth layers.Ethernet
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var udp layers.UDP
	var payload gopacket.Payload
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &udp, &payload)

	buf := make([]byte, 2048)
	for {
		ci, err := r.tPacket.ReadPacketDataTo(buf)

		if err != nil {
			return
		}

		if ci.CaptureLength > 2048 {
			continue
		}
		decoded := []gopacket.LayerType{}
		err = parser.DecodeLayers(buf[:ci.CaptureLength], &decoded)

		if err != nil {
			continue
		}

		isIPv4 := false
		isIPv6 := false
		for _, typ := range decoded {
			switch typ {
			case layers.LayerTypeIPv4:
				isIPv4 = true
			case layers.LayerTypeIPv6:
				isIPv6 = true
			}
		}

		if !isIPv4 && !isIPv6 {
			continue
		}

		var srcIP netip.Addr
		var dstIP netip.Addr
		if isIPv4 {
			srcIP, _ = netip.AddrFromSlice(ip4.SrcIP)
			dstIP, _ = netip.AddrFromSlice(ip4.DstIP)
		} else {
			srcIP, _ = netip.AddrFromSlice(ip6.SrcIP)
			dstIP, _ = netip.AddrFromSlice(ip6.DstIP)
		}

		if !r.check(dstIP) {
			continue
		}

		msg := r.pool.Get()

		msg.src = netip.AddrPortFrom(srcIP, uint16(udp.SrcPort))
		copy(msg.payload[:], payload.Payload())
		msg.len = len(payload.Payload())

		select {
		case r.ch <- msg:
		default:
		}
	}
}

func (r *rawsockReceiver) Recv(b []byte) (len int, src netip.AddrPort, err error) {
	var msg message
	select {
	case <-r.closed:
		return 0, netip.AddrPort{}, net.ErrClosed
	case msg = <-r.ch:
	}

	defer r.pool.Put(msg)

	copy(b, msg.payload[:msg.len])

	return msg.len, msg.src, nil
}

func (r *rawsockReceiver) Refresh() error {
	netAddrs, err := net.InterfaceAddrs()

	if err != nil {
		return fmt.Errorf("failed to get interface addrs: %w", err)
	}

	addrs := sets.FromSlice(sliceutil.Map(netAddrs, func(v net.Addr) netip.Addr {
		ipNet := v.(*net.IPNet)
		c, _ := netip.AddrFromSlice(ipNet.IP)

		return c.Unmap()
	}))

	r.lock.Lock()
	r.addrs = addrs
	r.lock.Unlock()

	return nil
}

func (r *rawsockReceiver) Close() error {
	defer func() {
		recover()
	}()

	close(r.closed)

	r.tPacket.Close()

	return nil
}
