package hijack

import (
	"fmt"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/hijack/rawsocksend"
)

type Conn struct {
	xdpController *xdpController
	sender        *rawsocksend.Sender
}

var _ PacketConn = &Conn{}

func NewConn(port int) (res *Conn, err error) {
	conn := &Conn{}

	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	ctrl, err := newXDPController(port)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize xdp controller: %w", err)
	}
	conn.xdpController = ctrl

	sender, err := rawsocksend.NewSender(port)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize raw socket sender: %w", err)
	}
	conn.sender = sender

	return conn, nil
}

func (c *Conn) ReadFrom(p []byte) (n int, addr netip.AddrPort, err error) {
	len, addr, err := c.xdpController.recv(p)

	if err != nil {
		return 0, netip.AddrPort{}, fmt.Errorf("recv failed: %w", err)
	}

	return len, addr, nil
}

func (c *Conn) WriteTo(p []byte, addr netip.AddrPort) (n int, err error) {
	return len(p), c.sender.Send(p, addr)
}

func (c *Conn) Close() error {
	if c.xdpController != nil {
		c.xdpController.close()
	}
	if c.sender != nil {
		c.sender.Close()
	}

	return nil
}
