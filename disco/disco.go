package disco

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/hijack"
	"github.com/miscord-dev/toxfu/pkg/syncmap"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
)

type Disco struct {
	privateKey wgkey.DiscoPrivateKey
	publicKey  wgkey.DiscoPublicKey

	closed   chan struct{}
	sendChan chan *EncryptedDiscoPacket
	conn     hijack.PacketConn
	peers    syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]
}

func New(privateKey wgkey.DiscoPrivateKey, port int) (*Disco, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen on :%d: %+v", port, err)
	}

	return &Disco{
		privateKey: privateKey,
		publicKey:  privateKey.Public(),
		closed:     make(chan struct{}),
		sendChan:   make(chan *EncryptedDiscoPacket),
		conn:       hijack.PacketConnFrom(conn),
		peers:      syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]{},
	}, nil
}

func NewFromPacketConn(privateKey wgkey.DiscoPrivateKey, packetConn hijack.PacketConn) (*Disco, error) {
	return &Disco{
		privateKey: privateKey,
		publicKey:  privateKey.Public(),
		closed:     make(chan struct{}),
		sendChan:   make(chan *EncryptedDiscoPacket),
		conn:       packetConn,
		peers:      syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]{},
	}, nil
}

func (d *Disco) runSender() {
	for pkt := range d.sendChan {
		b, ok := pkt.Marshal()

		if !ok {
			continue
		}

		_, err := d.conn.WriteTo(b, pkt.Endpoint)

		if err != nil {
			log.Printf("sending msg to %v failed: %+v", pkt.Endpoint, err)
		}
	}
}

func (d *Disco) runReceiver() {
	buf := make([]byte, 2048)

	for {
		select {
		case <-d.closed:
			return
		default:
		}

		n, addr, err := d.conn.ReadFrom(buf)

		select {
		case <-d.closed:
			return
		default:
		}
		if err != nil {
			log.Printf("reading from UDP failed: %+v", err)
			continue
		}
		addr = netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())

		pkt := EncryptedDiscoPacket{
			Endpoint: addr,
		}

		ok := pkt.Unmarshal(buf[:n])
		if !ok {
			log.Println("unmarshal failed")
			continue
		}

		peer, ok := d.peers.Load(pkt.SrcPublicDiscoKey)
		if !ok {
			log.Println("finding peer failed", base64.StdEncoding.EncodeToString(pkt.SrcPublicDiscoKey[:]))
			continue
		}

		peer.enqueueReceivedPacket(pkt)
	}
}

func (d *Disco) Start() {
	go d.runSender()
	go d.runReceiver()
}

func (d *Disco) AddPeer(pubKey wgkey.DiscoPublicKey) *DiscoPeer {
	if _, ok := d.peers.Load(pubKey); ok {
		return nil
	}

	peer := newDiscoPeer(d, pubKey)

	actual, loaded := d.peers.LoadOrStore(pubKey, peer)

	if loaded {
		peer.Close()
	}

	return actual
}

func (d *Disco) Close() error {
	close(d.closed)
	d.conn.Close()

	return nil
}
