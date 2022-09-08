package disco

// import (
// 	"net/netip"
// 	"testing"

// 	"github.com/golang/mock/gomock"
// 	mock_disco "github.com/miscord-dev/toxfu/disco/mock"
// 	"github.com/miscord-dev/toxfu/pkg/wgkey"
// )

// func newDiscoPrivKey(t *testing.T) wgkey.DiscoPrivateKey {
// 	t.Helper()

// 	privKey, err := wgkey.New()

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	return privKey
// }

// func TestDiscoPeerEndpoint(t *testing.T) {
// 	ctrl := gomock.NewController(t)

// 	endpoint := netip.MustParseAddrPort("192.168.1.2:65432")
// 	myPrivKey := newDiscoPrivKey(t)
// 	peerPubKey := newDiscoPrivKey(t).Public()
// 	sharedKey := myPrivKey.Shared(peerPubKey)
// 	sender := mock_disco.NewMockSender(ctrl)

// 	newDiscoPeerEndpoint(1, endpoint, myPrivKey.Public(), sharedKey, sender)
// }
