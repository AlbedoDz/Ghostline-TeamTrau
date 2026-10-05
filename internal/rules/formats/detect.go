package formats

import (
	"bytes"
	"net/netip"
	"path"
	"regexp"
	"strings"
)

const (
	detectLines     = 200
	detectThreshold = 0.6
)

var (
	reDnsmasq  = regexp.MustCompile(`^(address|server|local)=/`)
	reClashTyp = regexp.MustCompile(`^[A-Z][A-Z0-9-]+,`)
	reV2fly    = regexp.MustCompile(`^(domain|full|keyword|regexp|include):`)
)

// signatures say whether a line looks like a given format. Lenient parsers
// (Clash domain-set, v2fly bare domains) only count their distinctive forms
// here, so a plain domain list is not mistaken for them.
var signatures = map[Format]func(line string) bool{
	Adblock: func(l string) bool { return strings.HasPrefix(l, "||") || strings.HasPrefix(l, "@@||") },
	Dnsmasq: reDnsmasq.MatchString,
	Unbound: func(l string) bool {
		return l == "server:" || strings.HasPrefix(l, "local-zone:") || strings.HasPrefix(l, "local-data:")
	},
	RPZ: func(l string) bool {
		f := strings.Fields(l)
		if strings.HasPrefix(l, "$TTL") || strings.HasPrefix(l, "$ORIGIN") {
			return true
		}
		for _, x := range f[min(1, len(f)):] {
			if x == "CNAME" || x == "SOA" || x == "NS" {
				return true
			}
		}
		return len(f) > 0 && f[0] == "NS"
	},
	Clash: func(l string) bool {
		if l == "payload:" {
			return true
		}
		s := strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "- ")), `'"`)
		return reClashTyp.MatchString(s) || strings.HasPrefix(l, "- ") || strings.HasPrefix(s, "+.")
	},
	V2fly: reV2fly.MatchString,
	Hosts: func(l string) bool {
		f := strings.Fields(stripComment(l))
		if len(f) < 2 {
			return false
		}
		_, err := netip.ParseAddr(f[0])
		return err == nil
	},
	CIDR: func(l string) bool {
		s := stripComment(l)
		if _, err := netip.ParsePrefix(s); err == nil {
			return true
		}
		_, err := netip.ParseAddr(s)
		return err == nil
	},
	Domains: func(l string) bool {
		s := stripComment(l)
		if strings.ContainsAny(s, " \t") {
			return false
		}
		_, ok := host(strings.TrimPrefix(strings.TrimPrefix(s, "*."), "."))
		return ok
	},
}

// Detect guesses the format of data. name (a file name or URL) is used for
// its extension.
func Detect(name string, data []byte) (Format, error) {
	if isBinary(data) {
		return "", ErrUnsupported
	}
	if isGhostline(data) {
		return Ghostline, nil
	}
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	switch strings.ToLower(path.Ext(strings.ReplaceAll(name, `\`, "/"))) {
	case ".yaml", ".yml":
		return Clash, nil
	case ".json":
		return Singbox, nil
	case ".rpz":
		return RPZ, nil
	}
	if bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})), []byte("{")) {
		return Singbox, nil
	}
	var sample []string
	eachLine(data, func(_ int, line string) {
		if len(sample) >= detectLines || isComment(line) || strings.HasPrefix(line, "[") {
			return
		}
		sample = append(sample, line)
	})
	if len(sample) == 0 {
		return "", ErrUnsupported
	}
	best, bestScore := Format(""), 0
	for _, f := range All {
		sig := signatures[f]
		if sig == nil {
			continue
		}
		score := 0
		for _, l := range sample {
			if sig(l) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	if float64(bestScore) < detectThreshold*float64(len(sample)) {
		return "", ErrUnsupported
	}
	return best, nil
}
