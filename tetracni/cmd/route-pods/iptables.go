package main

import (
	"fmt"
	"strings"

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
		rule := fmt.Sprintf("-j %s", redirectedChain)

		if err := ipt.AppendUnique(iptablesFilterTable, chain, rule); err != nil {
			return fmt.Errorf("failed to add a rule %s to %s/%s: %w", rule, iptablesFilterTable, chain, err)
		}
	}

	rules := []string{
		fmt.Sprintf("-o %s -j ACCEPT", hostVeth.Name),
		fmt.Sprintf("-i %s -m state --state RELATED,ESTABLISHED -j ACCEPT", hostVeth.Name),
		fmt.Sprintf("-i %s -j REJECT --reject-with icmp-port-unreachable", hostVeth.Name),
	}

	if err := ipt.AppendUnique(iptablesFilterTable, redirectedChain, rules...); err != nil {
		return fmt.Errorf("failed to append rules %s to %s/%s", strings.Join(rules, ", "), iptablesFilterTable, redirectedChain)
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
