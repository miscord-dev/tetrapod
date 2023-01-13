package xdp

import (
	"net"
	"net/netip"
	"os"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/miscord-dev/toxfu/pkg/sets"
	"github.com/miscord-dev/toxfu/pkg/sliceutil"
	"github.com/pion/stun"
)

func TestXDPReceiver(t *testing.T) {
	if os.Getuid() != 0 {
		t.Fatal("This test must be run as root")
	}

	iface, err := net.InterfaceByName("lo")

	if err != nil {
		t.Fatal(err)
	}

	port := 24614
	recverConn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		t.Fatal(err)
	}
	defer recverConn.Close()

	go func() {
		b := make([]byte, 2048)
		for {
			if _, err := recverConn.Read(b); err != nil {
				return
			}
		}
	}()

	senderConn, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(port))

	if err != nil {
		t.Fatal(err)
	}
	defer senderConn.Close()

	senderConnV6, err := net.Dial("udp", "[::1]:"+strconv.Itoa(port))

	if err != nil {
		t.Fatal(err)
	}
	defer senderConnV6.Close()

	netAddrs, err := net.InterfaceAddrs()

	if err != nil {
		t.Fatal(err)
	}

	addrs := sets.FromSlice(sliceutil.Map(netAddrs, func(v net.Addr) netip.Addr {
		ipNet := v.(*net.IPNet)
		c, _ := netip.AddrFromSlice(ipNet.IP)

		return c.Unmap()
	}))

	recver, err := NewXDPReceiver(iface, port, func(a netip.Addr) bool {
		return addrs.Contains(a)
	})

	if err != nil {
		t.Fatal(err)
	}
	defer recver.Close()

	t.Log("Start receiving on ", port)

	t.Run("disco packet", func(t *testing.T) {
		b := make([]byte, 2048)

		expected := []byte{0xff, 0xff, 0xff, 0xff, 0xff}

		if _, err := senderConn.Write(expected); err != nil {
			t.Fatal(err)
		}

		len, src, err := recver.Recv(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if src.Addr() != netip.MustParseAddr("127.0.0.1") {
			t.Error("src is ", src.Addr())
		}

		if src.Port() == 0 {
			t.Error("src port is zero")
		}

		t.Log("received from", src)
	})

	t.Run("stun packet", func(t *testing.T) {
		b := make([]byte, 2048)

		message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		expected := message.Raw

		if _, err := senderConn.Write(expected); err != nil {
			t.Fatal(err)
		}

		len, src, err := recver.Recv(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if src.Addr() != netip.MustParseAddr("127.0.0.1") {
			t.Error("src is ", src.Addr())
		}

		if src.Port() == 0 {
			t.Error("src port is zero")
		}

		t.Log("received from", src)
	})

	t.Run("disco packet v6", func(t *testing.T) {
		b := make([]byte, 2048)

		expected := []byte{0xff, 0xff, 0xff, 0xff, 0xff}

		if _, err := senderConnV6.Write(expected); err != nil {
			t.Fatal(err)
		}

		len, src, err := recver.Recv(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if src.Addr() != netip.MustParseAddr("::1") {
			t.Error("src is ", src.Addr())
		}

		if src.Port() == 0 {
			t.Error("src port is zero")
		}

		t.Log("received from", src)
	})

	t.Run("stun packet v6", func(t *testing.T) {
		b := make([]byte, 2048)

		message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		expected := message.Raw

		if _, err := senderConnV6.Write(expected); err != nil {
			t.Fatal(err)
		}

		len, src, err := recver.Recv(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if src.Addr() != netip.MustParseAddr("::1") {
			t.Error("src is ", src.Addr())
		}

		if src.Port() == 0 {
			t.Error("src port is zero")
		}

		t.Log("received from", src)
	})
}
