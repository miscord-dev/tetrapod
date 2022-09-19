package discotests

import (
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miscord-dev/toxfu/disco"
	mock_disco "github.com/miscord-dev/toxfu/disco/mock"
	"github.com/miscord-dev/toxfu/disco/ticker"
	"github.com/miscord-dev/toxfu/pkg/testutil"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/zap/zaptest"
)

func newDiscoPrivKey(t *testing.T) wgkey.DiscoPrivateKey {
	t.Helper()

	privKey, err := wgkey.New()

	if err != nil {
		t.Fatal(err)
	}

	return privKey
}

type statusGetter func() (ticker.State, time.Duration)

func initDiscoPeerEndpoint(t *testing.T, filter func(*disco.DiscoPacket) bool) statusGetter {
	t.Helper()

	ctrl := gomock.NewController(t)
	logger := zaptest.NewLogger(t)

	localPrivKey := newDiscoPrivKey(t)
	peerPrivKey := newDiscoPrivKey(t)
	sharedKey := localPrivKey.Shared(peerPrivKey.Public())

	endpoint := netip.MustParseAddrPort("192.168.1.2:65432")
	sender := mock_disco.NewMockSender(ctrl)

	var dpe disco.DiscoPeerEndpoint

	sender.EXPECT().Send(&testutil.Matcher[*disco.EncryptedDiscoPacket]{
		MatchesFunc: func(x *disco.EncryptedDiscoPacket) bool {
			var dec disco.DiscoPacket
			dec.SharedKey = sharedKey

			if !dec.Decrypt(x) {
				t.Fatal("failed to decrypt packet")
			}
			if dec.Endpoint != endpoint {
				t.Fatalf("endpoint mismatch(expected: %v, got: %v)", endpoint, dec.Endpoint)
			}
			if dec.Header != disco.PingMessage {
				t.Fatalf("header mismatch: %x", dec.Header)
			}
			if wgkey.DiscoPublicKey(dec.SrcPublicDiscoKey) != localPrivKey.Public() {
				t.Fatalf("SrcPublicDiscoKey mismatch(expected: %v, actual: %v)", localPrivKey.Public(), wgkey.DiscoPublicKey(dec.SrcPublicDiscoKey))
			}
			if dec.EndpointID != 1 {
				t.Fatalf("unexpected endpoint id: %v", dec.EndpointID)
			}

			return true
		},
		StringFunc: func() string {
			return "is a valid EncryptedDiscoPacket"
		},
	}).Do(func(x *disco.EncryptedDiscoPacket) {
		var dec disco.DiscoPacket
		dec.SharedKey = sharedKey

		if !dec.Decrypt(x) {
			t.Fatal("failed to decrypt packet")
		}

		if !filter(&dec) {
			return
		}

		dpe.EnqueueReceivedPacket(dec)
	}).AnyTimes()

	dpe = disco.NewDiscoPeerEndpoint(1, netip.MustParseAddrPort("192.168.1.2:65432"),
		localPrivKey.Public(), sharedKey, sender, logger)

	var status struct {
		lock   sync.Mutex
		status ticker.State
		rtt    time.Duration
	}
	getStatus := func() (ticker.State, time.Duration) {
		status.lock.Lock()
		defer status.lock.Unlock()

		return status.status, status.rtt
	}
	dpe.Status().NotifyStatus(func(s disco.DiscoPeerEndpointStatusReadOnly) {
		status.lock.Lock()
		defer status.lock.Unlock()
		status.status = s.State
		status.rtt = s.RTT
	})

	t.Log("a pub key", localPrivKey.Public())
	t.Log("b pub key", localPrivKey.Public())

	return getStatus
}

func TestDiscoPeerEndpointSimple(t *testing.T) {
	filter := func(dec *disco.DiscoPacket) bool {
		dec.Header = disco.PongMessage

		return true
	}

	getStatus := initDiscoPeerEndpoint(t, filter)

	time.Sleep(1 * time.Second)

	if status, rtt := getStatus(); status != ticker.Connected {
		t.Error("not connected", status)
	} else if rtt == 0 || rtt >= 1*time.Second {
		t.Error("invalid duration", rtt)
	}
}

func TestDiscoPeerEndpointDisconnectAfterConnected(t *testing.T) {
	var connected atomic.Bool
	connected.Store(true)

	filter := func(dec *disco.DiscoPacket) bool {
		dec.Header = disco.PongMessage

		return connected.Load()
	}

	getStatus := initDiscoPeerEndpoint(t, filter)

	time.Sleep(1 * time.Second)

	if status, rtt := getStatus(); status != ticker.Connected {
		t.Error("not connected", status)
	} else if rtt == 0 || rtt >= 1*time.Second {
		t.Error("invalid duration", rtt)
	}
	connected.Store(false)

	time.Sleep(4 * time.Second)

	if status, rtt := getStatus(); status != ticker.Connecting {
		t.Error("connected", status)
	} else if rtt != 0 {
		t.Error("invalid duration", rtt)
	}

	connected.Store(true)

	time.Sleep(1 * time.Second)

	if status, rtt := getStatus(); status != ticker.Connected {
		t.Error("not connected", status)
	} else if rtt == 0 || rtt >= 1*time.Second {
		t.Error("invalid duration", rtt)
	}
}
