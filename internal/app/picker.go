package app

import (
	"context"
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

// ScanPicker picks servers from one ranking per network: the last scan of
// the whole list, kept in the scan cache.
//
//   - no ranking yet: scan the whole list, then take the fastest;
//   - a ranking under a day old: take the fastest from it at once;
//   - an older one: check its best servers in order until enough answer,
//     and let the caller refresh the whole ranking in the background;
//   - healing: the same check, skipping the servers that failed.
//
// Every server is scanned, but only those the settings allow (tags,
// custom, pinned) are picked; pinned servers come first.
type ScanPicker struct {
	Catalog   func() []model.Server
	Checker   scanner.Checker
	Cache     *scanner.Cache
	SaveCache func(*scanner.Cache) error
	NetKey    func() string
	Settings  func() store.Settings
	Now       func() time.Time
	Rand      *rand.Rand
	// Watch, when set, sees every full scan's results as they arrive and
	// its end (running false), whoever started it: the UI shows them live.
	Watch func(done, total int, r *scanner.Result, running bool)

	mu      sync.Mutex // guards Cache, Rand and running
	running *fullScan  // the full scan in progress, shared by every caller
}

// fullScan is one scan of the whole list. Callers that need one while it
// runs wait for it instead of starting another; it stops early only when
// every one of them has left.
type fullScan struct {
	done    chan struct{}
	cancel  context.CancelFunc
	rs      []scanner.Result
	err     error
	mu      sync.Mutex
	waiters int
	watch   map[int]func(done, total int, r scanner.Result)
	next    int
	at      [2]int // last progress, for a caller that joins late
}

const (
	// rankTTL: a ranking older than this is checked before use and
	// refreshed in the background.
	rankTTL      = 24 * time.Hour
	fullWorkers  = 32
	fullBudget   = 90 * time.Second
	checkWorkers = 16
	checkBudget  = 20 * time.Second
	defaultWants = 5
	// rankCoverage is the share of the list a ranking must have results
	// for: one written before the list grew would hide the new servers.
	rankCoverage = 0.9
)

// scanPool is what a full scan checks: the whole list, or the pinned
// servers with pinned-only on.
func (p *ScanPicker) scanPool(s store.Settings) []model.Server {
	all := p.Catalog()
	if !s.PinnedOnly {
		return all
	}
	return slices.DeleteFunc(all, func(sv model.Server) bool { return !slices.Contains(s.Pinned, sv.ID) })
}

// Eligible reports whether the settings let sv be picked automatically.
func Eligible(s store.Settings, sv model.Server) bool {
	if slices.Contains(s.Pinned, sv.ID) {
		return true
	}
	return !s.PinnedOnly && len(servers.Filter([]model.Server{sv}, s.IncludeTags)) == 1
}

// best returns up to want working servers from rs: pinned first, then the
// rest, each by latency; never an excluded or ineligible one.
func best(rs []scanner.Result, pool []model.Server, s store.Settings, exclude []string, want int) []model.Server {
	byID := make(map[string]model.Server, len(pool))
	for _, sv := range pool {
		if Eligible(s, sv) && !slices.Contains(exclude, sv.ID) {
			byID[sv.ID] = sv
		}
	}
	ok := slices.DeleteFunc(slices.Clone(rs), func(r scanner.Result) bool { _, found := byID[r.ServerID]; return !r.OK || !found })
	pinned := func(r scanner.Result) bool { return slices.Contains(s.Pinned, r.ServerID) }
	slices.SortStableFunc(ok, func(a, b scanner.Result) int {
		if pa, pb := pinned(a), pinned(b); pa != pb {
			if pa {
				return -1
			}
			return 1
		}
		return int(a.Latency - b.Latency)
	})
	var out []model.Server
	for _, r := range ok {
		if len(out) == want {
			break
		}
		out = append(out, byID[r.ServerID])
	}
	return out
}

func covers(rs []scanner.Result, pool []model.Server) bool {
	have := make(map[string]bool, len(rs))
	for _, r := range rs {
		have[r.ServerID] = true
	}
	n := 0
	for _, s := range pool {
		if have[s.ID] {
			n++
		}
	}
	return len(pool) > 0 && float64(n) >= rankCoverage*float64(len(pool))
}

func wants(s store.Settings) int {
	if s.MaxUpstreams <= 0 {
		return defaultWants
	}
	return s.MaxUpstreams
}

// ranking returns this network's ranking and whether it is complete and
// recent.
func (p *ScanPicker) ranking(pool []model.Server) (rs []scanner.Result, complete, fresh bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.Cache.Entries[p.NetKey()]
	complete = covers(e.Results, pool)
	fresh = complete && !e.FullAt.IsZero() && p.Now().Sub(e.FullAt) <= rankTTL
	return slices.Clone(e.Results), complete, fresh
}

// Stale reports whether this network's ranking is missing, incomplete or
// older than a day: the caller then refreshes it in the background.
func (p *ScanPicker) Stale() bool {
	_, _, fresh := p.ranking(p.scanPool(p.Settings()))
	return !fresh
}

// Pick implements Picker (spec §6.3). onProgress reports a full scan.
func (p *ScanPicker) Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error) {
	s := p.Settings()
	pool := p.scanPool(s)
	if s.PinnedOnly && len(pool) == 0 {
		return nil, &NoPinnedError{}
	}
	rs, complete, fresh := p.ranking(pool)
	switch {
	case fresh:
		if top := best(rs, pool, s, nil, wants(s)); len(top) > 0 {
			return top, nil
		}
		return p.check(ctx, s, pool, rs, nil) // nothing worked last time: look again
	case complete:
		return p.check(ctx, s, pool, rs, nil)
	}
	start := time.Now()
	rs, err := p.full(ctx, pool, fullBudget, func(done, total int, _ scanner.Result) {
		if onProgress != nil {
			onProgress(done, total)
		}
	})
	if err != nil {
		return nil, err
	}
	if top := best(rs, pool, s, nil, wants(s)); len(top) > 0 {
		return top, nil
	}
	return nil, p.noServers(s, len(rs), time.Since(start))
}

