package app

import (
	"context"
	"testing"

	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func TestConnect_EngineGetsRulesAndBlockMode(t *testing.T) {
	h := newHarness(t)
	holder := &rules.Holder{}
	h.o.d.Rules = holder.Load
	h.settings.DNSBlockMode = "nxdomain"
	require.NoError(t, h.o.Connect(context.Background()))
	require.NotNil(t, h.eng.lastCfg.Rules)
	require.Equal(t, "nxdomain", h.eng.lastCfg.BlockMode)
}

func TestBus_ProxyConnsOnlyWhenQueryLogOn(t *testing.T) {
	sh := newSvc(t)
	sh.bus.ProxyConn(proxy.ConnEvent{Target: "a.com:443"})
	require.Empty(t, sh.bus.ProxyConns())
	require.Zero(t, sh.em.count(EventProxyConn))

	sh.svc.SetQueryLog(true)
	sh.bus.ProxyConn(proxy.ConnEvent{Target: "b.com:443"})
	require.Len(t, sh.bus.ProxyConns(), 1)
	require.Equal(t, 1, sh.em.count(EventProxyConn))

	sh.svc.SetQueryLog(false)
	require.Empty(t, sh.bus.ProxyConns())
}
