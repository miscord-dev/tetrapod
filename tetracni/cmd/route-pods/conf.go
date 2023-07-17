package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
)

type Conf struct {
	types.NetConf

	Sandbox       string `json:"sandbox"`
	HostVeth      string `json:"hostVeth"`
	PeerVeth      string `json:"peerVeth"`
	Firewall      string `json:"firewall"`
	IPTablesChain string `json:"iptablesChain"`
}

const (
	FirewallIPTables = "iptables"
	FirewallNever    = "never"
)

func loadConfig(stdin []byte) (*Conf, *current.Result, error) {
	var conf Conf
	if err := json.Unmarshal(stdin, &conf); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal stsin: %w", err)
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return nil, nil, fmt.Errorf("failed to parse prev result: %w", err)
	}

	result, err := current.NewResultFromResult(conf.PrevResult)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve result: %w", err)
	}

	if conf.HostVeth == "" {
		conf.HostVeth = "tetrapod-host"
	}

	if conf.PeerVeth == "" {
		conf.PeerVeth = "tetrapod-net"
	}

	switch conf.Firewall {
	case "":
		conf.Firewall = FirewallIPTables
	case FirewallIPTables:
		// ok
	default:
		return nil, nil, fmt.Errorf("unknown firewall mode: %s", conf.Firewall)
	}

	if conf.IPTablesChain == "" {
		conf.IPTablesChain = "TETRAPOD"
	}

	return &conf, result, nil
}
