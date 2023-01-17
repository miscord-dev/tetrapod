package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/plugins/plugins/ipam/host-local/backend/allocator"
	"github.com/miscord-dev/tetrapod/tetrad/pkg/cniserver"
	"github.com/seancfoley/ipaddress-go/ipaddr"
)

type IPAM struct {
	SocketPath     string `json:"socketPath"`
	IPAMPlugin     string `json:"ipamPlugin"`
	ReserveAddress string `json:"reserve_address"`
}

type Config struct {
	IPAM IPAM `json:"ipam"`
}

const (
	ReserveAddressLast = "last"
	ReserveAddressOff  = "off"
)

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
		config.IPAM.IPAMPlugin = "host-local"
	}
	switch config.IPAM.ReserveAddress {
	case "":
		config.IPAM.ReserveAddress = ReserveAddressLast
	case ReserveAddressLast, ReserveAddressOff:
		// ok
	default:
		return nil, nil, fmt.Errorf("unknown reserve_address: %s", config.IPAM.ReserveAddress)
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

		ipAddr, _ := ipaddr.NewIPAddressFromNetIPNet(cidr)
		rangeStart := ipAddr.GetLower().Increment(1)
		rangeEnd := ipAddr.GetUpper().Increment(-1)

		switch config.IPAM.ReserveAddress {
		case ReserveAddressLast:
			rangeEnd = rangeEnd.Increment(-1)
		}

		netConfig.RuntimeConfig.IPRanges = append(netConfig.RuntimeConfig.IPRanges, []allocator.Range{
			{
				Subnet:     types.IPNet(*cidr),
				RangeStart: rangeStart.GetNetIP(),
				RangeEnd:   rangeEnd.GetNetIP(),
			},
		})
	}

	return nil
}

func MarshalConfig(net *allocator.Net) []byte {
	b, _ := json.Marshal(net)

	return b
}
