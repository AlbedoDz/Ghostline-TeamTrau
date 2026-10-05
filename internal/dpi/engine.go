// Package dpi runs a DPI bypass engine (GoodbyeDPI or zapret2): extracting
// its hash-pinned files, starting it hidden and cleaning up its driver.
// Each engine lives in a subpackage that only builds and checks argv.
package dpi

import (
	"errors"
	"strings"
)

// Scope selects which traffic the engine touches.
type Scope string

const (
	ScopeAll       Scope = "all"
	ScopeBlacklist Scope = "blacklist"
)

// Plan is what to run. Manager.Start takes absolute list paths; Engine.Args
// receives names relative to the engine's directory.
type Plan struct {
	Strategy     string // preset/strategy id, or "custom"
	Custom       string // user-typed args when Strategy == "custom"
	Scope        Scope
	Blacklist    string
	AutoHostlist string // "" = off (zapret2 only)
}

// Strategy is one autotune step as the UI shows it.
type Strategy struct {
	ID   string            `json:"id"`
	Name map[string]string `json:"name"`
}

// Engine describes one DPI bypass program. It builds argv; Manager runs it.
type Engine interface {
	ID() string
	Files() map[string]string // slash path relative to the engine dir → SHA-256
	Exe() string              // slash path relative to the engine dir
	Args(p Plan) ([]string, error)
	Strategies() []Strategy // autotune order, lightest first
	ValidateCustom(s string) error
	HotReloadsLists() bool // the engine re-reads list files by itself
}

// ErrForbiddenFlag rejects flags Ghostline does not allow in custom args.
var ErrForbiddenFlag = errors.New("dpi: flag not allowed")

// BlacklistEntries counts the domains in a blacklist file's text: one per
// line, blank lines and # comments skipped.
func BlacklistEntries(text string) int {
	n := 0
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			n++
		}
	}
	return n
}

// Tokenize splits user-typed arguments on spaces and tabs; double quotes
// group words.
func Tokenize(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote, have = !inQuote, true
		case (r == ' ' || r == '\t') && !inQuote:
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, errors.New("dpi: unbalanced quote")
	}
	if have {
		out = append(out, cur.String())
	}
	return out, nil
}
