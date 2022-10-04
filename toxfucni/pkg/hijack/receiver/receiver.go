package receiver

import "net/netip"

type Receiver interface {
	Recv(b []byte) (len int, src netip.AddrPort, err error)
	Refresh() error
	Close() error
}
