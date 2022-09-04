package sender

import "net/netip"

type Sender interface {
	Send(payload []byte, dst netip.AddrPort) error
	Refresh() error
	Close() error
}
