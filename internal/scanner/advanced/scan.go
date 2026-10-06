package advanced

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// DefaultMaxServers caps one scan when Options.MaxServers is 0.
const DefaultMaxServers = 500

// ErrTooMany rejects a scan of more servers than Options.MaxServers.
var ErrTooMany = errors.New("advanced: too many servers for one scan")

// progressEvery throttles progress callbacks to about 10 per second.
const progressEvery = 100 * time.Millisecond

// Scan grades servers in parallel and returns them sorted. A cancelled or
// timed-out scan returns the servers finished so far.
func Scan(ctx context.Context, list []model.Server, c Checker, workers int, onProgress func(done, total int, r Result)) ([]Result, error) {
	maxN := c.Opt.MaxServers
	if maxN <= 0 {
		maxN = DefaultMaxServers
	}
	if len(list) > maxN {
		return nil, ErrTooMany
	}
	if workers <= 0 {
		workers = 8
	}
	limit := c.Opt.MaxDuration
	if limit <= 0 {
		limit = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	jobs := make(chan model.Server)
	var (
		mu       sync.Mutex
		out      []Result
		lastSent time.Time
		wg       sync.WaitGroup
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				r := c.Run(ctx, s)
				if ctx.Err() != nil {
					continue // interrupted: its numbers are not real
				}
				mu.Lock()
				out = append(out, r)
				done := len(out)
				if onProgress != nil && (done == len(list) || time.Since(lastSent) >= progressEvery) {
					lastSent = time.Now()
					onProgress(done, len(list), r)
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, s := range list {
		select {
		case jobs <- s:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	Sort(out)
	return out, nil
}

// Sort puts reachable, unpoisoned servers first, by loss (rounded to 10%)
// then median latency; unreachable next; poisoned last.
func Sort(rs []Result) {
	group := func(r Result) int {
		switch {
		case len(r.Poisoned) > 0:
			return 2
		case !r.Reach.OK:
			return 1
		}
		return 0
	}
	slices.SortStableFunc(rs, func(a, b Result) int {
		return cmp.Or(
			cmp.Compare(group(a), group(b)),
			cmp.Compare(math.Round(a.Loss*10), math.Round(b.Loss*10)),
			cmp.Compare(a.MedianMs, b.MedianMs),
			cmp.Compare(a.ServerID, b.ServerID),
		)
	})
}
