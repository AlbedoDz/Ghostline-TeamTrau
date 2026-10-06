package lookup_test

import (
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/lookup"
	"github.com/stretchr/testify/require"
)

func ans(ips ...string) lookup.Answer {
	a := lookup.Answer{OK: true, Rcode: "NOERROR"}
	for _, ip := range ips {
		typ := "A"
		if netip.MustParseAddr(ip).Is6() {
			typ = "AAAA"
		}
		a.Records = append(a.Records, lookup.Record{Type: typ, TTL: 300, Data: ip})
	}
	return a
}

func nx() lookup.Answer     { return lookup.Answer{OK: true, Rcode: "NXDOMAIN"} }
func failed() lookup.Answer { return lookup.Answer{OK: false, Error: "timeout"} }

func TestCompare(t *testing.T) {
	type V = lookup.Verdict
	cases := []struct {
		name    string
		qtype   string
		in      []lookup.Answer
		overall V
		per     []V
	}{
		{"private IP is poisoned", "A", []lookup.Answer{ans("10.10.34.35"), ans("142.250.1.1")}, lookup.VerdictPoisoned, []V{lookup.VerdictPoisoned, lookup.VerdictMatch}},
		{"nxdomain vs addresses", "A", []lookup.Answer{nx(), ans("142.250.1.1")}, lookup.VerdictPoisoned, []V{lookup.VerdictPoisoned, lookup.VerdictMatch}},
		{"same CDN different IPs", "A", []lookup.Answer{ans("104.16.1.1"), ans("104.16.2.2")}, lookup.VerdictMatch, []V{lookup.VerdictMatch, lookup.VerdictMatch}},
		{"different sets", "A", []lookup.Answer{ans("1.1.1.1"), ans("9.9.9.9")}, lookup.VerdictDiffers, []V{lookup.VerdictDiffers, lookup.VerdictDiffers}},
		{"order and TTL ignored", "A", []lookup.Answer{ans("5.5.5.5", "6.6.6.6"), ans("6.6.6.6", "5.5.5.5")}, lookup.VerdictMatch, []V{lookup.VerdictMatch, lookup.VerdictMatch}},
		{"failed source does not count", "A", []lookup.Answer{failed(), ans("5.5.5.5"), ans("5.5.5.5")}, lookup.VerdictMatch, []V{lookup.VerdictFailed, lookup.VerdictMatch, lookup.VerdictMatch}},
		{"all failed", "A", []lookup.Answer{failed(), failed()}, lookup.VerdictFailed, []V{lookup.VerdictFailed, lookup.VerdictFailed}},
		{"AAAA private", "AAAA", []lookup.Answer{ans("::1"), ans("2606:4700::1")}, lookup.VerdictPoisoned, []V{lookup.VerdictPoisoned, lookup.VerdictMatch}},
		{"all nxdomain agree", "A", []lookup.Answer{nx(), nx()}, lookup.VerdictMatch, []V{lookup.VerdictMatch, lookup.VerdictMatch}},
		{"MX has no verdict", "MX", []lookup.Answer{ans("1.1.1.1"), ans("9.9.9.9")}, lookup.VerdictNone, []V{lookup.VerdictNone, lookup.VerdictNone}},
	}
	for _, c := range cases {
		overall, per := lookup.Compare(c.qtype, c.in)
		require.Equal(t, c.overall, overall, c.name)
		require.Equal(t, c.per, per, c.name)
	}
}

func TestCDNOf(t *testing.T) {
	for ip, want := range map[string]string{
		"104.16.1.1": "cloudflare", "2606:4700::1111": "cloudflare", "142.250.1.1": "google",
		"23.40.1.1": "akamai", "151.101.1.1": "fastly", "13.32.1.1": "cloudfront", "8.8.8.8": "",
	} {
		require.Equal(t, want, lookup.CDNOf(netip.MustParseAddr(ip)), ip)
	}
}
