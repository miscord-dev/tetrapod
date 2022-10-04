package splitconn

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	mock_types "github.com/miscord-dev/toxfu/toxfucni/pkg/types/mock"
)

func TestSplitConn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_types.NewMockPacketConn(ctrl)

	bundler := NewBundler(conn)

	aPayload := []byte("A")
	aAddr := netip.AddrPortFrom(netip.MustParseAddr("192.168.1.1"), 1)
	a := bundler.Add(func(b []byte, addr netip.AddrPort) bool {
		return bytes.Equal(b, aPayload)
	})
	defer a.Close()

	bPayload := []byte("B")
	bAddr := netip.AddrPortFrom(netip.MustParseAddr("192.168.1.2"), 2)
	b := bundler.Add(func(b []byte, addr netip.AddrPort) bool {
		return bytes.Equal(b, bPayload)
	})
	defer b.Close()

	allPayload := []byte("C")
	allAddr := netip.AddrPortFrom(netip.MustParseAddr("192.168.1.3"), 3)
	all := bundler.Add(func(b []byte, addr netip.AddrPort) bool {
		return true
	})
	defer all.Close()

	conn.EXPECT().Close().Return(nil).AnyTimes()

	buf := make([]byte, 4096)

	t.Run("A", func(t *testing.T) {
		conn.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(func(arg interface{}) (n int, addr netip.AddrPort, err error) {
			b := arg.([]byte)

			payload := aPayload
			copy(b, payload)
			return len(payload), aAddr, nil
		})

		n, addr, err := a.ReadFrom(buf)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(aPayload, buf[:n]); diff != "" {
			t.Fatal(err)
		}

		if diff := cmp.Diff(aAddr.String(), addr.String()); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("B", func(t *testing.T) {
		conn.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(func(arg interface{}) (n int, addr netip.AddrPort, err error) {
			b := arg.([]byte)

			payload := bPayload
			copy(b, payload)
			return len(payload), bAddr, nil
		})

		n, addr, err := b.ReadFrom(buf)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(bPayload, buf[:n]); diff != "" {
			t.Fatal(err)
		}

		if diff := cmp.Diff(bAddr.String(), addr.String()); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("All", func(t *testing.T) {
		conn.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(func(arg interface{}) (n int, addr netip.AddrPort, err error) {
			b := arg.([]byte)

			payload := allPayload
			copy(b, payload)
			return len(payload), allAddr, nil
		})

		n, addr, err := all.ReadFrom(buf)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(allPayload, buf[:n]); diff != "" {
			t.Fatal(err)
		}

		if diff := cmp.Diff(allAddr.String(), addr.String()); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("Write", func(t *testing.T) {
		conn.EXPECT().WriteTo(gomock.Any(), gomock.Any()).Return(10, nil)

		n, err := a.WriteTo(aPayload, netip.MustParseAddrPort("192.168.1.1:22"))

		if err != nil {
			t.Fatal(err)
		}

		if n != 10 {
			t.Fatal("got", n)
		}
	})
}
