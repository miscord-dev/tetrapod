package wgengine

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/miscord-dev/tetrapod/pkg/nsutil"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Engine interface {
	Reconfig(config wgtypes.Config, addrs []netlink.Addr) error
	io.Closer
}

var _ Engine = &wgEngine{}

func NewNetns(ifaceName, netns string, logger *zap.Logger) (Engine, error) {
	e := wgEngine{
		ifaceName:   ifaceName,
		wgNetnsName: netns,
		logger:      logger,
	}

	wgctrl, err := wgctrl.New()

	if err != nil {
		return nil, fmt.Errorf("failed to init wgctrl client: %w", err)
	}
	e.wgctrl = wgctrl

	if err := e.init(); err != nil {
		return nil, fmt.Errorf("failed to init wgengine: %w", err)
	}

	return &e, nil
}

type wgEngine struct {
	ifaceName   string
	wgNetnsName string

	netlink    *netlink.Handle
	wgNetns    netns.NsHandle
	wgNetlink  *netlink.Handle
	wireguard  *netlink.Wireguard
	prevConfig wgtypes.Config
	wgctrl     *wgctrl.Client

	logger *zap.Logger
}

func (e *wgEngine) garbageCollect() {
	link, err := e.netlink.LinkByName(e.ifaceName)

	if err == nil {
		e.netlink.LinkDel(link)
	}
}

func (e *wgEngine) initWireguard() (*netlink.Wireguard, error) {
	e.garbageCollect()

	var wg *netlink.Wireguard

	link, err := e.wgNetlink.LinkByName(e.ifaceName)

	switch {
	case err == nil:
		var ok bool
		wg, ok = link.(*netlink.Wireguard)

		if !ok {
			return nil, fmt.Errorf("the link %s is not wireguard", e.ifaceName)
		}
	case errors.As(err, &netlink.LinkNotFoundError{}):
		wg = &netlink.Wireguard{
			LinkAttrs: netlink.LinkAttrs{
				Name: e.ifaceName,
			},
		}

		if err := e.netlink.LinkAdd(wg); err != nil {
			return nil, fmt.Errorf("failed to create wireguard interface: %w", err)
		}

		if err := e.netlink.LinkSetNsFd(wg, int(e.wgNetns)); err != nil {
			return nil, fmt.Errorf("failed to set netns %s for wireguard interface: %w", e.wgNetnsName, err)
		}
	default:
		return nil, fmt.Errorf("failed to find the link %s: %w", e.ifaceName, err)
	}

	return wg, nil
}

func (e *wgEngine) initNetns() (netns.NsHandle, error) {
	nsHandle, err := netns.GetFromName(e.wgNetnsName)

	if os.IsNotExist(err) {
		return nsutil.CreateNamespace(e.wgNetnsName)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to find netns with name %s: %w", e.wgNetnsName, err)
	}

	return nsHandle, nil
}

func (e *wgEngine) init() error {
	var err error

	e.netlink, err = netlink.NewHandle()
	if err != nil {
		return fmt.Errorf("failed to initialize handle for main netns: %w", err)
	}

	nsHandle, err := e.initNetns()

	if err != nil {
		return fmt.Errorf("failed to init netns: %w", err)
	}
	e.wgNetns = nsHandle

	e.wgNetlink, err = netlink.NewHandleAt(nsHandle)

	if err != nil {
		return fmt.Errorf("failed to init handle for wg netns: %w", err)
	}

	wg, err := e.initWireguard()

	if err != nil {
		return fmt.Errorf("failed to init wireguard interface: %w", err)
	}
	e.wireguard = wg

	if err := e.wgNetlink.LinkSetMTU(wg, 1280); err != nil {
		return fmt.Errorf("failed to set MTU: %w", err)
	}

	if err := e.wgNetlink.LinkSetUp(wg); err != nil {
		return fmt.Errorf("ip link set %s up failed: %w", wg.LinkAttrs.Name, err)
	}

	err = nsutil.RunInNamespace(nsHandle, func() error {
		client, err := wgctrl.New()

		if err != nil {
			return fmt.Errorf("failed to init wgctrl: %w", err)
		}

		e.wgctrl = client

		return nil
	})

	if err != nil {
		return fmt.Errorf("running in netns %s failed: %w", e.wgNetnsName, err)
	}

	return nil
}

