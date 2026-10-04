package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

// Minor: Degraded returns to Protected once upstreams answer again.
func TestHealth_DegradedRecoversWithoutSwap(t *testing.T) {
	h := newHarness(t)
	ticks := make(chan time.Time)
	h.o.d.Ticker = func(time.Duration) (<-chan time.Time, func()) { return ticks, func() {} }
	require.NoError(t, h.o.Connect(context.Background()))
	h.o.update(func(s *Snapshot) { s.Status = StatusDegraded })
	ticks <- time.Now()
	require.Eventually(t, func() bool { return h.o.Snapshot().Status == StatusProtected }, time.Second, 5*time.Millisecond)
	require.NotContains(t, h.r.list(), "engine.swap")
	require.NoError(t, h.o.Disconnect(context.Background()))
}

// Minor: a network change applies IPv6 only if the engine listens on v6.
func TestNetworkChange_UsesConnectTimeIPv6Decision(t *testing.T) {
	h := newHarness(t)
	h.sys.noV6 = true
	require.NoError(t, h.o.Connect(context.Background()))
	h.sys.noV6 = false // IPv6 came up later; the engine still has no v6 listener
	h.dns.adapters = append(h.dns.adapters, sysdns.Adapter{GUID: "{B}", Alias: "Ethernet", IfType: 6, Up: true, HasGateway: true})
	h.o.OnNetworkChange(context.Background())
	require.Contains(t, h.r.list(), "dns.apply:{B}")
	require.False(t, h.dns.lastV6)
}

// Minor: state.json records a running GoodbyeDPI so the watchdog stops it.
func TestDPIStateIsPersisted(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	st, _ := h.states.Load()
	require.True(t, st.DPI.Running)
	require.Equal(t, 99, st.DPI.PID)
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), false))
	st, _ = h.states.Load()
	require.False(t, st.DPI.Running)
}

// Minor: Disconnect cancels a running autotune instead of waiting for it.
func TestDisconnect_CancelsAutotune(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.prober.block = true
	done := make(chan error, 1)
	go func() { done <- h.o.Autotune(context.Background(), nil) }()
	require.Eventually(t, func() bool { return h.dpi.running }, time.Second, 5*time.Millisecond)
	start := time.Now()
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Less(t, time.Since(start), time.Second)
	require.Error(t, <-done)
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
}

// Minor: autotune needs a connection (it probes through Ghostline's DNS).
func TestAutotune_RequiresConnection(t *testing.T) {
	h := newHarness(t)
	err := h.o.Autotune(context.Background(), nil)
	var ae *AppError
	require.True(t, errors.As(err, &ae))
	require.Equal(t, CodeNotConnected, ae.Code)
	require.NotContains(t, h.r.list(), "dpi.start")
}

// Minor: a successful manual restore also removes the logon recovery task.
func TestRestoreNow_FallbackDeletesRecoveryTask(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.RestoreNow(context.Background(), func() error { return nil }))
	require.Contains(t, h.r.list(), "safety.task.delete")
}

var _ = engine.Stats{}
