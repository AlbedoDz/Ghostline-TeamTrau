package rules

import (
	"slices"
)

// SNIDomains lists the Name Constraints a Fake SNI session CA needs: one
// entry per enabled rule with sni=, "domain" for domain and =domain
// patterns, ".domain" for *.domain (subdomains only). Sorted, no
// duplicates.
func (c *Compiled) SNIDomains() []string {
	var out []string
	for _, r := range c.user.rules {
		if !r.Enabled || r.SNI == "" {
			continue
		}
		p, err := ParsePattern(r.Pattern)
		if err != nil {
			continue
		}
		if d, ok := constraintFor(p); ok {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func constraintFor(p Pattern) (string, bool) {
	switch p.Kind {
	case KindDomain, KindExact:
		return p.Value, true
	case KindSubOnly:
		return "." + p.Value, true
	}
	return "", false
}
