package xdprecv

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/miscord-dev/toxfu/pkg/alarm"
	"github.com/miscord-dev/toxfu/pkg/bgsingleflight"
	"github.com/miscord-dev/toxfu/pkg/hijack/receiver/xdprecv/xdp"
	"github.com/miscord-dev/toxfu/pkg/sets"
	"github.com/miscord-dev/toxfu/pkg/sliceutil"
	"github.com/miscord-dev/toxfu/pkg/syncpool"
	"go.uber.org/zap"
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
	alarm  *alarm.Alarm

	lock  sync.RWMutex
	addrs sets.Set[netip.Addr]

	Logger *zap.Logger
}

func NewXDPController(port int) (*xdpController, error) {
	pool := syncpool.NewPool[message]()
	pool.New = func() message {
		return message{}
	}

	ctrl := &xdpController{
		port:   port,
		ch:     make(chan message, 10),
		pool:   pool,
		closed: make(chan struct{}),
		alarm:  alarm.New(),
		Logger: zap.NewNop(),
	}

	if err := ctrl.Refresh(); err != nil {
		return nil, fmt.Errorf("refresh failed: %w", err)
	}

	return ctrl, nil
}

func (c *xdpController) Refresh() error {
	c.alarm.WakeUpAll()

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
			logger := c.Logger.With(zap.String("interface", iface.Name))

			recver, err := xdp.NewXDPReceiver(&iface, c.port, checker)

			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}

			if err != nil {
				logger.Error("failed to set up xdp receiver", zap.Error(err))

				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				s := c.alarm.Subscribe()
				defer s.Close()

				for {
					select {
					case <-c.closed:
					case <-ctx.Done():
					case <-s.C():
						err := recver.RunHealthCheck()

						if err == nil {
							continue
						}
						logger.Debug("closing receiver", zap.Error(err))
					}

					recver.Close()

					return
				}
			}()

			buf := make([]byte, 2048)

			for {
				len, addrPort, err := recver.Recv(buf)

				if err != nil {
					logger.Error("failed to read packet", zap.Error(err))

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

func (c *xdpController) Recv(b []byte) (len int, src netip.AddrPort, err error) {
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

func (c *xdpController) Close() error {
	defer func() {
		recover()
	}()

	close(c.closed)

	return nil
}
