package discotests

import (
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miscord-dev/toxfu/toxfucni/disco"
	mock_disco "github.com/miscord-dev/toxfu/toxfucni/disco/mock"
	"github.com/miscord-dev/toxfu/toxfucni/disco/ticker"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/testutil"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/wgkey"
	"go.uber.org/atomic"
	"go.uber.org/zap/zaptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newDiscoPrivKey(t GinkgoTInterface) wgkey.DiscoPrivateKey {
	t.Helper()

	privKey, err := wgkey.New()

	if err != nil {
		t.Fatal(err)
	}

	return privKey
}

type statusGetter func() disco.DiscoPeerEndpointStatusReadOnly

func initDiscoPeerEndpoint(t GinkgoTInterface, filter func(*disco.DiscoPacket) bool) (statusGetter, io.Closer) {
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

		go dpe.EnqueueReceivedPacket(dec)
	}).AnyTimes()

	dpe = disco.NewDiscoPeerEndpoint(1, netip.MustParseAddrPort("192.168.1.2:65432"),
		localPrivKey.Public(), sharedKey, sender, logger)

	var lock sync.Mutex
	var status disco.DiscoPeerEndpointStatusReadOnly
	getStatus := func() disco.DiscoPeerEndpointStatusReadOnly {
		lock.Lock()
		defer lock.Unlock()

		return status
	}
	dpe.Status().NotifyStatus(func(s disco.DiscoPeerEndpointStatusReadOnly) {
		lock.Lock()
		defer lock.Unlock()

		status = s
	})

	return getStatus, dpe
}

func TestDisco(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disco Suite")
}

var _ = Describe("DiscoPeerEndpoint", func() {
	var statusGetter statusGetter
	var closer io.Closer

	AfterEach(func() {
		closer.Close()
	})

	It("simple", func() {
		statusGetter, closer = initDiscoPeerEndpoint(GinkgoT(), func(dec *disco.DiscoPacket) bool {
			dec.Header = disco.PongMessage

			return true
		})

		Eventually(statusGetter).WithTimeout(3 * time.Second).Should(And(
			HaveField("State", ticker.Connected),
			HaveField("RTT", Not(Equal(time.Duration(0)))),
		))
	})
	It("re-connect", func() {
		var connected atomic.Bool
		connected.Store(true)

		filter := func(dec *disco.DiscoPacket) bool {
			dec.Header = disco.PongMessage

			return connected.Load()
		}

		statusGetter, closer = initDiscoPeerEndpoint(GinkgoT(), filter)

		Eventually(statusGetter).WithTimeout(1 * time.Second).Should(And(
			HaveField("State", ticker.Connected),
			HaveField("RTT", Not(Equal(time.Duration(0)))),
		))

		connected.Store(false)

		Eventually(statusGetter).WithTimeout(4 * time.Second).Should(And(
			HaveField("State", ticker.Connecting),
			HaveField("RTT", Equal(time.Duration(0))),
		))

		connected.Store(true)

		Eventually(statusGetter).WithTimeout(1 * time.Second).Should(And(
			HaveField("State", ticker.Connected),
			HaveField("RTT", Not(Equal(time.Duration(0)))),
		))
	})
})
