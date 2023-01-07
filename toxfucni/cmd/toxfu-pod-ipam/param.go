package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/plugins/plugins/ipam/host-local/backend/allocator"
	"github.com/miscord-dev/toxfu/toxfud/pkg/cniserver"
)

type IPAM struct {
	SocketPath string `json:"socketPath"`
	IPAMPlugin string `json:"ipamPlugin"`
}

type Config struct {
	IPAM IPAM `json:"ipam"`
}

func LoadConfig(b []byte) (*allocator.Net, *Config, error) {
	var net allocator.Net
	if err := json.Unmarshal(b, &net); err != nil {
		return nil, nil, err
	}

	var config Config
	if err := json.Unmarshal(b, &config); err != nil {
		return nil, nil, err
	}

	if config.IPAM.SocketPath == "" {
		config.IPAM.SocketPath = cniserver.DefaultSocketPath
	}
	if config.IPAM.IPAMPlugin == "" {
		return nil, nil, fmt.Errorf("IPAM plugin is not set")
	}

	return &net, &config, nil
}

func MutateConfig(netConfig *allocator.Net, config *Config) error {
	client := cniserver.NewClient(config.IPAM.SocketPath)

	cidrClaimList, err := client.GetPodCIDRs()

	if err != nil {
		return fmt.Errorf("failed to get pod CIDRs: %w", err)
	}

	for _, c := range cidrClaimList.Items {
		cidr, err := types.ParseCIDR(c.Status.CIDR)

		if err != nil {
			return fmt.Errorf("failed to parse CIDR %s: %w", c.Status.CIDR, err)
		}

		netConfig.RuntimeConfig.IPRanges = append(netConfig.RuntimeConfig.IPRanges, []allocator.Range{
			{
				Subnet: types.IPNet(*cidr),
			},
		})
	}

	return nil
}

func MarshalConfig(net *allocator.Net) []byte {
	b, _ := json.Marshal(net)

	return b
}
