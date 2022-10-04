package testutil

import "github.com/golang/mock/gomock"

type Matcher[T any] struct {
	MatchesFunc func(x T) bool
	StringFunc  func() string
}

var _ gomock.Matcher = &Matcher[any]{}

func (m *Matcher[T]) Matches(x interface{}) bool {
	v, ok := x.(T)

	if !ok {
		return false
	}

	return m.MatchesFunc(v)
}

func (m *Matcher[T]) String() string {
	return m.StringFunc()
}
