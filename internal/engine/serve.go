package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/winutil"
	"github.com/miekg/dns"
)

// ServeConfig describes the DNS server for this PC and the LAN (spec 2B 7).
type ServeConfig struct {
	DoH   []netip.AddrPort        // DoH addresses: loopback, plus LAN IPs when sharing
	Plain []netip.AddrPort        // plain DNS (UDP+TCP) on LAN IPs
	Cert  func() *tls.Certificate // current DoH certificate
}

// ServeResult says which addresses were bound and why others were not.
type ServeResult struct {
	Bound   []netip.AddrPort
	Skipped map[netip.AddrPort]string
}

// ServeStats are the DNS server counters.
type ServeStats struct {
	Queries    uint64   `json:"queries"`
	Clients10m int      `json:"clients10m"`
	ClientIPs  []string `json:"clientIps"`
}

// ErrNoLoopbackDoH means DoH could not listen on any loopback address.
var ErrNoLoopbackDoH = errors.New("engine: DoH could not listen on loopback")

const (
	serveRateLimit = 100 // queries per second per client
	clientWindow   = 10 * time.Minute
)

type rateWindow struct {
	sec int64
	n   int
}

// Serve starts (or restarts) the extra listeners. They answer through the
// running engine, so rules, cache, statistics and the no-leak guarantee
// are the same as for the loopback listener.
func (e *Engine) Serve(ctx context.Context, sc ServeConfig) (ServeResult, error) {
	_ = e.StopServe(ctx)
	res := ServeResult{Skipped: map[netip.AddrPort]string{}}
	var doh []netip.AddrPort
	loop := false
	for _, a := range sc.DoH {
		b, err := probeTCP(a)
		if err != nil {
			res.Skipped[a] = err.Error()
			continue
		}
		doh = append(doh, b)
		loop = loop || b.Addr().IsLoopback()
	}
	if !loop {
		return res, ErrNoLoopbackDoH
	}
	var udp []*net.UDPAddr
	var tcp []*net.TCPAddr
	for _, a := range sc.Plain {
		if err := probeUDPTCP(a); err != nil {
			res.Skipped[a] = err.Error()
			continue
		}
		udp = append(udp, net.UDPAddrFromAddrPort(a))
		tcp = append(tcp, net.TCPAddrFromAddrPort(a))
		res.Bound = append(res.Bound, a)
	}
	res.Bound = append(res.Bound, doh...)
	p, err := proxy.New(&proxy.Config{
		Logger:         slog.New(slog.DiscardHandler),
		UDPListenAddr:  udp,
		TCPListenAddr:  tcp,
		HTTPConfig:     &proxy.HTTPConfig{ListenAddresses: doh},
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return sc.Cert(), nil }},
		UpstreamConfig: &proxy.UpstreamConfig{Upstreams: []upstream.Upstream{unusedUpstream{}}},
		RequestHandler: proxy.HandlerFunc(e.serveHandle),
	})
	if err != nil {
		return res, fmt.Errorf("engine: serve: %w", err)
	}
	if err := p.Start(ctx); err != nil {
		return res, fmt.Errorf("engine: serve: %w", err)
	}
	e.mu.Lock()
	e.serve = p
	e.mu.Unlock()
	return res, nil
}

// StopServe stops the extra listeners.
func (e *Engine) StopServe(ctx context.Context) error {
	e.mu.Lock()
	p := e.serve
	e.serve = nil
	e.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.Shutdown(ctx)
}

// ServeStats returns the DNS server counters.
func (e *Engine) ServeStats() ServeStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := ServeStats{Queries: e.serveQueries, ClientIPs: []string{}}
	cut := time.Now().Add(-clientWindow)
	for ip, seen := range e.clients {
		if seen.After(cut) && !ip.IsLoopback() {
			st.ClientIPs = append(st.ClientIPs, ip.String())
		}
	}
	sort.Strings(st.ClientIPs)
	st.Clients10m = len(st.ClientIPs)
	return st
}

// serveHandle filters a DNS server query, then answers it through the
// engine's main proxy (never the serve proxy's own placeholder upstream).
func (e *Engine) serveHandle(ctx context.Context, _ *proxy.Proxy, d *proxy.DNSContext) error {
	ip := d.Addr.Addr().Unmap()
	if !winutil.IsPrivateOrLocal(ip) || !e.allow(ip) || isANY(d.Req) {
		d.Res = new(dns.Msg).SetRcode(d.Req, dns.RcodeRefused)
		return nil
	}
	e.mu.Lock()
	main := e.p
	e.serveQueries++
	if e.clients == nil {
		e.clients = map[netip.Addr]time.Time{}
	}
	e.clients[ip] = time.Now()
	e.mu.Unlock()
	if main == nil {
		d.Res = new(dns.Msg).SetRcode(d.Req, dns.RcodeServerFailure)
		return nil
	}
	return e.handle(ctx, main, d)
}

func isANY(m *dns.Msg) bool {
	for _, q := range m.Question {
		if q.Qtype == dns.TypeANY {
			return true
		}
	}
	return false
}

// allow counts a query from ip and reports whether it is within the rate.
func (e *Engine) allow(ip netip.Addr) bool {
	now := time.Now().Unix()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.rates == nil {
		e.rates = map[netip.Addr]*rateWindow{}
	}
	w := e.rates[ip]
	if w == nil || w.sec != now {
		if len(e.rates) > 4096 {
			e.rates = map[netip.Addr]*rateWindow{}
		}
		w = &rateWindow{sec: now}
		e.rates[ip] = w
	}
	w.n++
	return w.n <= serveRateLimit
}

// limited reports whether ip is over the rate in the current second.
func (e *Engine) limited(ip netip.Addr) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	w := e.rates[ip]
	return w != nil && w.sec == time.Now().Unix() && w.n > serveRateLimit
}

func probeTCP(a netip.AddrPort) (netip.AddrPort, error) {
	ln, err := net.Listen("tcp", a.String())
	if err != nil {
		return a, err
	}
	b := netip.MustParseAddrPort(ln.Addr().String())
	_ = ln.Close()
	return netip.AddrPortFrom(b.Addr().Unmap(), b.Port()), nil
}

func probeUDPTCP(a netip.AddrPort) error {
	pc, err := net.ListenPacket("udp", a.String())
	if err != nil {
		return err
	}
	defer pc.Close()
	ln, err := net.Listen("tcp", a.String())
	if err != nil {
		return err
	}
	return ln.Close()
}

// unusedUpstream satisfies dnsproxy's config; serveHandle never resolves
// through the serve proxy.
type unusedUpstream struct{}

func (unusedUpstream) Exchange(context.Context, *dns.Msg) (*dns.Msg, error) {
	return nil, errors.New("engine: serve proxy has no upstream")
}
func (unusedUpstream) Address() string { return "unused" }
func (unusedUpstream) Close() error    { return nil }
