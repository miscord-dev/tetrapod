package discotests

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miscord-dev/toxfu/disco"
	mock_disco "github.com/miscord-dev/toxfu/disco/mock"
	"github.com/miscord-dev/toxfu/disco/ticker"
	"github.com/miscord-dev/toxfu/pkg/testutil"
	"github.com/miscord-dev/toxfu/pkg/wgkey"
	"go.uber.org/atomic"
	"go.uber.org/zap"
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

type dpe struct {
	dpe          disco.DiscoPeerEndpoint
	endpoint     netip.AddrPort
	localPrivKey wgkey.DiscoPrivateKey
	localPubKey  wgkey.DiscoPublicKey
	sharedKey    wgkey.DiscoSharedKey
	sender       disco.Sender
	logger       *zap.Logger

	pipe chan disco.DiscoPacket

	statusLock sync.Mutex
	status     ticker.State
	rtt        time.Duration

	filter atomic.Pointer[func() bool]
}

func newDPE(
	localPrivKey, peerPrivKey wgkey.DiscoPrivateKey,
	endpoint netip.AddrPort,
	sender disco.Sender,
	logger *zap.Logger,
) *dpe {
	localPubKey := localPrivKey.Public()
	sharedKey := localPrivKey.Shared(peerPrivKey.Public())

	return &dpe{
		endpoint:     endpoint,
		localPrivKey: localPrivKey,
		localPubKey:  localPubKey,
		sharedKey:    sharedKey,
		sender:       sender,
		logger:       logger,
		pipe:         make(chan disco.DiscoPacket, 1),
	}
}

func (d *dpe) start() {
	d.dpe = disco.NewDiscoPeerEndpoint(1, d.endpoint, d.localPubKey, d.sharedKey, d.sender, d.logger)

	d.dpe.Status().NotifyStatus(func(status disco.DiscoPeerEndpointStatusReadOnly) {
		d.statusLock.Lock()
		defer d.statusLock.Unlock()
		d.status = status.State
		d.rtt = status.RTT
	})

	go func() {
		for p := range d.pipe {
			d.dpe.EnqueueReceivedPacket(p)
		}
	}()
}

func (d *dpe) send(pkt disco.DiscoPacket) {
	go func() {
		filter := d.filter.Load()

		if filter != nil && *filter != nil {
			if !(*filter)() {
				return
			}
		}

		d.pipe <- pkt
	}()
}

func (d *dpe) getStatus() (ticker.State, time.Duration) {
	d.statusLock.Lock()
	defer d.statusLock.Unlock()

	return d.status, d.rtt
}

func TestDiscoPeerEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := zaptest.NewLogger(t)

	privKeyA := newDiscoPrivKey(t)
	privKeyB := newDiscoPrivKey(t)

	senderA := mock_disco.NewMockSender(ctrl)

	a := newDPE(privKeyA, privKeyB,
		netip.MustParseAddrPort("192.168.1.2:65432"), senderA, logger.With(zap.String("target", "a")))

	initSender := func(senderDPE, recverDPE *dpe, sender *mock_disco.MockSender) {
		sharedKey := senderDPE.sharedKey

		sender.EXPECT().Send(&testutil.Matcher[*disco.EncryptedDiscoPacket]{
			MatchesFunc: func(x *disco.EncryptedDiscoPacket) bool {
				var dec disco.DiscoPacket
				dec.SharedKey = sharedKey

				if !dec.Decrypt(x) {
					t.Fatal("failed to decrypt packet")
				}
				if dec.Endpoint != senderDPE.endpoint {
					t.Fatalf("endpoint mismatch(expected: %v, got: %v)", senderDPE.endpoint, dec.Endpoint)
				}
				if dec.Header != disco.PingMessage {
					t.Fatalf("header mismatch: %x", dec.Header)
				}
				if wgkey.DiscoPublicKey(dec.SrcPublicDiscoKey) != senderDPE.localPrivKey.Public() {
					t.Fatalf("SrcPublicDiscoKey mismatch(expected: %v, actual: %v)", senderDPE.localPrivKey.Public(), wgkey.DiscoPublicKey(dec.SrcPublicDiscoKey))
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

			if dec.Header == disco.PongMessage {
				recverDPE.send(dec)

				return
			}

			filter := senderDPE.filter.Load()

			if filter != nil && *filter != nil {
				if !(*filter)() {
					return
				}
			}

			senderDPE.dpe.EnqueueReceivedPacket(dec)
		}).AnyTimes()
	}

	filter := func() bool {
		return true
	}

	a.filter.Store(&filter)

	initSender(a, b, senderA)
	initSender(b, a, senderB)

	a.start()
	b.start()

	t.Log("a pub key", a.localPubKey)
	t.Log("b pub key", b.localPubKey)

	time.Sleep(1 * time.Second)

	if status, rtt := a.getStatus(); status != ticker.Connected {
		t.Error("not connected", status)
	} else if rtt == 0 || rtt >= 1*time.Second {
		t.Error("invalid duration", rtt)
	}

}
