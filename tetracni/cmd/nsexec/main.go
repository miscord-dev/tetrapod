package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/miscord-dev/tetrapod/pkg/nsutil"
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

	_, _ = nsutil.CreateNamespace(netConf.Sandbox)

	handle, err := netns.GetFromName(netConf.Sandbox)

	if err != nil {
		return err
	}

	paths := filepath.SplitList(os.Getenv("CNI_PATH"))
	pluginPath, err := invoke.FindInPath(netConf.Plugin, paths)
	if err != nil {
		return err
	}

	return nsutil.RunInNamespace(handle, func() error {
		cmd := exec.Command(pluginPath)

		cmd.Stdin = bytes.NewReader(args.StdinData)

		var buf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &buf)

		err := cmd.Run()

		if err != nil {
			return fmt.Errorf("executing plugin failed %s: %w", buf.String(), err)
		}

		return nil
	})
}
