// Package lists downloads, caches and schedules community lists and turns
// them into rules.ListSet values. It imports rules and formats; neither
// imports it.
package lists

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/formats"
)

// Result is a parsed list (re-exported for callers).
type Result = formats.Result

// ErrUnsupported means the list is binary or in no known format.
var ErrUnsupported = formats.ErrUnsupported

// List is one list source and its metadata, stored in rules.json.
type List struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Source         string         `json:"source"` // "url" | "file"
	URL            string         `json:"url,omitempty"`
	Path           string         `json:"path,omitempty"`
	Format         string         `json:"format"` // "auto" or a formats.Format
	Action         string         `json:"action"` // block | allow | fragment=on | upstream=<id> | fromFile
	Enabled        bool           `json:"enabled"`
	UpdateHours    int            `json:"updateHours"`
	LastUpdated    time.Time      `json:"lastUpdated"`
	ETag           string         `json:"etag,omitempty"`
	LastModified   string         `json:"lastModified,omitempty"`
	Detected       string         `json:"detected,omitempty"`
	Counts         map[string]int `json:"counts,omitempty"`
	Skipped        int            `json:"skipped"`
	SkippedSamples []string       `json:"skippedSamples,omitempty"`
	LastError      string         `json:"lastError,omitempty"`
}

// ToListSet maps a list and its parsed entries to a compilable set.
func ToListSet(l List, r Result) (rules.ListSet, error) {
	s := rules.ListSet{ID: l.ID, Entries: r.Entries}
	switch {
	case l.Action == "block":
		s.Action.Block = true
	case l.Action == "allow":
		s.Action.Allow = true
	case l.Action == "fragment=on":
		s.Action.Fragment = rules.FragOn
	case l.Action == "fromFile":
		s.FromFile = true
	case strings.HasPrefix(l.Action, "upstream=") && len(l.Action) > len("upstream="):
		s.Action.Upstream = strings.TrimPrefix(l.Action, "upstream=")
	default:
		return rules.ListSet{}, fmt.Errorf("lists: unknown action %q", l.Action)
	}
	return s, nil
}

// Due reports whether an enabled list with automatic updates is stale.
func Due(l List, now time.Time) bool {
	return l.Enabled && l.UpdateHours > 0 && now.Sub(l.LastUpdated) >= time.Duration(l.UpdateHours)*time.Hour
}
