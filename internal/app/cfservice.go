package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/netip"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/store"
)

// CFProgress is sent on EventToolsCFScan.
type CFProgress struct {
	Phase   string         `json:"phase"` // probe | speed
	Tried   int            `json:"tried"`
	OK      int            `json:"ok"`
	Total   int            `json:"total"`
	Result  *cfscan.Result `json:"result,omitempty"`
	Running bool           `json:"running"`
	Note    string         `json:"note,omitempty"`  // no_speed_endpoint
	Error   string         `json:"error,omitempty"` // error code of a failed scan
}

// CFView is the clean-IP tab's content: the running scan, or the last one
// on this network.
type CFView struct {
	ScannedAt time.Time       `json:"scannedAt"`
	Host      string          `json:"host"`
	Results   []cfscan.Result `json:"results"`
	Running   bool            `json:"running"`
}

// cfJob is the running clean-IP scan.
type cfJob struct {
	host    string
	results []cfscan.Result
}

// speedTop is how many of the best IPs get a download test (spec 3 §7.3).
const speedTop = 10

// maxRuleIPs caps the IPs in one clean-IP rule (spec 3 §7.5).
const maxRuleIPs = 4

func (s *Service) netKey() string {
	if s.x.NetKey == nil {
		return ""
	}
	return s.x.NetKey()
}

func (s *Service) prober(st store.CFScanTool) cfscan.Prober {
	return cfscan.Prober{Dial: s.x.DialDirect, Host: st.Host, Timeout: time.Duration(st.TimeoutMs) * time.Millisecond, Roots: s.cfRoots}
}

// StartCFScan looks for working Cloudflare IPs in the background; progress
// arrives on EventToolsCFScan. Connections go straight out, not through
// Ghostline's proxy.
func (s *Service) StartCFScan() error {
	st := s.x.Settings.Get().Tools.CFScan
	if !store.ValidHostname(st.Host) {
		return appErr(CodeCFScanHostInvalid, nil)
	}
	s.mu.Lock()
	if s.cfCancel != nil {
		s.mu.Unlock()
		return toolBusy("cfscan")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cfCancel = cancel
	s.cf = cfJob{host: st.Host}
	s.mu.Unlock()

	ips := cfscan.Sample(cfscan.Ranges(), st.MaxIPs, rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), rand.Uint64())))
	p := s.prober(st)
	go func() {
		final := CFProgress{Phase: "probe", Total: len(ips)}
		defer func() {
			if r := recover(); r != nil {
				slog.Error("tools: clean-IP scan panic", "panic", r, "stack", string(debug.Stack()))
				final.Error = CodeInternal
			}
			s.mu.Lock()
			s.cfCancel = nil
			s.mu.Unlock()
			cancel()
			final.Running = false
			s.x.Bus.Emit(EventToolsCFScan, final)
		}()
		var mu sync.Mutex
		rs, err := cfscan.Scan(ctx, ips, p, cfscan.Options{
			Concurrency: st.Concurrency, Want: st.Want, Limiter: s.cfLimiter(),
			OnProgress: func(tried, ok int, r cfscan.Result) {
				mu.Lock()
				defer mu.Unlock()
				final.Tried, final.OK = tried, ok
				if r.OK {
					s.mu.Lock()
					s.cf.results = append(s.cf.results, r)
					s.mu.Unlock()
				}
				s.x.Bus.Emit(EventToolsCFScan, CFProgress{Phase: "probe", Tried: tried, OK: ok, Total: len(ips), Result: &r, Running: true})
			},
		})
		if errors.Is(err, cfscan.ErrNoNetwork) {
			final.Error = CodeCFScanNoNetwork
			return
		}
		if st.SpeedTest && ctx.Err() == nil {
			final.Phase = "speed"
			err := cfscan.SpeedTop(ctx, rs, p, speedTop, st.SpeedBytes, 10*time.Second, func(r cfscan.Result) {
				s.x.Bus.Emit(EventToolsCFScan, CFProgress{Phase: "speed", Tried: final.Tried, OK: final.OK, Total: len(ips), Result: &r, Running: true})
			})
			if errors.Is(err, cfscan.ErrNoSpeedEndpoint) {
				final.Note = "no_speed_endpoint"
			}
			cfscan.Sort(rs)
		}
		s.mu.Lock()
		s.cf.results = slices.DeleteFunc(rs, func(r cfscan.Result) bool { return !r.OK })
		s.mu.Unlock()
		c := cfscan.LoadCache(s.x.Paths.CFScanCache)
		c.Put(s.netKey(), time.Now(), st.Host, rs)
		if err := cfscan.SaveCache(s.x.Paths.CFScanCache, c); err != nil {
			slog.Warn("tools: save clean-IP cache", "err", err)
		}
	}()
	return nil
}

func (s *Service) cfLimiter() cfscan.Limiter {
	return cfscan.NewLimiter(200)
}

