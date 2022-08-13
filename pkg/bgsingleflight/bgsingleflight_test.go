package bgsingleflight

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestGroup(t *testing.T) {
	g := Group{}

	buf := bytes.Buffer{}
	var lock sync.Mutex
	logger := func(s string) {
		lock.Lock()
		buf.WriteString(s)
		lock.Unlock()
	}

	key1 := "key1"
	key2 := "key2"

	g.NotifyExit(func(key string) {
		logger(fmt.Sprintf("finish %s\n", key))
	})

	var wg sync.WaitGroup
	wg.Add(2)

	g.Run(key1, func() {
		defer wg.Done()
		logger(fmt.Sprintf("start %s\n", key1))

		time.Sleep(100 * time.Millisecond)
	})
	time.Sleep(1 * time.Millisecond)

	g.Run(key2, func() {
		defer wg.Done()
		logger(fmt.Sprintf("start %s\n", key2))

		time.Sleep(110 * time.Millisecond)
	})

	wg.Wait()

	time.Sleep(10 * time.Millisecond)

	lock.Lock()
	str := buf.String()
	lock.Unlock()

	expected := `start key1
start key2
finish key1
finish key2
`

	if str != expected {
		t.Error(cmp.Diff(expected, str))
	}
}
