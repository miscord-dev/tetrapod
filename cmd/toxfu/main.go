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
	"tailscale.com/types/key"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/monitor"
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
	defer dev.Close()

	mon, err := monitor.New(logger)

	if err != nil {
		panic(err)
	}
	defer mon.Close()

	r, err := router.New(logger, dev, mon)
	if err != nil {
		panic(fmt.Errorf("router.New(%q): %w", devName, err))
	}
	defer r.Close()

	config := wgengine.Config{
		Tun:    dev,
		Router: r,
	}

	engine, err := wgengine.NewUserspaceEngine(logger, config)

	if err != nil {
		panic(err)
	}
	defer func() {
		engine.Close()
		engine.Wait()
	}()

	ctx := context.Background()
	target := "192.168.1.16:50051"

	logger("connecting")

	client, err := signalclient.New(ctx, target, logger)

	if err != nil {
		panic(err)
	}
	defer client.Close()

	client.Start()

	logger("connected?")

	backend := backend.New(&backend.Config{
		SignalClient: client,
		Engine:       engine,
		Logger:       logger,
		NodePrivate:  key.NewNode(),
	})
	defer backend.Close()

	backend.Start()

	c, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	<-c.Done()
}
