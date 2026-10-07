// Package engine runs Ghostline's loopback DNS server on top of dnsproxy.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/miekg/dns"
)

// VerifySuffix marks names the engine answers itself (leak verification and
// self tests); they are never forwarded upstream.
const VerifySuffix = "verify.ghostline.test."

// VerifyAnswer is the A record returned for VerifySuffix names.
var VerifyAnswer = netip.MustParseAddr("192.0.2.1")

// Config describes one engine run.
type Config struct {
	ListenV4     netip.AddrPort
	ListenV6     netip.AddrPort // zero value: no IPv6 listener
	Upstreams    []upstream.Upstream
	CacheEnabled bool
	Logger       *slog.Logger
	// Rules returns the current rules; nil means no rules.
	Rules func() *rules.Compiled
	// BlockMode is "zero" (0.0.0.0 / ::, default) or "nxdomain".
	BlockMode string
}

// ErrNoAddress means a name resolved to no A or AAAA record.
var ErrNoAddress = errors.New("engine: no address for host")

// ruleTTL is the TTL of answers synthesised from rules.
const ruleTTL = 60

// QueryEvent describes one forwarded query (kept in RAM only by callers).
type QueryEvent struct {
	Time     time.Time
	Domain   string
	Type     string
	Upstream string
	Latency  time.Duration
	Err      string
	Cached   bool
	Action   string // "" | "blocked" | "rewritten"
}

// UpstreamStat aggregates results per upstream address.
type UpstreamStat struct {
	Queries    uint64
	Errors     uint64
	AvgLatency time.Duration
	LastErrAt  time.Time
	LastOKAt   time.Time
}

// Stats is a snapshot of engine counters.
type Stats struct {
	Queries     uint64
	AvgLatency  time.Duration
	PerUpstream map[string]UpstreamStat
}

// Engine is a restartable loopback DNS server.
type Engine struct {
	onQuery func(QueryEvent)

	mu      sync.Mutex
	p       *proxy.Proxy
	cfg     Config
	addr    netip.AddrPort
	addr6   netip.AddrPort
	expect  map[string]bool
	seen    map[string]bool
	queries uint64
	latSum  time.Duration
	latN    uint64
	per     map[string]UpstreamStat

	// DNS server for this PC and the LAN (serve.go).
	serve        *proxy.Proxy
	serveDoH     []*http.Server // Ghostline-run DoH listeners of serve
	serveQueries uint64
	clients      map[netip.Addr]time.Time
	rates        map[netip.Addr]*rateWindow
}

// New creates an engine; onQuery may be nil.
func New(onQuery func(QueryEvent)) *Engine {
	return &Engine{onQuery: onQuery, expect: map[string]bool{}, seen: map[string]bool{}, per: map[string]UpstreamStat{}}
}

// Start begins serving on the configured loopback addresses.
func (e *Engine) Start(ctx context.Context, cfg Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.p != nil {
		return errors.New("engine: already running")
	}
	e.cfg = cfg
	e.queries, e.latSum, e.latN, e.per = 0, 0, 0, map[string]UpstreamStat{}
	return e.startLocked(ctx, cfg.ListenV4, cfg.ListenV6, cfg.Upstreams)
}

func (e *Engine) startLocked(ctx context.Context, v4, v6 netip.AddrPort, ups []upstream.Upstream) error {
	udp := []*net.UDPAddr{net.UDPAddrFromAddrPort(v4)}
	tcp := []*net.TCPAddr{net.TCPAddrFromAddrPort(v4)}
	if v6.IsValid() {
		udp = append(udp, net.UDPAddrFromAddrPort(v6))
		tcp = append(tcp, net.TCPAddrFromAddrPort(v6))
	}
	logger := e.cfg.Logger
	if logger == nil {
		// dnsproxy error logs can carry DoH URLs with the base64 query, i.e.
		// the domain. They must never reach the file log.
		logger = slog.New(slog.DiscardHandler)
	}
	p, err := proxy.New(&proxy.Config{
		Logger:         logger,
		UDPListenAddr:  udp,
		TCPListenAddr:  tcp,
		UpstreamConfig: &proxy.UpstreamConfig{Upstreams: ups},
		UpstreamMode:   proxy.UpstreamModeParallel,
		CacheEnabled:   e.cfg.CacheEnabled,
		RequestHandler: proxy.HandlerFunc(e.handle),
	})
	if err != nil {
		slog.Warn("engine: creating the DNS proxy failed", "upstreams", len(ups), "err", err)
		return fmt.Errorf("engine: %w", err)
	}
	if err := p.Start(ctx); err != nil {
		slog.Warn("engine: starting the DNS proxy failed", "listen", v4, "listen6", v6, "err", err)
		return fmt.Errorf("engine: %w", err)
	}
	e.p = p
	// With port 0 the OS picks one per listener; remember the real UDP one.
	if a, ok := p.Addr(proxy.ProtoUDP).(*net.UDPAddr); ok {
		e.addr = a.AddrPort()
		e.addr = netip.AddrPortFrom(e.addr.Addr().Unmap(), e.addr.Port())
	}
	e.addr6 = v6
	return nil
}

