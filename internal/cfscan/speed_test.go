package cfscan_test

import (
	"context"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

func down(t *testing.T, gotBytes *atomic.Int64, inFlight, maxInFlight *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__down" {
			http.NotFound(w, r)
			return
		}
		if inFlight != nil {
			n := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				m := maxInFlight.Load()
				if n <= m || maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
		}
		n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
		if gotBytes != nil {
			gotBytes.Store(int64(n))
		}
		_, _ = w.Write(make([]byte, n))
	}
}

func TestSpeed_Mbps(t *testing.T) {
	var got atomic.Int64
	e := newEdge(t, down(t, &got, nil, nil))
	mbps, err := prober(e.pki, e.srv.Listener.Addr().String(), nil).Speed(context.Background(), ip, 1<<20, 10*time.Second)
	require.NoError(t, err)
	require.Greater(t, mbps, 0.0)
	require.Equal(t, int64(1<<20), got.Load())
}

func TestSpeed_404(t *testing.T) {
	e := newEdge(t, trace("SIN"))
	_, err := prober(e.pki, e.srv.Listener.Addr().String(), nil).Speed(context.Background(), ip, 1<<20, time.Second)
	require.ErrorIs(t, err, cfscan.ErrNoSpeedEndpoint)
}

func TestSpeed_Limit(t *testing.T) {
	e := newEdge(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10000000")
		for i := 0; i < 100; i++ {
			if _, err := w.Write(make([]byte, 1000)); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	})
	start := time.Now()
	mbps, err := prober(e.pki, e.srv.Listener.Addr().String(), nil).Speed(context.Background(), ip, 10_000_000, 300*time.Millisecond)
	require.NoError(t, err)
	require.Greater(t, mbps, 0.0)
	require.Less(t, time.Since(start), time.Second)
}

func TestSpeedTop_OnlyTopOK(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	e := newEdge(t, down(t, nil, &inFlight, &maxInFlight))
	rs := []cfscan.Result{{IP: "104.16.0.1", OK: false}}
	for i := 2; i <= 12; i++ {
		rs = append(rs, cfscan.Result{IP: netip.AddrFrom4([4]byte{104, 16, 0, byte(i)}).String(), OK: true})
	}
	var each atomic.Int32
	err := cfscan.SpeedTop(context.Background(), rs, prober(e.pki, e.srv.Listener.Addr().String(), nil), 10, 100_000, 5*time.Second, func(cfscan.Result) { each.Add(1) })
	require.NoError(t, err)
	require.Zero(t, rs[0].Mbps)
	for _, r := range rs[1:11] {
		require.Greater(t, r.Mbps, 0.0, r.IP)
	}
	require.Zero(t, rs[11].Mbps)
	require.Equal(t, int32(10), each.Load())
	require.Equal(t, int32(1), maxInFlight.Load())
}

func TestSpeedTop_StopsOnNoEndpoint(t *testing.T) {
	var reqs atomic.Int32
	e := newEdge(t, func(w http.ResponseWriter, r *http.Request) { reqs.Add(1); http.NotFound(w, r) })
	rs := []cfscan.Result{{IP: "104.16.0.1", OK: true}, {IP: "104.16.0.2", OK: true}}
	err := cfscan.SpeedTop(context.Background(), rs, prober(e.pki, e.srv.Listener.Addr().String(), nil), 10, 100_000, time.Second, nil)
	require.ErrorIs(t, err, cfscan.ErrNoSpeedEndpoint)
	require.Equal(t, int32(1), reqs.Load())
}

func TestCache_PutKeeps100(t *testing.T) {
	var rs []cfscan.Result
	for i := 0; i < 150; i++ {
		rs = append(rs, cfscan.Result{IP: strconv.Itoa(i), OK: i%3 != 0, LatencyMs: int64(i)})
	}
	c := &cfscan.Cache{}
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	c.Put("net1", now, host, rs)
	e, ok := c.Get("net1")
	require.True(t, ok)
	require.Len(t, e.Results, 100)
	require.Equal(t, now, e.ScannedAt)
	require.Equal(t, host, e.Host)
	for i, r := range e.Results {
		require.True(t, r.OK)
		if i > 0 {
			require.LessOrEqual(t, e.Results[i-1].LatencyMs, r.LatencyMs)
		}
	}
}

func TestCache_PerNetworkAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfscan-cache.json")
	c := cfscan.LoadCache(path)
	c.Put("a", time.Now(), host, []cfscan.Result{{IP: "1", OK: true}})
	c.Put("b", time.Now(), host, []cfscan.Result{{IP: "2", OK: true}})
	require.NoError(t, cfscan.SaveCache(path, c))
	c2 := cfscan.LoadCache(path)
	a, _ := c2.Get("a")
	b, _ := c2.Get("b")
	require.Equal(t, "1", a.Results[0].IP)
	require.Equal(t, "2", b.Results[0].IP)
	_, ok := c2.Get("zzz")
	require.False(t, ok)
}

func TestLoadCache_Corrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfscan-cache.json")
	require.NoError(t, os.WriteFile(path, []byte("{nope"), 0o644))
	c := cfscan.LoadCache(path)
	_, ok := c.Get("a")
	require.False(t, ok)
}
