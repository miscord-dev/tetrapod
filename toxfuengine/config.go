package toxfuengine

import (
	"github.com/vishvananda/netlink"
)

type PeerConfig struct {
	ID             int64
	Endpoints      []string
	PublicKey      string
	PublicDiscoKey string
	Addresses      []string
	AllowedIPs     []string
}

type Config struct {
	PrivateKey string
	Addresses  []netlink.Addr
	Peers      []PeerConfig
}
