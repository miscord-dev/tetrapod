package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/miscord-dev/toxfu/backend"
	"github.com/miscord-dev/toxfu/signal/signalclient"
	"tailscale.com/net/tstun"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/router"
)

func main() {
	var logger logger.Logf = log.Printf

	name := "tailscale0"
	dev, devName, err := tstun.New(logger, name)

	if err != nil {
		tstun.Diagnose(logger, name)
		panic(fmt.Errorf("tstun.New(%q): %w", name, err))
	}

	r, err := router.New(logger, dev, nil)
	if err != nil {
		panic(fmt.Errorf("router.New(%q): %w", devName, err))
	}

	config := wgengine.Config{
		Tun:    dev,
		Router: r,
	}

	engine, err := wgengine.NewUserspaceEngine(logger, config)

	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	target := ""

	client, err := signalclient.New(ctx, target, logger)

	if err != nil {
		panic(err)
	}

	backend := backend.New(&backend.Config{
		SignalClient: client,
		Engine:       engine,
		Logger:       logger,
	})

	backend.Start()

	c, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	<-c.Done()
	backend.Start()
}
