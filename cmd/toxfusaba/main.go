package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/miscord-dev/toxfu/config"
	"github.com/miscord-dev/toxfu/persistent"
	"github.com/miscord-dev/toxfu/persistent/ent"
	"github.com/miscord-dev/toxfu/proto"
	"github.com/miscord-dev/toxfu/signal/signalserver"
	"google.golang.org/grpc"
	"inet.af/netaddr"
	"tailscale.com/types/logger"
)

func main() {
	var logger logger.Logf = log.Printf

	cfg, err := config.NewConfig()

	if err != nil {
		panic(err)
	}

	logger("initializing TCP server for %v", cfg.Port)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	logger("initializing DB")

	entClient, err := ent.Open("sqlite3", cfg.DSN)
	if err != nil {
		panic(err)
	}

	if err := entClient.Schema.Create(context.TODO()); err != nil {
		panic(err)
	}

	logger("initializing gRPC server")

	server := grpc.NewServer()
	pers := persistent.NewEnt(netaddr.MustParseIPPrefix(cfg.Prefix), entClient, 10*time.Second)
	nodeAPIServer := signalserver.NewServer("stun1.l.google.com", logger, pers)

	proto.RegisterNodeAPIServer(server, nodeAPIServer)

	err = server.Serve(listener)

	if err != nil {
		panic(err)
	}
}
