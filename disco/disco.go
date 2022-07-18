package disco

import (
	"fmt"
	"log"
	"net"

	"github.com/miscord-dev/toxfu/pkg/syncmap"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
)

type Disco struct {
	privateKey wgkey.DiscoPrivateKey
	publicKey  wgkey.DiscoPublicKey

	closed   chan struct{}
	sendChan chan *EncryptedDiscoPacket
	connv4   *net.UDPConn
	connv6   *net.UDPConn
	peers    syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]
}

func New(privateKey wgkey.DiscoPrivateKey, port int) (*Disco, error) {
	connv4, err := net.ListenUDP("udp4", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen on :%d: %+v", port, err)
	}

	connv6, err := net.ListenUDP("udp6", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen on :%d for IPv6: %+v", port, err)
	}

	return &Disco{
		privateKey: privateKey,
		publicKey:  privateKey.Public(),
		closed:     make(chan struct{}),
		sendChan:   make(chan *EncryptedDiscoPacket),
		connv4:     connv4,
		connv6:     connv6,
		peers:      syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]{},
	}, nil
}

func (d *Disco) runSender() {
	for pkt := range d.sendChan {
		b, ok := pkt.Marshal()

		if !ok {
			continue
		}

		if pkt.Endpoint.Addr().Is4() {
			_, err := d.connv4.WriteToUDPAddrPort(b, pkt.Endpoint)

			if err != nil {
				log.Printf("sending msg to %v failed: %+v", pkt.Endpoint, err)
			}
		} else {
			_, err := d.connv6.WriteToUDPAddrPort(b, pkt.Endpoint)

			if err != nil {
				log.Printf("sending msg to %v in IPv6 failed: %+v", pkt.Endpoint, err)
			}
		}
	}
}

func (d *Disco) runReceiverV4() {
	buf := make([]byte, 2048)

	for {
		select {
		case <-d.closed:
			return
		default:
		}

		n, addr, err := d.connv4.ReadFromUDPAddrPort(buf)

		select {
		case <-d.closed:
			return
		default:
		}

		if err != nil {
			log.Printf("reading from UDP failed: %+v", err)
			continue
		}

		pkt := EncryptedDiscoPacket{
			Endpoint: addr,
		}

		ok := pkt.Unmarshal(buf[:n])
		if !ok {
			continue
		}

		peer, ok := d.peers.Load(pkt.SrcPublicDiscoKey)
		if !ok {
			continue
		}

		fmt.Print(peer, pkt)
		peer.enqueueReceivedPacket(pkt)
	}
}

func (d *Disco) runReceiverV6() {
	buf := make([]byte, 2048)

	for {
		select {
		case <-d.closed:
			return
		default:
		}

		n, addr, err := d.connv6.ReadFromUDPAddrPort(buf)

		select {
		case <-d.closed:
			return
		default:
		}

		if err != nil {
			log.Printf("reading from UDP failed: %+v", err)
			continue
		}

		pkt := EncryptedDiscoPacket{
			Endpoint: addr,
		}

		ok := pkt.Unmarshal(buf[:n])
		if !ok {
			continue
		}

		peer, ok := d.peers.Load(pkt.SrcPublicDiscoKey)
		if !ok {
			continue
		}

		peer.enqueueReceivedPacket(pkt)
	}
}

func (d *Disco) Start() {
	go d.runSender()
	go d.runReceiverV4()
	go d.runReceiverV6()
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
	d.connv4.Close()
	d.connv6.Close()

	return nil
}
