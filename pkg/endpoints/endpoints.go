package endpoints

import (
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/miscord-dev/toxfu/pkg/splitconn"
	"github.com/miscord-dev/toxfu/pkg/stun"
	"github.com/miscord-dev/toxfu/pkg/types"
	"go.uber.org/zap"
)

type Collector struct {
	stunV4, stunV6 *stun.STUN
	logger         *zap.Logger
	listenPort     int

	addrV4, addrV6 atomic.Pointer[netip.AddrPort]
	callback       atomic.Pointer[func([]netip.AddrPort)]
}

func NewCollector(conn types.PacketConn, listenPort int, endpoint string, logger *zap.Logger) (res *Collector, err error) {
	bundler := splitconn.NewBundler(conn)
	v4Conn := bundler.Add(func(b []byte, addr netip.AddrPort) bool {
		return addr.Addr().Unmap().Is4()
	})
	v6Conn := bundler.Add(func(b []byte, addr netip.AddrPort) bool {
		return true
	})

	c := &Collector{
		logger:     logger,
		listenPort: listenPort,
	}

	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	stunV4, err := stun.NewSTUN(v4Conn, endpoint, false, logger.With(zap.String("service", "stun_v4")))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize STUN: %w", err)
	}
	stunV4.Notify(c.notifyV4)
	c.stunV4 = stunV4

	stunV6, err := stun.NewSTUN(v6Conn, endpoint, true, logger.With(zap.String("service", "stun_v6")))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize STUN for V6: %w", err)
	}
	stunV6.Notify(c.notifyV6)
	c.stunV6 = stunV6

	return c, nil
}

func (c *Collector) Close() error {
	if c.stunV4 != nil {
		c.stunV4.Close()
	}
	if c.stunV6 != nil {
		c.stunV6.Close()
	}

	return nil
}

func (c *Collector) notifyV4(v4 netip.AddrPort) {
	c.addrV4.Store(&v4)
	c.notify()
}

func (c *Collector) notifyV6(v6 netip.AddrPort) {
	c.addrV6.Store(&v6)
	c.notify()
}

func (c *Collector) notify() {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		c.logger.Error("failed to get interface addresses", zap.Error(err))
	}

	addrPorts := make([]netip.AddrPort, 0, len(addrs))
	for _, a := range addrs {
		ipNet := a.(*net.IPNet)
		addr, _ := netip.AddrFromSlice(ipNet.IP)

		addr = addr.Unmap()

		addrPorts = append(addrPorts, netip.AddrPortFrom(addr, uint16(c.listenPort)))
	}

	if addrPort := c.addrV4.Load(); addrPort != nil {
		addrPorts = append(addrPorts, *addrPort)
	}
	if addrPort := c.addrV6.Load(); addrPort != nil {
		addrPorts = append(addrPorts, *addrPort)
	}

	fn := c.callback.Load()

	if fn == nil || *fn == nil {
		return
	}

	(*fn)(addrPorts)
}

func (c *Collector) Notify(fn func([]netip.AddrPort)) {
	c.callback.Store(&fn)
}

func (c *Collector) Trigger() {
	c.stunV4.Trigger()
	c.stunV6.Trigger()
}
