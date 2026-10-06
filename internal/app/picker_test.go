package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type fChecker struct {
	ok    map[string]time.Duration // id → latency for OK servers
	calls atomic.Int32
}

func (c *fChecker) Check(_ context.Context, s model.Server) scanner.Result {
	c.calls.Add(1)
	if lat, ok := c.ok[s.ID]; ok {
		return scanner.Result{ServerID: s.ID, OK: true, Latency: lat}
	}
	return scanner.Result{ServerID: s.ID, Reason: "timeout"}
}

func catalog(n int) []model.Server {
	var out []model.Server
	for i := 0; i < n; i++ {
		out = append(out, model.Server{ID: fmt.Sprintf("s%02d", i), Name: fmt.Sprintf("S%d", i), Tags: []string{"no-filter"}})
	}
	return out
}

func newPicker(chk scanner.Checker, s store.Settings) (*ScanPicker, *int) {
	saves := 0
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	return &ScanPicker{
		Catalog:   func() []model.Server { return catalog(10) },
		Checker:   chk,
		Cache:     &scanner.Cache{},
		SaveCache: func(*scanner.Cache) error { saves++; return nil },
		NetKey:    func() string { return "net1" },
		Settings:  func() store.Settings { return s },
		Now:       func() time.Time { return now },
		Rand:      rand.New(rand.NewSource(1)),
	}, &saves
}

func TestPicker_FreshCacheSkipsScan(t *testing.T) {
	chk := &fChecker{}
	s := store.DefaultSettings()
	s.MaxUpstreams = 2
	p, _ := newPicker(chk, s)
	p.Cache.Put("net1", p.Now().Add(-time.Hour), coveringCache([]scanner.Result{
		{ServerID: "s03", OK: true, Latency: 10}, {ServerID: "s01", OK: true, Latency: 20}, {ServerID: "s02"}}))
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, []string{"s03", "s01"}, []string{got[0].ID, got[1].ID})
	require.Zero(t, chk.calls.Load())
}

func TestPicker_StaleCacheQuickScansAndSaves(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s05": 30, "s07": 10}}
	s := store.DefaultSettings()
	s.MaxUpstreams = 2
	p, saves := newPicker(chk, s)
	p.Cache.Put("net1", p.Now().Add(-25*time.Hour), []scanner.Result{{ServerID: "s01", OK: true}})
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, []string{"s07", "s05"}, []string{got[0].ID, got[1].ID})
	require.Equal(t, 1, *saves)
	rs, ok := p.Cache.Fresh("net1", p.Now(), 24*time.Hour)
	require.True(t, ok)
	require.NotEmpty(t, rs)
}

func TestPicker_PinnedOnly(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s01": 10, "s02": 10, "s09": 5}}
	s := store.DefaultSettings()
	s.PinnedOnly, s.Pinned = true, []string{"s01", "s09"}
	p, _ := newPicker(chk, s)
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, []string{"s09", "s01"}, []string{got[0].ID, got[1].ID})
	require.Equal(t, int32(2), chk.calls.Load())
}

func TestPicker_NoneOKReturnsNoServersError(t *testing.T) {
	p, _ := newPicker(&fChecker{}, store.DefaultSettings())
	_, err := p.Pick(context.Background(), nil)
	var ns *NoServersError
	require.True(t, errors.As(err, &ns))
	require.Equal(t, 10, ns.Checked)
}

func TestPicker_RescanScansAllAndSaves(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s01": 10}}
	p, saves := newPicker(chk, store.DefaultSettings())
	var progress int
	rs, err := p.Rescan(context.Background(), func(done, total int, r scanner.Result) { progress = done })
	require.NoError(t, err)
	require.Len(t, rs, 10)
	require.Equal(t, 10, progress)
	require.Equal(t, 1, *saves)
	require.Len(t, p.Results(), 10)
}

func TestPicker_PickFreshIgnoresCacheAndExcludes(t *testing.T) { // review I3
	chk := &fChecker{ok: map[string]time.Duration{"s01": 5, "s05": 30, "s07": 10}}
	s := store.DefaultSettings()
	s.MaxUpstreams = 2
	p, _ := newPicker(chk, s)
	p.Cache.Put("net1", p.Now(), []scanner.Result{{ServerID: "s01", OK: true}, {ServerID: "s03", OK: true}})
	got, err := p.PickFresh(context.Background(), []string{"s01", "s03"})
	require.NoError(t, err)
	require.Equal(t, []string{"s07", "s05"}, []string{got[0].ID, got[1].ID})
	require.Positive(t, chk.calls.Load(), "a fresh scan must run")
}

