package scanner_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestIsPublicIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"10.0.0.1": false, "192.168.1.1": false, "172.16.0.1": false, "127.0.0.1": false,
		"100.64.0.1": false, "192.0.2.1": false, "0.0.0.0": false, "169.254.1.1": false,
		"224.0.0.1": false, "255.255.255.255": false, "::1": false, "fd00::1": false,
		"fe80::1": false, "2001:db8::1": false, "198.18.0.1": false,
	} {
		require.Equal(t, want, scanner.IsPublicIP(netip.MustParseAddr(ip)), ip)
	}
}

type answerUp struct {
	fn func(ctx context.Context, req *dns.Msg) (*dns.Msg, error)
}

func (u *answerUp) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	return u.fn(ctx, req)
}
func (u *answerUp) Address() string { return "fake" }
func (u *answerUp) Close() error    { return nil }

func withA(ip string) func(context.Context, *dns.Msg) (*dns.Msg, error) {
	return func(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
		m := new(dns.Msg).SetReply(req)
		m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP(ip)}}
		return m, nil
	}
}

func checker(fn func(context.Context, *dns.Msg) (*dns.Msg, error)) scanner.DNSChecker {
	return scanner.DNSChecker{
		Build:      func(model.Server) (upstream.Upstream, error) { return &answerUp{fn: fn}, nil },
		TestDomain: "www.google.com", Timeout: 200 * time.Millisecond, Now: time.Now,
	}
}

func TestDNSChecker_ClassifiesAnswers(t *testing.T) {
	s := model.Server{ID: "x"}
	r := checker(withA("142.250.1.1")).Check(context.Background(), s)
	require.True(t, r.OK)
	require.Equal(t, "x", r.ServerID)

	r = checker(withA("10.0.0.1")).Check(context.Background(), s)
	require.False(t, r.OK)
	require.Equal(t, "poisoned", r.Reason)

	r = checker(func(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
		return new(dns.Msg).SetRcode(req, dns.RcodeNameError), nil
	}).Check(context.Background(), s)
	require.Equal(t, "rcode:NXDOMAIN", r.Reason)

	r = checker(func(_ context.Context, req *dns.Msg) (*dns.Msg, error) { return new(dns.Msg).SetReply(req), nil }).Check(context.Background(), s)
	require.Equal(t, "empty", r.Reason)

	r = checker(func(ctx context.Context, _ *dns.Msg) (*dns.Msg, error) { <-ctx.Done(); return nil, ctx.Err() }).Check(context.Background(), s)
	require.Equal(t, "timeout", r.Reason)

	r = checker(func(context.Context, *dns.Msg) (*dns.Msg, error) { return nil, errors.New("boom") }).Check(context.Background(), s)
	require.Equal(t, "error", r.Reason)
}

func TestDNSChecker_MeasuresSecondQuery(t *testing.T) {
	var n atomic.Int32
	r := checker(func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
		if n.Add(1) == 1 {
			time.Sleep(100 * time.Millisecond) // TLS warm-up
		}
		return withA("142.250.1.1")(ctx, req)
	}).Check(context.Background(), model.Server{ID: "x"})
	require.True(t, r.OK)
	require.Less(t, r.Latency, 50*time.Millisecond)
	require.Equal(t, int32(2), n.Load())
}

// fakeChecker returns OK after delay, tracking concurrency.
type fakeChecker struct {
	delay   time.Duration
	block   bool
	cur     atomic.Int32
	max     atomic.Int32
	checked atomic.Int32
	okIDs   map[string]time.Duration
}

func (f *fakeChecker) Check(ctx context.Context, s model.Server) scanner.Result {
	c := f.cur.Add(1)
	defer f.cur.Add(-1)
	for {
		m := f.max.Load()
		if c <= m || f.max.CompareAndSwap(m, c) {
			break
		}
	}
	f.checked.Add(1)
	if f.block {
		<-ctx.Done()
		return scanner.Result{ServerID: s.ID, Reason: "timeout"}
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return scanner.Result{ServerID: s.ID, Reason: "timeout"}
	}
	if f.okIDs != nil {
		lat, ok := f.okIDs[s.ID]
		return scanner.Result{ServerID: s.ID, OK: ok, Latency: lat}
	}
	return scanner.Result{ServerID: s.ID, OK: true, Latency: f.delay}
}

