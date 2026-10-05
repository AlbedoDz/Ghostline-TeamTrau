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
	Action         string         `json:"action"` // block | allow | fragment=on | upstream=<id> | fromFile | perLine
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
	// TrustedForSNI lets the list's sni= and connect= take effect.
	TrustedForSNI bool `json:"trustedForSNI,omitempty"`
	// Signed lists are only accepted with a valid .sig next to them.
	Signed      bool `json:"signed,omitempty"`
	SignatureOK bool `json:"signatureOk,omitempty"`
}

// ToListSet maps a list and its parsed entries to a compilable set.
func ToListSet(l List, r Result) (rules.ListSet, error) {
	s := rules.ListSet{ID: l.ID, Entries: r.Entries, TrustedForSNI: l.TrustedForSNI}
	if r.Format == formats.Ghostline {
		return s, nil // every entry carries its own action
	}
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
	case l.Action == "perLine":
		return rules.ListSet{}, fmt.Errorf("lists: per-line actions need a ghostline list, got %q", r.Format)
	default:
		return rules.ListSet{}, fmt.Errorf("lists: unknown action %q", l.Action)
	}
	return s, nil
}

// Due reports whether an enabled list with automatic updates is stale.
func Due(l List, now time.Time) bool {
	return l.Enabled && l.UpdateHours > 0 && now.Sub(l.LastUpdated) >= time.Duration(l.UpdateHours)*time.Hour
}
