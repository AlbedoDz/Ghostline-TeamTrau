// Package upstreams turns server entries into dnsproxy upstreams with
// Ghostline's bootstrap rules: pinned IPs first, then a configured plain-DNS
// bootstrap list, and never the system resolver.
package upstreams

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/fragdoh"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/miekg/dns"
)

// FragmentOptions enables ClientHello fragmentation for DoH upstreams.
type FragmentOptions struct {
	Chunks int
	Delay  time.Duration
}

// Options configures a Factory.
type Options struct {
	Bootstrap []string // plain DNS "ip:port" list
	Timeout   time.Duration
	Fragment  *FragmentOptions
	RootCAs   *x509.CertPool // tests only
}

// Factory builds upstreams.
type Factory struct {
	o    Options
	boot upstream.Resolver
}

// NewFactory validates options and prepares the shared bootstrap resolver.
func NewFactory(o Options) (*Factory, error) {
	if len(o.Bootstrap) == 0 {
		return nil, errors.New("upstreams: bootstrap list is empty")
	}
	if o.Timeout <= 0 {
		o.Timeout = 3 * time.Second
	}
	var par upstream.ParallelResolver
	for _, addr := range o.Bootstrap {
		r, err := upstream.NewUpstreamResolver(addr, &upstream.Options{Timeout: o.Timeout})
		if err != nil {
			return nil, fmt.Errorf("upstreams: bootstrap %q: %w", addr, err)
		}
		par = append(par, upstream.NewCachingResolver(r))
	}
	return &Factory{o: o, boot: par}, nil
}

// Build returns an upstream for s. Each call gets fresh upstream.Options
// because dnsproxy mutates them.
func (f *Factory) Build(s model.Server) (upstream.Upstream, error) {
	boot := f.boot
	if len(s.IPs) > 0 {
		var static upstream.StaticResolver
		for _, ip := range s.IPs {
			a, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, fmt.Errorf("upstreams: %s: bad IP %q", s.ID, ip)
			}
			static = append(static, a)
		}
		boot = static
	}
	if f.o.Fragment != nil && s.Protocol == model.ProtoDoH && strings.HasPrefix(s.Address, "https://") {
		// fragdoh bounds its own exchanges (dial and bootstrap included).
		fu, err := fragdoh.New(s.Address, fragdoh.Options{
			Bootstrap: boot, Chunks: f.o.Fragment.Chunks, Delay: f.o.Fragment.Delay,
			Timeout: f.o.Timeout, RootCAs: f.o.RootCAs,
		})
		if err != nil {
			return nil, err
		}
		return fu, nil
	}
	u, err := upstream.AddressToUpstream(s.Address, &upstream.Options{
		Bootstrap: boot, Timeout: f.o.Timeout, RootCAs: f.o.RootCAs,
	})
	if err != nil {
		return nil, err
	}
	return &bounded{Upstream: u, timeout: f.o.Timeout}, nil
}

// bounded caps every exchange (bootstrap included) at the factory timeout.
type bounded struct {
	upstream.Upstream
	timeout time.Duration
}

func (b *bounded) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	type result struct {
		m   *dns.Msg
		err error
	}
	ch := make(chan result, 1)
	go func() {
		m, err := b.Upstream.Exchange(ctx, req)
		ch <- result{m, err}
	}()
	select {
	case r := <-ch:
		return r.m, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("upstreams: %s: %w", b.Address(), ctx.Err())
	}
}

// Unwrap exposes the underlying upstream.
func (b *bounded) Unwrap() upstream.Upstream { return b.Upstream }
