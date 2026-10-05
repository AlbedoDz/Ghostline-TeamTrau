package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"github.com/AdguardTeam/dnsproxy/upstream"
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
// servers from the last scan, three sources in all. It never includes the
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
	for _, r := range rs {
		if len(out) == 3 {
			break
		}
		if sv, ok := byID[r.ServerID]; ok && r.OK {
			out = append(out, lookup.Source{Kind: "server", Ref: sv.ID, Label: sv.Name})
		}
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
	for i := range sources {
		u, src, err := s.lookupUpstream(sources[i])
		if err != nil {
			return LookupResult{}, err
		}
		ups[i], sources[i] = u, src
	}
	res := LookupResult{Answers: make([]lookup.Answer, len(sources))}
	var wg sync.WaitGroup
	for i := range sources {
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

// lookupUpstream resolves a source to an upstream and fills its label.
// Only kind "isp" (and Ghostline's own loopback engine) may be plain DNS.
func (s *Service) lookupUpstream(src lookup.Source) (upstream.Upstream, lookup.Source, error) {
	switch src.Kind {
	case "ghostline":
		if !s.connected() {
			return nil, src, appErr(CodeLookupNotConnected, nil)
		}
		src.Label = "Ghostline"
		u, err := s.x.PlainUpstream("127.0.0.1")
		return u, src, err
	case "server":
		i := slices.IndexFunc(s.x.Catalog(), func(sv model.Server) bool { return sv.ID == src.Ref })
		if i < 0 {
			return nil, src, fmt.Errorf("lookup: unknown server %q", src.Ref)
		}
		sv := s.x.Catalog()[i]
		src.Label = sv.Name
		u, err := s.x.BuildUpstream(sv)
		return u, src, err
	case "address":
		sv, err := servers.FromAddress(src.Ref, model.SourceCustom)
		if err != nil {
			return nil, src, fmt.Errorf("lookup: %w", err)
		}
		src.Label = src.Ref
		u, err := s.x.BuildUpstream(sv)
		return u, src, err
	case "isp":
		a, err := netip.ParseAddr(src.Ref)
		if err != nil {
			return nil, src, fmt.Errorf("lookup: %q is not an IP", src.Ref)
		}
		src.Ref, src.Label = a.String(), a.String()
		u, err := s.x.PlainUpstream(src.Ref)
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
