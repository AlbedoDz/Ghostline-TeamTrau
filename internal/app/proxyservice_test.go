package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func fakeProtect(s string) (string, error) { return "enc:" + strings.ToUpper(s), nil }

func TestSaveUpstream_PasswordEncryptedAndKept(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.x.Protect = fakeProtect
	require.NoError(t, rh.svc.SaveUpstreamProxy(store.UpstreamProxy{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9050", User: "u"}, "pw"))
	ups := rh.svc.GetSettings().Proxy.Upstreams
	require.Len(t, ups, 1)
	require.Equal(t, "enc:PW", ups[0].PassEnc)
	// Saving again with an empty password keeps the stored one.
	require.NoError(t, rh.svc.SaveUpstreamProxy(store.UpstreamProxy{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9150", User: "u"}, ""))
	ups = rh.svc.GetSettings().Proxy.Upstreams
	require.Len(t, ups, 1)
	require.Equal(t, "127.0.0.1:9150", ups[0].Addr)
	require.Equal(t, "enc:PW", ups[0].PassEnc)
	// Invalid upstreams are rejected.
	require.Error(t, rh.svc.SaveUpstreamProxy(store.UpstreamProxy{ID: "Bad ID", Type: "socks5", Addr: "1.2.3.4:1"}, ""))
}

func TestDeleteUpstream_ReferencedRefused(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.x.Protect = fakeProtect
	require.NoError(t, rh.svc.SaveUpstreamProxy(store.UpstreamProxy{ID: "corp", Type: "http", Addr: "10.0.0.1:3128"}, ""))
	require.Empty(t, rh.svc.SaveRulesText("x.com upstream=corp\n"))
	require.Error(t, rh.svc.DeleteUpstreamProxy("corp"))
	require.Empty(t, rh.svc.SaveRulesTable([]rules.Rule{}))
	_, err := rh.svc.AddList(lists.List{Name: "L", Source: "url", URL: "https://x/l", Action: "upstream=corp", UpdateHours: 0})
	require.NoError(t, err)
	require.Error(t, rh.svc.DeleteUpstreamProxy("corp"))
}

func TestSaveSettings_ProxyChangeReapplies(t *testing.T) {
	ph := newProxyHarness(t, true)
	sh := newSvc(t)
	// Reuse the proxy harness's orchestrator inside a service.
	sh.svc.o = ph.o
	box := NewSettingsBox(sh.paths.Settings, ph.settings)
	ph.o.d.Settings = box.Get
	sh.svc.x.Settings = box
	require.NoError(t, ph.o.Connect(context.Background()))
	starts := func() int {
		n := 0
		for _, c := range ph.r.list() {
			if c == "proxy.start" {
				n++
			}
		}
		return n
	}
	require.Equal(t, 1, starts())

	s := box.Get()
	s.Proxy.Fragment.Chunks = 9
	require.NoError(t, sh.svc.SaveSettings(s))
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, starts())

	s.Proxy.Port = 9090
	require.NoError(t, sh.svc.SaveSettings(s))
	require.Eventually(t, func() bool { return starts() == 2 }, 2*time.Second, 10*time.Millisecond)

	s.Proxy.Port = 53
	require.Error(t, sh.svc.SaveSettings(s))
}

func TestFragCacheBindings(t *testing.T) {
	rh := newRulesSvc(t)
	fc, err := store.LoadFragCache(rh.paths.FragCache, time.Now())
	require.NoError(t, err)
	rh.svc.x.FragCache = fc
	rh.svc.x.NetKey = func() string { return "net1" }
	fc.Add("net1", "youtube.com", time.Now().Add(time.Hour))
	fc.Add("net1", "x.com", time.Now().Add(time.Hour))
	require.Equal(t, []string{"x.com", "youtube.com"}, rh.svc.GetFragCache())
	require.NoError(t, rh.svc.ClearFragCache("x.com"))
	require.Equal(t, []string{"youtube.com"}, rh.svc.GetFragCache())
	require.NoError(t, rh.svc.ClearFragCache(""))
	require.Empty(t, rh.svc.GetFragCache())
}

func TestTestUpstreamProxy(t *testing.T) {
	rh := newRulesSvc(t)
	var got string
	rh.svc.x.TestUpstream = func(_ context.Context, id string) error { got = id; return errors.New("unreachable") }
	require.Error(t, rh.svc.TestUpstreamProxy("tor"))
	require.Equal(t, "tor", got)
}

func TestGetQR(t *testing.T) {
	rh := newRulesSvc(t)
	m, err := rh.svc.GetQR("192.168.1.5:8080")
	require.NoError(t, err)
	require.Len(t, m, 25)
}

func TestAskOverride_AnswerAndTimeout(t *testing.T) {
	rh := newRulesSvc(t)
	go func() {
		require.Eventually(t, func() bool {
			for _, w := range rh.o.Snapshot().Warnings {
				if w.Code == CodeSysProxyExisting {
					return true
				}
			}
			return false
		}, 2*time.Second, 10*time.Millisecond)
		rh.svc.AnswerSysProxyOverride(true)
	}()
	require.True(t, AskOverride(rh.svc, "10.0.0.1:3128", "", 2*time.Second))
	require.NotContains(t, warningCodes(rh.o.Snapshot()), CodeSysProxyExisting)
	require.False(t, AskOverride(rh.svc, "10.0.0.1:3128", "", 50*time.Millisecond))
}
