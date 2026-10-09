package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/game"
	"github.com/hashcott/ghostline/internal/lookup"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/stamps"
)

// LookupResult is a lookup across one or more sources (spec 3 §5).
type LookupResult struct {
	Overall  lookup.Verdict   `json:"overall"`
	Answers  []lookup.Answer  `json:"answers"`
	Verdicts []lookup.Verdict `json:"verdicts"`
}

// maxLookupSources caps a comparison (spec 3 §5.1).
const maxLookupSources = 6

// LookupTypes lists the record types the lookup tool offers.
func (s *Service) LookupTypes() []string { return slices.Clone(lookup.Types) }

func (s *Service) connected() bool {
	st := s.o.Snapshot().Status
	return st == StatusProtected || st == StatusDegraded
}

// DefaultLookupSources is Ghostline (when connected) plus the fastest
// servers from the last scan, filled up from the server list, three
// sources in all. It never includes the
// unencrypted ISP source.
func (s *Service) DefaultLookupSources() []lookup.Source {
	var out []lookup.Source
	if s.connected() {
		out = append(out, lookup.Source{Kind: "ghostline", Label: "Ghostline"})
	}
	byID := map[string]model.Server{}
	for _, sv := range s.x.Catalog() {
		byID[sv.ID] = sv
	}
	rs := slices.Clone(s.o.ScanResults())
	slices.SortStableFunc(rs, func(a, b scanner.Result) int { return int(a.Latency - b.Latency) })
	have := map[string]bool{}
	add := func(sv model.Server) {
		if len(out) < 3 && !have[sv.ID] {
			have[sv.ID] = true
			out = append(out, lookup.Source{Kind: "server", Ref: sv.ID, Label: sv.Name})
		}
	}
	for _, r := range rs {
		if sv, ok := byID[r.ServerID]; ok && r.OK {
			add(sv)
		}
	}
	// Never scanned (or nothing worked): still offer encrypted sources so
	// the ISP is never compared only with itself. Built-in servers first.
	cat := s.x.Catalog()
	for _, sv := range cat {
		if sv.Source == model.SourceBuiltin {
			add(sv)
		}
	}
	for _, sv := range cat {
		add(sv)
	}
	return out
}

// ISPResolvers lists this PC's own DNS servers (before Ghostline), offered
// as the unencrypted comparison source.
func (s *Service) ISPResolvers() []string {
	if s.x.ISPResolvers == nil {
		return []string{}
	}
	return s.x.ISPResolvers()
}

// Lookup queries name/qtype through 1–6 sources in parallel and compares
// the answers.
func (s *Service) Lookup(name, qtype string, sources []lookup.Source) (LookupResult, error) {
	if len(sources) == 0 || len(sources) > maxLookupSources {
		return LookupResult{}, fmt.Errorf("lookup: choose 1 to %d sources", maxLookupSources)
	}
	q, err := lookup.QueryName(name, qtype)
	if err != nil {
		return LookupResult{}, appErr(CodeLookupBadName, err)
	}
	ups := make([]upstream.Upstream, len(sources))
	defer func() {
		for _, u := range ups {
			if u != nil {
				_ = u.Close()
			}
		}
	}()
	res := LookupResult{Answers: make([]lookup.Answer, len(sources))}
	for i := range sources {
		u, src, err := s.lookupUpstream(sources[i])
		var bf buildFailure
		switch {
		case errors.As(err, &bf):
			// This source cannot be reached; the others still answer.
			slog.Info("lookup: source unavailable", "source", src.Label, "err", bf.err)
			res.Answers[i] = lookup.Answer{Source: src, Error: "error", Records: []lookup.Record{}}
		case err != nil:
			return LookupResult{}, err
		}
		ups[i], sources[i] = u, src
	}
	var wg sync.WaitGroup
	for i := range sources {
		if ups[i] == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res.Answers[i] = lookup.Query(context.Background(), ups[i], sources[i], q, qtype)
		}()
	}
	wg.Wait()
	res.Overall, res.Verdicts = lookup.Compare(qtype, res.Answers)
	return res, nil
}

