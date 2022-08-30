package monitor

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/mdlayher/netlink"
	"github.com/miscord-dev/toxfu/pkg/alarm"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

type Monitor interface {
	Subscribe() *alarm.Subscriber
	io.Closer
}

type monitor struct {
	alarm   *alarm.Alarm
	conn    *netlink.Conn
	stopped atomic.Bool
	Logger  *zap.Logger
}

func New() (Monitor, error) {
	var flag uint32 = unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR |
		unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE |
		unix.RTMGRP_IPV4_RULE

	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{
		Groups: flag,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}

	m := &monitor{
		conn:   conn,
		Logger: zap.NewNop(),
	}
	go m.run()

	return m, nil
}

func (m *monitor) run() {
	for {
		if m.stopped.Load() {
			return
		}

		msgs, err := m.conn.Receive()

		if err != nil {
			if m.stopped.Load() {
				return
			}

			m.Logger.Error("receive failed", zap.Error(err))
			time.Sleep(10 * time.Second)

			continue
		}

		if len(msgs) == 0 {
			continue
		}

	}
}

func (m *monitor) Subscribe() *alarm.Subscriber {
	return m.alarm.Subscribe()
}

func (m *monitor) Close() error {
	m.stopped.Store(true)

	err := m.conn.Close()

	if err != nil {
		return fmt.Errorf("failed to close: %w", err)
	}

	return err
}
