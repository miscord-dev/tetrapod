package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
)

func main() {
	skel.PluginMain(cmdAny, cmdAny, cmdAny, version.All, bv.BuildString("toxfu-pod-ipam"))
}

func cmdAny(args *skel.CmdArgs) error {
	net, config, err := LoadConfig(args.StdinData)
	if err != nil {
		return err
	}

	err = MutateConfig(net, config)

	if err != nil {
		return fmt.Errorf("failed to mutate config: %w", err)
	}

	cmd := exec.Command(config.IPAM.IPAMPlugin)
	cmd.Stdin = bytes.NewReader(MarshalConfig(net))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
