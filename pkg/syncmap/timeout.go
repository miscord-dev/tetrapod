package syncmap

import "time"

type timeoutMapValue[V any] struct {
	expiredAt time.Time
	value     V
}

type TimeoutMap[K comparable, V any] struct {
	m       Map[K, timeoutMapValue[V]]
	Timeout time.Duration
}

func (m *TimeoutMap[K, V]) initValue(v V) timeoutMapValue[V] {
	return timeoutMapValue[V]{
		expiredAt: time.Now().Add(m.Timeout),
		value:     v,
	}
}

func (m *TimeoutMap[K, V]) Delete(key K) {
	m.m.Delete(key)
}

func (m *TimeoutMap[K, V]) Load(key K) (value V, ok bool) {
	var zeroValue V

	v, ok := m.m.Load(key)

	if !ok {
		return zeroValue, false
	}
	if isExpired(v) {
		return zeroValue, false
	}

	return v.value, true
}

func (m *TimeoutMap[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	var zeroValue V

	v, ok := m.m.LoadAndDelete(key)

	if !ok || isExpired(v) {
		return zeroValue, false
	}

	return v.value, true
}

func (m *TimeoutMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	var zeroValue V

	v, ok := m.m.LoadOrStore(key, m.initValue(value))

	if !ok {
		return zeroValue, false
	}
	if isExpired(v) {
		m.m.Delete(key)
		return zeroValue, false
	}

	return v.value, true
}

func (m *TimeoutMap[K, V]) Store(key K, value V) {
	m.m.Store(key, m.initValue(value))

	m.m.Range(func(key K, value timeoutMapValue[V]) bool {
		if isExpired(value) {
			m.m.Delete(key)
		}

		return true
	})
}

func isExpired[V any](v timeoutMapValue[V]) bool {
	return time.Now().After(v.expiredAt)
}
