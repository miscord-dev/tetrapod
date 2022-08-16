package hijack

import (
	"fmt"
	"io"
	"net"
	"net/netip"
)

type PacketConn interface {
	ReadFrom(p []byte) (n int, addr netip.AddrPort, err error)
	WriteTo(p []byte, addr netip.AddrPort) (n int, err error)
	io.Closer
}

type packetConn struct {
	conn net.PacketConn
}

var _ PacketConn = &packetConn{}

func (c *packetConn) ReadFrom(p []byte) (n int, addr netip.AddrPort, err error) {
	n, a, err := c.conn.ReadFrom(p)

	if err != nil {
		return 0, netip.AddrPort{}, err
	}

	switch addr := a.(type) {
	case *net.UDPAddr:
		return n, addr.AddrPort(), nil
	case *net.TCPAddr:
		return n, addr.AddrPort(), nil
	}

	return 0, netip.AddrPort{}, fmt.Errorf("unsupported address type: %v", addr)
}

func (c *packetConn) WriteTo(p []byte, addr netip.AddrPort) (n int, err error) {
	a := net.UDPAddrFromAddrPort(addr)

	return c.conn.WriteTo(p, a)
}

func (c *packetConn) Close() error {
	return c.conn.Close()
}

func PacketConnFrom(c net.PacketConn) PacketConn {
	return &packetConn{conn: c}
}
