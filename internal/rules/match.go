package rules

import (
	"net/netip"
	"strings"
)

// normalize returns host in matcher form without allocating when it already
// is (lower-case ASCII, no trailing dot).
func normalize(host string) string {
	clean := true
	for i := 0; i < len(host); i++ {
		c := host[i]
		if c >= 'A' && c <= 'Z' || c >= 0x80 {
			clean = false
			break
		}
	}
	if clean && !strings.HasSuffix(host, ".") {
		return host
	}
	h, err := NormalizeHost(host)
	if err != nil {
		return strings.ToLower(strings.TrimSuffix(host, "."))
	}
	return h
}

// eachSuffix calls fn for host and each parent domain; full is true for the
// host itself. It stops when fn returns true.
func eachSuffix(host string, fn func(s string, full bool) bool) {
	s := host
	full := true
	for {
		if fn(s, full) {
			return
		}
		i := strings.IndexByte(s, '.')
		if i < 0 {
			return
		}
		s, full = s[i+1:], false
	}
}

func wantBits(full bool) uint8 {
	if full {
		return bitDomain | bitExact
	}
	return bitDomain | bitSub
}

func (u *userSet) matchHost(host string) (int, bool) {
	best, found := 0, false
	take := func(p int) {
		if !found || p < best {
			best, found = p, true
		}
	}
	eachSuffix(host, func(s string, full bool) bool {
		for _, r := range u.dom[s] {
			if r.bit&wantBits(full) != 0 {
				take(r.prio)
			}
		}
		return false
	})
	for _, k := range u.kw {
		if strings.Contains(host, k.s) {
			take(k.prio)
		}
	}
	for _, r := range u.rx {
		if (!found || r.prio < best) && r.re.MatchString(host) {
			take(r.prio)
		}
	}
	return best, found
}

// matchHost reports whether host matches m, and the line of the match.
func (m *lmatch) matchHost(host string) (int, bool) {
	line, found := 0, false
	eachSuffix(host, func(s string, full bool) bool {
		if m.dom[s]&wantBits(full) != 0 {
			line, found = m.line[s], true
			return true
		}
		return false
	})
	if found {
		return line, true
	}
	for _, k := range m.kw {
		if strings.Contains(host, k.s) {
			return k.prio, true
		}
	}
	for _, r := range m.rx {
		if r.re.MatchString(host) {
			return r.prio, true
		}
	}
	return 0, false
}

// actionFor is the list's action for a matched host.
func (l *listSet) actionFor(host string, line int) Action {
	if a, ok := l.perLine[line]; ok {
		return a
	}
	if !l.fromFile {
		return l.action
	}
	var ips []netip.Addr
	eachSuffix(host, func(s string, _ bool) bool {
		ips = l.ips[s]
		return ips != nil
	})
	if len(ips) > 0 {
		return Action{IPs: ips}
	}
	return Action{Block: true}
}

// decision wraps a list's action, dropping sni= and connect= unless the
// list is trusted for Fake SNI.
func (l *listSet) decision(a Action, line int) Decision {
	d := Decision{Action: a, Source: Source{Kind: "list", ListID: l.id, Line: line}}
	if !l.trusted && (d.SNI != "" || d.Connect != "") {
		d.SNI, d.Connect, d.SNIIgnored = "", "", true
	}
	return d
}

// Match decides what to do with host (may be "") and ip (may be invalid):
// user rules first (first match wins), then lists in order; CIDR patterns
// only fill fields a domain match left unset.
func (c *Compiled) Match(host string, ip netip.Addr) Decision {
	var d Decision
	matched := false
	if host != "" {
		host = normalize(host)
		if i, ok := c.user.matchHost(host); ok {
			d = Decision{Action: c.user.rules[i].Action, Source: Source{Kind: "rule", Index: i}}
			matched = true
		} else {
			for li := range c.lists {
				l := &c.lists[li]
				line, ok := l.inc.matchHost(host)
				if !ok {
					continue
				}
				if _, ex := l.exc.matchHost(host); ex {
					continue
				}
				d = l.decision(l.actionFor(host, line), line)
				matched = true
				break
			}
		}
	}
	if !ip.IsValid() || d.Block || d.Allow {
		return d
	}
	var cd Decision
	cfound := false
	if i, ok := c.user.cidr.lookup(ip); ok {
		cd = Decision{Action: c.user.rules[i].Action, Source: Source{Kind: "rule", Index: i}}
		cfound = true
	} else {
		for li := range c.lists {
			l := &c.lists[li]
			line, ok := l.inc.cidr.lookup(ip)
			if !ok {
				continue
			}
			if _, ex := l.exc.cidr.lookup(ip); ex {
				continue
			}
			if a, ok := l.perLine[line]; ok {
				cd = l.decision(a, line)
			} else {
				cd = l.decision(l.action, line)
				if l.fromFile {
					cd.Action = Action{Block: true}
				}
			}
			cfound = true
			break
		}
	}
	if !cfound {
		return d
	}
	if !matched {
		return cd
	}
	if d.Fragment == FragUnset {
		d.Fragment = cd.Fragment
	}
	if d.Upstream == "" {
		d.Upstream = cd.Upstream
	}
	return d
}

// Explain is Match for a host typed by the user (any case, IDN, trailing dot).
func (c *Compiled) Explain(host string) Decision {
	h, err := NormalizeHost(host)
	if err != nil {
		return Decision{}
	}
	return c.Match(h, netip.Addr{})
}
