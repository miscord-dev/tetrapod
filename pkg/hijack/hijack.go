package hijack

import (
	"fmt"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/hijack/rawsocksend"
	"github.com/miscord-dev/toxfu/pkg/types"
	"go.uber.org/zap"
)

type Conn struct {
	xdpController *xdpController
	sender        *rawsocksend.Sender
	Logger        *zap.Logger
}

var _ types.PacketConn = &Conn{}

func NewConn(port int) (res *Conn, err error) {
	return NewConnWithLogger(port, zap.NewNop())
}

func NewConnWithLogger(port int, logger *zap.Logger) (res *Conn, err error) {
	conn := &Conn{
		Logger: logger,
	}

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
	ctrl.Logger = conn.Logger.With(zap.String("component", "xdp_controller"))

	sender, err := rawsocksend.NewSender(port)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize raw socket sender: %w", err)
	}
	conn.sender = sender

	return conn, nil
}

func (c *Conn) Refresh() {
	if err := c.xdpController.refresh(); err != nil {
		c.Logger.Error("failed to refresh xdp controller", zap.Error(err))
	}

	if err := c.sender.RefreshRouter(); err != nil {
		c.Logger.Error("failed to refresh sender", zap.Error(err))
	}
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
