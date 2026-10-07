package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func after(calls []string, name string) []string {
	i := indexOf(calls, name)
	if i < 0 {
		return nil
	}
	return calls[i+1:]
}

// C1: a failed apply whose restore also fails must keep every safety layer.
func TestConnect_ApplyFailureWithRestoreFailureKeepsSafetyNet(t *testing.T) {
	h := newHarness(t)
	h.r.fail["dns.apply"] = true
	h.dns.restoreErr = true
	require.Error(t, h.o.Connect(context.Background()))
	require.Equal(t, []string{"dns.restore", "dns.flush"}, after(h.r.list(), "dns.apply"))
	sn := h.o.Snapshot()
	require.Equal(t, StatusError, sn.Status)
	require.Equal(t, CodeRestoreFailed, sn.Error.Code)
	require.Len(t, sn.Warnings, 1)
	require.Equal(t, CodeRestoreFailed, sn.Warnings[0].Code)
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase, "later safety layers must still see the snapshot")
}

// C1: same for a verify failure whose rollback restore fails.
func TestConnect_VerifyLeakWithRestoreFailureKeepsSafetyNet(t *testing.T) {
	h := newHarness(t)
	h.r.fail["engine.saw"] = true
	h.dns.restoreErr = true
	require.Error(t, h.o.Connect(context.Background()))
	require.Equal(t, []string{"dns.report", "dns.restore", "dns.flush"}, after(h.r.list(), "engine.saw"))
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
}

// C1: after such a halt, Disconnect retries the restore and cleans up.
func TestDisconnect_AfterHaltRetriesRestoreAndCleansUp(t *testing.T) {
	h := newHarness(t)
	h.r.fail["dns.apply"] = true
	h.dns.restoreErr = true
	require.Error(t, h.o.Connect(context.Background()))
	h.dns.restoreErr = false
	n := len(h.r.list())
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Equal(t, []string{"dns.restore", "dns.flush", "engine.stop", "state.clean", "safety.watchdog.stop", "safety.task.delete"}, h.r.list()[n:])
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
	require.Empty(t, h.o.Snapshot().Warnings)
}

// I1: connecting again after a halt restores the old snapshot first instead
// of saving 127.0.0.1 as the "original" DNS.
func TestConnect_DirtyOwnStateRestoresBeforeNewSnapshot(t *testing.T) {
	h := newHarness(t)
	h.r.fail["dns.apply"] = true
	h.dns.restoreErr = true
	require.Error(t, h.o.Connect(context.Background()))
	delete(h.r.fail, "dns.apply")
	h.dns.restoreErr = false
	n := len(h.r.list())
	require.NoError(t, h.o.Connect(context.Background()))
	calls := h.r.list()[n:]
	require.GreaterOrEqual(t, indexOf(calls, "dns.restore"), 0, calls)
	require.Less(t, indexOf(calls, "dns.restore"), indexOf(calls, "dns.snapshot"), calls)
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
}

// I1: if the old snapshot still cannot be restored, Connect refuses.
func TestConnect_DirtyOwnStateRestoreFailureRefusesToSnapshot(t *testing.T) {
	h := newHarness(t)
	h.r.fail["dns.apply"] = true
	h.dns.restoreErr = true
	require.Error(t, h.o.Connect(context.Background()))
	delete(h.r.fail, "dns.apply")
	n := len(h.r.list())
	require.Error(t, h.o.Connect(context.Background()))
	require.NotContains(t, h.r.list()[n:], "dns.snapshot")
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
}

// I2: a failed disconnect keeps the engine; a retry finishes the job.
func TestDisconnect_RetryAfterRestoreFailure(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dns.restoreErr = true
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
	h.dns.restoreErr = false
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
	require.Empty(t, h.o.Snapshot().Warnings)
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseClean, st.Phase)
}

// I6 (orchestrator side): RestoreNow goes through the orchestrator lock and
// reuses the in-memory snapshot when connected.
func TestRestoreNow_WhileConnectedDisconnects(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	fallbackCalled := false
	require.NoError(t, h.o.RestoreNow(context.Background(), func() error { fallbackCalled = true; return nil }))
	require.False(t, fallbackCalled)
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
}

func TestRestoreNow_WhenIdleUsesFallback(t *testing.T) {
	h := newHarness(t)
	h.o.AddWarning(AppError{Code: CodeRestoreFailed})
	called := false
	require.NoError(t, h.o.RestoreNow(context.Background(), func() error { called = true; return nil }))
	require.True(t, called)
	require.Empty(t, h.o.Snapshot().Warnings)
}

type freshPicker struct {
	*fPicker
	excluded []string
}

func (f *freshPicker) PickFresh(_ context.Context, exclude []string) ([]model.Server, error) {
	f.excluded = exclude
	_ = f.r.add("pick.fresh")
	return []model.Server{{ID: "q9", Name: "Quad9"}}, nil
}

// I3: healing bypasses the cache and excludes the servers that are failing.
func TestHeal_UsesFreshPickExcludingCurrentServers(t *testing.T) {
	h := newHarness(t)
	fp := &freshPicker{fPicker: h.pick}
	h.o.d.Picker = fp
	require.NoError(t, h.o.Connect(context.Background()))
	h.eng.mu.Lock()
	h.eng.selfE = errors.New("dead")
	h.eng.mu.Unlock()
	h.o.OnResume(context.Background())
	require.Equal(t, []string{"cf"}, fp.excluded)
	require.Equal(t, []string{"Quad9"}, h.o.Snapshot().Servers)
}

// I4: a failed hot swap must not leave DNS pointing at a dead engine.
func TestHeal_SwapFailureRestoresDNSAndErrors(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.r.fail["engine.swap"] = true
	h.eng.mu.Lock()
	h.eng.selfE = errors.New("dead")
	h.eng.mu.Unlock()
	h.o.OnResume(context.Background())
	tail := after(h.r.list(), "engine.swap")
	require.Equal(t, []string{"dns.restore", "dns.flush", "engine.stop", "state.clean", "safety.watchdog.stop", "safety.task.delete"}, tail)
	sn := h.o.Snapshot()
	require.Equal(t, StatusError, sn.Status)
	require.Equal(t, CodeEngineSelfTest, sn.Error.Code)
}

// I5: startup recovery outcomes become visible warnings.
func TestStartupWarnings(t *testing.T) {
	require.Empty(t, StartupWarnings(watchdog.NothingToDo, nil))
	require.Empty(t, StartupWarnings(watchdog.Restored, nil))
	w := StartupWarnings(watchdog.RestoredFromCorrupt, nil)
	require.Len(t, w, 1)
	require.Equal(t, CodeStateReset, w[0].Code)
	w = StartupWarnings(watchdog.Restored, errors.New("netsh failed"))
	require.Len(t, w, 1)
	require.Equal(t, CodeRestoreFailed, w[0].Code)
}

var _ = time.Second