func (e *wgEngine) reconfigWireguard(config wgtypes.Config) error {
	diff, hasDiff := diffConfigs(config, e.prevConfig)

	if !hasDiff {
		return nil
	}

	err := nsutil.RunInNamespace(e.wgNetns, func() error {
		if err := e.wgctrl.ConfigureDevice(e.ifaceName, diff); err != nil {
			return fmt.Errorf("failed to configure device: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("running in netns %s failed: %w", e.wgNetnsName, err)
	}

	e.prevConfig = config

	return nil
}

func (e *wgEngine) reconfigAddresses(addrs []netlink.Addr) error {
	current, err := e.wgNetlink.AddrList(e.wireguard, netlink.FAMILY_ALL)

	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
	}

	added, deleted := diffIPs(addrs, current)

	var lastErr error
	for _, d := range deleted {
		if err := e.wgNetlink.AddrDel(e.wireguard, &d); err != nil {
			lastErr = fmt.Errorf("failed to delete %s: %w", d, err)
			e.logger.Error("failed to delete an address", zap.Error(err), zap.String("addr", d.String()))
		}
	}
	for _, a := range added {
		if err := e.wgNetlink.AddrAdd(e.wireguard, &a); err != nil {
			lastErr = fmt.Errorf("failed to add %s: %w", a, err)
			e.logger.Error("failed to add an address", zap.Error(err), zap.String("addr", a.String()))
		}
	}

	return lastErr
}

func printRoutes(msg string, routes []netlink.Route) {
	fmt.Printf("%s: ", msg)
	for _, r := range routes {
		fmt.Printf("%s, ", r.Dst.String())
	}
	fmt.Println()
}

func (e *wgEngine) reconfigRoutes(config wgtypes.Config) error {
	table := 0

	current, err := e.wgNetlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
		LinkIndex: e.wireguard.Attrs().Index,
		Table:     table,
	}, netlink.RT_FILTER_OIF|netlink.RT_FILTER_TABLE)

	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
	}

	desired := generateRoutesFromWGConfig(config, e.wireguard, table)
	added, deleted := diffRoutes(desired, current)

	var lastErr error
	for _, d := range deleted {
		if d.Scope == 254 { // link scope
			continue
		}
		if d.Dst.String() == "ff00::/8" { // v6 multicast
			continue
		}

		if err := e.wgNetlink.RouteDel(&d); err != nil {
			lastErr = fmt.Errorf("failed to delete %s: %w", d.Dst, err)
			e.logger.Error("failed to delete a route", zap.Error(err), zap.String("dst", d.Dst.String()))
		}
	}
	for _, a := range added {
		if err := e.wgNetlink.RouteAdd(&a); err != nil {
			lastErr = fmt.Errorf("failed to add %s: %w", a.Dst, err)
			e.logger.Error("failed to add a route", zap.Error(err), zap.String("dst", a.Dst.String()))
		}
	}

	return lastErr
}

func (e *wgEngine) Reconfig(config wgtypes.Config, addrs []netlink.Addr) error {
	if err := e.reconfigWireguard(config); err != nil {
		return fmt.Errorf("failed to reconfig wireguard: %w", err)
	}

	var lastErr error

	if err := e.reconfigAddresses(addrs); err != nil {
		lastErr = err
		e.logger.Error("failed to reconfig addresses", zap.Error(err))
	}
	if err := e.reconfigRoutes(config); err != nil {
		lastErr = err
		e.logger.Error("failed to reconfig addresses", zap.Error(err))
	}

	return lastErr
}

func (e *wgEngine) Close() error {
	nsutil.RunInNamespace(e.wgNetns, func() error {
		return e.wgctrl.Close()
	})
	netns.DeleteNamed(e.wgNetnsName)

	return nil
}
