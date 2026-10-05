package rules

import (
	"slices"
)

// SNIDomains lists the Name Constraints a Fake SNI session CA needs: one
// entry per enabled rule with sni= (and per sni= entry of a list trusted
// for Fake SNI): the pattern's domain for domain, =domain and *.domain.
// Sorted, no duplicates.
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
	out = append(out, c.sniLists...)
	slices.Sort(out)
	return slices.Compact(out)
}

func constraintFor(p Pattern) (string, bool) {
	switch p.Kind {
	case KindDomain, KindExact, KindSubOnly:
		// RFC 5280 dNSName constraints have no "subdomains only" form (a
		// leading dot is not portable), so *.domain and =domain widen to
		// domain; the proxy still intercepts only what the rule matches.
		return p.Value, true
	}
	return "", false
}
