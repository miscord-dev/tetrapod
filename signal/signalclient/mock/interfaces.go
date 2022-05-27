package mock_signalclient

import "github.com/miscord-dev/toxfu/proto"

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=interfaces.go -destination=mock.go -package=mock_signalclient

// RecvCallbackInterface for tests
type RecvCallbackInterface interface {
	Call(*proto.NodeRefreshResponse)
}
