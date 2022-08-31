package wgengine

import (
	"fmt"
	"io"
	"os"

	"github.com/miscord-dev/toxfu/pkg/nsutil"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Engine interface {
	Reconfig(config wgtypes.Config, addrs []netlink.Addr) error
	io.Closer
}

var _ Engine = &wgEngine{}

func New(ifaceName string) (Engine, error) {
	e := wgEngine{
		ifaceName: ifaceName,
	}

	if err := e.init(); err != nil {
		return nil, fmt.Errorf("failed to init wgengine: %w", err)
	}

	return &e, nil
}

type wgEngine struct {
	ifaceName string
	nsName    string

	mainNSNetlink  *netlink.Handle
	ifaceNSNetlink *netlink.Handle
	ifaceNSHandle  netns.NsHandle
	iface          *netlink.Wireguard
}

func (e *wgEngine) init() error {
	wg := netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: e.ifaceName,
		},
	}

	if err := netlink.LinkAdd(&wg); err != nil {
		return fmt.Errorf("failed to create wireguard interface: %w", err)
	}

	nsName := fmt.Sprintf("toxfu_%d", os.Getpid())
	e.nsName = nsName

	ns, err := nsutil.CreateNamespace(nsName)

	if err != nil {
		return fmt.Errorf("failed to create netns %s: %w", nsName, err)
	}

	if err := netlink.LinkSetNsFd(&wg, int(ns)); err != nil {
		return fmt.Errorf("failed to set ns %s to %s: %w", nsName, e.ifaceName, err)
	}

	e.ifaceNSHandle = ns

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
func (e *wgEngine) Reconfig(config wgtypes.Config, addrs []netlink.Addr) error {
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

func (e *wgEngine) Close() error {
	if err := netns.DeleteNamed(e.nsName); err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", e.nsName, err)
	}

	return nil
}
