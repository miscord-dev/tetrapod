package wgengine

import (
	"sort"

	"github.com/vishvananda/netlink"
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
