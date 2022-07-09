package toxfuengine

import (
	"fmt"
	"os"

	"github.com/miscord-dev/toxfu/pkg/nsutil"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ToxfuEngine interface {
}

type toxfuEngine struct {
	ifaceName string

	mainNSNetlink  *netlink.Handle
	ifaceNSNetlink *netlink.Handle
	ifaceNSHandle  netns.NsHandle
	iface          *netlink.Wireguard

	currentConfig *Config
}

func New() ToxfuEngine {
	return &toxfuEngine{}
}

func (e *toxfuEngine) init() error {
	wg := netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: e.ifaceName,
		},
	}

	if err := netlink.LinkAdd(&wg); err != nil {
		return fmt.Errorf("failed to create wireguard interface: %w", err)
	}

	nsName := fmt.Sprintf("toxfu_%d", os.Getpid())
	ns, err := nsutil.CreateNamespace(nsName)

	if err != nil {
		return fmt.Errorf("failed to create netns %s: %w", nsName, err)
	}

	if err := netlink.LinkSetNsFd(&wg, int(ns)); err != nil {
		return fmt.Errorf("failed to set ns %s to %s: %w", nsName, e.ifaceName, err)
	}

	e.ifaceNSNetlink, err = netlink.NewHandleAt(ns)
	if err != nil {
		return fmt.Errorf("failed to initialize handle for %s ns: %w", nsName, err)
	}

	e.mainNSNetlink, err = netlink.NewHandle()
	if err != nil {
		return fmt.Errorf("failed to initialize handle for main ns: %w", err)
	}

	e.iface = &wg

	return nil
}

// TODO(tsuzu): Reuse wgctrl client for better performance
func (e *toxfuEngine) updateWireguardConfig(config wgtypes.Config, addrs []netlink.Addr) error {
	return nsutil.RunInNamespace(e.ifaceNSHandle, func() error {
		client, err := wgctrl.New()

		if err != nil {
			return fmt.Errorf("failed to set up wgctrl client: %w", err)
		}
		defer client.Close()

		if err := client.ConfigureDevice(e.ifaceName, config); err != nil {
			return fmt.Errorf("failed to configure device: %w", err)
		}

		current, err := e.ifaceNSNetlink.AddrList(e.iface, netlink.FAMILY_ALL)

		if err != nil {
			return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
		}

		added, deleted := diffIPs(addrs, current)

		for _, d := range deleted {
			if err := e.ifaceNSNetlink.AddrDel(e.iface, &d); err != nil {
				return fmt.Errorf("failed to delete %v: %w", d, err)
			}
		}
		for _, a := range added {
			if err := e.ifaceNSNetlink.AddrAdd(e.iface, &a); err != nil {
				return fmt.Errorf("failed to add %v: %w", a, err)
			}
		}

		return nil
	})
}

func (e *toxfuEngine) Reconfig() {

}
