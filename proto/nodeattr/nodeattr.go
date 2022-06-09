package nodeattr

import (
	"fmt"
	"os"
	"runtime"

	"github.com/hashicorp/go-envparse"
	"github.com/miscord-dev/toxfu/proto"
	"tailscale.com/types/logger"
)

func NewNodeAttribute(logger logger.Logf) *proto.NodeAttribute {
	osVersion, err := readOSRelease()

	if err != nil {
		logger("failed to read /etc/os-release: %v", err)

		osVersion = "unknown"
	}

	hostname, err := os.Hostname()

	if err != nil {
		logger("failed to read hostname: %v", err)

		hostname = "unknown"
	}

	return &proto.NodeAttribute{
		Os:       osVersion,
		Goos:     runtime.GOOS,
		Goarch:   runtime.GOARCH,
		HostName: hostname,
	}
}

func readOSRelease() (string, error) {
	fp, err := os.Open("/etc/os-release")

	if err != nil {
		return "", fmt.Errorf("failed to load /etc/os-release: %w", err)
	}

	parsed, err := envparse.Parse(fp)

	if err != nil {
		return "", err
	}

	return parsed["NAME"] + " " + parsed["VERSION"], nil
}
