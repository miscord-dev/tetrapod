package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/miscord-dev/tetrapod/tetrad/pkg/cniserver"
)

type Conf struct {
	types.NetConf

	SocketPath string `json:"socketPath"`
	VRF        string `json:"vrf"`
	Args       *Args  `json:"-"`
}

type Args struct {
	types.CommonArgs
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

func loadConfig(stdin []byte, cniArgs string) (*Conf, *current.Result, error) {
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

	var args Args
	if err := types.LoadArgs(cniArgs, &args); err != nil {
		return nil, nil, fmt.Errorf("failed to load args: %w", err)
	}
	conf.Args = &args

	return &conf, result, nil
}

func loadExtraRoutes(config *Conf) ([]string, error) {
	client := cniserver.NewClient(config.SocketPath)

	cidrClaimList, err := client.GetExtraPodCIDRs(string(config.Args.K8S_POD_NAMESPACE), string(config.Args.K8S_POD_NAME))

	if err != nil {
		return nil, fmt.Errorf("failed to get pod CIDRs: %w", err)
	}

	cidrs := make([]string, 0, len(cidrClaimList.Items))
	for _, claim := range cidrClaimList.Items {
		if claim.Status.CIDR == "" {
			continue
		}

		cidrs = append(cidrs, claim.Status.CIDR)
	}

	return cidrs, nil
}
