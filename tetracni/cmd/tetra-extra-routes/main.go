package main

import (
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("tetrapod-extra-routes"))
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, result, err := loadConfig(args.StdinData, args.Args)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	extraRoutes, err := loadExtraRoutes(conf)

	if err != nil {
		return fmt.Errorf("failed to load extra routes for %s/%s: %w", conf.Args.K8S_POD_NAMESPACE, conf.Args.K8S_POD_NAME, err)
	}

	var veth *netlink.Veth
	for _, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)

		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", iface.Name, err)
		}

		v, ok := link.(*netlink.Veth)

		if !ok {
			continue
		}

		veth = v
	}

	if veth == nil {
		return fmt.Errorf("no veth to route")
	}

	link, err := netlink.LinkByName(conf.VRF)

	if err != nil {
		return fmt.Errorf("failed to find a VRF: %w", err)
	}

	vrf, ok := link.(*netlink.Vrf)

	if !ok {
		return fmt.Errorf("%s is not VRF", conf.VRF)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
		Table: int(vrf.Table),
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return fmt.Errorf("failed to list routes for vrf: %w", err)
	}

	routesSet := map[string]struct{}{}

	for _, r := range routes {
		routesSet[r.Dst.String()] = struct{}{}
	}

	for _, r := range extraRoutes {
		_, cidr, err := net.ParseCIDR(r)

		if err != nil {
			return fmt.Errorf("failed to parse route %s: %w", cidr, err)
		}

		_, ok := routesSet[cidr.String()]
		if ok {
			continue
		}

		err = netlink.RouteAdd(&netlink.Route{
			Dst:       cidr,
			LinkIndex: veth.Index,
			Table:     int(vrf.Table),
		})

		if err != nil {
			return fmt.Errorf("failed to add a route for %s to %s: %w", cidr.String(), vrf.Name, err)
		}
	}

	return types.PrintResult(conf.PrevResult, conf.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	conf, result, err := loadConfig(args.StdinData, args.Args)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	extraRoutes, err := loadExtraRoutes(conf)

	if err != nil {
		return fmt.Errorf("failed to load extra routes for %s/%s: %w", conf.Args.K8S_POD_NAMESPACE, conf.Args.K8S_POD_NAME, err)
	}

	var veth *netlink.Veth
	for _, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)

		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", iface.Name, err)
		}

		v, ok := link.(*netlink.Veth)

		if !ok {
			continue
		}

		veth = v
	}

	if veth == nil {
		return fmt.Errorf("no veth to route")
	}

	link, err := netlink.LinkByName(conf.VRF)

	if err != nil {
		return fmt.Errorf("failed to find a VRF: %w", err)
	}

	vrf, ok := link.(*netlink.Vrf)

	if !ok {
		return fmt.Errorf("%s is not VRF", conf.VRF)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{
		Table: int(vrf.Table),
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return fmt.Errorf("failed to list routes for vrf: %w", err)
	}

	routesSet := map[string]struct{}{}

	for _, r := range routes {
		routesSet[r.Dst.String()] = struct{}{}
	}

	for _, r := range extraRoutes {
		_, cidr, err := net.ParseCIDR(r)

		if err != nil {
			return fmt.Errorf("failed to parse route %s: %w", cidr, err)
		}

		_, ok := routesSet[cidr.String()]
		if !ok {
			return fmt.Errorf("not route for %s", cidr)
		}
	}

	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}