// Pins are preferred even with pinned-only off: a pinned server that passes
// is always used, even when the cache has faster unpinned ones.
func TestPicker_PinnedPreferredWhenNotPinnedOnly(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s08": 40, "s09": 30}}
	s := store.DefaultSettings()
	s.MaxUpstreams = 3
	s.Pinned = []string{"s08", "s09", "s04"} // s04 is pinned but fails
	p, _ := newPicker(chk, s)
	p.Cache.Put("net1", p.Now().Add(-time.Hour), coveringCache([]scanner.Result{
		{ServerID: "s01", OK: true, Latency: 5}, {ServerID: "s02", OK: true, Latency: 6}, {ServerID: "s03", OK: true, Latency: 7}}))
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	var ids []string
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	require.Equal(t, []string{"s09", "s08", "s01"}, ids) // pinned (by latency) first, then the fastest others
	require.Equal(t, int32(3), chk.calls.Load())         // only the pinned were checked; the cache filled the rest
}

// Pinned servers outside the include tags are still used.
func TestPicker_PinnedOutsideTags(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"x1": 10, "s01": 20}}
	s := store.DefaultSettings()
	s.MaxUpstreams = 2
	s.Pinned = []string{"x1"}
	p, _ := newPicker(chk, s)
	p.Catalog = func() []model.Server {
		return append(catalog(2), model.Server{ID: "x1", Name: "AdBlocker", Tags: []string{"adblock"}})
	}
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "x1", got[0].ID)
	require.Equal(t, "s01", got[1].ID)
}

// Pinned-only with nothing pinned (or no pinned server passing) is its own
// error, not the generic NO_SERVERS.
func TestPicker_PinnedOnlyNothingUsable(t *testing.T) {
	s := store.DefaultSettings()
	s.PinnedOnly = true
	p, _ := newPicker(&fChecker{}, s)
	_, err := p.Pick(context.Background(), nil)
	var np *NoPinnedError
	require.ErrorAs(t, err, &np)
	require.Zero(t, np.Checked)

	s.Pinned = []string{"s01", "gone"}
	p, _ = newPicker(&fChecker{}, s)
	_, err = p.Pick(context.Background(), nil)
	require.ErrorAs(t, err, &np)
	require.Equal(t, 1, np.Checked)
}

// CheckOne re-tests one server and updates the cached result shown in the UI.
func TestPicker_CheckOneUpdatesCache(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s02": 15}}
	p, saves := newPicker(chk, store.DefaultSettings())
	p.Cache.Put("net1", p.Now(), []scanner.Result{{ServerID: "s01", OK: true, Latency: 9}, {ServerID: "s02", Reason: "timeout"}})
	r, err := p.CheckOne(context.Background(), "s02")
	require.NoError(t, err)
	require.True(t, r.OK)
	byID := map[string]scanner.Result{}
	for _, x := range p.Results() {
		byID[x.ServerID] = x
	}
	require.True(t, byID["s02"].OK)
	require.True(t, byID["s01"].OK)
	require.Equal(t, 1, *saves)
	_, err = p.CheckOne(context.Background(), "nope")
	require.Error(t, err)
}

// Reported: connecting with "use pinned servers only" wiped every other
// server's status on the Servers page.
func TestPicker_PinnedOnlyKeepsOtherResults(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{"s01": 10}}
	s := store.DefaultSettings()
	s.PinnedOnly, s.Pinned = true, []string{"s01"}
	p, _ := newPicker(chk, s)
	p.Cache.Put("net1", p.Now().Add(-time.Hour), []scanner.Result{
		{ServerID: "s02", OK: true, Latency: 5}, {ServerID: "s03", OK: true, Latency: 6}})
	_, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, r := range p.Results() {
		ids[r.ServerID] = true
	}
	require.True(t, ids["s01"])
	require.True(t, ids["s02"], "other servers' results must survive a pinned-only connect")
	require.True(t, ids["s03"])
}

