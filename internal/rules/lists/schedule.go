package lists

import (
	"context"
	"time"
)

const (
	startDelay    = 30 * time.Second
	checkInterval = 15 * time.Minute
)

// Scheduler refreshes stale lists one at a time: first check 30 s after
// start, then every 15 minutes, each download delayed by a random jitter.
type Scheduler struct {
	Lists  func() []List
	Due    func(l List, now time.Time) bool
	Run    func(ctx context.Context, id string) error
	Now    func() time.Time
	After  func(time.Duration) <-chan time.Time
	Jitter func() time.Duration // 0..10 minutes
}

// Loop runs until ctx is cancelled.
func (s *Scheduler) Loop(ctx context.Context) {
	wait := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-s.After(d):
			return ctx.Err() == nil
		}
	}
	if !wait(startDelay) {
		return
	}
	for {
		for _, l := range s.Lists() {
			if !s.Due(l, s.Now()) {
				continue
			}
			if !wait(s.Jitter()) {
				return
			}
			_ = s.Run(ctx, l.ID)
		}
		if !wait(checkInterval) {
			return
		}
	}
}
