package app

import (
	"context"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
)

// Scans exposes full scans to the UI.
type Scans interface {
	Rescan(ctx context.Context, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error)
	Results() []scanner.Result
}

// ScanPicker picks servers from the cached or a fresh quick scan.
type ScanPicker struct {
	Catalog   func() []model.Server
	Checker   scanner.Checker
	Cache     *scanner.Cache
	SaveCache func(*scanner.Cache) error
	NetKey    func() string
	Settings  func() store.Settings
	Now       func() time.Time
	Rand      *rand.Rand

	mu sync.Mutex // guards Cache and Rand
}

const (
	cacheTTL     = 24 * time.Hour
	quickBudget  = 20 * time.Second
	scanWorkers  = 16
	defaultWants = 5
)

func (p *ScanPicker) pool(s store.Settings) []model.Server {
	all := p.Catalog()
	if s.PinnedOnly {
		var out []model.Server
		for _, srv := range all {
			if slices.Contains(s.Pinned, srv.ID) {
				out = append(out, srv)
			}
		}
		return out
	}
	return servers.Filter(all, s.IncludeTags)
}

func topOK(rs []scanner.Result, pool []model.Server, want int) []model.Server {
	byID := make(map[string]model.Server, len(pool))
	for _, s := range pool {
		byID[s.ID] = s
	}
	ok := slices.Clone(rs)
	slices.SortStableFunc(ok, func(a, b scanner.Result) int { return int(a.Latency - b.Latency) })
	var out []model.Server
	for _, r := range ok {
		if s, found := byID[r.ServerID]; r.OK && found && len(out) < want {
			out = append(out, s)
		}
	}
	return out
}

// Pick implements Picker (spec §6.3).
func (p *ScanPicker) Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error) {
	s := p.Settings()
	want := s.MaxUpstreams
	if want <= 0 {
		want = defaultWants
	}
	pool := p.pool(s)
	key := p.NetKey()
	now := p.Now()

	p.mu.Lock()
	if !s.PinnedOnly {
		if rs, ok := p.Cache.Fresh(key, now, cacheTTL); ok {
			if top := topOK(rs, pool, want); len(top) >= want {
				p.mu.Unlock()
				return top, nil
			}
		}
	}
	ordered := scanner.Order(pool, p.Cache.EverOK(), p.Rand)
	p.mu.Unlock()

	start := time.Now()
	rs := scanner.Scan(ctx, ordered, p.Checker, scanner.Options{
		Workers: scanWorkers, Want: want, Budget: quickBudget,
		OnProgress: func(done, total int, _ scanner.Result) {
			if onProgress != nil {
				onProgress(done, total)
			}
		},
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.Cache.Put(key, now, rs)
	_ = p.SaveCache(p.Cache)
	p.mu.Unlock()

	top := topOK(rs, pool, want)
	if len(top) == 0 {
		return nil, &NoServersError{Checked: len(rs), Elapsed: time.Since(start)}
	}
	return top, nil
}

// Rescan checks every eligible server and caches the results.
func (p *ScanPicker) Rescan(ctx context.Context, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error) {
	s := p.Settings()
	pool := p.Catalog()
	if s.PinnedOnly {
		pool = p.pool(s)
	}
	rs := scanner.Scan(ctx, pool, p.Checker, scanner.Options{Workers: scanWorkers, OnProgress: onProgress})
	if err := ctx.Err(); err != nil {
		return rs, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Cache.Put(p.NetKey(), p.Now(), rs)
	return rs, p.SaveCache(p.Cache)
}

// Results returns the last scan for the current network, whatever its age.
func (p *ScanPicker) Results() []scanner.Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.Cache.Entries[p.NetKey()].Results)
}
