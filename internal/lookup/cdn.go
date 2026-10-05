package lookup

import "net/netip"

// cdnPrefixes are well-known address blocks of large CDNs. CDNs answer with
// different addresses by location, so two answers inside the same CDN are
// treated as agreeing. Kept small on purpose; lookup does not depend on
// cfscan's full Cloudflare list.
var cdnPrefixes = map[string][]string{
	"cloudflare": {
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22", "2606:4700::/32",
	},
	"google":     {"142.250.0.0/15", "172.217.0.0/16", "216.58.192.0/19"},
	"akamai":     {"23.32.0.0/11", "104.64.0.0/10"},
	"fastly":     {"151.101.0.0/16"},
	"cloudfront": {"13.32.0.0/15", "18.64.0.0/14", "52.84.0.0/15"},
}

var cdnTable = func() map[string][]netip.Prefix {
	t := map[string][]netip.Prefix{}
	for name, ps := range cdnPrefixes {
		for _, p := range ps {
			t[name] = append(t[name], netip.MustParsePrefix(p))
		}
	}
	return t
}()

// CDNOf names the CDN an address belongs to, or "".
func CDNOf(a netip.Addr) string {
	a = a.Unmap()
	for name, ps := range cdnTable {
		for _, p := range ps {
			if p.Contains(a) {
				return name
			}
		}
	}
	return ""
}
