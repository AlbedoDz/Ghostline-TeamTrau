package formats

import (
	"encoding/json"
	"net/netip"
	"strings"

	"github.com/hashcott/ghostline/internal/rules"
)

// localNames are hosts-file housekeeping entries, not blocks.
var localNames = map[string]bool{
	"localhost": true, "localhost.localdomain": true, "local": true, "broadcasthost": true,
	"ip6-localhost": true, "ip6-loopback": true, "ip6-localnet": true, "ip6-mcastprefix": true,
	"ip6-allnodes": true, "ip6-allrouters": true, "ip6-allhosts": true, "0.0.0.0": true,
}

func parseHosts(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	f := strings.Fields(stripComment(line))
	if len(f) < 2 {
		b.skip(line)
		return
	}
	ip, err := netip.ParseAddr(f[0])
	if err != nil {
		b.skip(line)
		return
	}
	var ips []netip.Addr
	if !sinkhole(ip) {
		ips = []netip.Addr{ip}
	}
	for _, name := range f[1:] {
		if localNames[strings.ToLower(name)] {
			continue
		}
		b.addHost(rules.KindExact, name, n, line, false, ips)
	}
}

func parseDomains(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	s := stripComment(line)
	switch {
	case strings.ContainsAny(s, " \t"):
		b.skip(line)
	case strings.HasPrefix(s, "*."):
		b.addHost(rules.KindSubOnly, s[2:], n, line, false, nil)
	default:
		b.addHost(rules.KindDomain, strings.TrimPrefix(s, "."), n, line, false, nil)
	}
}

// adblockMods are modifiers that do not change a DNS-level block.
var adblockMods = map[string]bool{"important": true, "all": true}

func parseAdblock(b *builder, line string, n int) {
	if strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "##") {
		return
	}
	s, except := line, false
	if strings.HasPrefix(s, "@@") {
		s, except = s[2:], true
	}
	if !strings.HasPrefix(s, "||") {
		b.skip(line)
		return
	}
	s = s[2:]
	rule, mods, _ := strings.Cut(s, "$")
	if mods != "" {
		for _, m := range strings.Split(mods, ",") {
			if !adblockMods[m] {
				b.skip(line)
				return
			}
		}
	}
	d, ok := strings.CutSuffix(rule, "^")
	if !ok || strings.ContainsAny(d, "/*|^") {
		b.skip(line)
		return
	}
	b.addHost(rules.KindDomain, d, n, line, except, nil)
}

func parseDnsmasq(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	key, val, ok := strings.Cut(line, "=")
	if !ok || !strings.HasPrefix(val, "/") {
		b.skip(line)
		return
	}
	parts := strings.Split(val[1:], "/")
	if len(parts) < 2 {
		b.skip(line)
		return
	}
	domains, target := parts[:len(parts)-1], parts[len(parts)-1]
	var ips []netip.Addr
	switch key {
	case "address":
		if target != "" && target != "#" {
			ip, err := netip.ParseAddr(target)
			if err != nil {
				b.skip(line)
				return
			}
			if !sinkhole(ip) {
				ips = []netip.Addr{ip}
			}
		}
	case "server", "local":
		if target != "" {
			b.skip(line) // forwarding to a resolver, not a block
			return
		}
	default:
		b.skip(line)
		return
	}
	for _, d := range domains {
		b.addHost(rules.KindDomain, d, n, line, false, ips)
	}
}

var unboundBlock = map[string]bool{"always_nxdomain": true, "always_null": true, "always_refuse": true, "refuse": true, "static": true, "redirect": true, "deny": true}

func parseUnbound(b *builder, line string, n int) {
	if isComment(line) || line == "server:" {
		return
	}
	rest, ok := strings.CutPrefix(line, "local-zone:")
	if !ok {
		if strings.HasPrefix(line, "local-data:") {
			return
		}
		b.skip(line)
		return
	}
	f := strings.Fields(rest)
	if len(f) != 2 || !unboundBlock[f[1]] {
		b.skip(line)
		return
	}
	b.addHost(rules.KindDomain, strings.Trim(f[0], `"`), n, line, false, nil)
}

