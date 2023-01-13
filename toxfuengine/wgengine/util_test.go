package wgengine

import (
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func genPrivKey(t *testing.T) wgtypes.Key {
	t.Helper()

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	return key
}

func ptr[T any](v T) *T {
	return &v
}

func parseKey(t *testing.T, key string) wgtypes.Key {
	t.Helper()

	k, err := wgtypes.ParseKey(key)
	if err != nil {
		t.Fatal(err)
	}

	return k
}

func TestDiffConfig(t *testing.T) {
	before := wgtypes.Config{
		PrivateKey: ptr(parseKey(t, "aI/WLgD+CJIGCcTmYjsvrxwPOLGT9pwbx/UITg4jx3Q=")),
		ListenPort: ptr(51820),
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: parseKey(t, "NJ+3GndjEO7UL5huitue58WE6+WRROWABAIH6VsaUww="),
				Endpoint: &net.UDPAddr{
					IP:   net.ParseIP("192.168.1.1"),
					Port: 10,
				},
				PersistentKeepaliveInterval: ptr(3 * time.Second),
				AllowedIPs: []net.IPNet{
					{
						IP:   net.ParseIP("10.16.1.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
			{
				PublicKey: parseKey(t, "BI1WkEAg9gFIbITZEeLwCaFuB+Z5PDAS91GXCvmyiXk="),
				Endpoint: &net.UDPAddr{
					IP:   net.ParseIP("192.168.1.2"),
					Port: 11,
				},
				PersistentKeepaliveInterval: ptr(3 * time.Second),
				AllowedIPs: []net.IPNet{
					{
						IP:   net.ParseIP("10.16.2.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
		},
	}

	t.Run("not_updated", func(t *testing.T) {
		_, hasDiff := diffConfigs(before, before)

		if hasDiff {
			t.Fatal("hasDiff is true")
		}
	})

	t.Run("listen_port", func(t *testing.T) {
		after := wgtypes.Config{
			PrivateKey: ptr(parseKey(t, "aI/WLgD+CJIGCcTmYjsvrxwPOLGT9pwbx/UITg4jx3Q=")),
			ListenPort: ptr(51821),
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey: parseKey(t, "NJ+3GndjEO7UL5huitue58WE6+WRROWABAIH6VsaUww="),
					Endpoint: &net.UDPAddr{
						IP:   net.ParseIP("192.168.1.1"),
						Port: 10,
					},
					PersistentKeepaliveInterval: ptr(3 * time.Second),
					AllowedIPs: []net.IPNet{
						{
							IP:   net.ParseIP("10.16.1.0"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				{
					PublicKey: parseKey(t, "BI1WkEAg9gFIbITZEeLwCaFuB+Z5PDAS91GXCvmyiXk="),
					Endpoint: &net.UDPAddr{
						IP:   net.ParseIP("192.168.1.2"),
						Port: 11,
					},
					PersistentKeepaliveInterval: ptr(3 * time.Second),
					AllowedIPs: []net.IPNet{
						{
							IP:   net.ParseIP("10.16.2.0"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		}

		diff, hasDiff := diffConfigs(after, before)

		if !hasDiff {
			t.Fatal("hasDiff is false")
		}

		if len(diff.Peers) != 0 {
			t.Fatal("len(diff.Peers) != 0")
		}
	})

	t.Run("listen_port", func(t *testing.T) {
		after := wgtypes.Config{
			PrivateKey: ptr(parseKey(t, "aI/WLgD+CJIGCcTmYjsvrxwPOLGT9pwbx/UITg4jx3Q=")),
			ListenPort: ptr(51820),
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey: parseKey(t, "BI1WkEAg9gFIbITZEeLwCaFuB+Z5PDAS91GXCvmyiXk="),
					Endpoint: &net.UDPAddr{
						IP:   net.ParseIP("192.168.1.2"),
						Port: 11,
					},
					PersistentKeepaliveInterval: ptr(3 * time.Second),
					AllowedIPs: []net.IPNet{
						{
							IP:   net.ParseIP("10.16.2.0"),
							Mask: net.CIDRMask(24, 32),
						},
						{
							IP:   net.ParseIP("10.16.3.0"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				{
					PublicKey: parseKey(t, "l5+etPkDtc+rdhI+/BfLQh/9/Q4NW5rSXe31m1cTSTw="),
					Endpoint: &net.UDPAddr{
						IP:   net.ParseIP("192.168.1.2"),
						Port: 11,
					},
					PersistentKeepaliveInterval: ptr(3 * time.Second),
					AllowedIPs: []net.IPNet{
						{
							IP:   net.ParseIP("10.16.2.0"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		}
		expectedDiff := []wgtypes.PeerConfig{
			{
				PublicKey: parseKey(t, "BI1WkEAg9gFIbITZEeLwCaFuB+Z5PDAS91GXCvmyiXk="),
				Endpoint: &net.UDPAddr{
					IP:   net.ParseIP("192.168.1.2"),
					Port: 11,
				},
				PersistentKeepaliveInterval: ptr(3 * time.Second),
				AllowedIPs: []net.IPNet{
					{
						IP:   net.ParseIP("10.16.2.0"),
						Mask: net.CIDRMask(24, 32),
					},
					{
						IP:   net.ParseIP("10.16.3.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
				ReplaceAllowedIPs: true,
			},
			{
				PublicKey: parseKey(t, "NJ+3GndjEO7UL5huitue58WE6+WRROWABAIH6VsaUww="),
				Endpoint: &net.UDPAddr{
					IP:   net.ParseIP("192.168.1.1"),
					Port: 10,
				},
				PersistentKeepaliveInterval: ptr(3 * time.Second),
				AllowedIPs: []net.IPNet{
					{
						IP:   net.ParseIP("10.16.1.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
				Remove: true,
			},
			{
				PublicKey: parseKey(t, "l5+etPkDtc+rdhI+/BfLQh/9/Q4NW5rSXe31m1cTSTw="),
				Endpoint: &net.UDPAddr{
					IP:   net.ParseIP("192.168.1.2"),
					Port: 11,
				},
				PersistentKeepaliveInterval: ptr(3 * time.Second),
				AllowedIPs: []net.IPNet{
					{
						IP:   net.ParseIP("10.16.2.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
		}

		diff, hasDiff := diffConfigs(after, before)

		if !hasDiff {
			t.Fatal("hasDiff is false")
		}

		diffPeers := diffPeers(diff.Peers, expectedDiff)

		t.Log(cmp.Diff(diff.Peers, nil))

		if len(diffPeers) != 0 {
			t.Fatal("len(diffPeers) != 0")
		}
	})

}
