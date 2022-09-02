package xdprecv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/miscord-dev/toxfu/pkg/syncpool"
	"github.com/vishvananda/netlink"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS -strip $BPF_STRIP -type event bpf xdp.c -- -I/usr/include/x86_64-linux-gnu -Ilinux-5.18.14/tools/lib -Ixdp-tools/headers

type XDPReceiver struct {
	iface         *net.Interface
	objs          *bpfObjects
	link          link.Link
	xdpFd         int
	ringReader    *ringbuf.Reader
	addressFilter AddressFilter

	ch   chan message
	pool *syncpool.Pool[message]

	closed chan struct{}
}

type message struct {
	src     netip.AddrPort
	payload [2048]byte
	len     int
}

type AddressFilter func(netip.Addr) bool

func NewXDPReceiver(iface *net.Interface, port int, addressFilter AddressFilter) (xdpReceiver *XDPReceiver, err error) {
	spec, err := loadBpf()
	if err != nil {
		return nil, fmt.Errorf("load bpf failed: %w", err)
	}

	if err := spec.RewriteConstants(map[string]interface{}{
		"port": uint16(port),
	}); err != nil {
		return nil, fmt.Errorf("rewriting port failed: %w", err)
	}

	xdpReceiver = &XDPReceiver{
		iface:         iface,
		ch:            make(chan message, 10),
		pool:          syncpool.NewPool[message](),
		closed:        make(chan struct{}),
		addressFilter: addressFilter,
	}
	xdpReceiver.pool.New = func() message {
		return message{}
	}

	xdpReceiverBackup := xdpReceiver
	defer func() {
		if err != nil {
			xdpReceiverBackup.Close()
			xdpReceiver = nil
		}
	}()

	objs := bpfObjects{}
	opts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogLevel: 1,
		},
	}
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("loading objects: %w", err)
	}
	xdpReceiver.objs = &objs

	l, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpParserFunc,
		Interface: iface.Index,
		Flags:     link.XDPGenericMode,
	})
	if err != nil {
		return nil, fmt.Errorf("could not attach XDP program: %w", err)
	}
	xdpReceiver.xdpFd = l.(*link.RawLink).FD()
	xdpReceiver.link = l

	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		return nil, fmt.Errorf("opening ringbuf reader: %w", err)
	}
	xdpReceiver.ringReader = rd

	go xdpReceiver.run()

	return xdpReceiver, nil
}

func (r *XDPReceiver) run() {
	var event bpfEvent

	var eth layers.Ethernet
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var udp layers.UDP
	var payload gopacket.Payload
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &udp, &payload)

	for {
		record, err := r.ringReader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			continue
		}

		// Parse the ringbuf event entry into a bpfEvent structure.
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			continue
		}

		if event.Len <= 2048 {
			decoded := []gopacket.LayerType{}
			err := parser.DecodeLayers(event.Packet[:event.Len], &decoded)

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

			if !r.addressFilter(dstIP) {
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
}

func (r *XDPReceiver) Recv(b []byte) (int, netip.AddrPort, error) {
	var msg message
	select {
	case <-r.closed:
		return 0, netip.AddrPort{}, fmt.Errorf("closed")
	case msg = <-r.ch:
	}

	defer r.pool.Put(msg)

	copy(b, msg.payload[:msg.len])

	return msg.len, msg.src, nil
}

func (r *XDPReceiver) RunHealthCheck() error {
	link, err := netlink.LinkByName(r.iface.Name)
	if err != nil {
		return fmt.Errorf("failed to find link by name %s: %w", r.iface.Name, err)
	}

	xdp := link.Attrs().Xdp

	if xdp == nil {
		return fmt.Errorf("xdp is nil")
	}

	if xdp.Fd != r.xdpFd {
		return fmt.Errorf("wrong xdp fd(actual: %v, expected: %v)", xdp.Fd, r.xdpFd)
	}

	return nil
}

func (r *XDPReceiver) Close() error {
	if r.objs != nil {
		r.objs.Close()
	}
	if r.link != nil {
		r.link.Close()
	}
	if r.ringReader != nil {
		r.ringReader.Close()
	}
	defer recover()
	close(r.closed)

	return nil
}