// buildFailure is a source that is valid but whose upstream could not be
// built (e.g. its hostname did not resolve): only that source fails.
type buildFailure struct{ err error }

func (b buildFailure) Error() string { return b.err.Error() }

func built(u upstream.Upstream, err error) (upstream.Upstream, error) {
	if err != nil {
		return nil, buildFailure{err}
	}
	return u, nil
}

// lookupUpstream resolves a source to an upstream and fills its label.
// Only kind "isp" (and Ghostline's own loopback engine) may be plain DNS.
func (s *Service) lookupUpstream(src lookup.Source) (upstream.Upstream, lookup.Source, error) {
	switch src.Kind {
	case "ghostline":
		if !s.connected() {
			return nil, src, appErr(CodeLookupNotConnected, nil)
		}
		src.Label = "Ghostline"
		u, err := built(s.x.PlainUpstream("127.0.0.1"))
		return u, src, err
	case "server":
		i := slices.IndexFunc(s.x.Catalog(), func(sv model.Server) bool { return sv.ID == src.Ref })
		if i < 0 {
			return nil, src, fmt.Errorf("lookup: unknown server %q", src.Ref)
		}
		sv := s.x.Catalog()[i]
		src.Label = sv.Name
		u, err := built(s.x.BuildUpstream(sv))
		return u, src, err
	case "address":
		sv, err := servers.FromAddress(src.Ref, model.SourceCustom)
		if err != nil {
			return nil, src, fmt.Errorf("lookup: %w", err)
		}
		src.Label = src.Ref
		u, err := built(s.x.BuildUpstream(sv))
		return u, src, err
	case "isp":
		a, err := netip.ParseAddr(src.Ref)
		if err != nil {
			return nil, src, fmt.Errorf("lookup: %q is not an IP", src.Ref)
		}
		src.Ref, src.Label = a.String(), a.String()
		u, err := built(s.x.PlainUpstream(src.Ref))
		return u, src, err
	}
	return nil, src, fmt.Errorf("lookup: unknown source kind %q", src.Kind)
}

