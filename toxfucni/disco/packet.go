package disco

import (
	"bytes"
	"encoding/binary"
	"net/netip"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/wgkey"
	"golang.org/x/exp/slices"
)

type EncryptedDiscoPacket struct {
	Header            DiscoPacketHeader
	SrcPublicDiscoKey wgkey.DiscoPublicKey
	EncryptedPayload  []byte

	Endpoint netip.AddrPort
}

func (d *EncryptedDiscoPacket) Marshal() ([]byte, bool) {
	buf := bytes.Buffer{}
	switch d.Header {
	case PingMessage, PongMessage:
	default:
		return nil, false
	}

	buf.WriteByte(byte(d.Header))
	buf.Write(d.SrcPublicDiscoKey[:])
	buf.Write(d.EncryptedPayload)

	return buf.Bytes(), true
}

func (d *EncryptedDiscoPacket) Unmarshal(b []byte) bool {
	if len(b) < 1+len(d.SrcPublicDiscoKey) {
		return false
	}

	payloadIdx := 1 + len(d.SrcPublicDiscoKey)

	d.Header = DiscoPacketHeader(b[0])
	copy(d.SrcPublicDiscoKey[:], b[1:payloadIdx])
	d.EncryptedPayload = slices.Clone(b[payloadIdx:])

	return true
}

type DiscoPacket struct {
	Header            DiscoPacketHeader
	SrcPublicDiscoKey [32]byte
	EndpointID        uint32
	ID                uint32

	Endpoint  netip.AddrPort
	SharedKey wgkey.DiscoSharedKey
}

func (d *DiscoPacket) Encrypt() (*EncryptedDiscoPacket, bool) {
	var encryptedPayload [40]byte
	binary.BigEndian.PutUint32(encryptedPayload[0:4], d.EndpointID)
	binary.BigEndian.PutUint32(encryptedPayload[4:8], d.ID)
	copy(encryptedPayload[8:], d.SrcPublicDiscoKey[:])

	ciphertext, ok := d.SharedKey.Encrypt(encryptedPayload[:])

	if !ok {
		return nil, false
	}

	return &EncryptedDiscoPacket{
		Header:            d.Header,
		SrcPublicDiscoKey: d.SrcPublicDiscoKey,
		EncryptedPayload:  ciphertext,
		Endpoint:          d.Endpoint,
	}, true
}

func (d *DiscoPacket) Decrypt(encrypted *EncryptedDiscoPacket) bool {
	decryptedPayload, ok := d.SharedKey.Decrypt(encrypted.EncryptedPayload)

	if !ok {
		return false
	}
	if len(decryptedPayload) < 40 {
		return false
	}

	d.Header = encrypted.Header
	d.SrcPublicDiscoKey = encrypted.SrcPublicDiscoKey
	d.EndpointID = binary.BigEndian.Uint32(decryptedPayload[0:4])
	d.ID = binary.BigEndian.Uint32(decryptedPayload[4:8])

	if !bytes.Equal(decryptedPayload[8:], d.SrcPublicDiscoKey[:]) {
		return false
	}

	d.Endpoint = encrypted.Endpoint

	return true
}

type DiscoPacketHeader byte

const (
	PingMessage DiscoPacketHeader = 128 + iota
	PongMessage
)