// CancelCFScan stops a running clean-IP scan within a second.
func (s *Service) CancelCFScan() {
	s.mu.Lock()
	c := s.cfCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// GetCFView returns the running scan's good IPs, or the cached scan for
// this network.
func (s *Service) GetCFView() CFView {
	s.mu.Lock()
	if s.cfCancel != nil {
		v := CFView{Host: s.cf.host, Results: slices.Clone(s.cf.results), Running: true}
		s.mu.Unlock()
		cfscan.Sort(v.Results)
		return v
	}
	s.mu.Unlock()
	e, ok := cfscan.LoadCache(s.x.Paths.CFScanCache).Get(s.netKey())
	if !ok {
		return CFView{Results: []cfscan.Result{}}
	}
	return CFView{ScannedAt: e.ScannedAt, Host: e.Host, Results: e.Results}
}

func parseCFIPs(ips []string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range ips {
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() || !cfscan.Contains(cfscan.Ranges(), a) {
			return nil, fmt.Errorf("%q is not a Cloudflare IPv4 address", s)
		}
		out = append(out, a)
	}
	return out, nil
}

// RecheckCF probes the given IPs again and updates them in this network's
// cache.
func (s *Service) RecheckCF(ips []string) ([]cfscan.Result, error) {
	addrs, err := parseCFIPs(ips)
	if err != nil {
		return nil, err
	}
	st := s.x.Settings.Get().Tools.CFScan
	p := s.prober(st)
	out := make([]cfscan.Result, len(addrs))
	var wg sync.WaitGroup
	for i, a := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = p.Probe(context.Background(), a)
		}()
	}
	wg.Wait()
	c := cfscan.LoadCache(s.x.Paths.CFScanCache)
	e, _ := c.Get(s.netKey())
	merged := slices.DeleteFunc(slices.Clone(e.Results), func(r cfscan.Result) bool {
		return slices.ContainsFunc(out, func(o cfscan.Result) bool { return o.IP == r.IP })
	})
	merged = append(merged, out...)
	host := e.Host
	if host == "" {
		host = st.Host
	}
	c.Put(s.netKey(), time.Now(), host, merged)
	return out, cfscan.SaveCache(s.x.Paths.CFScanCache, c)
}

// CFSuggestDomains offers the domain patterns of the user's rules.
func (s *Service) CFSuggestDomains() []string {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	out := []string{}
	for _, r := range s.rf.Rules {
		p, err := rules.ParsePattern(r.Pattern)
		if err != nil || p.Kind > rules.KindSubOnly || slices.Contains(out, r.Pattern) {
			continue
		}
		out = append(out, r.Pattern)
	}
	return out
}

// CreateCFRules sets ip=<ips> for each domain pattern: an existing rule for
// the pattern gets the IPs (its other actions are kept), otherwise a new
// rule is added. Nothing is written unless every pattern and IP is valid.
func (s *Service) CreateCFRules(patterns []string, ips []string) []rules.LineError {
	if len(ips) == 0 || len(ips) > maxRuleIPs {
		return []rules.LineError{{Line: 0, Msg: fmt.Sprintf("choose 1 to %d IPs", maxRuleIPs)}}
	}
	addrs, err := parseCFIPs(ips)
	if err != nil {
		return []rules.LineError{{Line: 0, Msg: err.Error()}}
	}
	var add []rules.Rule
	var errs []rules.LineError
	for i, p := range patterns {
		pp, err := rules.ParsePattern(p)
		if err != nil || pp.Kind > rules.KindSubOnly {
			errs = append(errs, rules.LineError{Line: i + 1, Msg: fmt.Sprintf("%q is not a domain pattern", p)})
			continue
		}
		add = append(add, rules.Rule{Pattern: p, Action: rules.Action{IPs: addrs}, Enabled: true, Comment: "cloudflare clean ip"})
	}
	if len(errs) > 0 {
		return errs
	}
	if len(add) == 0 {
		return []rules.LineError{{Line: 0, Msg: "no domain given"}}
	}
	ids := s.upstreamIDs()
	s.rmu.Lock()
	defer s.rmu.Unlock()
	// The first rule for a pattern wins, so a new rule behind an existing
	// one would never apply: put the IPs into the existing rule instead.
	cur := slices.Clone(s.rf.Rules)
	var fresh []rules.Rule
	for i, r := range add {
		j := slices.IndexFunc(cur, func(o rules.Rule) bool { return o.Pattern == r.Pattern })
		if j < 0 {
			fresh = append(fresh, r)
			continue
		}
		if cur[j].Block || cur[j].Connect != "" {
			errs = append(errs, rules.LineError{Line: i + 1, Msg: fmt.Sprintf("%q already has a block or connect= rule, which would win", r.Pattern)})
			continue
		}
		cur[j].IPs, cur[j].Enabled = addrs, true
	}
	if len(errs) > 0 {
		return errs
	}
	out, _, errs := rules.AppendUserRules(cur, fresh, ids)
	if len(errs) > 0 {
		return errs
	}
	old := s.rf.Rules
	s.rf.Rules = out
	if err := s.saveRulesLocked(); err != nil {
		s.rf.Rules = old
		return []rules.LineError{{Line: 0, Msg: err.Error()}}
	}
	s.recompileLocked("")
	return []rules.LineError{}
}
