// Package dialer opens the outbound side of a proxied connection: rules,
// name resolution through Ghostline's encrypted engine (never the system
// resolver), SSRF guard, upstream proxies and ClientHello fragmentation with
// automatic retry.
package dialer

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/tlsfrag"
)

// Resolver resolves host names (Ghostline's engine).
type Resolver interface {
	Resolve(ctx context.Context, host string) ([]netip.Addr, error)
}

// Matcher decides what rules say about a host and/or IP.
type Matcher interface {
	Match(host string, ip netip.Addr) rules.Decision
}

// FragCache remembers hosts that need a fragmented ClientHello on the
// current network.
type FragCache interface {
	Has(host string) bool
	Add(host string)
}

// Upstream is an upstream proxy with its password already decrypted.
type Upstream struct {
	ID, Type, Addr, User, Pass string
}

// FragConfig is the global web fragment configuration.
type FragConfig struct {
	Mode        string // auto | always | never
	Method      tlsfrag.Method
	Chunks      int
	Delay       time.Duration
	AutoTimeout time.Duration
}

// Config wires a Dialer.
type Config struct {
	Resolver  Resolver
	Matcher   func() Matcher
	Cache     FragCache
	Frag      func() FragConfig
	Upstreams func(id string) (Upstream, bool)
	SelfAddrs func() []netip.Addr
	Forbidden []netip.AddrPort // the proxy's own port and the engine's port 53
	Dial      func(ctx context.Context, ap netip.AddrPort) (net.Conn, error)
}

// Outcome classifies a connection for statistics.
type Outcome string

const (
	OutcomeDirect                Outcome = "direct"
	OutcomeFragmented            Outcome = "fragmented"
	OutcomeUpstream              Outcome = "upstream"
	OutcomeBlocked               Outcome = "blocked"
	OutcomeBlockedEvenFragmented Outcome = "blockedEvenFragmented"
	OutcomeFailed                Outcome = "failed"
)

// Result is an opened outbound connection. The client's hello has already
// been written to Conn; FirstServerBytes (possibly empty) must be sent to the
// client before relaying.
type Result struct {
	Conn             net.Conn
	Outcome          Outcome
	Source           rules.Source
	FirstServerBytes []byte
}

var (
	ErrBlocked               = errors.New("proxy: blocked by rule")
	ErrForbidden             = errors.New("proxy: destination not allowed")
	ErrUnreachable           = errors.New("proxy: destination unreachable")
	ErrBlockedEvenFragmented = errors.New("proxy: blocked even with fragmented ClientHello")
)

const (
	dialTimeout    = 10 * time.Second
	fallbackAfter  = 3 * time.Second
	maxHelloRecord = 16<<10 + 5
)

// Dialer opens outbound connections for the proxy.
type Dialer struct{ c Config }

// New creates a Dialer. A nil Dial uses a plain TCP dialer (by IP only).
func New(c Config) *Dialer {
	if c.Dial == nil {
		c.Dial = func(ctx context.Context, ap netip.AddrPort) (net.Conn, error) {
			return (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", ap.String())
		}
	}
	return &Dialer{c: c}
}

// Decide checks domain/IP rules before the handshake is answered, so a
// blocked target gets the protocol's "blocked" reply. CIDR rules on resolved
// addresses are applied later by Open.
func (d *Dialer) Decide(t wire.Target) error {
	if d.c.Matcher().Match(t.Host, t.IP).Block {
		return ErrBlocked
	}
	return nil
}

// Open connects to t for client and writes hello (the client's first bytes,
// nil if none yet).
func (d *Dialer) Open(ctx context.Context, client netip.Addr, t wire.Target, hello []byte) (Result, error) {
	m := d.c.Matcher()
	dec := m.Match(t.Host, t.IP)
	if dec.Block {
		return Result{Outcome: OutcomeBlocked, Source: dec.Source}, ErrBlocked
	}
	if dec.Upstream != "" {
		return d.viaUpstream(ctx, dec, t, hello)
	}
	addrs, err := d.resolve(ctx, t, dec)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Source: dec.Source}, err
	}
	if t.Host != "" || len(dec.IPs) > 0 {
		// Apply CIDR rules to the address actually used.
		dec = m.Match(t.Host, addrs[0])
		if dec.Block {
			return Result{Outcome: OutcomeBlocked, Source: dec.Source}, ErrBlocked
		}
		if dec.Upstream != "" {
			return d.viaUpstream(ctx, dec, t, hello)
		}
	}
	for _, a := range addrs {
		if err := d.allowed(client, netip.AddrPortFrom(a, t.Port)); err != nil {
			return Result{Outcome: OutcomeBlocked, Source: dec.Source}, err
		}
	}
	dial := func(ctx context.Context) (net.Conn, error) { return d.dialAny(ctx, addrs, t.Port) }
	r, err := d.writeHello(ctx, dial, d.fragMode(dec.Fragment), cacheHost(t, hello), hello)
	r.Source = dec.Source
	return r, err
}

func cacheHost(t wire.Target, hello []byte) string {
	if sni, ok := tlsfrag.SNI(hello); ok {
		return sni
	}
	if t.Host != "" {
		return t.Host
	}
	return t.IP.String()
}

func (d *Dialer) resolve(ctx context.Context, t wire.Target, dec rules.Decision) ([]netip.Addr, error) {
	switch {
	case len(dec.IPs) > 0:
		return dec.IPs, nil
	case t.IP.IsValid():
		return []netip.Addr{t.IP}, nil
	}
	addrs, err := d.c.Resolver.Resolve(ctx, t.Host)
	if err != nil || len(addrs) == 0 {
		return nil, errors.Join(ErrUnreachable, err)
	}
	return addrs, nil
}

// fragMode turns a rule override and the global mode into auto/always/never.
func (d *Dialer) fragMode(f rules.Frag) string {
	switch f {
	case rules.FragOn:
		return "always"
	case rules.FragOff:
		return "never"
	case rules.FragAuto:
		return "auto"
	}
	return d.c.Frag().Mode
}

// dialAny tries addrs in order; while more candidates remain an attempt gets
// fallbackAfter before the next one is tried.
func (d *Dialer) dialAny(ctx context.Context, addrs []netip.Addr, port uint16) (net.Conn, error) {
	var errs []error
	for i, a := range addrs {
		actx, cancel := ctx, context.CancelFunc(func() {})
		if i < len(addrs)-1 {
			actx, cancel = context.WithTimeout(ctx, fallbackAfter)
		}
		c, err := d.c.Dial(actx, netip.AddrPortFrom(a, port))
		cancel()
		if err == nil {
			return c, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(append([]error{ErrUnreachable}, errs...)...)
}
