package tetraengine

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type PeerConfig struct {
	Endpoints      []string
	PublicKey      string
	PublicDiscoKey string
	Addresses      []string
	AllowedIPs     []string
}

func (pc *PeerConfig) toWGConfig() (*wgtypes.PeerConfig, error) {
	wgc := &wgtypes.PeerConfig{}

	allowedIPs := make([]net.IPNet, len(pc.AllowedIPs))

	for j := range allowedIPs {
		cidr := pc.AllowedIPs[j]
		prefix, err := netip.ParsePrefix(cidr)

		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", cidr, err)
		}

		allowedIPs[j] = net.IPNet{
			IP:   prefix.Addr().AsSlice(),
			Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
		}
	}

	wgc.AllowedIPs = allowedIPs
	wgc.ReplaceAllowedIPs = true
	wgc.PersistentKeepaliveInterval = ptr(30 * time.Second)

	pubKey, err := wgtypes.ParseKey(pc.PublicKey)

	if err != nil {
		return nil, fmt.Errorf("failed to parse public key %s: %w", pc.PublicKey, err)
	}
	wgc.PublicKey = pubKey

	return wgc, nil
}

type Config struct {
	PrivateKey string
	// ListenPort is immutable field
	ListenPort   int
	STUNEndpoint string
	Addresses    []netlink.Addr
	Peers        []PeerConfig
}
