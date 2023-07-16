package main

import (
	"errors"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/seancfoley/ipaddress-go/ipaddr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("hostvrf"))
}

func findVeth(hostVethName, peerVethName string, peerNetnsFd int, peerNetnsNetlink *netlink.Handle) (hostVeth, peerVeth *netlink.Veth, err error) {
	hostVethLink, err := netlink.LinkByName(hostVethName)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to find veth %s: %w", hostVethName, err)
	}

	hostVeth, ok := hostVethLink.(*netlink.Veth)

	if !ok {
		return nil, nil, fmt.Errorf("link %s is not veth: %s", hostVethName, hostVethLink.Type())
	}

	peerVethLink, err := peerNetnsNetlink.LinkByName(peerVethName)

	if err != nil {
		return hostVeth, nil, fmt.Errorf("failed to find veth %s: %w", hostVethName, err)
	}

	peerVeth, ok = peerVethLink.(*netlink.Veth)

	if !ok {
		return hostVeth, nil, fmt.Errorf("link %s is not veth: %s", peerVethName, peerVethLink.Type())
	}

	return
}

func updateVeth(hostVethName, peerVethName string, peerNetnsFd int, peerNetnsNetlink *netlink.Handle) (hostVeth, peerVeth *netlink.Veth, err error) {
	hostVeth, peerVeth, err = findVeth(hostVethName, peerVethName, peerNetnsFd, peerNetnsNetlink)

	if err == nil {
		return hostVeth, peerVeth, nil
	}
	if err != nil && hostVeth == nil {
		return nil, nil, fmt.Errorf("failed to find veth pairs: %w", err)
	}

	peerVethLink, err := netlink.LinkByName(peerVethName)

	if err != nil {
		return hostVeth, nil, fmt.Errorf("failed to find veth %s: %w", hostVethName, err)
	}

	peerVeth, ok := peerVethLink.(*netlink.Veth)

	if !ok {
		return nil, nil, fmt.Errorf("link %s is not veth: %s", peerVethName, peerVethLink.Type())
	}

	if err := netlink.LinkSetNsFd(peerVethLink, peerNetnsFd); err != nil {
		return nil, nil, fmt.Errorf("failed to set netns for peer veth %s: %w", peerVethName, err)
	}

	return
}

func setupVeth(hostVethName, peerVethName, peerNetns string) (hostVeth, peerVeth *netlink.Veth, err error) {
	ns, err := netns.GetFromName(peerNetns)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to find netns %s: %w", peerNetns, err)
	}

	peerNetnsNetlink, err := netlink.NewHandleAt(ns)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to find netns %s: %w", peerNetns, err)
	}

	hostVeth, peerVeth, err = updateVeth(hostVethName, peerVethName, int(ns), peerNetnsNetlink)

	switch {
	case err == nil:
		return hostVeth, peerVeth, nil
	case errors.As(err, &netlink.LinkNotFoundError{}):
		hostVethLink := &netlink.Veth{
			LinkAttrs: netlink.LinkAttrs{
				Name: hostVethName,
			},
			PeerName: peerVethName,
		}

		if err := netlink.LinkAdd(hostVethLink); err != nil {
			return nil, nil, fmt.Errorf("faile create a veth %s: %w", hostVethName, err)
		}

		return updateVeth(hostVethName, peerVethName, int(ns), peerNetnsNetlink)
	default:
		return nil, nil, fmt.Errorf("failed to find %s, %s: %w", hostVethName, peerVethName, err)
	}
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, result, err := loadConfig(args.StdinData)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	hostVeth, peerVeth, err := setupVeth(conf.HostVeth, conf.PeerVeth, conf.Sandbox)

	if err != nil {
		return fmt.Errorf("failed to set up veth: %w", err)
	}

	var bridge *netlink.Bridge
	for _, iface := range result.Interfaces {
		if iface.Sandbox != "" {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)

		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", iface.Name, err)
		}

		var ok bool
		bridge, ok = link.(*netlink.Bridge)

		if ok {
			break
		}
	}

	if bridge == nil {
		return fmt.Errorf("bridge not found")
	}

	if err := netlink.LinkSetMaster(peerVeth, bridge); err != nil {
		return fmt.Errorf("failed to set master of %s %s: %w", peerVeth.Name, bridge.Name, err)
	}

	for _, ip := range result.IPs {
		addr, _ := ipaddr.NewIPAddressFromNetIPNet(&ip.Address)
		cidr := addr.GetUpper().Increment(-1)

		netlinkAddr := netlink.Addr{
			IPNet: &net.IPNet{
				IP:   cidr.GetNetIP(),
				Mask: net.CIDRMask(cidr.GetPrefixLen().Len(), cidr.GetBitCount()),
			},
		}

		if err := netlink.AddrReplace(hostVeth, &netlinkAddr); err != nil {
			return fmt.Errorf("failed to add an address %s to %s: %w", cidr, hostVeth.Name, err)
		}
	}

	ns, err := netns.GetFromName(conf.Sandbox)

	if err != nil {
		return fmt.Errorf("failed to find netns %s: %w", conf.Sandbox, err)
	}

	if err := setUpFirewall(ns, hostVeth, conf); err != nil {
		return fmt.Errorf("failed to set up firewall: %w", err)
	}

	if err := netlink.LinkSetUp(hostVeth); err != nil {
		return fmt.Errorf("failed to set %s up: %w", hostVeth.Name, err)
	}
	if err := netlink.LinkSetUp(peerVeth); err != nil {
		return fmt.Errorf("failed to set %s up: %w", peerVeth.Name, err)
	}

	return types.PrintResult(conf.PrevResult, conf.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	conf, _, err := loadConfig(args.StdinData)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ns, err := netns.GetFromName(conf.Sandbox)

	if err != nil {
		return fmt.Errorf("failed to find netns %s: %w", conf.Sandbox, err)
	}

	peerNetnsNetlink, err := netlink.NewHandleAt(ns)

	if err != nil {
		return fmt.Errorf("failed to find netns %s: %w", conf.Sandbox, err)
	}

	_, _, err = findVeth(conf.HostVeth, conf.PeerVeth, int(ns), peerNetnsNetlink)

	if err != nil {
		return fmt.Errorf("failed to find veth: %w", err)
	}

	//TODO(tstsuchi): check firewall rules

	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}
