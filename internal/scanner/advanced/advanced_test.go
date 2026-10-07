package advanced_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/scanner/advanced"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// fakeUp answers by name through fn; a nil reply with a nil error blocks
// until ctx ends.
type fakeUp struct {
	fn    func(name string, req *dns.Msg) (*dns.Msg, error)
	mu    sync.Mutex
	names []string
}

func (u *fakeUp) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	name := strings.TrimSuffix(req.Question[0].Name, ".")
	u.mu.Lock()
	u.names = append(u.names, name)
	u.mu.Unlock()
	m, err := u.fn(name, req)
	if m == nil && err == nil {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return m, err
}
func (u *fakeUp) Address() string { return "fake" }
func (u *fakeUp) Close() error    { return nil }

func a(req *dns.Msg, ip string) *dns.Msg {
	m := new(dns.Msg).SetReply(req)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP(ip)}}
	return m
}

func rcode(req *dns.Msg, rc int) *dns.Msg { return new(dns.Msg).SetRcode(req, rc) }

// honest is a good resolver: public IPs, validates DNSSEC, no ad filtering.
func honest(name string, req *dns.Msg) (*dns.Msg, error) {
	if name == "dnssec-failed.org" {
		return rcode(req, dns.RcodeServerFailure), nil
	}
	return a(req, "93.184.216.34"), nil
}

func isRound(name string) bool {
	return strings.HasPrefix(name, "r") && strings.HasSuffix(name, ".www.google.com")
}

func newChecker(fn func(string, *dns.Msg) (*dns.Msg, error), latencies ...time.Duration) (advanced.Checker, *fakeUp) {
	u := &fakeUp{fn: fn}
	var i atomic.Int32
	c := advanced.Checker{
		Build: func(model.Server) (upstream.Upstream, error) { return u, nil },
		Opt: advanced.Options{
			Rounds: 5, Timeout: 200 * time.Millisecond, TestDomain: "www.google.com",
			PoisonDomains: []string{"youtube.com"},
			Label:         func() string { return fmt.Sprintf("r%07d", i.Add(1)) },
			Sleep:         func(context.Context, time.Duration) error { return nil },
		},
	}
	if latencies != nil {
		var n atomic.Int32
		c.Exchange = func(ctx context.Context, up upstream.Upstream, name string, qt uint16, do bool) (*dns.Msg, time.Duration, error) {
			m, d, err := scanner.Exchange(ctx, up, name, qt, do)
			if isRound(name) {
				d = latencies[int(n.Add(1)-1)%len(latencies)]
			}
			return m, d, err
		}
	}
	return c, u
}

var srv = model.Server{ID: "s1"}

func TestRun_LatencyStats(t *testing.T) {
	ms := time.Millisecond
	c, u := newChecker(honest, 30*ms, 10*ms, 50*ms, 20*ms, 40*ms)
	r := c.Run(context.Background(), srv)
	require.True(t, r.Reach.OK)
	require.Equal(t, "s1", r.ServerID)
	require.Equal(t, int64(10), r.MinMs)
	require.Equal(t, int64(30), r.MedianMs)
	require.Equal(t, int64(50), r.P90Ms)
	require.InDelta(t, 14.14, r.JitterMs, 0.01)
	require.Zero(t, r.Loss)
	require.Equal(t, advanced.Tri("yes"), r.DNSSEC)
	require.Equal(t, advanced.Tri("no"), r.AdFilter)
	require.Empty(t, r.Poisoned)

	var rounds []string
	for _, n := range u.names {
		if isRound(n) {
			rounds = append(rounds, n)
		}
	}
	require.Equal(t, []string{"r0000001.www.google.com", "r0000002.www.google.com", "r0000003.www.google.com", "r0000004.www.google.com", "r0000005.www.google.com"}, rounds)
}

func TestRun_Loss(t *testing.T) {
	var n atomic.Int32
	c, _ := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
		if isRound(name) {
			switch n.Add(1) {
			case 2, 4:
				return nil, nil // timeout
			case 3:
				return rcode(req, dns.RcodeNameError), nil // NXDOMAIN counts as success
			}
		}
		return honest(name, req)
	})
	r := c.Run(context.Background(), srv)
	require.InDelta(t, 0.4, r.Loss, 1e-9)
}

func TestRun_DNSSEC(t *testing.T) {
	for i, tc := range []struct {
		fn   func(*dns.Msg) (*dns.Msg, error)
		ad   bool
		want advanced.Tri
	}{
		{func(r *dns.Msg) (*dns.Msg, error) { return rcode(r, dns.RcodeServerFailure), nil }, false, "yes"},
		{func(r *dns.Msg) (*dns.Msg, error) { return a(r, "1.2.3.4"), nil }, false, "no"},
		{func(r *dns.Msg) (*dns.Msg, error) { return nil, errors.New("boom") }, false, "unknown"},
		{func(r *dns.Msg) (*dns.Msg, error) { return nil, errors.New("boom") }, true, "yes"},
	} {
		c, _ := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
			switch name {
			case "dnssec-failed.org":
				return tc.fn(req)
			case "cloudflare.com":
				m := a(req, "104.16.1.1")
				m.AuthenticatedData = tc.ad
				return m, nil
			}
			return honest(name, req)
		})
		require.Equal(t, tc.want, c.Run(context.Background(), srv).DNSSEC, i)
	}
}

