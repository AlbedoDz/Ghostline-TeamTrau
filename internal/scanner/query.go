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
// how long the exchange took.
func Exchange(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, time.Duration, error) {
	q := new(dns.Msg).SetQuestion(dns.Fqdn(name), qtype)
	if do {
		q.SetEdns0(1232, true)
	}
	start := time.Now()
	resp, err := u.Exchange(ctx, q)
	return resp, time.Since(start), err
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