// Swap restarts the proxy with new upstreams on the same addresses. The old
// upstreams are closed.
func (e *Engine) Swap(ctx context.Context, ups []upstream.Upstream) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.p == nil {
		return errors.New("engine: not running")
	}
	v4 := e.addr
	if err := e.p.Shutdown(ctx); err != nil {
		slog.Warn("engine: shutdown before upstream swap failed", "err", err)
		return fmt.Errorf("engine: shutdown: %w", err)
	}
	e.p = nil
	e.per = map[string]UpstreamStat{} // old upstreams must not count
	return e.startLocked(ctx, v4, e.addr6, ups)
}

// Stop shuts the proxy down and closes its upstreams.
func (e *Engine) Stop(ctx context.Context) error {
	if err := e.StopServe(ctx); err != nil {
		slog.Warn("engine: stopping the DNS server failed", "err", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.p == nil {
		return nil
	}
	err := e.p.Shutdown(ctx)
	e.p = nil
	if err != nil {
		slog.Warn("engine: shutdown failed", "err", err)
	}
	return err
}

// ListenAddr is the actual UDP address the engine serves on.
func (e *Engine) ListenAddr() netip.AddrPort {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.addr
}

// ExpectVerify registers a nonce whose <nonce>.verify.ghostline.test query
// should reach the engine.
func (e *Engine) ExpectVerify(nonce string) {
	e.mu.Lock()
	e.expect[strings.ToLower(nonce)] = true
	e.mu.Unlock()
}

// SawVerify reports whether the nonce's query reached the engine.
func (e *Engine) SawVerify(nonce string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seen[strings.ToLower(nonce)]
}

// SelfTest queries the engine through its own listener.
func (e *Engine) SelfTest(ctx context.Context) error {
	e.mu.Lock()
	running, addr := e.p != nil, e.addr
	e.mu.Unlock()
	if !running {
		return errors.New("engine: not running")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "selftest-" + hex.EncodeToString(b) + "." + VerifySuffix
	c := &dns.Client{Timeout: 3 * time.Second}
	r, _, err := c.ExchangeContext(ctx, new(dns.Msg).SetQuestion(name, dns.TypeA), addr.String())
	if err != nil {
		return fmt.Errorf("engine: self test: %w", err)
	}
	if len(r.Answer) == 0 {
		return errors.New("engine: self test: empty answer")
	}
	return nil
}

// Stats returns a copy of the counters.
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Stats{Queries: e.queries, PerUpstream: make(map[string]UpstreamStat, len(e.per))}
	if e.latN > 0 {
		st.AvgLatency = e.latSum / time.Duration(e.latN)
	}
	for k, v := range e.per {
		st.PerUpstream[k] = v
	}
	return st
}

func (e *Engine) handle(ctx context.Context, p *proxy.Proxy, d *proxy.DNSContext) error {
	if len(d.Req.Question) == 1 {
		name := strings.ToLower(d.Req.Question[0].Name)
		if strings.HasSuffix(name, "."+VerifySuffix) {
			nonce := strings.TrimSuffix(name, "."+VerifySuffix)
			e.mu.Lock()
			if e.expect[nonce] {
				e.seen[nonce] = true
			}
			e.mu.Unlock()
			m := new(dns.Msg).SetReply(d.Req)
			if d.Req.Question[0].Qtype == dns.TypeA {
				m.Answer = []dns.RR{&dns.A{
					Hdr: dns.RR_Header{Name: d.Req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
					A:   VerifyAnswer.AsSlice(),
				}}
			}
			d.Res = m
			return nil
		}
		if m := e.applyRules(d.Req, name); m != nil {
			d.Res = m
			return nil
		}
	}

	started := time.Now()
	err := p.Resolve(ctx, d)
	e.record(d, started, err)
	return err
}

func (e *Engine) record(d *proxy.DNSContext, started time.Time, err error) {
	ev := QueryEvent{Time: started, Latency: time.Since(started)}
	if len(d.Req.Question) > 0 {
		ev.Domain = d.Req.Question[0].Name
		ev.Type = dns.TypeToString[d.Req.Question[0].Qtype]
	}
	if d.Upstream != nil {
		ev.Upstream = d.Upstream.Address()
	}
	if err != nil {
		ev.Err = err.Error()
	}
	e.mu.Lock()
	e.queries++
	if qs := d.QueryStatistics(); qs != nil {
		for _, us := range qs.Main() {
			s := e.per[us.Address]
			s.Queries++
			if us.Error != nil {
				s.Errors++
				s.LastErrAt = started
			} else {
				s.AvgLatency = (s.AvgLatency*time.Duration(s.Queries-1) + us.QueryDuration) / time.Duration(s.Queries)
				s.LastOKAt = started
			}
			ev.Cached = ev.Cached || us.IsCached
			e.per[us.Address] = s
		}
	}
	if err == nil && !ev.Cached {
		e.latSum += ev.Latency
		e.latN++
	}
	e.mu.Unlock()
	if e.onQuery != nil {
		e.onQuery(ev)
	}
}

// applyRules answers req from the rules, or returns nil to forward it.
func (e *Engine) applyRules(req *dns.Msg, name string) *dns.Msg {
	e.mu.Lock()
	get, mode := e.cfg.Rules, e.cfg.BlockMode
	e.mu.Unlock()
	if get == nil {
		return nil
	}
	dec := get().Match(strings.TrimSuffix(name, "."), netip.Addr{})
	if !dec.Block && len(dec.IPs) == 0 {
		return nil
	}
	q := req.Question[0]
	m := new(dns.Msg).SetReply(req)
	hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: ruleTTL}
	action := "rewritten"
	switch {
	case dec.Block && mode == "nxdomain":
		m.Rcode = dns.RcodeNameError
		action = "blocked"
	case dec.Block:
		action = "blocked"
		switch q.Qtype {
		case dns.TypeA:
			hdr.Rrtype = dns.TypeA
			m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: net.IPv4zero.To4()}}
		case dns.TypeAAAA:
			hdr.Rrtype = dns.TypeAAAA
			m.Answer = []dns.RR{&dns.AAAA{Hdr: hdr, AAAA: net.IPv6unspecified}}
		}
	default:
		for _, ip := range dec.IPs {
			switch {
			case q.Qtype == dns.TypeA && ip.Is4():
				h := hdr
				h.Rrtype = dns.TypeA
				m.Answer = append(m.Answer, &dns.A{Hdr: h, A: ip.AsSlice()})
			case q.Qtype == dns.TypeAAAA && ip.Is6():
				h := hdr
				h.Rrtype = dns.TypeAAAA
				m.Answer = append(m.Answer, &dns.AAAA{Hdr: h, AAAA: ip.AsSlice()})
			}
		}
	}
	if e.onQuery != nil {
		e.onQuery(QueryEvent{Time: time.Now(), Domain: q.Name, Type: dns.TypeToString[q.Qtype], Action: action})
	}
	return m
}

