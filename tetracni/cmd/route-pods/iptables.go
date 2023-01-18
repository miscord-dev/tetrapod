package main

import (
	"fmt"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
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

func setUpIPTables(ipt *iptables.IPTables, hostVeth *netlink.Veth, redirectedChain string) error {
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
		{"-o", hostVeth.Name, "-j", "ACCEPT"},
		{"-i", hostVeth.Name, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		{"-i", hostVeth.Name, "-j", "DROP"},
	}

	for _, rule := range rules {
		if err := ipt.AppendUnique(iptablesFilterTable, redirectedChain, rule...); err != nil {
			return fmt.Errorf("failed to append rules %v to %s/%s: %w", rule, iptablesFilterTable, redirectedChain, err)
		}
	}

	return nil
}

func setUpFirewall(hostVeth *netlink.Veth, conf *Conf) error {
	switch conf.Firewall {
	case FirewallNever:
		return nil
	case FirewallIPTables:
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)

		if err != nil {
			return fmt.Errorf("failed to set up iptables: %w", err)
		}

		if err := setUpIPTables(ipt, hostVeth, conf.IPTablesChain); err != nil {
			return fmt.Errorf("failed to set up iptables for v4: %w", err)
		}

		ipt, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)

		if err != nil {
			return fmt.Errorf("failed to set up iptables: %w", err)
		}

		if err := setUpIPTables(ipt, hostVeth, conf.IPTablesChain); err != nil {
			return fmt.Errorf("failed to set up iptables for v4: %w", err)
		}
	}

	return nil
}
