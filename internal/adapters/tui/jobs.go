package tui

import (
	"sync"
	"sync/atomic"
)

// jobQueue holds work submitted from other goroutines for the loop.
type jobQueue struct {
	mu   sync.Mutex
	list []func()
	live atomic.Bool // the loop is running and can be woken
}

func (q *jobQueue) add(fn func()) {
	q.mu.Lock()
	q.list = append(q.list, fn)
	q.mu.Unlock()
}

// run executes the queued jobs in order. Jobs queued by a job wait for the
// next frame.
func (q *jobQueue) run() {
	q.mu.Lock()
	list := q.list
	q.list = nil
	q.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}