// Resolve looks host up through the engine's own listener (so rules, cache
// and the encrypted upstreams apply) and returns IPv4 addresses first, then
// IPv6. It never uses the system resolver.
func (e *Engine) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	e.mu.Lock()
	running, addr := e.p != nil, e.addr
	e.mu.Unlock()
	if !running {
		return nil, errors.New("engine: not running")
	}
	name := dns.Fqdn(strings.ToLower(host))
	type res struct {
		ips []netip.Addr
		err error
	}
	ask := func(qt uint16, out chan<- res) {
		c := &dns.Client{Timeout: 3 * time.Second}
		r, _, err := c.ExchangeContext(ctx, new(dns.Msg).SetQuestion(name, qt), addr.String())
		if err != nil {
			out <- res{err: err}
			return
		}
		var ips []netip.Addr
		for _, rr := range r.Answer {
			switch v := rr.(type) {
			case *dns.A:
				if qt == dns.TypeA {
					if a, ok := netip.AddrFromSlice(v.A.To4()); ok {
						ips = append(ips, a)
					}
				}
			case *dns.AAAA:
				if qt == dns.TypeAAAA {
					if a, ok := netip.AddrFromSlice(v.AAAA); ok {
						ips = append(ips, a)
					}
				}
			}
		}
		out <- res{ips: ips}
	}
	c4, c6 := make(chan res, 1), make(chan res, 1)
	go ask(dns.TypeA, c4)
	go ask(dns.TypeAAAA, c6)
	r4, r6 := <-c4, <-c6
	ips := append(r4.ips, r6.ips...)
	if len(ips) > 0 {
		return ips, nil
	}
	if r4.err != nil {
		return nil, fmt.Errorf("engine: resolve: %w", r4.err)
	}
	return nil, ErrNoAddress
}
