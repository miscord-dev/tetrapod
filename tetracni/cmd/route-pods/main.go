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
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("hostvrf"))
}

func findVeth(hostVethName, peerVethName string) (hostVeth, peerVeth *netlink.Veth, err error) {
	hostVethLink, err := netlink.LinkByName(hostVethName)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to find veth %s: %w", hostVethName, err)
	}

	hostVeth, ok := hostVethLink.(*netlink.Veth)

	if !ok {
		return nil, nil, fmt.Errorf("link %s is not veth: %s", hostVethName, hostVethLink.Type())
	}

	peerVethLink, err := netlink.LinkByName(peerVethName)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to find veth %s: %w", peerVethName, err)
	}

	peerVeth, ok = peerVethLink.(*netlink.Veth)

	if !ok {
		return nil, nil, fmt.Errorf("link %s is not veth: %s", peerVethName, peerVethLink.Type())
	}

	return
}

func setupVeth(hostVethName, peerVethName string) (hostVeth, peerVeth *netlink.Veth, err error) {
	hostVeth, peerVeth, err = findVeth(hostVethName, peerVethName)

	switch {
	case err == nil:
		return hostVeth, peerVeth, nil
	case errors.As(err, &netlink.LinkNotFoundError{}):
		hostVethLink := &netlink.Veth{
			PeerName: peerVethName,
		}

		if err := netlink.LinkAdd(hostVethLink); err != nil {
			return nil, nil, fmt.Errorf("faile create a veth %s: %w", hostVethName, err)
		}

		return findVeth(hostVethName, peerVethName)
	default:
		return nil, nil, fmt.Errorf("failed to find %s, %s: %w", hostVethName, peerVethName, err)
	}
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, result, err := loadConfig(args.StdinData)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	hostVeth, peerVeth, err := setupVeth(conf.HostVeth, conf.PeerVeth)

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

	if err := netlink.LinkSetMaster(hostVeth, bridge); err != nil {
		return fmt.Errorf("failed to set master of %s %s: %w", hostVeth.Name, bridge.Name, err)
	}

	for _, ip := range result.IPs {
		addr, _ := ipaddr.NewIPAddressFromNetIPNet(&ip.Address)
		cidr, _ := addr.ToZeroHost()

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

	if err := setUpFirewall(hostVeth, conf); err != nil {
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

	_, _, err = findVeth(conf.HostVeth, conf.PeerVeth)

	if err != nil {
		return fmt.Errorf("failed to find veth: %w", err)
	}

	//TODO(tstsuchi): check firewall rules

	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}
