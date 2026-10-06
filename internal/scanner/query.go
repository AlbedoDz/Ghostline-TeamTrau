package scanner

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// Exchange sends one recursive query for name/qtype through u. With do set,
// the query carries EDNS0 with the DNSSEC OK bit. It returns the reply and
// how long the exchange took. It returns when ctx ends even if u does not.
func Exchange(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, time.Duration, error) {
	q := new(dns.Msg).SetQuestion(dns.Fqdn(name), qtype)
	if do {
		q.SetEdns0(1232, true)
	}
	type reply struct {
		m   *dns.Msg
		err error
	}
	ch := make(chan reply, 1)
	start := time.Now()
	// Some upstreams (dnsproxy's plain UDP) ignore ctx; never wait past it.
	go func() {
		m, err := u.Exchange(ctx, q)
		ch <- reply{m, err}
	}()
	select {
	case r := <-ch:
		return r.m, time.Since(start), r.err
	case <-ctx.Done():
		return nil, time.Since(start), ctx.Err()
	}
}

// Classify names a failed exchange: "bootstrap" when the server's own
// hostname could not be resolved, "timeout", or "error". ctxErr is the
// query context's error, if any.
func Classify(err, ctxErr error) string {
	switch {
	case strings.Contains(strings.ToLower(err.Error()), "bootstrap"):
		return "bootstrap"
	case errors.Is(err, context.DeadlineExceeded) || ctxErr != nil || isTimeout(err):
		return "timeout"
	}
	return "error"
}
