package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/nsutil"
	"github.com/vishvananda/netns"
)

type NetConf struct {
	types.NetConf
	Plugin  string `json:"plugin"`
	Sandbox string `json:"sandbox"`
}

func main() {
	skel.PluginMain(cmd, cmd, cmd, version.All, bv.BuildString("none"))
}

func parseConf(data []byte) (*NetConf, error) {
	conf := &NetConf{}
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse")
	}

	return conf, nil
}

func cmd(args *skel.CmdArgs) error {
	netConf, err := parseConf(args.StdinData)

	if err != nil {
		return err
	}

	handle, err := netns.GetFromName(netConf.Sandbox)

	if err != nil {
		return err
	}

	return nsutil.RunInNamespace(handle, func() error {
		cmd := exec.Command(netConf.Plugin)

		cmd.Stdin = bytes.NewReader(args.StdinData)

		return cmd.Run()
	})
}
