package pktmgr

import (
	"time"

	"github.com/miscord-dev/toxfu/pkg/syncmap"
)

// Manager is a utility to manage rtt/dropped packets.
type Manager struct {
	m            syncmap.Map[uint32, time.Time]
	timeout      time.Duration
	dropCallback func()
}

func New(timeout time.Duration, dropCallback func()) *Manager {
	return &Manager{
		m:            syncmap.Map[uint32, time.Time]{},
		timeout:      timeout,
		dropCallback: dropCallback,
	}
}

func (m *Manager) AddPacket(id uint32) {
	m.m.Store(id, time.Now())

	after := time.After(m.timeout)
	go func() {
		<-after

		m.m.Delete(id)
		if m.dropCallback != nil {
			m.dropCallback()
		}
	}()
}

func (m *Manager) RecvAck(id uint32) (time.Duration, bool) {
	recvedAt, ok := m.m.LoadAndDelete(id)

	if !ok {
		return 0, false
	}

	now := time.Now()

	m.m.Range(func(key uint32, value time.Time) bool {
		if recvedAt.After(value) {
			m.m.Delete(key)
		}

		return true
	})

	return now.Sub(recvedAt), true
}
