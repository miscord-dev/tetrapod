package hijack

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/miscord-dev/toxfu/pkg/bgsingleflight"
	"github.com/miscord-dev/toxfu/pkg/hijack/xdprecv"
	"github.com/miscord-dev/toxfu/pkg/sets"
	"github.com/miscord-dev/toxfu/pkg/sliceutil"
	"github.com/miscord-dev/toxfu/pkg/syncpool"
)

type message struct {
	src     netip.AddrPort
	payload [2048]byte
	len     int
}

type xdpController struct {
	port  int
	group bgsingleflight.Group

	ch     chan message
	pool   *syncpool.Pool[message]
	closed chan struct{}

	lock  sync.RWMutex
	addrs sets.Set[netip.Addr]
}

func newXDPController(port int) (*xdpController, error) {
	pool := syncpool.NewPool[message]()
	pool.New = func() message {
		return message{}
	}

	ctrl := &xdpController{
		port:   port,
		ch:     make(chan message, 10),
		pool:   pool,
		closed: make(chan struct{}),
	}

	if err := ctrl.refresh(); err != nil {
		return nil, fmt.Errorf("refresh failed: %w", err)
	}

	return ctrl, nil
}

func (c *xdpController) refresh() error {
	netAddrs, err := net.InterfaceAddrs()

	if err != nil {
		return fmt.Errorf("failed to get interface addrs: %w", err)
	}

	addrs := sets.FromSlice(sliceutil.Map(netAddrs, func(v net.Addr) netip.Addr {
		ipNet := v.(*net.IPNet)
		c, _ := netip.AddrFromSlice(ipNet.IP)

		return c.Unmap()
	}))

	c.lock.Lock()
	c.addrs = addrs
	c.lock.Unlock()

	checker := func(addr netip.Addr) bool {
		c.lock.RLock()
		defer c.lock.RUnlock()
		return c.addrs.Contains(addr)
	}

	ifaces, err := net.Interfaces()

	if err != nil {
		return fmt.Errorf("failed to get interfaces: %w", err)
	}

	for _, iface := range ifaces {
		iface := iface
		c.group.Run(iface.Name, func() {
			recver, err := xdprecv.NewXDPReceiver(&iface, c.port, checker)

			if err != nil {
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				select {
				case <-c.closed:
				case <-ctx.Done():
				}

				recver.Close()
			}()

			buf := make([]byte, 2048)

			for {
				len, addrPort, err := recver.Recv(buf)

				if err != nil {
					return
				}

				msg := c.pool.Get()
				copy(msg.payload[:], buf[:len])
				msg.src = addrPort
				msg.len = len

				select {
				case c.ch <- msg:
				default:
				}
			}
		})
	}

	return nil
}

func (c *xdpController) recv(b []byte) (len int, src netip.AddrPort, err error) {
	var msg message
	select {
	case <-c.closed:
		return 0, netip.AddrPort{}, net.ErrClosed
	case msg = <-c.ch:
	}

	defer c.pool.Put(msg)

	copy(b, msg.payload[:msg.len])

	return msg.len, msg.src, nil
}

func (c *xdpController) close() {
	defer recover()
	close(c.closed)
}
