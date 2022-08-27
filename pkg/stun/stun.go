package stun

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/miscord-dev/toxfu/pkg/types"
	"github.com/pion/stun"
	"go.uber.org/zap"
)

type STUN struct {
	client *stun.Client

	trigger chan struct{}
	closed  chan struct{}
	logger  *zap.Logger

	lock sync.RWMutex
	fn   func(netip.AddrPort)
}

func NewSTUN(c types.PacketConn, endpoint string, isV6 bool, logger *zap.Logger) (*STUN, error) {
	addr, err := lookup(endpoint, isV6)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint: %w", err)
	}

	wrapped := &conn{
		packetConn: c,
		addrPort:   addr,
	}

	stunClient, err := stun.NewClient(wrapped)

	if err != nil {
		return nil, fmt.Errorf("failed to initialize stun client: %w", err)
	}

	s := &STUN{
		client:  stunClient,
		closed:  make(chan struct{}),
		trigger: make(chan struct{}, 1),
		logger:  logger,
	}
	go s.run()

	return s, nil
}

func (s *STUN) run() {
	for {
		message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		if err := s.client.Do(message, func(res stun.Event) {
			logger := s.logger.With(zap.String("transaction_id", hex.EncodeToString(message.TransactionID[:])))

			if res.Error != nil {
				logger.Error("stun failed", zap.Error(res.Error))

				return
			}
			// Decoding XOR-MAPPED-ADDRESS attribute from message.
			var xorAddr stun.XORMappedAddress
			if err := xorAddr.GetFrom(res.Message); err != nil {
				logger.Error("getting address failed", zap.Error(res.Error))

				return
			}
			logger.Info("succeed in getting ip address", zap.String("result", xorAddr.String()))

			s.lock.RLock()
			fn := s.fn
			s.lock.RUnlock()

			addr, _ := netip.AddrFromSlice(xorAddr.IP)
			addrPort := netip.AddrPortFrom(addr, uint16(xorAddr.Port))

			fn(addrPort)
		}); err != nil {
			s.logger.Error("sending STUN request failed", zap.Error(err))
		}

		select {
		case <-s.trigger:
		case <-s.closed:
			return
		}
	}
}

func (s *STUN) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *STUN) Notify(fn func(addr netip.AddrPort)) {
	s.lock.Lock()
	s.fn = fn
	s.lock.Unlock()
}

func (s *STUN) Close() error {
	close(s.closed)

	return s.client.Close()
}

func lookup(endpoint string, isV6 bool) (netip.AddrPort, error) {
	endpointAddr, endpointPort, _ := strings.Cut(endpoint, ":")

	port, err := strconv.Atoi(endpointPort)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("parsing port failed: %w", err)
	}

	ips, err := net.LookupIP(endpointAddr)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("failed to lookup ips: %w", err)
	}

	if len(ips) == 0 {
		return netip.AddrPort{}, fmt.Errorf("unknown host: %s", endpoint)
	}
	idx := rand.Intn(len(ips))

	addr, _ := netip.AddrFromSlice(ips[idx])

	return netip.AddrPortFrom(addr, uint16(port)), nil
}
