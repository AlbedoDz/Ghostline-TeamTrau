package cfscan

import (
	"context"
	"sync"
	"time"
)

// Limiter paces new connections.
type Limiter interface {
	Wait(ctx context.Context) error
}

type intervalLimiter struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

// NewLimiter allows perSec waits per second, evenly spaced.
func NewLimiter(perSec int) Limiter {
	return &intervalLimiter{gap: time.Second / time.Duration(perSec)}
}

func (l *intervalLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	at := l.next
	l.next = l.next.Add(l.gap)
	l.mu.Unlock()
	t := time.NewTimer(time.Until(at))
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
