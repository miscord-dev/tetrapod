package bgsingleflight

import (
	"sync/atomic"

	"github.com/miscord-dev/toxfu/toxfucni/pkg/syncmap"
)

type NotifyExitFunc func(string)

type Group struct {
	notifyExit atomic.Pointer[NotifyExitFunc]

	set syncmap.Map[string, struct{}]
}

func (g *Group) Run(key string, fn func()) {
	_, loaded := g.set.LoadOrStore(key, struct{}{})

	if loaded {
		return
	}

	go func() {
		defer func() {
			g.set.Delete(key)

			fn := g.notifyExit.Load()

			if fn != nil {
				(*fn)(key)
			}
		}()

		fn()
	}()
}

func (g *Group) NotifyExit(fn func(key string)) {
	g.notifyExit.Store((*NotifyExitFunc)(&fn))
}
