package alarm

func NewCombinator(a, b Subscriber) Subscriber {
	t := &combinator{
		ch:     make(chan struct{}),
		closed: make(chan struct{}),
	}

	go func() {
		defer close(t.ch)
		defer a.Close()
		defer b.Close()

		for {
			select {
			case <-a.C():
			case <-b.C():
			case <-t.closed:
				return
			}

			t.ch <- struct{}{}
		}
	}()

	return t
}

type combinator struct {
	ch     chan struct{}
	closed chan struct{}
}

func (t *combinator) C() <-chan struct{} {
	return t.ch
}

func (t *combinator) Close() error {
	defer func() {
		recover()
	}()

	close(t.closed)

	return nil
}
