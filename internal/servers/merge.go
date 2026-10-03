package servers

import (
	"bufio"
	"bytes"
	"slices"
	"strings"

	"github.com/hashcott/ghostline/internal/model"
)

// ParseImport reads one server URL or stamp per line, skipping blanks and
// "#" comments. Lines that are not encrypted servers are returned in bad.
func ParseImport(text []byte) (out []model.Server, bad []string) {
	sc := bufio.NewScanner(bytes.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, err := FromAddress(line, model.SourceCustom)
		if err != nil {
			bad = append(bad, line)
			continue
		}
		out = append(out, s)
	}
	return out, bad
}

// Merge combines all sources. The remote list replaces the built-in one only
// when it is newer. Duplicates (same address) resolve custom > list > dnscrypt.
// The result is sorted by ID.
func Merge(builtin, remote List, dnscrypt, custom []model.Server) []model.Server {
	base := builtin.Servers
	if remote.GeneratedAt.After(builtin.GeneratedAt) {
		base = remote.Servers
	}
	seen := map[string]bool{}
	var out []model.Server
	for _, group := range [][]model.Server{custom, base, dnscrypt} {
		for _, s := range group {
			if seen[s.Address] {
				continue
			}
			seen[s.Address] = true
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b model.Server) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Filter keeps servers carrying at least one of includeTags; custom servers
// are always kept.
func Filter(all []model.Server, includeTags []string) []model.Server {
	var out []model.Server
	for _, s := range all {
		if s.Source == model.SourceCustom {
			out = append(out, s)
			continue
		}
		for _, t := range s.Tags {
			if slices.Contains(includeTags, t) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}