func parseRPZ(b *builder, line string, n int) {
	if isComment(line) || strings.HasPrefix(line, "$") {
		return
	}
	f := strings.Fields(stripComment(strings.ReplaceAll(line, ";", "#")))
	if len(f) == 0 {
		return
	}
	i := 1
	for i < len(f) && (f[i] == "IN" || isNumber(f[i])) {
		i++
	}
	if f[0] == "@" || i < len(f) && (f[i] == "SOA" || f[i] == "NS") || len(f) >= 1 && (f[0] == "NS" || f[0] == "SOA") {
		return
	}
	if i+1 >= len(f) || f[i] != "CNAME" {
		b.skip(line)
		return
	}
	switch f[i+1] {
	case ".", "*.":
	default:
		b.skip(line)
		return
	}
	name := strings.TrimSuffix(f[0], ".")
	if s, ok := strings.CutPrefix(name, "*."); ok {
		b.addHost(rules.KindSubOnly, s, n, line, false, nil)
		return
	}
	b.addHost(rules.KindExact, name, n, line, false, nil)
}

func isNumber(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

var clashKinds = map[string]rules.PatternKind{
	"DOMAIN":         rules.KindExact,
	"DOMAIN-SUFFIX":  rules.KindDomain,
	"DOMAIN-KEYWORD": rules.KindKeyword,
	"DOMAIN-REGEX":   rules.KindRegexp,
	"IP-CIDR":        rules.KindCIDR,
	"IP-CIDR6":       rules.KindCIDR,
}

func parseClash(b *builder, line string, n int) {
	if isComment(line) || line == "payload:" {
		return
	}
	s := strings.TrimSpace(strings.TrimPrefix(line, "- "))
	s = strings.Trim(s, `'"`)
	typ, val, ok := strings.Cut(s, ",")
	if ok {
		kind, known := clashKinds[strings.ToUpper(typ)]
		if !known {
			b.skip(line)
			return
		}
		val, _, _ = strings.Cut(val, ",") // drop policy / no-resolve
		b.addPattern(kind, strings.TrimSpace(val), n, line)
		return
	}
	// Domain-set / ipcidr behaviour: "+.d", ".d", "*.d", "d" or a CIDR.
	switch {
	case strings.HasPrefix(s, "+."):
		b.addHost(rules.KindDomain, s[2:], n, line, false, nil)
	case strings.HasPrefix(s, "*."):
		b.addHost(rules.KindSubOnly, s[2:], n, line, false, nil)
	case strings.HasPrefix(s, "."):
		b.addHost(rules.KindSubOnly, s[1:], n, line, false, nil)
	default:
		if _, err := netip.ParsePrefix(s); err == nil {
			b.addPattern(rules.KindCIDR, s, n, line)
			return
		}
		b.addHost(rules.KindExact, s, n, line, false, nil)
	}
}

func parseV2fly(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	s := strings.Fields(stripComment(line))[0] // drop "@attr" and "&affiliation"
	prefix, val, ok := strings.Cut(s, ":")
	if !ok {
		b.addHost(rules.KindDomain, s, n, line, false, nil)
		return
	}
	switch prefix {
	case "domain":
		b.addHost(rules.KindDomain, val, n, line, false, nil)
	case "full":
		b.addHost(rules.KindExact, val, n, line, false, nil)
	case "keyword":
		b.addPattern(rules.KindKeyword, val, n, line)
	case "regexp":
		b.addPattern(rules.KindRegexp, val, n, line)
	case "include":
		b.r.Includes = append(b.r.Includes, val)
	default:
		b.skip(line)
	}
}

func parseCIDR(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	b.addPattern(rules.KindCIDR, stripComment(line), n, line)
}

type singboxRule struct {
	Type          string   `json:"type"`
	Domain        []string `json:"domain"`
	DomainSuffix  []string `json:"domain_suffix"`
	DomainKeyword []string `json:"domain_keyword"`
	DomainRegex   []string `json:"domain_regex"`
	IPCIDR        []string `json:"ip_cidr"`
}

func parseSingbox(b *builder, data []byte) error {
	var doc struct {
		Rules []singboxRule `json:"rules"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ErrUnsupported
	}
	for _, r := range doc.Rules {
		if r.Type != "" && r.Type != "default" {
			b.skip(r.Type + " rule")
			continue
		}
		for _, d := range r.Domain {
			b.addHost(rules.KindExact, d, 1, d, false, nil)
		}
		for _, d := range r.DomainSuffix {
			if s, ok := strings.CutPrefix(d, "."); ok {
				b.addHost(rules.KindSubOnly, s, 1, d, false, nil)
			} else {
				b.addHost(rules.KindDomain, d, 1, d, false, nil)
			}
		}
		for _, k := range r.DomainKeyword {
			b.addPattern(rules.KindKeyword, k, 1, k)
		}
		for _, re := range r.DomainRegex {
			b.addPattern(rules.KindRegexp, re, 1, re)
		}
		for _, c := range r.IPCIDR {
			b.addPattern(rules.KindCIDR, c, 1, c)
		}
	}
	return nil
}
