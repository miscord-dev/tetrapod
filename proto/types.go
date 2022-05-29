package proto

import (
	"fmt"
	"strconv"

	"inet.af/netaddr"
	"tailscale.com/tailcfg"
)

func (prefix *IPPrefix) Netaddr() (netaddr.IPPrefix, error) {
	ip, err := netaddr.ParseIP(prefix.GetAddress())

	if err != nil {
		return netaddr.IPPrefix{}, fmt.Errorf("failed to parse %s: %w", prefix.String(), err)
	}

	return netaddr.IPPrefixFrom(ip, uint8(prefix.GetBits())), nil
}

func IPPrefixiesToNetaddrs(prefixies []*IPPrefix) ([]netaddr.IPPrefix, error) {
	results := make([]netaddr.IPPrefix, 0, len(prefixies))

	for i, prefix := range prefixies {
		if prefix == nil {
			continue
		}
		np, err := prefix.Netaddr()

		if err != nil {
			return nil, fmt.Errorf("failed to parse %dth: %w", i, err)
		}

		results = append(results, np)
	}

	return results, nil
}

func (n *Node) TailcfgNode() (*tailcfg.Node, error) {
	addrs, err := IPPrefixiesToNetaddrs(n.GetAddresses())

	if err != nil {
		return nil, fmt.Errorf("failed to parse addresses: %w", err)
	}

	advertised, err := IPPrefixiesToNetaddrs(n.GetAdvertisedPrefixes())

	if err != nil {
		return nil, fmt.Errorf("failed to parse advertised prefixes: %w", err)
	}

	idStr := strconv.FormatInt(n.Id, 10)
	tn := &tailcfg.Node{
		ID:         tailcfg.NodeID(n.GetId()),
		StableID:   tailcfg.StableNodeID(idStr),
		Name:       idStr,
		Addresses:  addrs,
		AllowedIPs: advertised,
		Endpoints:  n.GetEndpoints(),
	}
	if err := tn.Key.UnmarshalText([]byte(n.GetPublicKey())); err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	if err := tn.DiscoKey.UnmarshalText([]byte(n.GetPublicDiscoKey())); err != nil {
		return nil, fmt.Errorf("failed to parse public disco key: %w", err)
	}

	return tn, nil
}
