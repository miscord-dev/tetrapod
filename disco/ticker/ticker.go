package ticker

import (
	"io"
	"sync"
	"time"
)

type State int

const (
	Connecting State = iota
	Connected
	unsetState
)

type Priority int

const (
	Primary Priority = iota
	Sub
	unsetPriority
)

// Ticker is a utility to manage when to send ping to peers
type Ticker interface {
	C() <-chan struct{}

	SetState(state State, priority Priority, forceReinit bool)

	io.Closer
}

type ticker struct {
	wakeCh, reset, closed chan struct{}

	lock            sync.Mutex
	multiplier      float64
	currentInterval time.Duration
	maxInterval     time.Duration

	state    State
	priority Priority
}

func NewTicker() Ticker {
	ticker := &ticker{
		wakeCh: make(chan struct{}, 1),
		reset:  make(chan struct{}, 1),
		closed: make(chan struct{}),

		state:    unsetState,
		priority: unsetPriority,
	}
	ticker.SetState(Connecting, Primary, false)
	ticker.wake()

	go ticker.run()

	return ticker
}

func (t *ticker) C() <-chan struct{} {
	return t.wakeCh
}

func (t *ticker) wake() {
	select {
	case t.wakeCh <- struct{}{}:
	default:
	}
}

func (t *ticker) run() {
	runPrev := time.Time{}

	after := time.NewTimer(0)
	defer after.Stop()
	for {
		t.lock.Lock()

		after.Stop()
		select {
		case <-after.C:
		default:
		}
		after.Reset(time.Until(runPrev.Add(t.currentInterval)))

		t.lock.Unlock()

		select {
		case <-t.reset:
			continue
		case <-t.closed:
			return
		case <-after.C:
		}

		t.wake()
		runPrev = time.Now()

		t.lock.Lock()
		t.currentInterval = time.Duration(float64(t.currentInterval) * t.multiplier)

		if t.currentInterval > t.maxInterval {
			t.currentInterval = t.maxInterval
		}
		t.lock.Unlock()
	}
}

func (t *ticker) SetState(state State, priority Priority, forceReinit bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if !forceReinit && t.state == state && t.priority == priority {
		return
	}

	if priority == Primary {
		t.maxInterval = 3 * time.Second
	} else {
		t.maxInterval = 10 * time.Second
	}

	switch state {
	case Connecting:
		t.currentInterval = 100 * time.Millisecond
	case Connected:
		t.currentInterval = 3 * time.Second
	}
	t.multiplier = 2

	t.state = state
	t.priority = priority

	select {
	case t.reset <- struct{}{}:
	default:
	}
}

func (t *ticker) Close() error {
	close(t.closed)
	return nil
}
