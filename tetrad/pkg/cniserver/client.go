package cniserver

import (
	"fmt"
	"net/rpc"

	controlplanev1alpha1 "github.com/miscord-dev/tetrapod/controlplane/api/v1alpha1"
)

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
	}
}

type Client struct {
	socketPath string
}

func (c *Client) rpcCall(method string, args, reply interface{}) error {
	client, err := rpc.DialHTTP("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("error dialing DHCP daemon: %w", err)
	}

	err = client.Call(method, args, reply)
	if err != nil {
		return fmt.Errorf("error calling %v: %w", method, err)
	}

	return nil
}

func (c *Client) GetPodCIDRs() (*controlplanev1alpha1.CIDRClaimList, error) {
	var claimList controlplanev1alpha1.CIDRClaimList

	err := c.rpcCall("Handler.GetPodCIDRs", &GetPodCIDRsArgs{}, &claimList)

	if err != nil {
		return nil, fmt.Errorf("calling Handler.GetPodCIDRs failed: %w", err)
	}

	return &claimList, nil
}

func (c *Client) GetExtraPodCIDRs(namespace, name string) (*controlplanev1alpha1.CIDRClaimList, error) {
	var claimList controlplanev1alpha1.CIDRClaimList

	err := c.rpcCall("Handler.GetExtraPodCIDRs", &GetExtraPodCIDRsArgs{
		Namespace: namespace,
		Name:      name,
	}, &claimList)

	if err != nil {
		return nil, fmt.Errorf("calling Handler.GetExtraPodCIDRs failed: %w", err)
	}

	return &claimList, nil
}
