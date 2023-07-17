package wgengine_test

import (
	"net"
	"testing"

	"github.com/miscord-dev/tetrapod/tetraengine/wgengine"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestInit(t *testing.T) {
	wgName := "tetra0"
	netnsName := "netns"

	engine, err := wgengine.NewNetns(wgName, netnsName, zap.NewNop())

	if err != nil {
		t.Fatal(err)
	}
	// defer engine.Close()

	handle, err := netns.GetFromName(netnsName)

	if err != nil {
		t.Fatal(err)
	}

	wgNetlink, err := netlink.NewHandleAt(handle)

	if err != nil {
		t.Fatal(err)
	}

	wg, err := wgNetlink.LinkByName(wgName)

	if err != nil {
		t.Fatal(err)
	}

	key, _ := wgtypes.GeneratePrivateKey()

	route := net.IPNet{
		IP:   net.IPv4(10, 0, 1, 0),
		Mask: net.CIDRMask(24, 32),
	}
	ip, _ := netlink.ParseAddr("10.0.2.1/24")

	config, addrs := wgtypes.Config{
		PrivateKey: &key,
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: key.PublicKey(),
				AllowedIPs: []net.IPNet{
					route,
				},
			},
		},
	}, []netlink.Addr{
		*ip,
	}

	t.Run("configure first", func(t *testing.T) {
		err := engine.Reconfig(config, addrs)

		if err != nil {
			t.Fatal(err)
		}

		routes, err := wgNetlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
			LinkIndex: wg.Attrs().Index,
		}, netlink.RT_FILTER_OIF)

		if err != nil {
			t.Fatal(err)
		}

		if expected := "10.0.1.0/24"; routes[0].Dst.String() != expected {
			t.Error("expected", expected, "got", routes[0].Dst.String())
		}

		if wg.Attrs().MTU != 1280 {
			t.Error("MTU is", wg.Attrs().MTU)
		}
	})

	t.Run("add routes/addrs", func(t *testing.T) {
		route2 := net.IPNet{
			IP:   net.IPv4(10, 0, 2, 0),
			Mask: net.CIDRMask(24, 32),
		}
		ip2, _ := netlink.ParseAddr("10.1.2.1/24")

		config.Peers = append(config.Peers, wgtypes.PeerConfig{
			PublicKey: key.PublicKey(),
			AllowedIPs: []net.IPNet{
				route2,
			},
		})
		addrs = append(addrs, *ip2)

		if err := engine.Reconfig(config, addrs); err != nil {
			t.Fatal(err)
		}

		routes, err := wgNetlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
			LinkIndex: wg.Attrs().Index,
		}, netlink.RT_FILTER_OIF)

		if err != nil {
			t.Fatal(err)
		}

		if len(routes) != 2 {
			t.Error("mismatched len of routes", routes)
		}

		addrs, err := wgNetlink.AddrList(wg, netlink.FAMILY_ALL)

		if err != nil {
			t.Fatal(err)
		}

		if len(addrs) != 2 {
			t.Error("mismatched len of addrs", addrs)
		}
	})

	t.Run("remove routes/addrs", func(t *testing.T) {
		config.Peers = config.Peers[:1]
		addrs = addrs[:1]

		if err := engine.Reconfig(config, addrs); err != nil {
			t.Fatal(err)
		}

		routes, err := wgNetlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
			LinkIndex: wg.Attrs().Index,
		}, netlink.RT_FILTER_OIF)

		if err != nil {
			t.Fatal(err)
		}

		if len(routes) != 1 {
			t.Error("mismatched len of routes", routes)
		}

		addrs, err := wgNetlink.AddrList(wg, netlink.FAMILY_ALL)

		if err != nil {
			t.Fatal(err)
		}

		if len(addrs) != 1 {
			t.Error("mismatched len of addrs", addrs)
		}
	})
}
