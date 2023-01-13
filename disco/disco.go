package disco

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/syncmap"
	"github.com/miscord-dev/toxfu/pkg/types"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap"
)

//go:generate mockgen -source=$GOFILE -package=mock_$GOPACKAGE -destination=./mock/mock_$GOFILE

type Disco interface {
	AddPeer(pubKey wgkey.DiscoPublicKey) DiscoPeer
	SetPeers(peers map[wgkey.DiscoPublicKey][]netip.AddrPort)
	GetAllStatuses() (res map[wgkey.DiscoPublicKey]DiscoPeerStatusReadOnly)
	Send(pkt *EncryptedDiscoPacket)
	SetStatusCallback(fn func(pubKey wgkey.DiscoPublicKey, status DiscoPeerStatusReadOnly))
	Close() error
}

type disco struct {
	privateKey  wgkey.DiscoPrivateKey
	publicKey   wgkey.DiscoPublicKey
	newPeer     NewDiscoPeerFunc
	newEndpoint NewDiscoPeerEndpointFunc

	closed   chan struct{}
	sendChan chan *EncryptedDiscoPacket
	conn     types.PacketConn
	peers    syncmap.Map[wgkey.DiscoPublicKey, DiscoPeer]

	statusCallback func(pubKey wgkey.DiscoPublicKey, status DiscoPeerStatusReadOnly)

	logger *zap.Logger
}

type Sender interface {
	Send(pkt *EncryptedDiscoPacket)
}

var _ Disco = &disco{}
var _ Sender = &disco{}

func NewListen(privateKey wgkey.DiscoPrivateKey, port int, logger *zap.Logger) (Disco, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to listen on :%d: %+v", port, err)
	}

	return NewFromPacketConn(privateKey, types.PacketConnFrom(conn), logger), nil
}

func NewFromPacketConn(
	privateKey wgkey.DiscoPrivateKey,
	packetConn types.PacketConn,
	logger *zap.Logger,
) Disco {
	return New(privateKey, packetConn, logger, NewDiscoPeer, NewDiscoPeerEndpoint)
}

func New(
	privateKey wgkey.DiscoPrivateKey,
	packetConn types.PacketConn,
	logger *zap.Logger,
	newPeer NewDiscoPeerFunc,
	newEndpoint NewDiscoPeerEndpointFunc,
) Disco {
	d := &disco{
		privateKey:  privateKey,
		publicKey:   privateKey.Public(),
		newPeer:     newPeer,
		newEndpoint: newEndpoint,
		closed:      make(chan struct{}),
		sendChan:    make(chan *EncryptedDiscoPacket),
		conn:        packetConn,
		peers:       syncmap.Map[wgkey.DiscoPublicKey, DiscoPeer]{},
		logger:      logger.With(zap.String("service", "disco")),
	}

	go d.runSender()
	go d.runReceiver()

	return d
}

func (d *disco) runSender() {
	for pkt := range d.sendChan {
		b, ok := pkt.Marshal()

		if !ok {
			continue
		}

		_, err := d.conn.WriteTo(b, pkt.Endpoint)

		if err != nil {
			d.logger.Debug("sending msg failed", zap.String("endpoint", pkt.Endpoint.String()), zap.Error(err))
		}
	}
}

func (d *disco) runReceiver() {
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
			d.logger.Debug("reading from UDP failed", zap.Error(err))
			continue
		}
		addr = netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())

		pkt := EncryptedDiscoPacket{
			Endpoint: addr,
		}

		ok := pkt.Unmarshal(buf[:n])
		if !ok {
			d.logger.Debug("unmarshal failed", zap.String("key", base64.StdEncoding.EncodeToString(pkt.SrcPublicDiscoKey[:])))
			continue
		}

		peer, ok := d.peers.Load(pkt.SrcPublicDiscoKey)
		if !ok {
			d.logger.Debug("finding peer failed", zap.String("endpoint", pkt.Endpoint.String()), zap.String("key", base64.StdEncoding.EncodeToString(pkt.SrcPublicDiscoKey[:])))
			continue
		}

		d.logger.Debug("receiving disco packet", zap.String("endpoint", pkt.Endpoint.String()), zap.String("public_disco_key", pkt.SrcPublicDiscoKey.String()))

		peer.EnqueueReceivedPacket(pkt)
	}
}

func (d *disco) AddPeer(pubKey wgkey.DiscoPublicKey) DiscoPeer {
	if peer, ok := d.peers.Load(pubKey); ok {
		return peer
	}

	temporal := true
	onClose := func() {
		if temporal {
			return
		}

		d.peers.Delete(pubKey)
	}

	peer := NewDiscoPeer(d, d.privateKey, pubKey, onClose, d.logger, d.newEndpoint)

	actual, loaded := d.peers.LoadOrStore(pubKey, peer)

	if loaded {
		peer.Close()
	} else {
		temporal = false

		peer.Status().NotifyStatus(func(status DiscoPeerStatusReadOnly) {
			d.statusCallback(pubKey, status)
		})

		select {
		case <-d.closed:
			peer.Close()

			return nil
		default:
		}
	}

	return actual
}

func (d *disco) SetPeers(peers map[wgkey.DiscoPublicKey][]netip.AddrPort) {
	for k, v := range peers {
		peer := d.AddPeer(k)

		peer.SetEndpoints(v)
	}

	d.peers.Range(func(key wgkey.DiscoPublicKey, value DiscoPeer) bool {
		_, ok := peers[key]

		if !ok {
			value.Close()
		}

		return true
	})
}

func (d *disco) GetAllStatuses() (res map[wgkey.DiscoPublicKey]DiscoPeerStatusReadOnly) {
	res = make(map[wgkey.DiscoPublicKey]DiscoPeerStatusReadOnly)
	d.peers.Range(func(key wgkey.DiscoPublicKey, value DiscoPeer) bool {
		res[key] = value.Status().Get()

		return true
	})

	return res
}

func (d *disco) Send(pkt *EncryptedDiscoPacket) {
	d.sendChan <- pkt
}

func (d *disco) SetStatusCallback(fn func(pubKey wgkey.DiscoPublicKey, status DiscoPeerStatusReadOnly)) {
	d.statusCallback = fn
}

func (d *disco) closeAllPeers() {
	d.peers.Range(func(key wgkey.DiscoPublicKey, value DiscoPeer) bool {
		d.peers.Delete(key)
		value.Close()

		return true
	})
}

func (d *disco) Close() error {
	close(d.closed)
	d.closeAllPeers()
	d.conn.Close()

	return nil
}
