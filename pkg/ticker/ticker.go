package ticker

import (
	"io"
	"sync"
	"time"
)

type Ticker interface {
	C() <-chan struct{}

	StartSearching()

	Found()

	UseMainLine(flag bool)

	io.Closer
}

type ticker struct {
	ch, reset, closed chan struct{}

	lock            sync.Mutex
	multiplier      float64
	currentInterval time.Duration
	maxInterval     time.Duration
}

func NewTicker() Ticker {
	return &ticker{
		ch:     make(chan struct{}),
		reset:  make(chan struct{}),
		closed: make(chan struct{}),
	}
}

func (t *ticker) C() <-chan struct{} {
	return t.ch
}

func (t *ticker) wake() {
	select {
	case t.ch <- struct{}{}:
	default:
	}
}

func (t *ticker) run() {
	for {
		t.lock.Lock()
		after := time.NewTicker(t.currentInterval)
		t.lock.Unlock()
		select {
		case <-t.reset:
			after.Stop()
			continue
		case <-after.C:
			after.Stop()
		case <-t.closed:
			after.Stop()
			return
		}

		t.wake()

		t.lock.Lock()
		t.currentInterval = time.Duration(float64(t.currentInterval) * t.multiplier)

		if t.currentInterval > t.maxInterval {
			t.currentInterval = t.maxInterval
		}
		t.lock.Unlock()
	}
}

func (t *ticker) StartSearching() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.currentInterval = 100 * time.Millisecond
	t.multiplier = 2
	t.maxInterval = 3 * time.Second
}

func (t *ticker) Found() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.currentInterval = 3 * time.Second
	t.multiplier = 1
}

func (t *ticker) UseMainLine(mainLine bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.multiplier = 1

	t.currentInterval = 10 * time.Second

	if mainLine {
		t.currentInterval = 3 * time.Second
	}
}

func (t *ticker) Close() error {
	close(t.closed)
	return nil
}
