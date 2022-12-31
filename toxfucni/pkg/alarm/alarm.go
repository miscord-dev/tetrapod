package alarm

import (
	"io"
	"sync/atomic"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/syncmap"
)

type Alarm struct {
	m       syncmap.Map[int64, *subscriber]
	counter atomic.Int64
}

func New() *Alarm {
	return &Alarm{}
}

func (a *Alarm) WakeUpAll() {
	a.m.Range(func(key int64, value *subscriber) bool {
		value.wake()

		return true
	})
}

func (a *Alarm) Subscribe() Subscriber {
	id := a.counter.Add(1)

	subscirber := &subscriber{
		alarm: a,
		id:    id,
		ch:    make(chan struct{}, 1),
	}

	a.m.Store(id, subscirber)

	return subscirber
}

type Subscriber interface {
	C() <-chan struct{}
	io.Closer
}

type subscriber struct {
	alarm *Alarm
	id    int64
	ch    chan struct{}
}

func (s *subscriber) wake() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *subscriber) C() <-chan struct{} {
	return s.ch
}

func (s *subscriber) Close() error {
	s.alarm.m.Delete(s.id)
	close(s.ch)

	return nil
}
