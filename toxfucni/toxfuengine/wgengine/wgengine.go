package wgengine

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/nsutil"
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

func New(ifaceName, nsName string) (Engine, error) {
	e := wgEngine{
		ifaceName: ifaceName,
		nsName:    nsName,
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
	prevConfig     wgtypes.Config
}

func (e *wgEngine) initNamespace() (netns.NsHandle, error) {
	ns, err := netns.GetFromName(e.nsName)

	switch {
	case err == nil:
		return ns, nil
	case os.IsNotExist(err):
		// continue
	default:
		return 0, fmt.Errorf("failed to find netns %s: %w", e.nsName, err)
	}

	ns, err = nsutil.CreateNamespace(e.nsName)

	if err != nil {
		return 0, fmt.Errorf("failed to create netns %s: %w", e.nsName, err)
	}

	return ns, nil
}

func (e *wgEngine) initWireguard() (*netlink.Wireguard, error) {
	var wg *netlink.Wireguard

	// Found in toxfu netns

	link, err := e.ifaceNSNetlink.LinkByName(e.ifaceName)

	switch {
	case err == nil:
		var ok bool
		wg, ok = link.(*netlink.Wireguard)

		if !ok {
			return nil, fmt.Errorf("the link %s is not wireguard", e.ifaceName)
		}

		return wg, nil
	case errors.As(err, &netlink.LinkNotFoundError{}):
		// continue
	default:
		return nil, fmt.Errorf("failed to find the link %s: %w", e.ifaceName, err)
	}

	// Found in main netns

	link, err = e.mainNSNetlink.LinkByName(e.ifaceName)

	switch {
	case err == nil:
		var ok bool
		wg, ok = link.(*netlink.Wireguard)

		if !ok {
			return nil, fmt.Errorf("the link %s is not wireguard", e.ifaceName)
		}
	case errors.As(err, &netlink.LinkNotFoundError{}):
		// continue
	default:
		return nil, fmt.Errorf("failed to find the link %s: %w", e.ifaceName, err)
	}

	if wg == nil {
		wg = &netlink.Wireguard{
			LinkAttrs: netlink.LinkAttrs{
				Name: e.ifaceName,
			},
		}

		if err := netlink.LinkAdd(wg); err != nil {
			return nil, fmt.Errorf("failed to create wireguard interface: %w", err)
		}
	}

	if err := netlink.LinkSetNsFd(wg, int(e.ifaceNSHandle)); err != nil {
		return nil, fmt.Errorf("failed to set ns %s to %s: %w", e.nsName, e.ifaceName, err)
	}

	return wg, nil
}

func (e *wgEngine) init() error {
	ns, err := e.initNamespace()
	if err != nil {
		return fmt.Errorf("failed to init netns: %w", err)
	}

	e.ifaceNSHandle = ns

	e.ifaceNSNetlink, err = netlink.NewHandleAt(ns)
	if err != nil {
		return fmt.Errorf("failed to initialize handle for %s ns: %w", e.nsName, err)
	}

	e.mainNSNetlink, err = netlink.NewHandle()
	if err != nil {
		return fmt.Errorf("failed to initialize handle for main ns: %w", err)
	}

	wg, err := e.initWireguard()

	if err != nil {
		return fmt.Errorf("failed to init wireguard interface: %w", err)
	}

	err = nsutil.RunInNamespace(e.ifaceNSHandle, func() error {
		if err := netlink.LinkSetUp(wg); err != nil {
			return fmt.Errorf("ip link set %s up failed: %w", wg.LinkAttrs.Name, err)
		}

		lo, err := netlink.LinkByName("lo")
		if err != nil {
			return fmt.Errorf("finding lo device failed: %w", err)
		}

		if err := netlink.LinkSetUp(lo); err != nil {
			return fmt.Errorf("ip link set lo up failed: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to set up interfaces: %w", err)
	}

	e.iface = wg

	return nil
}

func (e *wgEngine) reconfigWireguard(client *wgctrl.Client, config wgtypes.Config) error {
	diff, hasDiff := diffConfigs(config, e.prevConfig)

	if !hasDiff {
		return nil
	}

	if err := client.ConfigureDevice(e.ifaceName, diff); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	e.prevConfig = config

	return nil
}

func (e *wgEngine) reconfigAddresses(addrs []netlink.Addr) error {
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
}

func (e *wgEngine) reconfigRoutes(config wgtypes.Config) error {
	current, err := e.ifaceNSNetlink.RouteList(e.iface, netlink.FAMILY_ALL)

	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
	}

	desired := generateRoutesFromWGConfig(config, e.iface)
	added, deleted := diffRoutes(desired, current)

	for _, d := range deleted {
		if err := e.ifaceNSNetlink.RouteDel(&d); err != nil {
			return fmt.Errorf("failed to delete %s: %w", d.Dst, err)
		}
	}
	for _, a := range added {
		if err := e.ifaceNSNetlink.RouteAdd(&a); err != nil {
			return fmt.Errorf("failed to add %s: %w", a.Dst, err)
		}
	}

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

		if err := e.reconfigWireguard(client, config); err != nil {
			return fmt.Errorf("failed to reconfig wireguard: %w", err)
		}
		if err := e.reconfigAddresses(addrs); err != nil {
			return fmt.Errorf("failed to reconfig addresses: %w", err)
		}
		if err := e.reconfigRoutes(config); err != nil {
			return fmt.Errorf("failed to reconfig routes: %w", err)
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
