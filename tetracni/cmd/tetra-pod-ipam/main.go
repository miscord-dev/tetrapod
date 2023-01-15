package main

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/ipam"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/containernetworking/plugins/plugins/ipam/host-local/backend/allocator"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDelete, version.All, bv.BuildString("tetra-pod-ipam"))
}

func prepareConfig(args *skel.CmdArgs) (*allocator.Net, *Config, error) {
	net, config, err := LoadConfig(args.StdinData)
	if err != nil {
		return nil, nil, err
	}

	err = MutateConfig(net, config)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to mutate config: %w", err)
	}

	return net, config, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	net, config, err := prepareConfig(args)

	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	result, err := ipam.ExecAdd(config.IPAM.IPAMPlugin, MarshalConfig(net))

	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", config.IPAM.IPAMPlugin, err)
	}

	return types.PrintResult(result, net.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	net, config, err := prepareConfig(args)

	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	err = ipam.ExecCheck(config.IPAM.IPAMPlugin, MarshalConfig(net))

	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", config.IPAM.IPAMPlugin, err)
	}

	return nil
}

func cmdDelete(args *skel.CmdArgs) error {
	net, config, err := prepareConfig(args)

	if err != nil {
		return fmt.Errorf("failed to prepare config: %w", err)
	}

	err = ipam.ExecDel(config.IPAM.IPAMPlugin, MarshalConfig(net))

	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", config.IPAM.IPAMPlugin, err)
	}

	return nil
}
