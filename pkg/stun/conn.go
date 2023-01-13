package stun

import (
	"net/netip"

	"github.com/miscord-dev/tetrapod/pkg/types"
	"github.com/pion/stun"
)

type conn struct {
	packetConn types.PacketConn
	addrPort   netip.AddrPort
}

var _ stun.Connection = &conn{}

func (c *conn) Read(p []byte) (n int, err error) {
	for {
		n, addr, err := c.packetConn.ReadFrom(p)

		if err != nil {
			return 0, err
		}

		if addr != c.addrPort {
			continue
		}

		return n, nil
	}
}

func (c *conn) Write(p []byte) (n int, err error) {
	return c.packetConn.WriteTo(p, c.addrPort)
}

func (c *conn) Close() error {
	return c.packetConn.Close()
}
