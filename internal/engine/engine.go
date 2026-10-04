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
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
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
}

// QueryEvent describes one forwarded query (kept in RAM only by callers).
type QueryEvent struct {
	Time     time.Time
	Domain   string
	Type     string
	Upstream string
	Latency  time.Duration
	Err      string
	Cached   bool
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
		return fmt.Errorf("engine: %w", err)
	}
	if err := p.Start(ctx); err != nil {
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
		return fmt.Errorf("engine: shutdown: %w", err)
	}
	e.p = nil
	e.per = map[string]UpstreamStat{} // old upstreams must not count
	return e.startLocked(ctx, v4, e.addr6, ups)
}

// Stop shuts the proxy down and closes its upstreams.
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.p == nil {
		return nil
	}
	err := e.p.Shutdown(ctx)
	e.p = nil
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
