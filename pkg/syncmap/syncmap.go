package syncmap

import "sync"

type Map[K comparable, V any] struct {
	m sync.Map
}

func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.m.Load(key)

	if !ok {
		return value, false
	}

	return v.(V), true
}
func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, ok := m.m.LoadAndDelete(key)

	if !ok {
		return value, false
	}

	return v.(V), true
}
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, ok := m.m.LoadOrStore(key, value)

	if !ok {
		return v.(V), false
	}

	return v.(V), true
}
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key any, value any) bool {
		return f(key.(K), value.(V))
	})
}
func (m *Map[K, V]) Store(key K, value V) {
	m.m.Store(key, value)
}
