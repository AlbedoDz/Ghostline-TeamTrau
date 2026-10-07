package store

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// MaxTestDomains caps the test domains: each one adds a query per server
// to every scan.
const MaxTestDomains = 5

// testDomainRe matches a plain domain name with at least two labels.
var testDomainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// TestDomains splits the testDomain setting (one or more domains, by line,
// space or comma) into lowercase domains without repeats.
func TestDomains(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// ValidateTestDomains checks the testDomain setting: 1 to MaxTestDomains
// domain names.
func ValidateTestDomains(s string) error {
	ds := TestDomains(s)
	if len(ds) == 0 || len(ds) > MaxTestDomains {
		return fmt.Errorf("settings: give 1 to %d test domains", MaxTestDomains)
	}
	for _, d := range ds {
		if !testDomainRe.MatchString(d) {
			return fmt.Errorf("settings: test domain %q must be a domain name like www.google.com", d)
		}
	}
	return nil
}
