package cfscan_test

import (
	"context"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

func sample(n int) []netip.Addr {
	return cfscan.Sample(cfscan.Ranges(), n, rand.New(rand.NewPCG(7, 7)))
}

type countLimiter struct{ n atomic.Int32 }

func (l *countLimiter) Wait(ctx context.Context) error { l.n.Add(1); return ctx.Err() }

func TestScan_WantStopsEarly(t *testing.T) {
	e := newEdge(t, trace("HKG"))
	var dials atomic.Int32
	lim := &countLimiter{}
	rs, err := cfscan.Scan(context.Background(), sample(100), prober(e.pki, e.srv.Listener.Addr().String(), &dials),
		cfscan.Options{Concurrency: 4, Want: 10, Limiter: lim})
	require.NoError(t, err)
	ok := 0
	for _, r := range rs {
		if r.OK {
			ok++
		}
	}
	require.Equal(t, 10, ok)
	require.Less(t, int(dials.Load()), 100)
	// Every dial waited first; a worker may also wait and then stop at cancel.
	require.LessOrEqual(t, dials.Load(), lim.n.Load(), "every probe waits on the limiter first")
}

func TestScan_NoNetworkStopsEarly(t *testing.T) {
	var dials atomic.Int32
	p := cfscan.Prober{Host: host, Timeout: 20 * time.Millisecond, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		dials.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	start := time.Now()
	_, err := cfscan.Scan(context.Background(), sample(2000), p, cfscan.Options{Concurrency: 16, Limiter: &countLimiter{}})
	require.ErrorIs(t, err, cfscan.ErrNoNetwork)
	require.LessOrEqual(t, int(dials.Load()), 200+16)
	require.Less(t, time.Since(start), 2*time.Second)
}

func TestScan_ProgressAndSort(t *testing.T) {
	e := newEdge(t, trace("SIN"))
	var calls atomic.Int32
	var lastTried, lastOK atomic.Int32
	rs, err := cfscan.Scan(context.Background(), sample(20), prober(e.pki, e.srv.Listener.Addr().String(), nil),
		cfscan.Options{Concurrency: 4, Limiter: &countLimiter{}, OnProgress: func(tried, ok int, r cfscan.Result) {
			calls.Add(1)
			lastTried.Store(int32(tried))
			lastOK.Store(int32(ok))
		}})
	require.NoError(t, err)
	require.Len(t, rs, 20)
	require.Equal(t, int32(20), calls.Load())
	require.Equal(t, int32(20), lastTried.Load())
	require.Equal(t, int32(20), lastOK.Load())
}

// trackConn counts itself once in closed on its first Close.
type trackConn struct {
	net.Conn
	once   *sync.Once
	closed *atomic.Int32
}

func (c trackConn) Close() error {
	c.once.Do(func() { c.closed.Add(1) })
	return c.Conn.Close()
}

func TestScan_CancelClosesConns(t *testing.T) {
	var opened, closed atomic.Int32
	var mu sync.Mutex
	var peers []net.Conn
	p := cfscan.Prober{Host: host, Timeout: time.Minute, Dial: func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe() // nobody answers the ClientHello
		mu.Lock()
		peers = append(peers, b)
		mu.Unlock()
		opened.Add(1)
		go func() {
			buf := make([]byte, 4096)
			for {
				if _, err := b.Read(buf); err != nil {
					return
				}
			}
		}()
		return trackConn{Conn: a, once: &sync.Once{}, closed: &closed}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := cfscan.Scan(ctx, sample(500), p, cfscan.Options{Concurrency: 64, Limiter: &countLimiter{}})
	require.NoError(t, err)
	require.Less(t, time.Since(start), 1200*time.Millisecond)
	require.Equal(t, int32(64), opened.Load())
	require.Equal(t, opened.Load(), closed.Load())
	mu.Lock()
	for _, b := range peers {
		_ = b.Close()
	}
	mu.Unlock()
}

func TestScan_RejectsOutOfRange(t *testing.T) {
	var dials atomic.Int32
	p := cfscan.Prober{Host: host, Timeout: time.Second, Dial: func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, net.ErrClosed
	}}
	rs, _ := cfscan.Scan(context.Background(), []netip.Addr{netip.MustParseAddr("8.8.8.8")}, p, cfscan.Options{Concurrency: 8, Limiter: &countLimiter{}})
	require.Zero(t, dials.Load())
	require.Empty(t, rs)
}

func TestNewLimiter_Rate(t *testing.T) {
	l := cfscan.NewLimiter(200)
	start := time.Now()
	for range 50 {
		require.NoError(t, l.Wait(context.Background()))
	}
	require.GreaterOrEqual(t, time.Since(start), 240*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slow := cfscan.NewLimiter(1)
	_ = slow.Wait(context.Background())
	require.Error(t, slow.Wait(ctx))
}

func TestSort(t *testing.T) {
	rs := []cfscan.Result{
		{IP: "a", OK: false},
		{IP: "b", OK: true, LatencyMs: 50},
		{IP: "c", OK: true, LatencyMs: 10},
		{IP: "d", OK: true, LatencyMs: 90, Mbps: 20},
		{IP: "e", OK: true, LatencyMs: 80, Mbps: 50},
	}
	cfscan.Sort(rs)
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.IP)
	}
	require.Equal(t, []string{"e", "d", "c", "b", "a"}, ids)
}
