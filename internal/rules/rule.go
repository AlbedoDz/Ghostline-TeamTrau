// Package rules holds Ghostline's domain/CIDR rules: the model, the text
// format, and the compiled matcher shared by the DNS engine and the proxy.
// It depends on no other Ghostline package.
package rules

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

// PatternKind says how a pattern matches a host.
type PatternKind uint8

const (
	KindDomain  PatternKind = iota // domain and every subdomain
	KindExact                      // =domain: only the domain itself
	KindSubOnly                    // *.domain: subdomains only
	KindKeyword                    // ~word: substring of the host
	KindRegexp                     // /re/: RE2 regexp on the host
	KindCIDR                       // IP prefix (proxy only)
)

// Pattern is a parsed, normalised match pattern.
type Pattern struct {
	Kind   PatternKind
	Value  string       // normalised host, keyword or regexp source
	Prefix netip.Prefix // KindCIDR only
}

// Frag is a per-rule fragment override.
type Frag string

const (
	FragUnset Frag = ""
	FragAuto  Frag = "auto"
	FragOn    Frag = "on"
	FragOff   Frag = "off"
)

// Action is what a rule does. The zero value does nothing.
type Action struct {
	Block    bool         `json:"block,omitempty"`
	Allow    bool         `json:"allow,omitempty"`
	IPs      []netip.Addr `json:"ips,omitempty"`
	Fragment Frag         `json:"fragment,omitempty"`
	Upstream string       `json:"upstream,omitempty"`
	SNI      string       `json:"sni,omitempty"` // reserved for phase 2B; ignored in 2A
}

// Empty reports whether the action does nothing.
func (a Action) Empty() bool {
	return !a.Block && !a.Allow && len(a.IPs) == 0 && a.Fragment == FragUnset && a.Upstream == "" && a.SNI == ""
}

// Rule is one user-written rule.
type Rule struct {
	Pattern string `json:"pattern"` // as typed, e.g. "*.ads.com"
	Action
	Enabled bool   `json:"enabled"`
	Comment string `json:"comment,omitempty"`
}

// LineError is a parse error on a 1-based line.
type LineError struct {
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

// MaxUserRules caps the number of user-written rules.
const MaxUserRules = 10000

var idnaProfile = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.Transitional(false))

// NormalizeHost lower-cases a host, converts IDN to punycode and drops a
// trailing dot.
func NormalizeHost(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if s == "" {
		return "", errors.New("empty host")
	}
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		s = strings.ToLower(s)
	} else {
		a, err := idnaProfile.ToASCII(s)
		if err != nil {
			return "", fmt.Errorf("invalid host %q: %w", s, err)
		}
		s = a
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' {
			return "", fmt.Errorf("invalid host %q", s)
		}
	}
	if strings.HasPrefix(s, ".") || strings.Contains(s, "..") {
		return "", fmt.Errorf("invalid host %q", s)
	}
	return s, nil
}

// ParsePattern parses domain, =domain, *.domain, ~keyword, /regexp/ or a
// CIDR/IP.
func ParsePattern(s string) (Pattern, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return Pattern{}, errors.New("empty pattern")
	case strings.HasPrefix(s, "/"):
		if len(s) < 3 || !strings.HasSuffix(s, "/") {
			return Pattern{}, fmt.Errorf("regexp %q must look like /expr/", s)
		}
		src := s[1 : len(s)-1]
		if _, err := regexp.Compile(src); err != nil {
			return Pattern{}, fmt.Errorf("bad regexp: %v", err)
		}
		return Pattern{Kind: KindRegexp, Value: src}, nil
	case strings.HasPrefix(s, "~"):
		k := strings.ToLower(s[1:])
		if k == "" {
			return Pattern{}, errors.New("empty keyword")
		}
		return Pattern{Kind: KindKeyword, Value: k}, nil
	}
	if p, err := netip.ParsePrefix(s); err == nil {
		return Pattern{Kind: KindCIDR, Prefix: p.Masked()}, nil
	} else if strings.Contains(s, "/") {
		return Pattern{}, fmt.Errorf("bad CIDR %q", s)
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return Pattern{Kind: KindCIDR, Prefix: netip.PrefixFrom(a, a.BitLen())}, nil
	}
	kind := KindDomain
	switch {
	case strings.HasPrefix(s, "="):
		kind, s = KindExact, s[1:]
	case strings.HasPrefix(s, "*."):
		kind, s = KindSubOnly, s[2:]
	}
	h, err := NormalizeHost(s)
	if err != nil {
		return Pattern{}, err
	}
	return Pattern{Kind: kind, Value: h}, nil
}
