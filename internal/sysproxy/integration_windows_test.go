//go:build windows && integration

package sysproxy_test

import (
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

// Run from an admin terminal: go test -tags integration ./internal/sysproxy/
func TestIntegration_ApplyRestoreRoundTrip(t *testing.T) {
	m := sysproxy.Manager{API: sysproxy.NewWindowsAPI()}
	snap, err := m.Snapshot()
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.API.Set(snap) })

	changed := make(chan struct{}, 8)
	stop, err := sysproxy.Watch(func() { changed <- struct{}{} })
	require.NoError(t, err)
	defer stop()

	require.NoError(t, m.Apply("127.0.0.1:18080"))
	ours, err := m.IsOurs("127.0.0.1:18080")
	require.NoError(t, err)
	require.True(t, ours)
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not report the change")
	}

	restored, err := m.RestoreIfOurs("127.0.0.1:18080", snap)
	require.NoError(t, err)
	require.True(t, restored)
	got, err := m.Snapshot()
	require.NoError(t, err)
	require.Equal(t, snap, got)
}
