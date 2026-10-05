package watchdog_test

import (
	"errors"
	"os"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

// certHooks records cert, rule, sysproxy, DNS and sweep calls in order.
func certHooks(d watchdog.Deps, log *[]string, removeErr error) watchdog.Deps {
	d = withProxyHooks(d, log, nil)
	d.DeleteRule = func(name string) error { *log = append(*log, "rule:"+name); return nil }
	d.RemoveCert = func(thumb string) error {
		*log = append(*log, "cert:"+thumb)
		return removeErr
	}
	d.SweepSession = func(keep []string) error {
		*log = append(*log, "sweep")
		return nil
	}
	return d
}

func dirtyWith2B() *store.State {
	st := dirtyWithProxy()
	st.AddFirewallRule("Ghostline DNS (TCP)")
	st.AddSessionCert("aa")
	st.AddSessionCert("bb")
	return st
}

func TestRecover_OrderSessionCertFirst(t *testing.T) {
	var log []string
	d, _, stops := setup(t, false, dirtyWith2B())
	d = certHooks(d, &log, nil)
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.Restored, out)
	require.Equal(t, []string{"cert:aa", "cert:bb", "sysproxy:127.0.0.1:8080", "rule:Ghostline Proxy", "rule:Ghostline DNS (TCP)", "dns", "sweep"}, log)
	require.Equal(t, 1, *stops)
	st, _ := d.States.Load()
	require.Equal(t, store.CleanState(), st)
}

func TestRecover_SessionCertBeforeInstall(t *testing.T) {
	// Crash between recording the thumbprint and installing the CA: the
	// store has nothing to remove, which RemoveCert treats as success.
	var log []string
	st := dirty()
	st.AddSessionCert("aa")
	d, _, _ := setup(t, false, st)
	d = certHooks(d, &log, nil)
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	got, _ := d.States.Load()
	require.Equal(t, store.CleanState(), got)
}

func TestRecover_RemoveCertFailsKeepsState(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, dirtyWith2B())
	d = certHooks(d, &log, errors.New("access denied"))
	_, err := watchdog.RestoreIfOrphaned(d)
	require.Error(t, err)
	st, _ := d.States.Load()
	require.Equal(t, []string{"aa", "bb"}, st.Certs.Session, "kept for the next recovery layer")
	require.Contains(t, log, "dns", "DNS is still restored")
}

func TestRecover_CorruptStateDeletesAllRules(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, nil)
	d = certHooks(d, &log, nil)
	require.NoError(t, writeCorrupt(d.States.Path()))
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.RestoredFromCorrupt, out)
	for _, n := range []string{"Ghostline Proxy", "Ghostline DNS (TCP)", "Ghostline DNS (UDP)", "Ghostline Setup"} {
		require.Contains(t, log, "rule:"+n)
	}
	require.Contains(t, log, "sweep")
}

func TestRecover_SweepWhenNothingToDo(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, nil)
	d = certHooks(d, &log, nil)
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.NothingToDo, out)
	require.Equal(t, []string{"sweep"}, log)
}

func TestRecover_NoSweepWhileOwnerAlive(t *testing.T) {
	var log []string
	d, _, _ := setup(t, true, dirtyWith2B())
	d = certHooks(d, &log, nil)
	out, _ := watchdog.RestoreIfOrphaned(d)
	require.Equal(t, watchdog.OwnerAlive, out)
	require.Empty(t, log)
}

func writeCorrupt(path string) error { return os.WriteFile(path, []byte("{bad"), 0o644) }

// state.json is user-writable: only Ghostline's own rule names are deleted.
func TestRecover_IgnoresUnknownRuleNames(t *testing.T) {
	var log []string
	st := dirty()
	st.AddFirewallRule("Block Telemetry")
	st.AddFirewallRule("Ghostline DNS (UDP)")
	d, _, _ := setup(t, false, st)
	d = certHooks(d, &log, nil)
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Contains(t, log, "rule:Ghostline DNS (UDP)")
	require.NotContains(t, log, "rule:Block Telemetry")
}
