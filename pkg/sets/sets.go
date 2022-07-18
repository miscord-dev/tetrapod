package sets

type Set[K comparable] struct {
	m map[K]struct{}
}

func NewSet[K comparable]() Set[K] {
	return Set[K]{
		m: make(map[K]struct{}),
	}
}

func FromSlice[K comparable](slice []K) Set[K] {
	set := NewSet[K]()

	for _, k := range slice {
		set.Add(k)
	}

	return set
}

func (s *Set[K]) Add(k K) {
	s.m[k] = struct{}{}
}

func (s *Set[K]) Delete(k K) {
	delete(s.m, k)
}

func (s *Set[K]) Contains(k K) bool {
	_, ok := s.m[k]
	return ok
}
