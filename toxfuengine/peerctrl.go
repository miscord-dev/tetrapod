package toxfuengine

import "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

type PeerController interface {
	Start()

	Reconfig(*PeerConfig)

	GetWireguardPeer() wgtypes.Peer

	Callback(func())

	Stop()
}
