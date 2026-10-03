package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/servers"
)

// Generate validates the seed, fills in IPs (resolving hostnames with
// resolve, reading stamps directly) and returns a list sorted by ID.
func Generate(seed []model.Server, resolve func(host string) ([]string, error), now time.Time) (servers.List, error) {
	out := make([]model.Server, 0, len(seed))
	for _, s := range seed {
		parsed, err := servers.FromAddress(s.Address, model.SourceBuiltin)
		if err != nil {
			return servers.List{}, fmt.Errorf("%s: %w", s.ID, err)
		}
		s.Protocol = parsed.Protocol
		s.Source = model.SourceBuiltin
		if s.Name == "" {
			s.Name = parsed.Name
		}
		if s.Provider == "" {
			s.Provider = parsed.Provider
		}
		if strings.HasPrefix(s.Address, "sdns://") {
			s.IPs = parsed.IPs
			if len(s.Tags) == 0 {
				s.Tags = parsed.Tags
			}
		} else {
			ips, err := resolve(hostOnly(s.Address))
			if err != nil || len(ips) == 0 {
				return servers.List{}, fmt.Errorf("%s: resolve: %v", s.ID, err)
			}
			s.IPs = ips
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b model.Server) int { return strings.Compare(a.ID, b.ID) })
	return servers.List{GeneratedAt: now, Servers: out}, nil
}

func hostOnly(addr string) string {
	rest := addr[strings.Index(addr, "://")+3:]
	if i := strings.IndexAny(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.Contains(rest, "]") {
		rest = rest[:i]
	}
	return rest
}
