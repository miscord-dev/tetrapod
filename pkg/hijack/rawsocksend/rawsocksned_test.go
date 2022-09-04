package rawsocksend

import (
	"fmt"
	"net"
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSend(t *testing.T) {
	senderPort := 14613
	sender, err := NewSender(senderPort)

	if err != nil {
		t.Fatal(err)
	}

	receiver, err := net.ListenUDP("udp", nil)

	if err != nil {
		t.Fatal(err)
	}

	receiverPort := receiver.LocalAddr().(*net.UDPAddr).Port

	payload := []byte("Hello")

	t.Run("v4", func(t *testing.T) {
		receiverAddrPort := netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", receiverPort))
		senderAddrPort := netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", senderPort))

		if err := sender.Send(payload, receiverAddrPort); err != nil {
			t.Fatal(err)
		}

		p := make([]byte, 1024)
		n, addrPort, err := receiver.ReadFromUDPAddrPort(p)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(payload, p[:n]); diff != "" {
			t.Error(diff)
		}
		addrPort = netip.AddrPortFrom(addrPort.Addr().Unmap(), addrPort.Port())

		if addrPort != senderAddrPort {
			t.Errorf("expected: %v, but got %v", senderAddrPort, addrPort)
		}
	})

	t.Run("v6", func(t *testing.T) {
		receiverAddrPort := netip.MustParseAddrPort(fmt.Sprintf("[::1]:%d", receiverPort))
		senderAddrPort := netip.MustParseAddrPort(fmt.Sprintf("[::1]:%d", senderPort))

		if err := sender.Send(payload, receiverAddrPort); err != nil {
			t.Fatal(err)
		}

		p := make([]byte, 1024)
		n, addrPort, err := receiver.ReadFromUDPAddrPort(p)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(payload, p[:n]); diff != "" {
			t.Error(diff)
		}
		addrPort = netip.AddrPortFrom(addrPort.Addr().Unmap(), addrPort.Port())

		if addrPort != senderAddrPort {
			t.Errorf("expected: %v, but got %v", senderAddrPort, addrPort)
		}
	})

}
