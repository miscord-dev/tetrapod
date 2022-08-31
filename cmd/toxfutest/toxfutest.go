package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/miscord-dev/toxfu/pkg/monitor"
	"github.com/miscord-dev/toxfu/toxfuengine"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func main() {
	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	addr, err := netlink.ParseAddr("192.168.72.1/24")
	if err != nil {
		panic(err)
	}

	e, err := toxfuengine.New("toxfu0", &toxfuengine.Config{
		PrivateKey:   privKey.String(),
		ListenPort:   57134,
		STUNEndpoint: "stun.l.google.com:19302",
		Addresses:    []netlink.Addr{*addr},
	}, logger)

	if err != nil {
		panic(err)
	}

	e.Notify(func(pc toxfuengine.PeerConfig) {
		json.NewEncoder(os.Stdout).Encode(pc)
	})

	mon, err := monitor.New()

	if err != nil {
		panic(err)
	}

	subscriber := mon.Subscribe()
	defer subscriber.Close()

	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-subscriber.C():
		case <-ticker.C:
		}

		e.Trigger()
	}
}