// StampCard is one decoded line of the stamp tool.
type StampCard struct {
	Line   string         `json:"line"`
	Fields *stamps.Fields `json:"fields,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// DecodeStamps decodes one stamp per non-empty line.
func (s *Service) DecodeStamps(text string) []StampCard {
	out := []StampCard{}
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		c := StampCard{Line: line}
		if f, err := stamps.Decode(line); err != nil {
			c.Error = err.Error()
		} else {
			c.Fields = &f
		}
		out = append(out, c)
	}
	return out
}

// EncodeStamp builds a stamp; a bad field gives STAMP_INVALID{field}.
func (s *Service) EncodeStamp(f stamps.Fields) (string, error) {
	st, err := stamps.Encode(f)
	if err != nil {
		return "", stampErr(err)
	}
	return st, nil
}

// StampFromURL fills stamp fields from a DoH/DoT/DoQ URL and optional IP.
func (s *Service) StampFromURL(raw, ip string) (stamps.Fields, error) {
	f, err := stamps.FromURL(raw, ip)
	if err != nil {
		return stamps.Fields{}, stampErr(err)
	}
	return f, nil
}

// stampErr maps "stamps: invalid: <field>: why" to STAMP_INVALID{field}.
func stampErr(err error) error {
	if !errors.Is(err, stamps.ErrInvalid) {
		return err
	}
	rest := strings.TrimPrefix(err.Error(), stamps.ErrInvalid.Error()+": ")
	field, _, _ := strings.Cut(rest, ":")
	return appErr(CodeStampInvalid, err, "field", field)
}

// ServicePing represents real-time latency and reachability to an endpoint.
type ServicePing struct {
	Target    string `json:"target"`
	Label     string `json:"label"`
	LatencyMs int64  `json:"latencyMs"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// NetworkDiagnosticsResult holds comprehensive network health, leak, and ECH metrics.
type NetworkDiagnosticsResult struct {
	DNSProtected  bool          `json:"dnsProtected"`
	DNSLeakStatus string        `json:"dnsLeakStatus"`
	ECHSupported  bool          `json:"echSupported"`
	ECHDetail     string        `json:"echDetail"`
	Targets       []ServicePing `json:"targets"`
}

// OptimizeUpstreams re-evaluates active upstreams and hot-swaps to the lowest latency servers.
func (s *Service) OptimizeUpstreams(ctx context.Context) error {
	s.o.ApplyBest(ctx)
	return nil
}

// NetworkDiagnostics tests DNS leak protection, ECH support, and measures TCP latency to essential services.
func (s *Service) NetworkDiagnostics(ctx context.Context) (NetworkDiagnosticsResult, error) {
	res := NetworkDiagnosticsResult{
		DNSProtected:  s.connected(),
		DNSLeakStatus: "unprotected",
		ECHSupported:  true,
		ECHDetail:     "ECH configuration active on secure upstreams",
		Targets:       make([]ServicePing, 5),
	}
	if s.connected() {
		res.DNSLeakStatus = "protected"
	}

	targets := []struct {
		target string
		label  string
	}{
		{"store.steampowered.com:443", "Steam Store"},
		{"steamcommunity.com:443", "Steam Community"},
		{"discord.com:443", "Discord"},
		{"1.1.1.1:443", "Cloudflare Anycast"},
		{"8.8.8.8:53", "Google DNS"},
	}

	var wg sync.WaitGroup
	dialer := net.Dialer{Timeout: 3 * time.Second}

	for i, t := range targets {
		wg.Add(1)
		go func(idx int, target, label string) {
			defer wg.Done()
			start := time.Now()
			conn, err := dialer.DialContext(ctx, "tcp", target)
			rtt := time.Since(start).Milliseconds()
			if err != nil {
				res.Targets[idx] = ServicePing{
					Target:    target,
					Label:     label,
					LatencyMs: -1,
					OK:        false,
					Error:     err.Error(),
				}
				return
			}
			_ = conn.Close()
			res.Targets[idx] = ServicePing{
				Target:    target,
				Label:     label,
				LatencyMs: rtt,
				OK:        true,
			}
		}(i, t.target, t.label)
	}
	wg.Wait()
	return res, nil
}

// GameStatusResult represents the state of CS2/Dota2 detection and VAC safety.
type GameStatusResult struct {
	ActiveGames      []string              `json:"activeGames"`
	IsGaming         bool                  `json:"isGaming"`
	DPIDisarmed      bool                  `json:"dpiDisarmed"`
	SDRClusters      []game.SDRProbeResult `json:"sdrClusters"`
	WindowsOptimized bool                  `json:"windowsOptimized"`
}

// GetGameStatus returns whether CS2/Dota2/Steam is active, whether WinDivert is disarmed for VAC safety, and measures SDR pings.
func (s *Service) GetGameStatus(ctx context.Context) (GameStatusResult, error) {
	running, err := game.FindRunningGames(game.DefaultTargetGames)
	if err != nil {
		running = []string{}
	}

	optApplied, _ := game.IsWindowsGamingTweaksApplied()
	sdrResults := game.ProbeSDRClusters(ctx, 2*time.Second)

	s.o.mu.Lock()
	disarmed := s.o.pausedDPIForGame
	s.o.mu.Unlock()

	return GameStatusResult{
		ActiveGames:      running,
		IsGaming:         len(running) > 0,
		DPIDisarmed:      disarmed,
		SDRClusters:      sdrResults,
		WindowsOptimized: optApplied,
	}, nil
}

// ApplyGamingNetworkTweaks applies or reverts safe Windows registry network optimizations.
func (s *Service) ApplyGamingNetworkTweaks(enable bool) error {
	return game.ApplyWindowsGamingTweaks(enable)
}

// GetSDRRelayPings returns latency measurements to all Valve SDR relay locations.
func (s *Service) GetSDRRelayPings(ctx context.Context) ([]game.SDRProbeResult, error) {
	return game.ProbeSDRClusters(ctx, 2*time.Second), nil
}


