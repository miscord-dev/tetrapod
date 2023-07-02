package main

import (
	"fmt"

	"github.com/coreos/go-iptables/iptables"
	"github.com/miscord-dev/tetrapod/pkg/nsutil"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	iptablesFilterTable = "filter"
)

var (
	iptablesChains = []string{
		"INPUT",
		"OUTPUT",
		"FORWARD",
	}
)

func setUpIPTables(ipt *iptables.IPTables, peerVeth *netlink.Veth, redirectedChain string) error {
	exists, err := ipt.ChainExists(iptablesFilterTable, redirectedChain)

	if err != nil {
		return fmt.Errorf("failed to check the existence of %s: %w", redirectedChain, err)
	}

	if !exists {
		if err := ipt.NewChain(iptablesFilterTable, redirectedChain); err != nil {
			return fmt.Errorf("failed to add a chain %s: %w", redirectedChain, err)
		}
	}

	for _, chain := range iptablesChains {
		rule := []string{"-j", redirectedChain}

		if err := ipt.AppendUnique(iptablesFilterTable, chain, rule...); err != nil {
			return fmt.Errorf("failed to add a rule %v to %s/%s: %w", rule, iptablesFilterTable, chain, err)
		}
	}

	rules := [][]string{
		{"-i", peerVeth.Name, "-j", "ACCEPT"},
		{"-o", peerVeth.Name, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		{"-o", peerVeth.Name, "-j", "DROP"},
	}

	for _, rule := range rules {
		if err := ipt.AppendUnique(iptablesFilterTable, redirectedChain, rule...); err != nil {
			return fmt.Errorf("failed to append rules %v to %s/%s: %w", rule, iptablesFilterTable, redirectedChain, err)
		}
	}

	return nil
}

func setUpFirewall(peerNetns netns.NsHandle, peerVeth *netlink.Veth, conf *Conf) error {
	switch conf.Firewall {
	case FirewallNever:
		return nil
	case FirewallIPTables:
		err := nsutil.RunInNamespace(peerNetns, func() error {
			ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)

			if err != nil {
				return fmt.Errorf("failed to set up iptables: %w", err)
			}

			if err := setUpIPTables(ipt, peerVeth, conf.IPTablesChain); err != nil {
				return fmt.Errorf("failed to set up iptables for v4: %w", err)
			}

			ipt, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)

			if err != nil {
				return fmt.Errorf("failed to set up iptables: %w", err)
			}

			if err := setUpIPTables(ipt, peerVeth, conf.IPTablesChain); err != nil {
				return fmt.Errorf("failed to set up iptables for v4: %w", err)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("manipulating iptables in %s netns failed: %w", peerNetns, err)
		}
	}

	return nil
}
