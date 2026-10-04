package app

import "sync"

// Ring keeps the last n items in memory only.
type Ring[T any] struct {
	mu    sync.Mutex
	items []T
	n     int
}

// NewRing creates a ring holding at most n items.
func NewRing[T any](n int) *Ring[T] { return &Ring[T]{n: n} }

// Add appends v, dropping the oldest item when full.
func (r *Ring[T]) Add(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, v)
	if len(r.items) > r.n {
		r.items = r.items[len(r.items)-r.n:]
	}
}

// All returns a copy, oldest first.
func (r *Ring[T]) All() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]T(nil), r.items...)
}

// Reset drops everything.
func (r *Ring[T]) Reset() {
	r.mu.Lock()
	r.items = nil
	r.mu.Unlock()
}

// LogBuffer is the in-memory UI log.
type LogBuffer = Ring[LogEvent]

// NewLogBuffer creates a log ring of n lines.
func NewLogBuffer(n int) *LogBuffer { return NewRing[LogEvent](n) }
