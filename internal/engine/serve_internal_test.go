package engine

import (
	"context"
	"net/netip"
	"testing"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func ctxFrom(addr string, qtype uint16) *proxy.DNSContext {
	return &proxy.DNSContext{Req: new(dns.Msg).SetQuestion("example.com.", qtype), Addr: netip.MustParseAddrPort(addr)}
}

func TestServeHandle_RefusesPublicSource(t *testing.T) {
	e := New(nil)
	d := ctxFrom("8.8.8.8:5353", dns.TypeA)
	require.NoError(t, e.serveHandle(context.Background(), nil, d))
	require.Equal(t, dns.RcodeRefused, d.Res.Rcode)
}

func TestServeHandle_RateLimit(t *testing.T) {
	e := New(nil)
	refused := 0
	for i := 0; i < 150; i++ {
		d := ctxFrom("192.168.1.9:5353", dns.TypeANY) // ANY: refused without upstream either way
		require.NoError(t, e.serveHandle(context.Background(), nil, d))
		if d.Res.Rcode == dns.RcodeRefused {
			refused++
		}
	}
	require.Equal(t, 150, refused)
	require.True(t, e.limited(netip.MustParseAddr("192.168.1.9")))
	require.False(t, e.limited(netip.MustParseAddr("192.168.1.10")))
}
