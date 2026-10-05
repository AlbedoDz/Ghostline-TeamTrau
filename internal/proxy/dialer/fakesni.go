package dialer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/tlsfrag"
)

// Fake SNI outcomes (spec 2B 8.3).
const (
	OutcomeFakeSNI               Outcome = "fakesni"
	OutcomeFakeSNIFallback       Outcome = "fakesni_fallback"
	OutcomeFakeSNIClientRejected Outcome = "fakesni_client_rejected"
	OutcomeFakeSNIVerifyFailed   Outcome = "fakesni_verify_failed"
)

// Plan says whether the connection whose ClientHello is hello should be
// intercepted for Fake SNI: the hello names a host (not an IP), has no
// ECH, and the rule matched on that name asks for sni=. It returns the
// decision and the host from the hello.
func (d *Dialer) Plan(t wire.Target, hello []byte) (rules.Decision, string, bool) {
	host, ok := tlsfrag.SNI(hello)
	if !ok || tlsfrag.HasECH(hello) {
		return rules.Decision{}, "", false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return rules.Decision{}, "", false
	}
	dec := d.c.Matcher().Match(host, netip.Addr{})
	if dec.SNI == "" || dec.Block {
		return rules.Decision{}, "", false
	}
	return dec, host, true
}

// OpenRaw connects for a Fake SNI interception without writing anything:
// the same rules, resolver, SSRF guard and upstream as Open, but the
// address comes from connect= when the rule sets it. With fragment=on the
// first write (the fake ClientHello) is split like Open would.
func (d *Dialer) OpenRaw(ctx context.Context, client netip.Addr, t wire.Target, dec rules.Decision) (net.Conn, error) {
	target := t
	if dec.Connect != "" {
		target = wire.Target{Host: dec.Connect, Port: t.Port}
	}
	var c net.Conn
	if dec.Upstream != "" {
		up, ok := d.c.Upstreams(dec.Upstream)
		if !ok {
			return nil, fmt.Errorf("%w: unknown upstream %q", ErrUnreachable, dec.Upstream)
		}
		var err error
		if c, err = d.dialUpstream(ctx, up, target); err != nil {
			return nil, err
		}
	} else {
		var addrs []netip.Addr
		var err error
		if dec.Connect != "" {
			if addrs, err = d.c.Resolver.Resolve(ctx, dec.Connect); err != nil || len(addrs) == 0 {
				return nil, errors.Join(ErrUnreachable, err)
			}
		} else if addrs, err = d.resolve(ctx, t, dec); err != nil {
			return nil, err
		}
		for _, a := range addrs {
			if err := d.allowed(client, netip.AddrPortFrom(a, t.Port)); err != nil {
				return nil, err
			}
		}
		if c, err = d.dialAny(ctx, addrs, t.Port); err != nil {
			return nil, err
		}
	}
	if dec.Fragment == rules.FragOn {
		c = &fragFirstWrite{Conn: c, cfg: d.c.Frag()}
	}
	return c, nil
}

// fragFirstWrite splits the first write (a ClientHello) like Open does.
type fragFirstWrite struct {
	net.Conn
	cfg  FragConfig
	once sync.Once
}

func (f *fragFirstWrite) Write(b []byte) (int, error) {
	n, err, first := 0, error(nil), false
	f.once.Do(func() {
		first = true
		if err = writeFragmented(f.Conn, b, f.cfg); err == nil {
			n = len(b)
		}
	})
	if first {
		return n, err
	}
	return f.Conn.Write(b)
}
