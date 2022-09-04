package hijack

import (
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap/zaptest"
)

func TestHijack(t *testing.T) {
	port := 13613
	oppositePort := 44614

	hijackedConn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})

	if err != nil {
		t.Fatal(err)
	}
	defer hijackedConn.Close()
	hijackedLocalAddr := net.UDPAddr{
		IP:   net.ParseIP("10.28.100.114"),
		Port: port,
	}

	oppositeConn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: oppositePort,
	})

	if err != nil {
		t.Fatal(err)
	}
	defer oppositeConn.Close()
	oppositeLocalAddr := net.UDPAddr{
		IP:   net.ParseIP("10.28.100.114"),
		Port: oppositePort,
	}

	logger := zaptest.NewLogger(t)

	conn, err := NewConnWithLogger(port, logger)

	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	t.Run("hijacked -> opposite", func(t *testing.T) {
		expected := []byte("Hello")

		if _, err := hijackedConn.WriteTo(expected, &oppositeLocalAddr); err != nil {
			t.Fatal(err)
		}

		b := make([]byte, 2048)
		len, addr, err := oppositeConn.ReadFrom(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if diff := cmp.Diff(addr.String(), hijackedLocalAddr.String()); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("opposite -> hijacked", func(t *testing.T) {
		expected := append([]byte{0x00}, []byte("Hello")...)

		if _, err := oppositeConn.WriteTo(expected, &hijackedLocalAddr); err != nil {
			t.Fatal(err)
		}

		b := make([]byte, 2048)
		len, addr, err := hijackedConn.ReadFrom(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if diff := cmp.Diff(addr.String(), oppositeLocalAddr.String()); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("hijacker -> opposite", func(t *testing.T) {
		expected := []byte("Hello")

		if _, err := conn.WriteTo(expected, oppositeLocalAddr.AddrPort()); err != nil {
			t.Fatal(err)
		}

		b := make([]byte, 2048)
		len, addr, err := oppositeConn.ReadFrom(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if diff := cmp.Diff(addr.String(), hijackedLocalAddr.String()); diff != "" {
			t.Error(diff)
		}
	})

	time.Sleep(1 * time.Second)
	t.Run("opposite -> hijacker", func(t *testing.T) {
		expected := append([]byte{0x80}, []byte("Hello")...)

		if _, err := oppositeConn.WriteTo(expected, &hijackedLocalAddr); err != nil {
			t.Fatal(err)
		}

		b := make([]byte, 2048)
		len, addr, err := conn.ReadFrom(b)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(expected, b[:len]); diff != "" {
			t.Error(diff)
		}

		if diff := cmp.Diff(net.UDPAddrFromAddrPort(addr).String(), oppositeLocalAddr.String()); diff != "" {
			t.Error(diff)
		}
	})
}
