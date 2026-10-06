package app

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/lookup"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/stamps"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// ipUp answers every A query with ip after delay.
type ipUp struct {
	ip    string
	delay time.Duration
}

func (u ipUp) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	select {
	case <-time.After(u.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m := new(dns.Msg).SetReply(req)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP(u.ip)}}
	return m, nil
}
func (u ipUp) Address() string { return u.ip }
func (u ipUp) Close() error    { return nil }

type toolsHarness struct {
	*svcHarness
	mu    sync.Mutex
	plain []string
	built []string
}

func newTools(t *testing.T) *toolsHarness {
	h := &toolsHarness{svcHarness: newSvc(t)}
	h.custom = []model.Server{
		{ID: "fast", Name: "Fast", Address: "https://fast.example/dns-query", Source: model.SourceCustom},
		{ID: "slow", Name: "Slow", Address: "https://slow.example/dns-query", Source: model.SourceCustom},
	}
	h.svc.x.BuildUpstream = func(s model.Server) (upstream.Upstream, error) {
		h.mu.Lock()
		h.built = append(h.built, s.ID)
		h.mu.Unlock()
		return ipUp{ip: "93.184.216.34", delay: 100 * time.Millisecond}, nil
	}
	h.svc.x.PlainUpstream = func(ip string) (upstream.Upstream, error) {
		h.mu.Lock()
		h.plain = append(h.plain, ip)
		h.mu.Unlock()
		if ip == "127.0.0.1" {
			return ipUp{ip: "93.184.216.34", delay: 100 * time.Millisecond}, nil
		}
		return ipUp{ip: "10.10.34.35", delay: 100 * time.Millisecond}, nil
	}
	h.svc.x.ISPResolvers = func() []string { return []string{"203.162.4.191"} }
	h.svc.x.RulesPath = h.paths.Rules
	h.o.d.Scans = &fScans{results: []scanner.Result{
		{ServerID: "slow", OK: true, Latency: 90 * time.Millisecond},
		{ServerID: "cf", OK: true, Latency: 40 * time.Millisecond},
		{ServerID: "fast", OK: true, Latency: 10 * time.Millisecond},
	}}
	return h
}

func code(t *testing.T, err error) string {
	t.Helper()
	var ae *AppError
	require.True(t, errors.As(err, &ae), "%v", err)
	return ae.Code
}

func TestLookup_GhostlineNeedsConnect(t *testing.T) {
	h := newTools(t)
	_, err := h.svc.Lookup("example.com", "A", []lookup.Source{{Kind: "ghostline"}})
	require.Equal(t, CodeLookupNotConnected, code(t, err))
	h.o.update(func(s *Snapshot) { s.Status = StatusProtected })
	res, err := h.svc.Lookup("example.com", "A", []lookup.Source{{Kind: "ghostline"}})
	require.NoError(t, err)
	require.True(t, res.Answers[0].OK)
	require.Equal(t, []string{"127.0.0.1"}, h.plain)
}

func TestLookup_BadName(t *testing.T) {
	h := newTools(t)
	_, err := h.svc.Lookup("a b", "A", []lookup.Source{{Kind: "server", Ref: "fast"}})
	require.Equal(t, CodeLookupBadName, code(t, err))
}

func TestLookup_SourceCount(t *testing.T) {
	h := newTools(t)
	_, err := h.svc.Lookup("example.com", "A", nil)
	require.Error(t, err)
	seven := make([]lookup.Source, 7)
	for i := range seven {
		seven[i] = lookup.Source{Kind: "server", Ref: "fast"}
	}
	_, err = h.svc.Lookup("example.com", "A", seven)
	require.Error(t, err)

	start := time.Now()
	res, err := h.svc.Lookup("example.com", "A", seven[:6])
	require.NoError(t, err)
	require.Less(t, time.Since(start), 400*time.Millisecond, "sources run in parallel")
	require.Len(t, res.Answers, 6)
	require.Equal(t, lookup.VerdictMatch, res.Overall)
	require.Equal(t, "Fast", res.Answers[0].Source.Label)
}

func TestLookup_ISPComparison(t *testing.T) {
	h := newTools(t)
	res, err := h.svc.Lookup("youtube.com", "A", []lookup.Source{{Kind: "server", Ref: "fast"}, {Kind: "isp", Ref: "203.162.4.191"}})
	require.NoError(t, err)
	require.Equal(t, lookup.VerdictPoisoned, res.Overall)
	require.Equal(t, []lookup.Verdict{lookup.VerdictMatch, lookup.VerdictPoisoned}, res.Verdicts)
	_, err = h.svc.Lookup("youtube.com", "A", []lookup.Source{{Kind: "isp", Ref: "not-an-ip"}})
	require.Error(t, err)
}

