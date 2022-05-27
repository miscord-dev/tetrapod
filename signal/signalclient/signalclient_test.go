package signalclient

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miscord-dev/toxfu/proto"
	mock_proto "github.com/miscord-dev/toxfu/proto/mock"
	mock_signalclient "github.com/miscord-dev/toxfu/signal/signalclient/mock"
	"tailscale.com/types/logger"
)

var (
	nopLogger = logger.Logf(func(format string, args ...any) {
		// do nothing
	})
)

func TestRefreshRecv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_proto.NewMockNodeAPIClient(ctrl)
	refreshClient := mock_proto.NewMockNodeAPI_RefreshClient(ctrl)
	recvCallback := mock_signalclient.NewMockRecvCallbackInterface(ctrl)

	nrresp := &proto.NodeRefreshResponse{}

	// Ensure Refresh is called after Start or Recv returned an error
	client.EXPECT().Refresh(gomock.Any()).Return(refreshClient, nil).Times(1)

	// After Refresh(), Recv() is called
	refreshClient.EXPECT().Recv().Return(nrresp, nil).MinTimes(1)

	// Then, callback is called
	recvCallback.EXPECT().Call(nrresp).MinTimes(1)

	signalClient := new(io.NopCloser(nil), client, nopLogger)

	signalClient.RegisterRecvCallback(recvCallback.Call)
	signalClient.Start()

	time.Sleep(100 * time.Millisecond)

	signalClient.Stop()
	signalClient.Close()
}

func TestRefreshSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_proto.NewMockNodeAPIClient(ctrl)
	refreshClient := mock_proto.NewMockNodeAPI_RefreshClient(ctrl)
	recvCallback := mock_signalclient.NewMockRecvCallbackInterface(ctrl)

	nrreq := &proto.NodeRefreshRequest{}
	nrresp := &proto.NodeRefreshResponse{}

	// Ensure Refresh is called after Start or Recv returned an error
	client.EXPECT().Refresh(gomock.Any()).Return(refreshClient, nil).Times(1)

	// After Refresh(), Recv() is called
	refreshClient.EXPECT().Recv().Return(nrresp, nil).MinTimes(1)
	recvCallback.EXPECT().Call(nrresp).MinTimes(1)

	// Ensure Send is called
	refreshClient.EXPECT().Send(nrreq).Return(nil).Times(1)

	signalClient := new(io.NopCloser(nil), client, nopLogger)

	signalClient.RegisterRecvCallback(recvCallback.Call)
	signalClient.Start()

	time.Sleep(100 * time.Millisecond)
	if err := signalClient.Send(nrreq); err != nil {
		t.Fatal(err)
	}

	signalClient.Stop()
	signalClient.Close()
}

func TestRefreshNotEstablished(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_proto.NewMockNodeAPIClient(ctrl)

	nrreq := &proto.NodeRefreshRequest{}

	// Ensure Refresh is called after Start or Recv returned an error
	signalClient := new(io.NopCloser(nil), client, nopLogger)

	if err := signalClient.Send(nrreq); err != ErrConnectionNotEstablished {
		t.Fatal(err)
	}

	signalClient.Stop()
	signalClient.Close()
}

func TestRefreshReconnect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_proto.NewMockNodeAPIClient(ctrl)
	refreshClient := mock_proto.NewMockNodeAPI_RefreshClient(ctrl)
	recvCallback := mock_signalclient.NewMockRecvCallbackInterface(ctrl)

	nrresp := &proto.NodeRefreshResponse{}

	// Ensure Refresh is called after Start or Recv returned an error
	firstErrorCall := client.EXPECT().Refresh(gomock.Any()).Return(nil, fmt.Errorf("error")).Times(1)
	client.EXPECT().Refresh(gomock.Any()).Return(refreshClient, nil).Times(1).After(firstErrorCall)

	// After Refresh(), Recv() is called
	refreshClient.EXPECT().Recv().Return(nrresp, nil).MinTimes(1)

	// Then, callback is called
	recvCallback.EXPECT().Call(nrresp).MinTimes(1)

	signalClient := new(io.NopCloser(nil), client, nopLogger)

	signalClient.RegisterRecvCallback(recvCallback.Call)
	signalClient.Start()

	time.Sleep(1200 * time.Millisecond)

	signalClient.Stop()
	signalClient.Close()
}
