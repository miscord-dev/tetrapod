package ipamrpc

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
)

type Handler struct {
	listener net.Listener
}

var (
	unixSocketPath = "/var/run/toxfu.sock"
)

func init() {
	sockPath := os.Getenv("TOXFU_SOCKET")

	if sockPath != "" {
		unixSocketPath = sockPath
	}
}

func NewHandler() (*Handler, error) {
	h := &Handler{}

	rpc := rpc.NewServer()
	rpc.Register(h)

	listener, err := net.Listen("unix", unixSocketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", unixSocketPath, err)
	}

	h.listener = listener

	go rpc.Accept(listener)

	return h, nil
}

func (h *Handler) Close() error {
	return h.listener.Close()
}