func TestRun_AdFilter(t *testing.T) {
	blocked0 := func(r *dns.Msg) (*dns.Msg, error) { return a(r, "0.0.0.0"), nil }
	blockedNX := func(r *dns.Msg) (*dns.Msg, error) { return rcode(r, dns.RcodeNameError), nil }
	blockedPriv := func(r *dns.Msg) (*dns.Msg, error) { return a(r, "10.0.0.1"), nil }
	open := func(r *dns.Msg) (*dns.Msg, error) { return a(r, "142.250.1.1"), nil }
	broken := func(r *dns.Msg) (*dns.Msg, error) { return nil, errors.New("x") }
	for i, tc := range []struct {
		dc, gas func(*dns.Msg) (*dns.Msg, error)
		want    advanced.Tri
	}{
		{blocked0, blockedNX, "yes"},
		{blockedPriv, open, "partial"},
		{open, open, "no"},
		{broken, open, "unknown"},
	} {
		c, _ := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
			switch name {
			case "doubleclick.net":
				return tc.dc(req)
			case "googleadservices.com":
				return tc.gas(req)
			}
			return honest(name, req)
		})
		require.Equal(t, tc.want, c.Run(context.Background(), srv).AdFilter, i)
	}
}

func TestRun_Poisoned(t *testing.T) {
	c, _ := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
		switch name {
		case "youtube.com":
			return a(req, "10.10.34.35"), nil
		case "x.com":
			return rcode(req, dns.RcodeNameError), nil
		}
		return honest(name, req)
	})
	c.Opt.PoisonDomains = []string{"youtube.com", "discord.com", "x.com"}
	require.Equal(t, []string{"youtube.com", "x.com"}, c.Run(context.Background(), srv).Poisoned)
}

func TestRun_ReachFailStops(t *testing.T) {
	c, u := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
		return rcode(req, dns.RcodeRefused), nil
	})
	r := c.Run(context.Background(), srv)
	require.False(t, r.Reach.OK)
	require.Equal(t, "rcode:REFUSED", r.Reach.Reason)
	require.Len(t, u.names, 1)
	require.Equal(t, advanced.Tri("unknown"), r.DNSSEC)
	require.Equal(t, advanced.Tri("unknown"), r.AdFilter)
}

func TestSort(t *testing.T) {
	ok := scanner.Result{OK: true}
	rs := []advanced.Result{
		{ServerID: "down"},
		{ServerID: "poison", Reach: ok, Poisoned: []string{"x"}, MedianMs: 1},
		{ServerID: "lossy", Reach: ok, Loss: 0.4, MedianMs: 5},
		{ServerID: "slow", Reach: ok, Loss: 0.04, MedianMs: 90},
		{ServerID: "fast", Reach: ok, Loss: 0, MedianMs: 20},
	}
	advanced.Sort(rs)
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.ServerID)
	}
	require.Equal(t, []string{"fast", "slow", "lossy", "down", "poison"}, ids)
}

func servers(n int) []model.Server {
	out := make([]model.Server, n)
	for i := range out {
		out[i] = model.Server{ID: fmt.Sprint("s", i)}
	}
	return out
}

func TestScan_TooMany(t *testing.T) {
	c, _ := newChecker(honest)
	_, err := advanced.Scan(context.Background(), servers(501), c, 8, nil)
	require.ErrorIs(t, err, advanced.ErrTooMany)
}

func TestScan_AllWithProgress(t *testing.T) {
	c, _ := newChecker(honest)
	var last, calls atomic.Int32
	rs, err := advanced.Scan(context.Background(), servers(20), c, 4, func(done, total int, r advanced.Result) {
		if total != 20 {
			panic("total")
		}
		calls.Add(1)
		last.Store(int32(done))
	})
	require.NoError(t, err)
	require.Len(t, rs, 20)
	require.Equal(t, int32(20), last.Load(), "the final progress call is never throttled")
	require.LessOrEqual(t, calls.Load(), int32(3), "fast scans are throttled to ~10 calls/s")
}

func TestScan_CancelFast(t *testing.T) {
	// Once one server is done, every other query hangs and the scan is
	// cancelled: it must stop at once and keep that result. (Hanging after a
	// fixed number of queries was flaky: 8 servers in parallel could share
	// them and none finish.)
	var first atomic.Bool
	c, _ := newChecker(func(name string, req *dns.Msg) (*dns.Msg, error) {
		if first.Load() {
			return nil, nil // block until cancelled
		}
		return honest(name, req)
	})
	c.Opt.Timeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	rs, err := advanced.Scan(ctx, servers(50), c, 8, func(int, int, advanced.Result) {
		if !first.Swap(true) {
			cancel()
		}
	})
	require.NoError(t, err)
	require.Less(t, time.Since(start), 1200*time.Millisecond)
	require.NotEmpty(t, rs)
	require.Less(t, len(rs), 50)
}

func TestScan_DurationCap(t *testing.T) {
	c, _ := newChecker(func(string, *dns.Msg) (*dns.Msg, error) { return nil, nil })
	c.Opt.Timeout = time.Minute
	c.Opt.MaxDuration = 100 * time.Millisecond
	start := time.Now()
	_, err := advanced.Scan(context.Background(), servers(10), c, 4, nil)
	require.NoError(t, err)
	require.Less(t, time.Since(start), time.Second)
}

func TestScan_MaxServersOption(t *testing.T) {
	c, _ := newChecker(honest)
	c.Opt.MaxServers = 10
	_, err := advanced.Scan(context.Background(), servers(11), c, 4, nil)
	require.ErrorIs(t, err, advanced.ErrTooMany)
	c.Opt.MaxServers = 600
	rs, err := advanced.Scan(context.Background(), servers(501), c, 32, nil)
	require.NoError(t, err, "a configured limit above 500 is honoured")
	require.Len(t, rs, 501)
}
