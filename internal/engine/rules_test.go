package engine_test

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// zoneUp answers A/AAAA for known names and NXDOMAIN otherwise.
type zoneUp struct {
	fakeUp
	v4, v6 map[string]string
}

func (z *zoneUp) Exchange(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	z.calls.Add(1)
	q := req.Question[0]
	m := new(dns.Msg).SetReply(req)
	_, k4 := z.v4[q.Name]
	_, k6 := z.v6[q.Name]
	if !k4 && !k6 {
		m.Rcode = dns.RcodeNameError
		return m, nil
	}
	hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: 30}
	switch q.Qtype {
	case dns.TypeA:
		if ip, ok := z.v4[q.Name]; ok {
			hdr.Rrtype = dns.TypeA
			m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: net.ParseIP(ip)}}
		}
	case dns.TypeAAAA:
		if ip, ok := z.v6[q.Name]; ok {
			hdr.Rrtype = dns.TypeAAAA
			m.Answer = []dns.RR{&dns.AAAA{Hdr: hdr, AAAA: net.ParseIP(ip)}}
		}
	}
	return m, nil
}

func startRules(t *testing.T, text, mode string, up upstream.Upstream) *engine.Engine {
	t.Helper()
	rs, errs := rules.ParseText(text, []string{"x"})
	require.Empty(t, errs)
	c, err := rules.Compile(rs, nil)
	require.NoError(t, err)
	e := engine.New(nil)
	require.NoError(t, e.Start(context.Background(), engine.Config{
		ListenV4:  netip.MustParseAddrPort("127.0.0.1:0"),
		Upstreams: []upstream.Upstream{up},
		Rules:     func() *rules.Compiled { return c },
		BlockMode: mode,
	}))
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	return e
}

func ask(t *testing.T, e *engine.Engine, name string, qt uint16) *dns.Msg {
	t.Helper()
	c := &dns.Client{Timeout: 2 * time.Second}
	r, _, err := c.Exchange(new(dns.Msg).SetQuestion(name, qt), e.ListenAddr().String())
	require.NoError(t, err)
	return r
}

func TestRules_BlockZero(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "up"}
	e := startRules(t, "ads.com block\n", "zero", up)
	r := ask(t, e, "x.ads.com.", dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, r.Rcode)
	require.Equal(t, "0.0.0.0", r.Answer[0].(*dns.A).A.String())
	require.Equal(t, uint32(60), r.Answer[0].Header().Ttl)
	r = ask(t, e, "ads.com.", dns.TypeAAAA)
	require.Equal(t, "::", r.Answer[0].(*dns.AAAA).AAAA.String())
	r = ask(t, e, "ads.com.", dns.TypeMX)
	require.Equal(t, dns.RcodeSuccess, r.Rcode)
	require.Empty(t, r.Answer)
	require.Zero(t, up.calls.Load())
}

func TestRules_BlockNXDomain(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "up"}
	e := startRules(t, "ads.com block\n", "nxdomain", up)
	r := ask(t, e, "ads.com.", dns.TypeA)
	require.Equal(t, dns.RcodeNameError, r.Rcode)
	require.Zero(t, up.calls.Load())
}

func TestRules_Rewrite(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "up"}
	e := startRules(t, "x.lan ip=192.168.1.1\nboth.lan ip=10.0.0.1 ip=fd00::1\n", "zero", up)
	r := ask(t, e, "x.lan.", dns.TypeA)
	require.Equal(t, "192.168.1.1", r.Answer[0].(*dns.A).A.String())
	r = ask(t, e, "x.lan.", dns.TypeAAAA)
	require.Equal(t, dns.RcodeSuccess, r.Rcode)
	require.Empty(t, r.Answer)
	r = ask(t, e, "both.lan.", dns.TypeAAAA)
	require.Equal(t, "fd00::1", r.Answer[0].(*dns.AAAA).AAAA.String())
	require.Zero(t, up.calls.Load())
}

func TestRules_VerifyBypasses(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "up"}
	e := startRules(t, "ghostline.test block\n~verify block\n", "zero", up)
	e.ExpectVerify("n1")
	r := ask(t, e, "n1.verify.ghostline.test.", dns.TypeA)
	require.Equal(t, "192.0.2.1", r.Answer[0].(*dns.A).A.String())
	require.True(t, e.SawVerify("n1"))
	require.NoError(t, e.SelfTest(context.Background()))
}

func TestRules_AllowAndFragmentIgnored(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "up"}
	e := startRules(t, "a.com allow\nb.com fragment=on\nc.com upstream=x\n", "zero", up)
	for _, n := range []string{"a.com.", "b.com.", "c.com."} {
		r := ask(t, e, n, dns.TypeA)
		require.Equal(t, "192.0.2.80", r.Answer[0].(*dns.A).A.String(), n)
	}
	require.EqualValues(t, 3, up.calls.Load())
}

func TestRules_QueryEventAction(t *testing.T) {
	var got []engine.QueryEvent
	rs, _ := rules.ParseText("ads.com block\n", nil)
	c, _ := rules.Compile(rs, nil)
	e := engine.New(func(ev engine.QueryEvent) { got = append(got, ev) })
	require.NoError(t, e.Start(context.Background(), engine.Config{
		ListenV4: netip.MustParseAddrPort("127.0.0.1:0"), Upstreams: []upstream.Upstream{&fakeUp{ip: net.IPv4(1, 1, 1, 1)}},
		Rules: func() *rules.Compiled { return c },
	}))
	defer e.Stop(context.Background())
	ask(t, e, "ads.com.", dns.TypeA)
	require.Len(t, got, 1)
	require.Equal(t, "blocked", got[0].Action)
}

func TestResolve(t *testing.T) {
	up := &zoneUp{fakeUp: fakeUp{name: "zone"},
		v4: map[string]string{"site.example.": "203.0.113.5", "dual.example.": "203.0.113.6"},
		v6: map[string]string{"dual.example.": "2001:db8::6", "v6only.example.": "2001:db8::7"}}
	e := startRules(t, "ads.com block\n", "zero", up)
	ctx := context.Background()
	ips, err := e.Resolve(ctx, "site.example")
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("203.0.113.5")}, ips)
	ips, err = e.Resolve(ctx, "dual.example")
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("203.0.113.6"), netip.MustParseAddr("2001:db8::6")}, ips)
	ips, err = e.Resolve(ctx, "v6only.example")
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("2001:db8::7")}, ips)
	ips, err = e.Resolve(ctx, "ads.com")
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("0.0.0.0"), ips[0])
	_, err = e.Resolve(ctx, "missing.example")
	require.ErrorIs(t, err, engine.ErrNoAddress)
}

func TestResolve_NotRunning(t *testing.T) {
	_, err := engine.New(nil).Resolve(context.Background(), "x.com")
	require.Error(t, err)
}
