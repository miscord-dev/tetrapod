package splitconn

import (
	"net/netip"
	"sync"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/syncpool"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/types"
	"golang.org/x/sync/singleflight"
)

type Bundler struct {
	conn     types.PacketConn
	group    singleflight.Group
	children []*Conn
	pool     *syncpool.Pool[*packet]

	writeLock sync.Mutex
}

func NewBundler(conn types.PacketConn) *Bundler {
	pool := syncpool.NewPool[*packet]()
	pool.New = func() *packet {
		return &packet{}
	}

	return &Bundler{
		conn: conn,
		pool: pool,
	}
}

func (b *Bundler) Add(filter func(b []byte, addr netip.AddrPort) bool) *Conn {
	child := &Conn{
		b: b,
		filter: func(p *packet) bool {
			return filter(p.b[:p.len], p.addr)
		},
		queue: make(chan *packet, 5),
	}

	b.children = append(b.children, child)

	return child
}

type packet struct {
	b    [4096]byte
	addr netip.AddrPort
	len  int
}

type Conn struct {
	b *Bundler

	filter func(p *packet) bool
	queue  chan *packet
}

func (c *Conn) ReadFrom(p []byte) (n int, addr netip.AddrPort, err error) {
	for {
		select {
		case pkt := <-c.queue:
			defer c.b.pool.Put(pkt)

			return copy(p, pkt.b[:pkt.len]), pkt.addr, nil
		default:
		}

		_, err, _ := c.b.group.Do("", func() (interface{}, error) {
			var err error
			pkt := c.b.pool.Get()

			pkt.len, pkt.addr, err = c.b.conn.ReadFrom(pkt.b[:])

			if err != nil {
				c.b.pool.Put(pkt)
				return nil, err
			}

			for _, child := range c.b.children {
				if child.filter(pkt) {
					select {
					case child.queue <- pkt:
					default:
						c.b.pool.Put(pkt)
					}
					break
				}
			}

			return nil, nil
		})

		if err != nil {
			return 0, netip.AddrPort{}, err
		}
	}
}

func (c *Conn) WriteTo(p []byte, addr netip.AddrPort) (n int, err error) {
	c.b.writeLock.Lock()
	defer c.b.writeLock.Unlock()

	return c.b.conn.WriteTo(p, addr)
}

func (c *Conn) Close() error {
	return c.b.conn.Close()
}
