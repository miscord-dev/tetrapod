package alarm

import (
	"sync/atomic"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/syncmap"
)

type Alarm struct {
	m       syncmap.Map[int64, *Subscriber]
	counter atomic.Int64
}

func New() *Alarm {
	return &Alarm{}
}

func (a *Alarm) WakeUpAll() {
	a.m.Range(func(key int64, value *Subscriber) bool {
		value.wake()

		return true
	})
}

func (a *Alarm) Subscribe() *Subscriber {
	id := a.counter.Add(1)

	subscirber := &Subscriber{
		alarm: a,
		id:    id,
		ch:    make(chan struct{}, 1),
	}

	a.m.Store(id, subscirber)

	return subscirber
}

type Subscriber struct {
	alarm *Alarm
	id    int64
	ch    chan struct{}
}

func (s *Subscriber) wake() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *Subscriber) C() <-chan struct{} {
	return s.ch
}

func (s *Subscriber) Close() error {
	s.alarm.m.Delete(s.id)
	close(s.ch)

	return nil
}
