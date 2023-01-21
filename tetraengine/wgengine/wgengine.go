package wgengine

import (
	"errors"
	"fmt"
	"io"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Engine interface {
	Reconfig(config wgtypes.Config, addrs []netlink.Addr) error
	io.Closer
}

var _ Engine = &wgEngine{}

func NewVRF(ifaceName, vrf string, table uint32, logger *zap.Logger) (Engine, error) {
	e := wgEngine{
		ifaceName: ifaceName,
		vrf:       vrf,
		table:     table,
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
	ifaceName string
	vrf       string
	table     uint32

	netlink    *netlink.Handle
	wireguard  *netlink.Wireguard
	vrfLink    *netlink.Vrf
	prevConfig wgtypes.Config
	wgctrl     *wgctrl.Client

	logger *zap.Logger
}

func (e *wgEngine) initWireguard() (*netlink.Wireguard, error) {
	var wg *netlink.Wireguard

	link, err := e.netlink.LinkByName(e.ifaceName)

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

		if err := netlink.LinkAdd(wg); err != nil {
			return nil, fmt.Errorf("failed to create wireguard interface: %w", err)
		}
	default:
		return nil, fmt.Errorf("failed to find the link %s: %w", e.ifaceName, err)
	}

	if err := netlink.LinkSetMTU(wg, 1280); err != nil {
		return nil, fmt.Errorf("failed to set MTU: %w", err)
	}

	return wg, nil
}

func (e *wgEngine) initVRF() (*netlink.Vrf, error) {
	var vrf *netlink.Vrf

	link, err := e.netlink.LinkByName(e.vrf)

	switch {
	case err == nil:
		var ok bool
		vrf, ok = link.(*netlink.Vrf)

		if !ok {
			return nil, fmt.Errorf("the link %s is not vrf", e.vrf)
		}
	case errors.As(err, &netlink.LinkNotFoundError{}):
		vrf = &netlink.Vrf{
			LinkAttrs: netlink.LinkAttrs{
				Name: e.vrf,
			},
			Table: e.table,
		}

		if err := netlink.LinkAdd(vrf); err != nil {
			return nil, fmt.Errorf("failed to create vrf interface: %w", err)
		}
	default:
		return nil, fmt.Errorf("failed to find the link %s: %w", e.vrf, err)
	}

	if err := netlink.LinkSetMaster(e.wireguard, vrf); err != nil {
		return nil, fmt.Errorf("failed to find the link %s: %w", e.vrf, err)
	}

	if err := netlink.LinkSetUp(vrf); err != nil {
		return nil, fmt.Errorf("ip link set %s up failed: %w", vrf.LinkAttrs.Name, err)
	}

	return vrf, nil
}

func (e *wgEngine) init() error {
	var err error

	e.netlink, err = netlink.NewHandle()
	if err != nil {
		return fmt.Errorf("failed to initialize handle for main ns: %w", err)
	}

	wg, err := e.initWireguard()

	if err != nil {
		return fmt.Errorf("failed to init wireguard interface: %w", err)
	}
	e.wireguard = wg

	if e.vrf != "" {
		vrf, err := e.initVRF()

		if err != nil {
			return fmt.Errorf("failed to init vrf: %w", err)
		}
		e.vrfLink = vrf
	}

	if err := netlink.LinkSetUp(wg); err != nil {
		return fmt.Errorf("ip link set %s up failed: %w", wg.LinkAttrs.Name, err)
	}

	return nil
}

func (e *wgEngine) reconfigWireguard(config wgtypes.Config) error {
	diff, hasDiff := diffConfigs(config, e.prevConfig)

	if !hasDiff {
		return nil
	}

	if err := e.wgctrl.ConfigureDevice(e.ifaceName, diff); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	e.prevConfig = config

	return nil
}

func (e *wgEngine) reconfigAddresses(addrs []netlink.Addr) error {
	current, err := e.netlink.AddrList(e.wireguard, netlink.FAMILY_ALL)

	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
	}

	added, deleted := diffIPs(addrs, current)

	for _, d := range deleted {
		if err := e.netlink.AddrDel(e.wireguard, &d); err != nil {
			return fmt.Errorf("failed to delete %v: %w", d, err)
		}
	}
	for _, a := range added {
		if err := e.netlink.AddrAdd(e.wireguard, &a); err != nil {
			return fmt.Errorf("failed to add %v: %w", a, err)
		}
	}

	return nil
}

func printRoutes(msg string, routes []netlink.Route) {
	fmt.Printf("%s: ", msg)
	for _, r := range routes {
		fmt.Printf("%s, ", r.Dst.String())
	}
	fmt.Println()
}

func (e *wgEngine) reconfigRoutes(config wgtypes.Config) error {
	current, err := e.netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
		LinkIndex: e.wireguard.Attrs().Index,
		Table:     int(e.table),
	}, netlink.RT_FILTER_OIF|netlink.RT_FILTER_TABLE)

	if err != nil {
		return fmt.Errorf("failed to list addresses for %s: %w", e.ifaceName, err)
	}

	desired := generateRoutesFromWGConfig(config, e.wireguard, int(e.table))
	added, deleted := diffRoutes(desired, current)

	var lastErr error
	for _, d := range deleted {
		if d.Scope == 254 { // link scope
			continue
		}
		if d.Dst.String() == "ff00::/8" { // v6 multicast
			continue
		}

		if err := e.netlink.RouteDel(&d); err != nil {
			lastErr = fmt.Errorf("failed to delete %s: %w", d.Dst, err)
			e.logger.Error("failed to delete a route", zap.Error(err), zap.String("dst", d.Dst.String()))
		}
	}
	for _, a := range added {
		if err := e.netlink.RouteAdd(&a); err != nil {
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
	if err := e.reconfigAddresses(addrs); err != nil {
		return fmt.Errorf("failed to reconfig addresses: %w", err)
	}
	if err := e.reconfigRoutes(config); err != nil {
		return fmt.Errorf("failed to reconfig routes: %w", err)
	}

	return nil
}

func (e *wgEngine) Close() error {
	netlink.LinkDel(e.wireguard)
	if e.vrfLink != nil {
		netlink.LinkDel(e.vrfLink)
	}
	e.wgctrl.Close()

	return nil
}