func TestLookup_ISPOnlyWhenAsked(t *testing.T) {
	h := newTools(t)
	for _, connected := range []bool{false, true} {
		if connected {
			h.o.update(func(s *Snapshot) { s.Status = StatusProtected })
		}
		srcs := h.svc.DefaultLookupSources()
		require.Len(t, srcs, 3)
		for _, s := range srcs {
			require.NotEqual(t, "isp", s.Kind)
		}
		if connected {
			require.Equal(t, "ghostline", srcs[0].Kind)
			require.Equal(t, []string{"fast", "cf"}, []string{srcs[1].Ref, srcs[2].Ref})
		} else {
			require.Equal(t, []string{"fast", "cf", "slow"}, []string{srcs[0].Ref, srcs[1].Ref, srcs[2].Ref})
		}
	}
	require.Equal(t, []string{"203.162.4.191"}, h.svc.ISPResolvers())
}

func TestLookup_AddressSource(t *testing.T) {
	h := newTools(t)
	_, err := h.svc.Lookup("example.com", "A", []lookup.Source{{Kind: "address", Ref: "udp://1.1.1.1"}})
	require.Error(t, err, "only kind isp may be plain DNS")
	res, err := h.svc.Lookup("example.com", "A", []lookup.Source{{Kind: "address", Ref: "https://dns.example/dns-query"}})
	require.NoError(t, err)
	require.True(t, res.Answers[0].OK)
	require.Empty(t, h.plain)
}

func TestDecodeStamps_PerLine(t *testing.T) {
	h := newTools(t)
	cards := h.svc.DecodeStamps("sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ\n\n  nope \n")
	require.Len(t, cards, 2)
	require.Equal(t, "doh", cards[0].Fields.Proto)
	require.Empty(t, cards[0].Error)
	require.Nil(t, cards[1].Fields)
	require.Equal(t, "nope", cards[1].Line)
	require.NotEmpty(t, cards[1].Error)
}

func TestEncodeStamp_InvalidCode(t *testing.T) {
	h := newTools(t)
	_, err := h.svc.EncodeStamp(stamps.Fields{Proto: "doh", Host: "a.com", Path: "/q", Hashes: []string{"abcd"}})
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeStampInvalid, ae.Code)
	require.Equal(t, "hashes", ae.Params["field"])
	s, err := h.svc.EncodeStamp(stamps.Fields{Proto: "dot", Host: "one.one.one.one"})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(s, "sdns://"))
	f, err := h.svc.StampFromURL("https://dns.google/dns-query", "8.8.8.8")
	require.NoError(t, err)
	require.Equal(t, "doh", f.Proto)
	_, err = h.svc.StampFromURL("ftp://x", "")
	require.Equal(t, CodeStampInvalid, code(t, err))
}

func TestLookup_BuildFailureMarksOnlyThatSource(t *testing.T) {
	h := newTools(t)
	build := h.svc.x.BuildUpstream
	h.svc.x.BuildUpstream = func(s model.Server) (upstream.Upstream, error) {
		if s.ID == "slow" {
			return nil, errors.New("bootstrap failed")
		}
		return build(s)
	}
	res, err := h.svc.Lookup("example.com", "A", []lookup.Source{{Kind: "server", Ref: "fast"}, {Kind: "server", Ref: "slow"}})
	require.NoError(t, err)
	require.True(t, res.Answers[0].OK)
	require.False(t, res.Answers[1].OK)
	require.Equal(t, "error", res.Answers[1].Error)
	require.Equal(t, "Slow", res.Answers[1].Source.Label)
	require.Equal(t, []lookup.Verdict{lookup.VerdictMatch, lookup.VerdictFailed}, res.Verdicts)
	require.Equal(t, lookup.VerdictMatch, res.Overall)
}

func TestDefaultLookupSources_FallBackToCatalogWithoutScan(t *testing.T) {
	h := newTools(t)
	h.o.d.Scans = &fScans{} // fresh install: never scanned
	srcs := h.svc.DefaultLookupSources()
	require.Len(t, srcs, 3, "always offer encrypted sources, never only the ISP")
	require.Equal(t, "cf", srcs[0].Ref, "built-in servers first")
	for _, s := range srcs {
		require.Equal(t, "server", s.Kind)
	}
	h.o.update(func(s *Snapshot) { s.Status = StatusProtected })
	srcs = h.svc.DefaultLookupSources()
	require.Equal(t, []string{"ghostline", "server", "server"}, []string{srcs[0].Kind, srcs[1].Kind, srcs[2].Kind})
}
