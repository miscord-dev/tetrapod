package disco

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/syncmap"
	"github.com/miscord-dev/toxfu/pkg/types"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap"
)

type Disco struct {
	privateKey wgkey.DiscoPrivateKey
	publicKey  wgkey.DiscoPublicKey

	closed   chan struct{}
	sendChan chan *EncryptedDiscoPacket
	conn     types.PacketConn
	peers    syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]

	statusCallback func(pubKey wgkey.DiscoPublicKey, status DiscoPeerStatusReadOnly)

	logger *zap.Logger
}

func New(privateKey wgkey.DiscoPrivateKey, port int, logger *zap.Logger) (*Disco, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen on :%d: %+v", port, err)
	}

	return NewFromPacketConn(privateKey, types.PacketConnFrom(conn), logger), nil
}

func NewFromPacketConn(privateKey wgkey.DiscoPrivateKey, packetConn types.PacketConn, logger *zap.Logger) *Disco {
	d := &Disco{
		privateKey: privateKey,
		publicKey:  privateKey.Public(),
		closed:     make(chan struct{}),
		sendChan:   make(chan *EncryptedDiscoPacket),
		conn:       packetConn,
		peers:      syncmap.Map[wgkey.DiscoPublicKey, *DiscoPeer]{},
		logger:     logger.With(zap.String("service", "disco")),
	}

	go d.runSender()
	go d.runReceiver()

	return d
}

func (d *Disco) runSender() {
	for pkt := range d.sendChan {
		b, ok := pkt.Marshal()

		if !ok {
			continue
		}

		_, err := d.conn.WriteTo(b, pkt.Endpoint)

		if err != nil {
			d.logger.Info("sending msg failed", zap.String("endpoint", pkt.Endpoint.String()), zap.Error(err))
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
			d.logger.Info("unmarshal failed", zap.String("key", base64.StdEncoding.EncodeToString(pkt.SrcPublicDiscoKey[:])))
			continue
		}

		peer, ok := d.peers.Load(pkt.SrcPublicDiscoKey)
		if !ok {
			d.logger.Info("finding peer failed", zap.String("key", base64.StdEncoding.EncodeToString(pkt.SrcPublicDiscoKey[:])))
			continue
		}

		peer.enqueueReceivedPacket(pkt)
	}
}

func (d *Disco) AddPeer(pubKey wgkey.DiscoPublicKey) *DiscoPeer {
	if peer, ok := d.peers.Load(pubKey); ok {
		return peer
	}

	peer := newDiscoPeer(d, pubKey, d.logger)

	actual, loaded := d.peers.LoadOrStore(pubKey, peer)

	if loaded {
		peer.Close()
	} else {
		peer.Status().NotifyStatus(func(status DiscoPeerStatusReadOnly) {
			d.statusCallback(pubKey, status)
		})
	}

	return actual
}

func (d *Disco) SetPeers(peers map[wgkey.DiscoPublicKey][]netip.AddrPort) {
	for k, v := range peers {
		peer := d.AddPeer(k)

		peer.SetEndpoints(v)
	}

	d.peers.Range(func(key wgkey.DiscoPublicKey, value *DiscoPeer) bool {
		_, ok := peers[key]

		if !ok {
			value.Close()
		}

		return true
	})
}

func (d *Disco) GetAllStatuses() (res map[wgkey.DiscoPublicKey]DiscoPeerStatusReadOnly) {
	res = make(map[wgkey.DiscoPublicKey]DiscoPeerStatusReadOnly)
	d.peers.Range(func(key wgkey.DiscoPublicKey, value *DiscoPeer) bool {
		res[key] = value.status.readonly()

		return true
	})

	return res
}

func (d *Disco) Close() error {
	close(d.closed)
	d.conn.Close()

	return nil
}

func (d *Disco) SetStatusCallback(fn func(pubKey wgkey.DiscoPublicKey, status DiscoPeerStatusReadOnly)) {
	d.statusCallback = fn
}
