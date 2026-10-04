package app

import (
	"context"
	"errors"
	"fmt"
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
	return p.pick(ctx, onProgress, true, nil)
}

// PickFresh skips the cache and the excluded servers (used when healing).
func (p *ScanPicker) PickFresh(ctx context.Context, exclude []string) ([]model.Server, error) {
	return p.pick(ctx, nil, false, exclude)
}

func (p *ScanPicker) pick(ctx context.Context, onProgress func(done, total int), useCache bool, exclude []string) ([]model.Server, error) {
	s := p.Settings()
	want := s.MaxUpstreams
	if want <= 0 {
		want = defaultWants
	}
	notExcluded := func(list []model.Server) []model.Server {
		return slices.DeleteFunc(list, func(sv model.Server) bool { return slices.Contains(exclude, sv.ID) })
	}
	if s.PinnedOnly {
		pool := notExcluded(p.pool(s))
		if len(pool) == 0 {
			return nil, &NoPinnedError{}
		}
		top, err := p.pickFrom(ctx, pool, want, false, onProgress, nil)
		var ns *NoServersError
		if errors.As(err, &ns) {
			return nil, &NoPinnedError{Checked: ns.Checked}
		}
		return top, err
	}

	// Pinned servers are preferred: check them all first (they are few)
	// and use every one that passes before filling the remaining slots.
	var pins []model.Server
	for _, sv := range p.Catalog() {
		if slices.Contains(s.Pinned, sv.ID) {
			pins = append(pins, sv)
		}
	}
	pins = notExcluded(pins)
	var chosen []model.Server
	var pinRes []scanner.Result
	if len(pins) > 0 {
		pinRes = scanner.Scan(ctx, pins, p.Checker, scanner.Options{Workers: scanWorkers, Budget: quickBudget})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chosen = topOK(pinRes, pins, want)
	}
	if len(chosen) >= want {
		return chosen, nil
	}
	rest := notExcluded(slices.DeleteFunc(p.pool(s), func(sv model.Server) bool { return slices.Contains(s.Pinned, sv.ID) }))
	top, err := p.pickFrom(ctx, rest, want-len(chosen), useCache, onProgress, pinRes)
	if err != nil {
		if len(chosen) > 0 && ctx.Err() == nil {
			return chosen, nil // the pinned servers that passed are enough
		}
		var ns *NoServersError
		if errors.As(err, &ns) {
			ns.Checked += len(pinRes)
		}
		return nil, err
	}
	return append(chosen, top...), nil
}

// pickFrom returns the want fastest working servers of pool, from a fresh
// cache when allowed or a quick scan. extra results (the pinned check) are
// saved into the scan cache together with the scan.
func (p *ScanPicker) pickFrom(ctx context.Context, pool []model.Server, want int, useCache bool, onProgress func(done, total int), extra []scanner.Result) ([]model.Server, error) {
	key := p.NetKey()
	now := p.Now()

	p.mu.Lock()
	if useCache {
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
	p.Cache.Put(key, now, append(slices.Clone(rs), extra...))
	_ = p.SaveCache(p.Cache)
	p.mu.Unlock()

	top := topOK(rs, pool, want)
	if len(top) == 0 {
		return nil, &NoServersError{Checked: len(rs), Elapsed: time.Since(start)}
	}
	return top, nil
}

// CheckOne re-tests one server and records the result in the current
// network's cached scan (without making the rest of the cache look fresh).
func (p *ScanPicker) CheckOne(ctx context.Context, id string) (scanner.Result, error) {
	var srv *model.Server
	for _, sv := range p.Catalog() {
		if sv.ID == id {
			sv := sv
			srv = &sv
			break
		}
	}
	if srv == nil {
		return scanner.Result{}, fmt.Errorf("app: no server %q", id)
	}
	r := p.Checker.Check(ctx, *srv)
	key := p.NetKey()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Cache.Entries == nil {
		p.Cache.Entries = map[string]scanner.CacheEntry{}
	}
	e := p.Cache.Entries[key]
	i := slices.IndexFunc(e.Results, func(x scanner.Result) bool { return x.ServerID == id })
	if i >= 0 {
		e.Results[i] = r
	} else {
		e.Results = append(e.Results, r)
	}
	p.Cache.Entries[key] = e
	_ = p.SaveCache(p.Cache)
	return r, nil
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
