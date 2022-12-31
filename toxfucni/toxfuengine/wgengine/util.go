package wgengine

import (
	"reflect"
	"sort"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func diffIPs(desired, current []netlink.Addr) (added, deleted []netlink.Addr) {
	desiredMap := map[string]netlink.Addr{}
	for _, d := range desired {
		desiredMap[d.IPNet.String()] = d
	}

	for _, c := range current {
		_, ok := desiredMap[c.IPNet.String()]

		if !ok {
			deleted = append(deleted, c)
		} else {
			delete(desiredMap, c.IPNet.String())
		}
	}

	for _, d := range desiredMap {
		added = append(added, d)
	}

	sort.Slice(added, func(i, j int) bool {
		return added[i].IPNet.String() < added[j].IPNet.String()
	})

	return
}

func diffRoutes(desired, current []netlink.Route) (added, deleted []netlink.Route) {
	desiredMap := map[string]netlink.Route{}
	for _, d := range desired {
		desiredMap[d.Dst.String()] = d
	}

	for _, c := range current {
		r, ok := desiredMap[c.Dst.String()]

		if !ok {
			deleted = append(deleted, c)
		} else if r.LinkIndex != c.LinkIndex {
			deleted = append(deleted, c)
		} else {
			delete(desiredMap, c.Dst.String())
		}
	}

	for _, d := range desiredMap {
		added = append(added, d)
	}

	sort.Slice(added, func(i, j int) bool {
		return added[i].Dst.String() < added[j].Dst.String()
	})

	return
}

func generateRoutesFromWGConfig(config wgtypes.Config, link netlink.Link) []netlink.Route {
	routes := []netlink.Route{}

	for _, peer := range config.Peers {
		for _, allowedIPs := range peer.AllowedIPs {
			routes = append(routes, netlink.Route{
				Dst:       &allowedIPs,
				LinkIndex: link.Attrs().Index,
			})
		}
	}

	return routes
}

func diffConfigs(expected, current wgtypes.Config) (diff wgtypes.Config, hasDiff bool) {
	if !reflect.DeepEqual(expected.FirewallMark, current.FirewallMark) {
		hasDiff = true
	}
	diff.FirewallMark = expected.FirewallMark

	if !reflect.DeepEqual(expected.ListenPort, current.ListenPort) {
		hasDiff = true
	}
	diff.ListenPort = expected.ListenPort

	if !reflect.DeepEqual(expected.PrivateKey, current.PrivateKey) {
		hasDiff = true
	}
	diff.PrivateKey = expected.PrivateKey

	diff.Peers = diffPeers(expected.Peers, current.Peers)

	hasDiff = hasDiff || len(diff.Peers) != 0

	return diff, hasDiff
}

func diffPeers(expected, current []wgtypes.PeerConfig) (diff []wgtypes.PeerConfig) {
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].PublicKey.String() < expected[j].PublicKey.String()
	})

	sort.Slice(current, func(i, j int) bool {
		return current[i].PublicKey.String() < current[j].PublicKey.String()
	})

	eIndex, cIndex := 0, 0
	for {
		if eIndex >= len(expected) {
			for _, c := range current[cIndex:] {
				c.Remove = true
				diff = append(diff, c)
			}

			break
		}
		if cIndex >= len(current) {
			diff = append(diff, expected[eIndex:]...)

			break
		}

		eKey := expected[eIndex].PublicKey.String()
		cKey := current[cIndex].PublicKey.String()

		switch {
		case eKey > cKey:
			c := current[cIndex]
			c.Remove = true

			diff = append(diff, c)
			cIndex++
		case eKey < cKey:
			diff = append(diff, expected[eIndex])
			eIndex++
		default: // ==
			e := expected[eIndex]
			c := current[cIndex]

			differs := func() bool {
				isNilE, isNilC := e.Endpoint == nil, c.Endpoint == nil
				if isNilE != isNilC || (!isNilE && e.Endpoint.String() != c.Endpoint.String()) {
					return true
				}

				isNilE, isNilC = e.PersistentKeepaliveInterval == nil, c.PersistentKeepaliveInterval == nil
				if isNilE != isNilC || (!isNilE && *e.PersistentKeepaliveInterval != *c.PersistentKeepaliveInterval) {
					return true
				}

				isNilE, isNilC = e.PresharedKey == nil, c.PresharedKey == nil
				if isNilE != isNilC || (!isNilE && *e.PresharedKey != *c.PresharedKey) {
					return true
				}

				if e.PublicKey != c.PublicKey {
					return true
				}

				if len(e.AllowedIPs) != len(c.AllowedIPs) {
					return true
				}

				for i := range e.AllowedIPs {
					if e.AllowedIPs[i].String() != c.AllowedIPs[i].String() {
						return true
					}
				}

				return false
			}()

			if differs {
				e := expected[eIndex]
				e.ReplaceAllowedIPs = true
				diff = append(diff, e)
			}

			eIndex++
			cIndex++
		}
	}

	return diff
}
