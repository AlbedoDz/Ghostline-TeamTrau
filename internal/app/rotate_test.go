package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/stretchr/testify/require"
)

// manualTimers replaces time.AfterFunc: tests fire the pending call.
type manualTimers struct {
	mu      sync.Mutex
	pending func()
	started int
	stopped int
}

func (m *manualTimers) afterFunc(_ time.Duration, f func()) func() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started++
	m.pending = f
	return func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stopped++
		m.pending = nil
		return true
	}
}

func (m *manualTimers) fire() {
	m.mu.Lock()
	f := m.pending
	m.pending = nil
	m.mu.Unlock()
	if f != nil {
		f()
	}
}

func newRotateHarness(t *testing.T) (*sniHarness, *manualTimers) {
	h := newSNIHarness(t)
	mt := &manualTimers{}
	h.o.d.AfterFunc = mt.afterFunc
	require.NoError(t, h.o.Connect(context.Background()))
	return h, mt
}

func (h *sniHarness) issuer() *certs.Issuer { return h.active().(*certs.Issuer) }

func TestRotate_OnDomainChange(t *testing.T) {
	h, mt := newRotateHarness(t)
	old := h.issuer().CA().Thumbprint()
	h.setRules(t, "youtube.com sni=www.google.com\nvercel.com sni=nextjs.org")
	h.o.OnRulesCompiled()
	mt.fire()
	cur := h.issuer().CA()
	require.NotEqual(t, old, cur.Thumbprint())
	require.Equal(t, []string{"vercel.com", "youtube.com"}, cur.Cert.PermittedDNSDomains)
	require.False(t, h.certs.store.Has(old))
	require.True(t, h.certs.store.Has(cur.Thumbprint()))
	st, _ := h.states.Load()
	require.Equal(t, []string{cur.Thumbprint()}, st.Certs.Session)
	requireOrder(t, h.r.list(), "certs.install", "mitm.set", "certs.install", "mitm.set", "certs.remove")
	require.Equal(t, 2, h.o.Snapshot().FakeSNI.Domains)
}

func TestRotate_SameDomainsNoop(t *testing.T) {
	h, mt := newRotateHarness(t)
	old := h.issuer()
	h.o.OnRulesCompiled()
	mt.fire()
	require.Same(t, old, h.issuer())
}

func TestRotate_Debounced(t *testing.T) {
	h, mt := newRotateHarness(t)
	h.setRules(t, "vercel.com sni=nextjs.org")
	for i := 0; i < 5; i++ {
		h.o.OnRulesCompiled()
	}
	require.Equal(t, 5, mt.started)
	require.Equal(t, 4, mt.stopped)
	before := h.sets
	mt.fire()
	require.Equal(t, before+1, h.sets, "one rotation for the burst")
}

func TestRotate_InstallFailsKeepsOld(t *testing.T) {
	h, mt := newRotateHarness(t)
	old := h.issuer()
	h.setRules(t, "vercel.com sni=nextjs.org")
	h.r.fail["certs.install"] = true
	h.o.OnRulesCompiled()
	mt.fire()
	require.Same(t, old, h.issuer())
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeCertInstallFailed)
	st, _ := h.states.Load()
	require.Equal(t, []string{old.CA().Thumbprint()}, st.Certs.Session)
}

func TestRotate_RemoveOldFails(t *testing.T) {
	h, mt := newRotateHarness(t)
	old := h.issuer().CA().Thumbprint()
	h.setRules(t, "vercel.com sni=nextjs.org")
	h.r.fail["certs.remove"] = true
	h.o.OnRulesCompiled()
	mt.fire()
	require.NotEqual(t, old, h.issuer().CA().Thumbprint())
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeCertRemoveFailed)
	st, _ := h.states.Load()
	require.Contains(t, st.Certs.Session, old, "an old CA still in Root stays recorded")
}

func TestRotate_NoDomainsStopsFakeSNI(t *testing.T) {
	h, mt := newRotateHarness(t)
	h.setRules(t, "ads.com block")
	h.o.OnRulesCompiled()
	mt.fire()
	require.Nil(t, h.active())
	require.False(t, h.o.Snapshot().FakeSNI.Active)
	st, _ := h.states.Load()
	require.Nil(t, st.Certs)
}

func TestRotate_FirstSNIRuleStartsFakeSNI(t *testing.T) {
	h := newSNIHarness(t)
	mt := &manualTimers{}
	h.o.d.AfterFunc = mt.afterFunc
	h.setRules(t, "ads.com block")
	require.NoError(t, h.o.Connect(context.Background()))
	require.Nil(t, h.active())
	h.setRules(t, "youtube.com sni=www.google.com")
	h.o.OnRulesCompiled()
	mt.fire()
	require.NotNil(t, h.active())
}

func TestRotate_NearExpiry(t *testing.T) {
	h, _ := newRotateHarness(t)
	old := h.issuer()
	now := time.Now()
	h.o.d.Now = func() time.Time { return now.Add(26 * 24 * time.Hour) }
	h.o.checkSNIHealth(context.Background())
	require.Same(t, old, h.issuer(), "4 days left: keep")
	h.o.d.Now = func() time.Time { return now.Add(27*24*time.Hour + 2*time.Hour) }
	h.o.checkSNIHealth(context.Background())
	require.NotSame(t, old, h.issuer())
}

func TestRotate_IgnoredWhenDisconnected(t *testing.T) {
	h := newSNIHarness(t)
	mt := &manualTimers{}
	h.o.d.AfterFunc = mt.afterFunc
	h.o.OnRulesCompiled()
	mt.fire()
	require.NotContains(t, h.r.list(), "certs.install")
}
