package syncutil

import "sync"

type Cond struct {
	cond   *sync.Cond
	closed chan struct{}
}

func New() *Cond {
	return &Cond{
		cond:   sync.NewCond(&sync.Mutex{}),
		closed: make(chan struct{}),
	}
}

func (c *Cond) Broadcast() {
	c.cond.Broadcast()
}

func (c *Cond) Signal() {
	c.cond.Signal()
}

func (c *Cond) NewSubscriber() *Subscriber {
	ch := make(chan struct{})
	s := &Subscriber{
		cond:    c,
		closed:  c.closed,
		sclosed: make(chan struct{}),
		ch:      ch,
		C:       ch,
	}

	s.run()

	return s
}

func (c *Cond) Close() error {
	close(c.closed)
	c.Broadcast()

	return nil
}

type Subscriber struct {
	cond    *Cond
	closed  <-chan struct{}
	sclosed chan struct{}

	ch chan<- struct{}
	C  <-chan struct{}
}

func (s *Subscriber) run() {
	go func() {
		s.cond.cond.Wait()

		select {
		case <-s.closed:
			close(s.ch)

			return
		case <-s.sclosed:
			close(s.ch)

			return
		case s.ch <- struct{}{}:
		}
	}()
}

func (s *Subscriber) Close() error {
	close(s.sclosed)

	return nil
}
