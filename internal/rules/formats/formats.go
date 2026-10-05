// Package formats reads community block/rule lists (hosts, plain domains,
// AdBlock, dnsmasq, Unbound, RPZ, Clash, v2fly, sing-box, CIDR) into
// rules.Entry values.
package formats

import (
	"bytes"
	"errors"
	"net/netip"
	"strings"

	"github.com/hashcott/ghostline/internal/rules"
)

// Format names a list format.
type Format string

const (
	Hosts   Format = "hosts"
	Domains Format = "domains"
	Adblock Format = "adblock"
	Dnsmasq Format = "dnsmasq"
	Unbound Format = "unbound"
	RPZ     Format = "rpz"
	Clash   Format = "clash"
	V2fly   Format = "v2fly"
	Singbox Format = "singbox"
	CIDR    Format = "cidr"
	// Ghostline is Ghostline's own rule text with an action per line.
	Ghostline Format = "ghostline"
)

// All lists every format, most specific first (used to break ties).
var All = []Format{Ghostline, Adblock, Dnsmasq, Unbound, RPZ, Clash, V2fly, Singbox, Hosts, CIDR, Domains}

// Result is a parsed list.
type Result struct {
	Format   Format         `json:"format"`
	Entries  []rules.Entry  `json:"-"`
	Includes []string       `json:"includes,omitempty"`
	Counts   map[string]int `json:"counts"`
	Skipped  int            `json:"skipped"`
	Samples  []string       `json:"samples"`
}

// ErrUnsupported means the content is binary or in no known format.
var ErrUnsupported = errors.New("formats: unsupported or binary content")

// MaxRegexpPerList caps regexps per list; extra ones count as skipped.
const MaxRegexpPerList = 1000

const maxSamples = 5

// Parse reads data in format f.
func Parse(f Format, data []byte) (Result, error) {
	if isBinary(data) {
		return Result{}, ErrUnsupported
	}
	b := &builder{r: Result{Format: f, Counts: map[string]int{}, Samples: []string{}}}
	if f == Singbox {
		if err := parseSingbox(b, data); err != nil {
			return Result{}, err
		}
		return b.r, nil
	}
	fn := lineParsers[f]
	if fn == nil {
		return Result{}, ErrUnsupported
	}
	eachLine(data, func(n int, line string) { fn(b, line, n) })
	return b.r, nil
}

var lineParsers = map[Format]func(b *builder, line string, n int){
	Hosts:     parseHosts,
	Domains:   parseDomains,
	Adblock:   parseAdblock,
	Dnsmasq:   parseDnsmasq,
	Unbound:   parseUnbound,
	RPZ:       parseRPZ,
	Clash:     parseClash,
	V2fly:     parseV2fly,
	CIDR:      parseCIDR,
	Ghostline: parseGhostline,
}

func isBinary(data []byte) bool {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.IndexByte(head, 0) >= 0
}

// eachLine calls fn with each trimmed line (1-based numbers), BOM and CR
// removed. Empty lines are skipped.
func eachLine(data []byte, fn func(n int, line string)) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line != "" {
			fn(i+1, line)
		}
	}
}

// stripComment removes a trailing " #…" comment.
func stripComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func isComment(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, ";")
}

type builder struct {
	r   Result
	nre int
}

var countKey = map[rules.PatternKind]string{
	rules.KindDomain:  "domain",
	rules.KindExact:   "exact",
	rules.KindSubOnly: "suffix",
	rules.KindKeyword: "keyword",
	rules.KindRegexp:  "regexp",
	rules.KindCIDR:    "cidr",
}

func (b *builder) add(p rules.Pattern, n int, raw string, except bool, ips []netip.Addr) {
	if p.Kind == rules.KindRegexp {
		if b.nre >= MaxRegexpPerList {
			b.skip(raw)
			return
		}
		b.nre++
	}
	b.r.Entries = append(b.r.Entries, rules.Entry{Pattern: p, Except: except, IPs: ips, Line: n})
	b.r.Counts[countKey[p.Kind]]++
}

func (b *builder) skip(raw string) {
	b.r.Skipped++
	if len(b.r.Samples) < maxSamples {
		b.r.Samples = append(b.r.Samples, raw)
	}
}

// host normalises s as a domain name; IP literals are not hosts.
func host(s string) (string, bool) {
	if _, err := netip.ParseAddr(s); err == nil {
		return "", false
	}
	h, err := rules.NormalizeHost(s)
	if err != nil || !strings.Contains(h, ".") && len(h) < 2 {
		return "", false
	}
	return h, true
}

// addHost adds a domain pattern of kind for s, or skips raw.
func (b *builder) addHost(kind rules.PatternKind, s string, n int, raw string, except bool, ips []netip.Addr) {
	h, ok := host(s)
	if !ok {
		b.skip(raw)
		return
	}
	b.add(rules.Pattern{Kind: kind, Value: h}, n, raw, except, ips)
}

func (b *builder) addPattern(kind rules.PatternKind, s string, n int, raw string) {
	switch kind {
	case rules.KindKeyword:
		if s == "" {
			b.skip(raw)
			return
		}
		b.add(rules.Pattern{Kind: kind, Value: strings.ToLower(s)}, n, raw, false, nil)
	case rules.KindRegexp:
		p, err := rules.ParsePattern("/" + s + "/")
		if err != nil {
			b.skip(raw)
			return
		}
		b.add(p, n, raw, false, nil)
	case rules.KindCIDR:
		p, err := rules.ParsePattern(s)
		if err != nil || p.Kind != rules.KindCIDR {
			b.skip(raw)
			return
		}
		b.add(p, n, raw, false, nil)
	default:
		b.addHost(kind, s, n, raw, false, nil)
	}
}

// sinkhole reports whether ip is a "block" address in hosts/dnsmasq lists.
func sinkhole(ip netip.Addr) bool {
	return ip.IsUnspecified() || ip.IsLoopback()
}
