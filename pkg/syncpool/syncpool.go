package syncpool

import "sync"

type Pool[T any] struct {
	pool sync.Pool

	New func() T
}

func NewPool[T any]() *Pool[T] {
	p := Pool[T]{}

	p.pool.New = func() any {
		return p.New()
	}

	return &p
}

func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}

func (p *Pool[T]) Put(value T) {
	p.pool.Put(value)
}