func list(n int) []model.Server {
	out := make([]model.Server, n)
	for i := range out {
		out[i] = model.Server{ID: fmt.Sprintf("s%02d", i)}
	}
	return out
}

func TestScan_StopsAtWant(t *testing.T) {
	fc := &fakeChecker{delay: 10 * time.Millisecond}
	rs := scanner.Scan(context.Background(), list(40), fc, scanner.Options{Workers: 4, Want: 5})
	ok := 0
	for _, r := range rs {
		if r.OK {
			ok++
		}
	}
	require.Equal(t, 5, ok)
	require.Less(t, int(fc.checked.Load()), 40)
}

func TestScan_RespectsWorkerLimit(t *testing.T) {
	fc := &fakeChecker{delay: 5 * time.Millisecond}
	scanner.Scan(context.Background(), list(80), fc, scanner.Options{Workers: 16})
	require.LessOrEqual(t, fc.max.Load(), int32(16))
	require.Equal(t, int32(80), fc.checked.Load())
}

func TestScan_BudgetStops(t *testing.T) {
	fc := &fakeChecker{block: true}
	start := time.Now()
	scanner.Scan(context.Background(), list(10), fc, scanner.Options{Workers: 2, Budget: 200 * time.Millisecond})
	require.Less(t, time.Since(start), 400*time.Millisecond)
}

func TestScan_SortsOKByLatency(t *testing.T) {
	fc := &fakeChecker{okIDs: map[string]time.Duration{"s01": 30 * time.Millisecond, "s02": 10 * time.Millisecond}}
	rs := scanner.Scan(context.Background(), list(3), fc, scanner.Options{Workers: 3})
	require.Equal(t, []string{"s02", "s01", "s00"}, []string{rs[0].ServerID, rs[1].ServerID, rs[2].ServerID})
}

func TestScan_ReportsProgress(t *testing.T) {
	var calls atomic.Int32
	scanner.Scan(context.Background(), list(5), &fakeChecker{}, scanner.Options{Workers: 2,
		OnProgress: func(done, total int, r scanner.Result) { calls.Add(1); require.Equal(t, 5, total) }})
	require.Equal(t, int32(5), calls.Load())
}

func TestOrder_EverOKFirst(t *testing.T) {
	got := scanner.Order(list(6), map[string]bool{"s04": true, "s02": true}, rand.New(rand.NewSource(1)))
	require.Equal(t, "s02", got[0].ID)
	require.Equal(t, "s04", got[1].ID)
	require.Len(t, got, 6)
}

func TestCache_FreshTTLAndPerNetwork(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	k1 := scanner.NetworkKey("192.168.1.1", "aa:bb:cc:dd:ee:ff")
	k2 := scanner.NetworkKey("10.0.0.1", "11:22:33:44:55:66")
	require.Len(t, k1, 64)
	c := &scanner.Cache{}
	c.Put(k1, now, []scanner.Result{{ServerID: "a", OK: true}, {ServerID: "b"}})
	rs, ok := c.Fresh(k1, now.Add(23*time.Hour), 24*time.Hour)
	require.True(t, ok)
	require.Len(t, rs, 2)
	_, ok = c.Fresh(k1, now.Add(25*time.Hour), 24*time.Hour)
	require.False(t, ok)
	_, ok = c.Fresh(k2, now, 24*time.Hour)
	require.False(t, ok)
	require.Equal(t, map[string]bool{"a": true}, c.EverOK())

	path := filepath.Join(t.TempDir(), "scan-cache.json")
	require.NoError(t, scanner.SaveCache(path, c))
	c2, err := scanner.LoadCache(path)
	require.NoError(t, err)
	_, ok = c2.Fresh(k1, now, 24*time.Hour)
	require.True(t, ok)

	empty, err := scanner.LoadCache(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	require.Empty(t, empty.Entries)
}
