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

	VRF string `json:"vrf"`
}

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

	return &conf, result, nil
}
