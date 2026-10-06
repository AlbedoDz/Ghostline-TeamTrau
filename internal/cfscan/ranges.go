// Package cfscan finds Cloudflare edge IPs that work from the current
// network (spec 3 §7).
package cfscan

import (
	_ "embed"
	"net/netip"
	"strings"
)

// rangesV4 is written by tools/gencfranges from cloudflare.com/ips-v4.
//
//go:embed ranges_v4.txt
var rangesV4 string

var ranges = func() []netip.Prefix {
	var out []netip.Prefix
	for _, line := range strings.Split(rangesV4, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, netip.MustParsePrefix(line))
	}
	return out
}()

// Ranges returns Cloudflare's IPv4 ranges.
func Ranges() []netip.Prefix { return ranges }

// Contains reports whether a lies in one of rs.
func Contains(rs []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range rs {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
