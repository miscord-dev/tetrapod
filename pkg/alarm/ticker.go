package alarm

import "time"

func NewTicker(interval time.Duration) Subscriber {
	timeTicker := time.NewTicker(interval)

	t := &ticker{
		ch:     make(chan struct{}),
		closed: make(chan struct{}),
	}

	go func() {
		defer close(t.ch)
		defer timeTicker.Stop()

		for {
			select {
			case <-timeTicker.C:
			case <-t.closed:
				return
			}

			t.ch <- struct{}{}
		}
	}()

	return t
}

type ticker struct {
	ch     chan struct{}
	closed chan struct{}
}

func (t *ticker) C() <-chan struct{} {
	return t.ch
}

func (t *ticker) Close() error {
	defer func() {
		recover()
	}()

	close(t.closed)

	return nil
}