// PickFresh checks servers again, skipping the excluded ones (healing).
func (p *ScanPicker) PickFresh(ctx context.Context, exclude []string) ([]model.Server, error) {
	s := p.Settings()
	pool := p.scanPool(s)
	rs, _, _ := p.ranking(pool)
	return p.check(ctx, s, pool, rs, exclude)
}

// check tries the allowed servers, the ranking's best first, until enough
// answer, and returns the fastest of those.
func (p *ScanPicker) check(ctx context.Context, s store.Settings, pool []model.Server, rs []scanner.Result, exclude []string) ([]model.Server, error) {
	want := wants(s)
	latency := map[string]time.Duration{}
	for _, r := range rs {
		if r.OK {
			latency[r.ServerID] = r.Latency
		}
	}
	var ranked, rest []model.Server
	for _, sv := range pool {
		switch _, ok := latency[sv.ID]; {
		case !Eligible(s, sv) || slices.Contains(exclude, sv.ID):
		case ok:
			ranked = append(ranked, sv)
		default:
			rest = append(rest, sv)
		}
	}
	slices.SortStableFunc(ranked, func(a, b model.Server) int { return int(latency[a.ID] - latency[b.ID]) })
	p.mu.Lock()
	p.Rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	p.mu.Unlock()

	start := time.Now()
	got := scanner.Scan(ctx, append(ranked, rest...), p.Checker, scanner.Options{Workers: checkWorkers, Want: want, Budget: checkBudget})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = p.save(got, false) // a pick still works if the cache cannot be written
	if top := best(got, pool, s, exclude, want); len(top) > 0 {
		return top, nil
	}
	return nil, p.noServers(s, len(got), time.Since(start))
}

func (p *ScanPicker) noServers(s store.Settings, checked int, elapsed time.Duration) error {
	if s.PinnedOnly {
		return &NoPinnedError{Checked: checked}
	}
	return &NoServersError{Checked: checked, Elapsed: elapsed}
}

// full scans pool and saves the results, or waits for the full scan
// already running. A cancelled scan keeps what it checked but does not count
// as a full one. budget bounds a scan someone waits on to connect (0: none).
func (p *ScanPicker) full(ctx context.Context, pool []model.Server, budget time.Duration, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error) {
	p.mu.Lock()
	f := p.running
	if f == nil {
		sctx, cancel := context.WithCancel(context.Background())
		f = &fullScan{done: make(chan struct{}), cancel: cancel, watch: map[int]func(int, int, scanner.Result){}}
		p.running = f
		go p.runFull(sctx, f, pool, budget)
	}
	f.mu.Lock()
	f.waiters++
	id := f.next
	f.next++
	if onProgress != nil {
		f.watch[id] = onProgress
		if f.at[1] > 0 {
			onProgress(f.at[0], f.at[1], scanner.Result{})
		}
	}
	f.mu.Unlock()
	p.mu.Unlock()

	select {
	case <-f.done:
		return f.rs, f.err
	case <-ctx.Done():
	}
	f.mu.Lock()
	delete(f.watch, id)
	f.waiters--
	last := f.waiters == 0
	f.mu.Unlock()
	if !last {
		return nil, ctx.Err()
	}
	f.cancel() // nobody waits any more
	<-f.done
	return f.rs, ctx.Err()
}

func (p *ScanPicker) runFull(ctx context.Context, f *fullScan, pool []model.Server, budget time.Duration) {
	defer f.cancel()
	rs := scanner.Scan(ctx, pool, p.Checker, scanner.Options{Workers: fullWorkers, Budget: budget,
		OnProgress: func(done, total int, r scanner.Result) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.at = [2]int{done, total}
			if p.Watch != nil {
				p.Watch(done, total, &r, true)
			}
			for _, w := range f.watch {
				w(done, total, r)
			}
		}})
	err := ctx.Err()
	if saveErr := p.save(rs, err == nil); err == nil {
		err = saveErr
	}
	p.mu.Lock()
	p.running = nil
	p.mu.Unlock()
	f.rs, f.err = rs, err
	if p.Watch != nil {
		p.Watch(0, 0, nil, false)
	}
	close(f.done)
}

// Watched reports whether full scans report themselves (see Watch).
func (p *ScanPicker) Watched() bool { return p.Watch != nil }

func (p *ScanPicker) save(rs []scanner.Result, full bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	key, now := p.NetKey(), p.Now()
	p.Cache.Merge(key, now, rs)
	if full {
		p.Cache.MarkFull(key, now)
	}
	return p.SaveCache(p.Cache)
}

// CheckOne re-tests one server and merges the result into the ranking.
func (p *ScanPicker) CheckOne(ctx context.Context, id string) (scanner.Result, error) {
	i := slices.IndexFunc(p.Catalog(), func(sv model.Server) bool { return sv.ID == id })
	if i < 0 {
		return scanner.Result{}, fmt.Errorf("app: no server %q", id)
	}
	r := p.Checker.Check(ctx, p.Catalog()[i])
	_ = p.save([]scanner.Result{r}, false)
	return r, nil
}

// Rescan scans the whole list again.
func (p *ScanPicker) Rescan(ctx context.Context, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error) {
	return p.full(ctx, p.scanPool(p.Settings()), 0, onProgress)
}

// Results returns the ranking for the current network, whatever its age.
func (p *ScanPicker) Results() []scanner.Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.Cache.Entries[p.NetKey()].Results)
}
