package toxfuengine

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"net/netip"

	"github.com/miscord-dev/toxfu/pkg/wgkey"
)

type Disco struct {
	sendChan chan DiscoPacket
	conn     net.UDPConn
}

func (d *Disco) send() {
	for pkt := range d.sendChan {
		b, ok := pkt.Marshal()

		if !ok {
			continue
		}

		_, err := d.conn.WriteToUDPAddrPort(b, pkt.Endpoint)

		if err != nil {
			log.Printf("sending msg to %v failed: %+v", pkt.Endpoint, err)
		}
	}
}

type DiscoSubscriber struct {
	d *Disco

	srcPublicDiscoKey wgkey.DiscoPublicKey
}

func (s *DiscoSubscriber) SetPeers(
	endpoints []netip.AddrPort,
) {

}

func (d *Disco) run() {

}

type DiscoEndpointSubscriber struct {
	desID uint32
}

func (s *DiscoEndpointSubscriber) run() {

}

type DiscoPacket struct {
	Header            DiscoPacketHeader
	SrcPublicDiscoKey [32]byte
	DESID             uint32
	ID                uint32

	Endpoint  netip.AddrPort
	SharedKey wgkey.DiscoSharedKey
}

func (d *DiscoPacket) Marshal() ([]byte, bool) {
	buf := bytes.Buffer{}
	switch d.Header {
	case PingMessage, PongMessage:
	default:
		panic("unknown header type")
	}

	buf.WriteByte(byte(d.Header))
	buf.Write(d.SrcPublicDiscoKey[:])

	var encryptedPayload [8]byte
	binary.BigEndian.PutUint32(encryptedPayload[0:4], d.DESID)
	binary.BigEndian.PutUint32(encryptedPayload[4:8], d.DESID)

	ciphertext, ok := d.SharedKey.Encrypt(encryptedPayload[:])

	if !ok {
		return nil, false
	}

	buf.Write(ciphertext)

	return buf.Bytes(), true
}

func (d *DiscoPacket) Unmarshal(b []byte) bool {
	if len(b) < 1+len(d.SrcPublicDiscoKey) {
		return false
	}

	payloadIdx := 1 + len(d.SrcPublicDiscoKey)

	d.Header = DiscoPacketHeader(b[0])
	copy(d.SrcPublicDiscoKey[:], b[1:payloadIdx])

	decrypted, ok := d.SharedKey.Decrypt(b[payloadIdx:])

	if !ok || len(decrypted) < 8 {
		return false
	}

	d.DESKey = binary.BigEndian.Uint32(decrypted[0:4])
	d.DESKey = binary.BigEndian.Uint32(decrypted[4:8])

	return true
}

type DiscoPacketHeader byte

const (
	PingMessage DiscoPacketHeader = 128 + iota
	PongMessage
)
