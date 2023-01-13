package discotests

import "github.com/miscord-dev/toxfu/disco"

//go:generate mockgen -source=$GOFILE -package=mock_$GOPACKAGE -destination=./mock/mock_$GOFILE

type DiscoPeerEndpointStatusCallback interface {
	Callback(status disco.DiscoPeerEndpointStatusReadOnly)
}
