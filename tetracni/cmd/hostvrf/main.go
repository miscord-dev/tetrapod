package main

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("hostvrf"))
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, result, err := loadConfig(args.StdinData)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	link, err := netlink.LinkByName(conf.VRF)

	if err != nil {
		return fmt.Errorf("failed to find a VRF: %w", err)
	}

	vrf, ok := link.(*netlink.Vrf)

	if !ok {
		return fmt.Errorf("%s is not VRF", conf.VRF)
	}

	for _, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)

		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", iface.Name, err)
		}

		if link.Attrs().MasterIndex != 0 {
			continue
		}

		if err := netlink.LinkSetMaster(link, vrf); err != nil {
			return fmt.Errorf("failed to change master of %s to %s: %w", link.Attrs().Name, vrf.Name, err)
		}
	}

	return types.PrintResult(conf.PrevResult, conf.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	_, result, err := loadConfig(args.StdinData)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for _, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)

		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", iface.Name, err)
		}

		if link.Attrs().MasterIndex == 0 {
			return fmt.Errorf("master of %s is not set", link.Attrs().Name)
		}
	}

	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}
