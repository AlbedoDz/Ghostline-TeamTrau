// Package lookup runs one DNS query against chosen sources and compares the
// answers (spec 3 §5).
package lookup

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/miekg/dns"
	"golang.org/x/net/idna"
)

// Source is where a query goes.
type Source struct {
	Kind  string `json:"kind"` // ghostline | server | address | isp
	Ref   string `json:"ref"`  // server ID, address, or resolver IP
	Label string `json:"label"`
}

// Record is one answer record.
type Record struct {
	Name string `json:"name"`
	Type string `json:"type"`
	TTL  uint32 `json:"ttl"`
	Data string `json:"data"`
}

// Answer is the result of one query against one source.
type Answer struct {
	Source    Source   `json:"source"`
	OK        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"` // bootstrap | timeout | error
	Rcode     string   `json:"rcode"`
	Records   []Record `json:"records"`
	AD        bool     `json:"ad"`
	TC        bool     `json:"tc"`
	RA        bool     `json:"ra"`
	LatencyMs int64    `json:"latencyMs"`
	Dig       string   `json:"dig"`
}

// Types are the record types the tool offers.
var Types = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "HTTPS", "CAA", "PTR"}

// ErrBadName rejects a name or IP that cannot be queried.
var ErrBadName = errors.New("lookup: bad name")

// Timeout bounds one query.
const Timeout = 5 * time.Second

var idnaProfile = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.Transitional(false))

// QueryName turns user input into the name to query: a pasted URL becomes
// its host, IDN becomes punycode, and for PTR an IP becomes its reverse
// name. An IP is rejected for any other type.
func QueryName(input, qtype string) (string, error) {
	if !slices.Contains(Types, qtype) {
		return "", ErrBadName
	}
	s := strings.TrimSpace(input)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", ErrBadName
		}
		s = u.Hostname()
	}
	s = strings.TrimSuffix(s, ".")
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		if qtype != "PTR" {
			return "", ErrBadName
		}
		rev, err := dns.ReverseAddr(a.String())
		if err != nil {
			return "", ErrBadName
		}
		return strings.TrimSuffix(rev, "."), nil
	}
	if s == "" || len(s) > 253 {
		return "", ErrBadName
	}
	a, err := idnaProfile.ToASCII(s)
	if err != nil {
		return "", ErrBadName
	}
	a = strings.ToLower(a)
	if len(a) > 253 {
		return "", ErrBadName
	}
	for _, l := range strings.Split(a, ".") {
		if l == "" || len(l) > 63 {
			return "", ErrBadName
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return "", ErrBadName
			}
		}
	}
	return a, nil
}

// Query sends name/qtype (DNSSEC OK set) through u, bounded by Timeout.
func Query(ctx context.Context, u upstream.Upstream, src Source, name, qtype string) Answer {
	a := Answer{Source: src, Records: []Record{}}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	m, d, err := scanner.Exchange(ctx, u, name, dns.StringToType[qtype], true)
	a.LatencyMs = d.Milliseconds()
	if err != nil {
		a.Error = scanner.Classify(err, ctx.Err())
		return a
	}
	a.OK = true
	a.Rcode = dns.RcodeToString[m.Rcode]
	a.AD, a.TC, a.RA = m.AuthenticatedData, m.Truncated, m.RecursionAvailable
	for _, r := range m.Answer {
		h := r.Header()
		a.Records = append(a.Records, Record{
			Name: h.Name, Type: dns.TypeToString[h.Rrtype], TTL: h.Ttl,
			Data: strings.TrimPrefix(r.String(), h.String()),
		})
	}
	a.Dig = Dig(m, d, src)
	return a
}
