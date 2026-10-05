// Package advanced grades DNS servers on latency, loss, DNSSEC validation,
// ad filtering and poisoning (spec 3 §6).
package advanced

import (
	"context"
	"math"
	"math/rand/v2"
	"net/netip"
	"slices"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/miekg/dns"
)

// Tri is a yes/no answer that may be partial or unknown.
type Tri string

const (
	Yes     Tri = "yes"
	No      Tri = "no"
	Partial Tri = "partial"
	Unknown Tri = "unknown"
)

// Options tune the checks.
type Options struct {
	Rounds        int
	Timeout       time.Duration // per query
	TestDomain    string
	PoisonDomains []string
	MaxDuration   time.Duration                              // whole scan; 0 = 10 minutes
	Label         func() string                              // random label for latency rounds; nil = 8 random chars
	Sleep         func(context.Context, time.Duration) error // pause between rounds; nil = timer
}

// Result is one server's grade.
type Result struct {
	ServerID string         `json:"serverId"`
	Reach    scanner.Result `json:"reach"`
	MinMs    int64          `json:"minMs"`
	MedianMs int64          `json:"medianMs"`
	P90Ms    int64          `json:"p90Ms"`
	JitterMs float64        `json:"jitterMs"`
	Loss     float64        `json:"loss"` // 0..1
	DNSSEC   Tri            `json:"dnssec"`
	AdFilter Tri            `json:"adFilter"`
	Poisoned []string       `json:"poisoned"`
}

// ExchangeFunc sends one query; scanner.Exchange is the default.
type ExchangeFunc func(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, time.Duration, error)

// Checker grades one server.
type Checker struct {
	Build    func(model.Server) (upstream.Upstream, error)
	Opt      Options
	Exchange ExchangeFunc // nil = scanner.Exchange
}

// Probe domains (spec 3 §6.2).
const (
	dnssecFailDomain = "dnssec-failed.org"
	dnssecGoodDomain = "cloudflare.com"
)

var adDomains = []string{"doubleclick.net", "googleadservices.com"}

// keepOpen lets the reach check reuse the upstream without closing it.
type keepOpen struct{ upstream.Upstream }

func (keepOpen) Close() error { return nil }

// Run grades s. Checks after the reach check fail independently: a failure
// leaves that criterion Unknown.
func (c Checker) Run(ctx context.Context, s model.Server) Result {
	r := Result{ServerID: s.ID, DNSSEC: Unknown, AdFilter: Unknown, Poisoned: []string{}}
	u, err := c.Build(s)
	if err != nil {
		r.Reach = scanner.Result{ServerID: s.ID, Reason: "error", CheckedAt: time.Now()}
		return r
	}
	defer u.Close()
	r.Reach = scanner.DNSChecker{
		Build:      func(model.Server) (upstream.Upstream, error) { return keepOpen{u}, nil },
		TestDomain: c.Opt.TestDomain, Timeout: c.Opt.Timeout,
	}.Check(ctx, s)
	if !r.Reach.OK {
		return r
	}
	c.latency(ctx, u, &r)
	r.DNSSEC = c.dnssec(ctx, u)
	r.AdFilter = c.adFilter(ctx, u)
	for _, d := range c.Opt.PoisonDomains {
		if m, err := c.query(ctx, u, d, dns.TypeA, false); err == nil && poisoned(m) {
			r.Poisoned = append(r.Poisoned, d)
		}
	}
	return r
}

func (c Checker) query(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, error) {
	m, _, err := c.timed(ctx, u, name, qtype, do)
	return m, err
}

func (c Checker) timed(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, time.Duration, error) {
	ex := c.Exchange
	if ex == nil {
		ex = scanner.Exchange
	}
	ctx, cancel := context.WithTimeout(ctx, c.Opt.Timeout)
	defer cancel()
	return ex(ctx, u, name, qtype, do)
}

func (c Checker) latency(ctx context.Context, u upstream.Upstream, r *Result) {
	label, sleep := c.Opt.Label, c.Opt.Sleep
	if label == nil {
		label = randomLabel
	}
	if sleep == nil {
		sleep = sleepCtx
	}
	var ok []time.Duration
	for i := 0; i < c.Opt.Rounds; i++ {
		if i > 0 && sleep(ctx, 100*time.Millisecond) != nil {
			return
		}
		m, d, err := c.timed(ctx, u, label()+"."+c.Opt.TestDomain, dns.TypeA, false)
		if err == nil && (m.Rcode == dns.RcodeSuccess || m.Rcode == dns.RcodeNameError) {
			ok = append(ok, d)
		}
	}
	if c.Opt.Rounds > 0 {
		r.Loss = float64(c.Opt.Rounds-len(ok)) / float64(c.Opt.Rounds)
	}
	if len(ok) == 0 {
		return
	}
	slices.Sort(ok)
	n := len(ok)
	r.MinMs = ok[0].Milliseconds()
	r.MedianMs = ok[n/2].Milliseconds()
	r.P90Ms = ok[int(math.Ceil(0.9*float64(n)))-1].Milliseconds()
	var sum float64
	for _, d := range ok {
		sum += float64(d.Milliseconds())
	}
	mean := sum / float64(n)
	var sq float64
	for _, d := range ok {
		x := float64(d.Milliseconds()) - mean
		sq += x * x
	}
	r.JitterMs = math.Sqrt(sq / float64(n))
}

// dnssec: a validating resolver answers SERVFAIL for a broken signature.
func (c Checker) dnssec(ctx context.Context, u upstream.Upstream) Tri {
	m, err := c.query(ctx, u, dnssecFailDomain, dns.TypeA, true)
	if err == nil {
		if m.Rcode == dns.RcodeServerFailure {
			return Yes
		}
		if len(addrs(m)) > 0 {
			return No
		}
	}
	if m, err := c.query(ctx, u, dnssecGoodDomain, dns.TypeA, true); err == nil && m.AuthenticatedData {
		return Yes
	}
	return Unknown
}

func (c Checker) adFilter(ctx context.Context, u upstream.Upstream) Tri {
	blocked := 0
	for _, d := range adDomains {
		m, err := c.query(ctx, u, d, dns.TypeA, false)
		if err != nil {
			return Unknown
		}
		if m.Rcode == dns.RcodeNameError || slices.ContainsFunc(addrs(m), func(a netip.Addr) bool { return !scanner.IsPublicIP(a) }) {
			blocked++
		}
	}
	switch blocked {
	case len(adDomains):
		return Yes
	case 0:
		return No
	}
	return Partial
}

func poisoned(m *dns.Msg) bool {
	return m.Rcode == dns.RcodeNameError || slices.ContainsFunc(addrs(m), func(a netip.Addr) bool { return !scanner.IsPublicIP(a) })
}

func addrs(m *dns.Msg) []netip.Addr {
	var out []netip.Addr
	for _, rr := range m.Answer {
		switch v := rr.(type) {
		case *dns.A:
			if a, ok := netip.AddrFromSlice(v.A.To4()); ok {
				out = append(out, a)
			}
		case *dns.AAAA:
			if a, ok := netip.AddrFromSlice(v.AAAA); ok {
				out = append(out, a)
			}
		}
	}
	return out
}

func randomLabel() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.IntN(len(chars))]
	}
	return string(b)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
