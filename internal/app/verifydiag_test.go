package app

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

func verifyLeakLog(t *testing.T, h *harness) map[string]any {
	t.Helper()
	for _, e := range h.sink.events() {
		if e.Code == CodeVerifyLeak {
			return e.Params
		}
	}
	t.Fatalf("no %s log event", CodeVerifyLeak)
	return nil
}

func TestVerifyLeak_NamesUnmanagedAndDriftedAdapters(t *testing.T) {
	h := newHarness(t)
	h.res.err = &net.DNSError{Err: "no such host", Name: "x.verify.ghostline.test", IsNotFound: true}
	h.dns.report = []sysdns.AdapterDNS{
		{Adapter: sysdns.Adapter{GUID: "{A}", Alias: "Wi-Fi", IfType: 71}, IPv4: []string{"192.168.1.1"}},
		{Adapter: sysdns.Adapter{GUID: "{P}", Alias: "VNPT", IfType: 23}, IPv4: []string{"203.162.4.191"}},
		{Adapter: sysdns.Adapter{GUID: "{H}", Alias: "vEthernet", IfType: 6}},
	}
	require.Error(t, h.o.Connect(context.Background()))

	p := h.o.Snapshot().Error.Params
	require.Equal(t, verifyNXDomain, p["reason"])
	require.Equal(t, "no such host", p["detail"])
	require.Equal(t, "VNPT [ppp] v4=203.162.4.191", p["others"])
	require.Equal(t, "Wi-Fi [wifi] v4=192.168.1.1", p["drifted"])
	require.Equal(t, "Wi-Fi, VNPT", p["adapters"])
	require.Equal(t, p, verifyLeakLog(t, h))
}

func TestVerifyLeak_Reasons(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(h *harness)
		reason string
	}{
		{"timeout", func(h *harness) { h.res.err = &net.DNSError{Err: "i/o timeout", IsTimeout: true} }, verifyTimeout},
		{"other error", func(h *harness) { h.res.err = &net.DNSError{Err: "server misbehaving"} }, verifyLookupError},
		{"wrong answer", func(h *harness) { h.res.ips = []netip.Addr{netip.MustParseAddr("1.2.3.4")} }, verifyWrongAnswer},
		{"engine never saw it", func(h *harness) { h.eng.saw = false }, verifyNotSeen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.setup(h)
			require.Error(t, h.o.Connect(context.Background()))
			sn := h.o.Snapshot()
			require.Equal(t, CodeVerifyLeak, sn.Error.Code)
			require.Equal(t, c.reason, sn.Error.Params["reason"])
			require.NotContains(t, sn.Error.Params, "adapters")
		})
	}
}
