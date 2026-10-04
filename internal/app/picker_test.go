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
	p.Cache.Put("net1", p.Now().Add(-time.Hour), []scanner.Result{
		{ServerID: "s03", OK: true, Latency: 10}, {ServerID: "s01", OK: true, Latency: 20}, {ServerID: "s02"}})
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
