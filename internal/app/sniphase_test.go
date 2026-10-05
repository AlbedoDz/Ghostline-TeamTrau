package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type sniHarness struct {
	*dnsHarness
	mu      sync.Mutex
	mitm    mitm.LeafSource
	sets    int
	selfErr error
	rules   *rules.Compiled
}

func compileRules(t *testing.T, text string) *rules.Compiled {
	t.Helper()
	rs, errs := rules.ParseText(text, nil)
	require.Empty(t, errs)
	c, err := rules.Compile(rs, nil)
	require.NoError(t, err)
	return c
}

func newSNIHarness(t *testing.T) *sniHarness {
	h := &sniHarness{dnsHarness: newDNSHarness(t)}
	h.settings.DNSServer.Enabled = false
	h.settings.FakeSNI = sniOn()
	h.rules = compileRules(t, "youtube.com sni=www.google.com\n*.googlevideo.com sni=www.google.com")
	h.o.d.Rules = func() *rules.Compiled { h.mu.Lock(); defer h.mu.Unlock(); return h.rules }
	h.o.d.SetMITM = func(l mitm.LeafSource) {
		h.mu.Lock()
		h.mitm = l
		h.sets++
		h.mu.Unlock()
		name := "mitm.set"
		if l == nil {
			name = "mitm.clear"
		}
		_ = h.r.add(name)
	}
	h.o.d.MITMSelfTest = func(context.Context) error { _ = h.r.add("mitm.selftest"); return h.selfErr }
	return h
}

func sniOn() store.FakeSNISettings {
	return store.FakeSNISettings{Enabled: true, AckVersion: FakeSNIWarningVersion}
}

func (h *sniHarness) active() mitm.LeafSource { h.mu.Lock(); defer h.mu.Unlock(); return h.mitm }

func (h *sniHarness) setRules(t *testing.T, text string) {
	c := compileRules(t, text)
	h.mu.Lock()
	h.rules = c
	h.mu.Unlock()
}

func TestPhaseS_StepsInOrder(t *testing.T) {
	h := newSNIHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	requireOrder(t, h.r.list(), "proxy.start", "certs.install", "mitm.set", "mitm.selftest")
	is, ok := h.active().(*certs.Issuer)
	require.True(t, ok)
	require.Equal(t, []string{"googlevideo.com", "youtube.com"}, is.CA().Cert.PermittedDNSDomains)
	sn := h.o.Snapshot()
	require.Equal(t, StatusProtected, sn.Status)
	require.True(t, sn.FakeSNI.Active)
	require.Equal(t, 2, sn.FakeSNI.Domains)
	require.Equal(t, is.CA().Thumbprint(), sn.FakeSNI.Thumbprint)
	st, _ := h.states.Load()
	require.Equal(t, []string{is.CA().Thumbprint()}, st.Certs.Session)
	require.True(t, h.certs.store.Has(is.CA().Thumbprint()))
}

func TestPhaseS_Skipped(t *testing.T) {
	cases := map[string]func(h *sniHarness){
		"disabled":   func(h *sniHarness) { h.settings.FakeSNI.Enabled = false },
		"not acked":  func(h *sniHarness) { h.settings.FakeSNI.AckVersion = 0 },
		"no domains": func(h *sniHarness) { h.setRules(t, "ads.com block") },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			h := newSNIHarness(t)
			mod(h)
			require.NoError(t, h.o.Connect(context.Background()))
			require.NotContains(t, h.r.list(), "certs.install")
			require.Nil(t, h.active())
			sn := h.o.Snapshot()
			require.Equal(t, StatusProtected, sn.Status)
			require.False(t, sn.FakeSNI.Active)
		})
	}
}

func TestPhaseS_NeedsProxy(t *testing.T) {
	h := newSNIHarness(t)
	h.settings.Proxy.Enabled = false
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.True(t, sn.FakeSNI.NeedsProxy)
	require.False(t, sn.FakeSNI.Active)
	require.Equal(t, StatusProtected, sn.Status, "needing the proxy is not a degradation")
	require.NotContains(t, h.r.list(), "certs.install")
}

func TestPhaseS_InstallFails(t *testing.T) {
	h := newSNIHarness(t)
	h.r.fail["certs.install"] = true
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, StatusDegraded, sn.Status)
	require.Contains(t, sn.Reasons, reasonFakeSNI)
	require.Equal(t, CodeCertInstallFailed, sn.FakeSNI.Error.Code)
	require.Nil(t, h.active())
	st, _ := h.states.Load()
	require.Nil(t, st.Certs, "thumbprint removed when nothing was installed")
}

func TestPhaseS_SelfTestFails(t *testing.T) {
	h := newSNIHarness(t)
	h.selfErr = errors.New("handshake failed")
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, CodeFakeSNISelfTest, sn.FakeSNI.Error.Code)
	require.Nil(t, h.active())
	requireOrder(t, h.r.list(), "mitm.selftest", "mitm.clear", "certs.remove")
	st, _ := h.states.Load()
	require.Nil(t, st.Certs)
	left, _ := h.certs.store.List(certs.SessionPrefix)
	require.Empty(t, left)
}

func TestPhaseS_RemoveFailsKeepsThumbprint(t *testing.T) {
	h := newSNIHarness(t)
	h.selfErr = errors.New("handshake failed")
	h.r.fail["certs.remove"] = true
	require.NoError(t, h.o.Connect(context.Background()))
	st, _ := h.states.Load()
	require.NotNil(t, st.Certs, "a CA still in Root must stay recorded for recovery")
	require.Len(t, st.Certs.Session, 1)
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeCertRemoveFailed)
}

func TestPhaseS_TooMany(t *testing.T) {
	h := newSNIHarness(t)
	text := ""
	for i := 0; i < 1001; i++ {
		text += fmt.Sprintf("d%d.com sni=x.com\n", i)
	}
	h.setRules(t, text)
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, CodeFakeSNITooMany, sn.FakeSNI.Error.Code)
	require.Equal(t, StatusDegraded, sn.Status)
	require.NotContains(t, h.r.list(), "certs.install")
}

func TestDisconnect_SNIFirst(t *testing.T) {
	h := newSNIHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.Disconnect(context.Background()))
	requireOrder(t, h.r.list(), "mitm.clear", "certs.remove", "sysproxy.restore", "proxy.stop", "dns.restore")
	st, _ := h.states.Load()
	require.Nil(t, st.Certs)
	left, _ := h.certs.store.List(certs.SessionPrefix)
	require.Empty(t, left)
}

func TestReapplyProxy_RerunsSNI(t *testing.T) {
	h := newSNIHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	first := h.active()
	require.NoError(t, h.o.ReapplyProxy(context.Background()))
	require.NotNil(t, h.active())
	require.NotSame(t, first, h.active())
	left, _ := h.certs.store.List(certs.SessionPrefix)
	require.Len(t, left, 1, "the old session CA is gone")
}