func TestPicker_FirstScanCoversEveryServerAndPicksFastest(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{}}
	for i := 0; i < 60; i++ {
		chk.ok[fmt.Sprintf("s%02d", i)] = time.Duration(160-i) * time.Millisecond // s59 is fastest
	}
	s := store.DefaultSettings()
	s.MaxUpstreams = 5
	p, _ := newPicker(slowChecker{chk}, s) // answers take time, as on a real network
	p.Catalog = func() []model.Server { return catalog(60) }
	var last [2]int
	got, err := p.Pick(context.Background(), func(done, total int) { last = [2]int{done, total} })
	require.NoError(t, err)
	require.Equal(t, int32(60), chk.calls.Load(), "no fresh cache: every server is checked")
	require.Equal(t, [2]int{60, 60}, last)
	ids := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID, got[4].ID}
	require.Equal(t, []string{"s59", "s58", "s57", "s56", "s55"}, ids)

	// Next connect on the same network: straight from the cache.
	got2, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, int32(60), chk.calls.Load())
	require.Equal(t, "s59", got2[0].ID)
}

func TestPicker_PickFreshStaysQuickButCollectsCandidates(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{}}
	for i := 0; i < 200; i++ {
		chk.ok[fmt.Sprintf("s%03d", i)] = time.Duration(100+i) * time.Millisecond
	}
	s := store.DefaultSettings()
	s.MaxUpstreams = 5
	p, _ := newPicker(slowChecker{chk}, s)
	p.Catalog = func() []model.Server {
		var out []model.Server
		for i := 0; i < 200; i++ {
			out = append(out, model.Server{ID: fmt.Sprintf("s%03d", i), Tags: []string{"no-filter"}})
		}
		return out
	}
	got, err := p.PickFresh(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, got, 5)
	calls := int(chk.calls.Load())
	require.GreaterOrEqual(t, calls, 20, "collects 4x what it needs")
	require.Less(t, calls, 200, "a swap while connected does not wait for a full scan")
}

// slowChecker delays each answer so a scan cannot finish everything at once.
type slowChecker struct{ c *fChecker }

func (s slowChecker) Check(ctx context.Context, srv model.Server) scanner.Result {
	time.Sleep(5 * time.Millisecond)
	return s.c.Check(ctx, srv)
}

func TestPicker_CancelledRescanKeepsWhatItChecked(t *testing.T) {
	chk := &fChecker{ok: map[string]time.Duration{}}
	for i := 0; i < 60; i++ {
		chk.ok[fmt.Sprintf("s%02d", i)] = time.Duration(10+i) * time.Millisecond
	}
	p, saves := newPicker(slowChecker{chk}, store.DefaultSettings())
	p.Catalog = func() []model.Server { return catalog(60) }
	ctx, cancel := context.WithCancel(context.Background())
	_, err := p.Rescan(ctx, func(d, _ int, _ scanner.Result) {
		if d == 10 {
			cancel()
		}
	})
	require.Error(t, err)
	require.Equal(t, 1, *saves, "the servers checked before cancel are saved")
	require.GreaterOrEqual(t, len(p.Results()), 10)
}

func TestPicker_CacheMissingMostOfTheListScansAgain(t *testing.T) {
	// The cache was written when only 13 built-in servers were known; the
	// full list has arrived since.
	chk := &fChecker{ok: map[string]time.Duration{}}
	for i := 0; i < 60; i++ {
		chk.ok[fmt.Sprintf("s%02d", i)] = time.Duration(100-i) * time.Millisecond
	}
	s := store.DefaultSettings()
	s.MaxUpstreams = 5
	p, _ := newPicker(chk, s)
	p.Catalog = func() []model.Server { return catalog(60) }
	var old []scanner.Result
	for i := 0; i < 13; i++ {
		old = append(old, scanner.Result{ServerID: fmt.Sprintf("s%02d", i), OK: true, Latency: 50 * time.Millisecond, CheckedAt: p.Now()})
	}
	p.Cache.Put("net1", p.Now(), old)
	got, err := p.Pick(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, int32(60), chk.calls.Load(), "a cache covering 13 of 60 servers is not enough")
	require.Equal(t, "s59", got[0].ID)
}

// coveringCache adds failed results for the rest of catalog(10), so the
// cache covers the whole list and stands in for a scan.
func coveringCache(rs []scanner.Result) []scanner.Result {
	have := map[string]bool{}
	for _, r := range rs {
		have[r.ServerID] = true
	}
	for _, sv := range catalog(10) {
		if !have[sv.ID] {
			rs = append(rs, scanner.Result{ServerID: sv.ID, Reason: "timeout"})
		}
	}
	return rs
}
