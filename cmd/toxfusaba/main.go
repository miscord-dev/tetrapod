package main

import (
	"fmt"
	"log"
	"net"

	"github.com/miscord-dev/toxfu/proto"
	"github.com/miscord-dev/toxfu/signal/signalserver"
	"google.golang.org/grpc"
)

func main() {
	port := 50051
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	nodeAPIServer := signalserver.Server{}

	proto.RegisterNodeAPIServer(server, &nodeAPIServer)

	err = server.Serve(listener)

	if err != nil {
		panic(err)
	}
}
