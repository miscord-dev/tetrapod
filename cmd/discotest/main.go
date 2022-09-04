package main

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"

	"github.com/miscord-dev/toxfu/disco"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap"
)

func main() {
	privKey, err := wgkey.New()

	if err != nil {
		panic(err)
	}

	rand.Seed(time.Now().Unix())
	port := 60000 + rand.Intn(1000)

	fmt.Println("publicKey: ", privKey.Public().Marshal())
	fmt.Println("port: ", port)

	var pubKey string
	var peerPort int
	fmt.Scan(&pubKey)
	fmt.Scan(&peerPort)
	// pubKey = "bDxkZ6qkhsV1hHT73oBArJixEDoGdg8dinnuwV88aDo="
	// peerPort = 60013

	peerKey, err := wgkey.Parse(pubKey)

	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewProduction()

	d, err := disco.New(privKey, port, logger)

	if err != nil {
		panic(err)
	}

	peer := d.AddPeer(peerKey)

	peer.SetEndpoints([]netip.AddrPort{
		netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(peerPort)),
	})

	peer.Status().NotifyStatus(func(status disco.DiscoPeerStatusReadOnly) {
		fmt.Println(status)
	})

	d.Close()
}
