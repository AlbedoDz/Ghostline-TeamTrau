package proxy

import "net/netip"

var lanPrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// AllowedSource says whether a client may use the proxy: loopback always,
// private and link-local addresses only when sharing on the LAN.
func AllowedSource(a netip.Addr, shareLAN bool) bool {
	a = a.Unmap()
	if a.IsLoopback() {
		return true
	}
	if !shareLAN {
		return false
	}
	for _, p := range lanPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
