package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/miscord-dev/toxfu/pkg/monitor"
	"github.com/miscord-dev/toxfu/toxfuengine"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func main() {
	rand.Seed(time.Now().Unix())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	addr, err := netlink.ParseAddr(fmt.Sprintf("192.168.72.%d/24", rand.Intn(254)+1))
	if err != nil {
		panic(err)
	}

	config := &toxfuengine.Config{
		PrivateKey:   privKey.String(),
		ListenPort:   57134,
		STUNEndpoint: "stun.l.google.com:19302",
		Addresses:    []netlink.Addr{*addr},
	}

	e, err := toxfuengine.New("toxfu0", config, logger)

	if err != nil {
		panic(err)
	}
	defer e.Close()

	e.Notify(func(pc toxfuengine.PeerConfig) {
		json.NewEncoder(os.Stdout).Encode(pc)
	})

	mon, err := monitor.New()

	if err != nil {
		panic(err)
	}
	defer mon.Close()

	subscriber := mon.Subscribe()
	defer subscriber.Close()

	ticker := time.NewTicker(30 * time.Second)

	go func() {
		for {
			var peer string
			fmt.Scan(&peer)

			var peerConfig toxfuengine.PeerConfig
			if err := json.NewDecoder(strings.NewReader(peer)).Decode(&peerConfig); err != nil {
				fmt.Println(err)

				continue
			}
			fmt.Println(peerConfig)

			config.Peers = []toxfuengine.PeerConfig{peerConfig}

			e.Reconfig(config)
		}
	}()

	for {
		select {
		case <-subscriber.C():
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		e.Trigger()
	}
}
