package syncmap

import "testing"

func TestStoreAndGet(t *testing.T) {
	m := Map[[1]byte, string]{}

	m.Store([1]byte{0xff}, "hello")

	v, ok := m.Load([1]byte{0xff})

	if !ok {
		t.Fatal("entry not found")
	}

	if v != "hello" {
		t.Errorf("want %v, got %v", "hello", v)
	}
}
