package lookup_test

import (
	"context"
	"flag"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/lookup"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files")

func startDNS(t *testing.T, h dns.HandlerFunc) upstream.Upstream {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &dns.Server{PacketConn: pc, Handler: h}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	u, err := upstream.AddressToUpstream("udp://"+pc.LocalAddr().String(), &upstream.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = u.Close() })
	return u
}

func rr(t *testing.T, s string) dns.RR {
	r, err := dns.NewRR(s)
	require.NoError(t, err)
	return r
}

var src = lookup.Source{Kind: "server", Ref: "x", Label: "Test"}

func TestQuery_Types(t *testing.T) {
	answers := map[string]string{
		"A":     "example.com. 300 IN A 93.184.216.34",
		"AAAA":  "example.com. 300 IN AAAA 2606:2800:220:1::1",
		"CNAME": "example.com. 300 IN CNAME other.example.",
		"MX":    "example.com. 300 IN MX 10 mail.example.com.",
		"TXT":   `example.com. 300 IN TXT "v=spf1 -all"`,
		"NS":    "example.com. 300 IN NS a.iana-servers.net.",
		"SOA":   "example.com. 300 IN SOA ns.icann.org. noc.dns.icann.org. 1 7200 3600 1209600 3600",
		"HTTPS": "example.com. 300 IN HTTPS 1 . alpn=h2",
		"CAA":   `example.com. 300 IN CAA 0 issue "letsencrypt.org"`,
		"PTR":   "34.216.184.93.in-addr.arpa. 300 IN PTR example.com.",
	}
	u := startDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg).SetReply(req)
		m.Answer = []dns.RR{rr(t, answers[dns.TypeToString[req.Question[0].Qtype]])}
		_ = w.WriteMsg(m)
	})
	for _, typ := range lookup.Types {
		name := "example.com"
		if typ == "PTR" {
			name = "93.184.216.34"
		}
		q, err := lookup.QueryName(name, typ)
		require.NoError(t, err)
		a := lookup.Query(context.Background(), u, src, q, typ)
		require.True(t, a.OK, typ)
		require.Equal(t, "NOERROR", a.Rcode)
		require.Len(t, a.Records, 1, typ)
		require.Equal(t, typ, a.Records[0].Type)
		require.Equal(t, uint32(300), a.Records[0].TTL)
		require.NotEmpty(t, a.Records[0].Data)
		require.Contains(t, a.Dig, ";; ANSWER SECTION:")
	}
	a := lookup.Query(context.Background(), u, src, "example.com", "A")
	require.Equal(t, "93.184.216.34", a.Records[0].Data)
}

func TestQuery_Flags(t *testing.T) {
	var sawDO bool
	u := startDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		sawDO = req.IsEdns0() != nil && req.IsEdns0().Do()
		m := new(dns.Msg).SetRcode(req, dns.RcodeNameError)
		m.AuthenticatedData, m.RecursionAvailable = true, true
		_ = w.WriteMsg(m)
	})
	a := lookup.Query(context.Background(), u, src, "nope.example", "A")
	require.True(t, sawDO)
	require.True(t, a.OK)
	require.Equal(t, "NXDOMAIN", a.Rcode)
	require.True(t, a.AD)
	require.True(t, a.RA)
	require.False(t, a.TC)
	require.Equal(t, src, a.Source)
}

func TestQuery_Timeout(t *testing.T) {
	u := startDNS(t, func(w dns.ResponseWriter, req *dns.Msg) { time.Sleep(6 * time.Second) })
	start := time.Now()
	a := lookup.Query(context.Background(), u, src, "example.com", "A")
	require.Less(t, time.Since(start), 5500*time.Millisecond)
	require.False(t, a.OK)
	require.Equal(t, "timeout", a.Error)
}

func TestQueryName_Normalises(t *testing.T) {
	ok := map[[2]string]string{
		{"Example.COM.", "A"}:          "example.com",
		{"https://x.com/a?b", "A"}:     "x.com",
		{"http://X.com:8080/", "AAAA"}: "x.com",
		{"bücher.de", "A"}:             "xn--bcher-kva.de",
		{"8.8.4.4", "PTR"}:             "4.4.8.8.in-addr.arpa",
		{"2001:db8::1", "PTR"}:         "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa",
		{"4.4.8.8.in-addr.arpa", "PTR"}: "4.4.8.8.in-addr.arpa",
		{" _dmarc.example.com ", "TXT"}: "_dmarc.example.com",
	}
	for in, want := range ok {
		got, err := lookup.QueryName(in[0], in[1])
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	long := ""
	for len(long) < 254 {
		long += "a."
	}
	for _, in := range [][2]string{{"1.2.3.4", "A"}, {"", "A"}, {"a b", "A"}, {long + "com", "A"}, {"example.com", "ANY"}, {"a..b", "A"}} {
		_, err := lookup.QueryName(in[0], in[1])
		require.ErrorIs(t, err, lookup.ErrBadName, in)
	}
}

func TestDig_Golden(t *testing.T) {
	m := new(dns.Msg).SetQuestion("example.com.", dns.TypeA)
	m.Id = 4242
	m.Response, m.RecursionAvailable = true, true
	m.Answer = []dns.RR{rr(t, "example.com. 300 IN A 93.184.216.34")}
	got := lookup.Dig(m, 23*time.Millisecond, src)
	path := filepath.Join("testdata", "dig_a.golden")
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
	require.Contains(t, got, ";; Query time: 23 msec")
	require.Contains(t, got, ";; SERVER: Test")
}
