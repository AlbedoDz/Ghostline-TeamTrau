//go:build windows && integration

// Run from an elevated terminal: go test -tags integration ./internal/startup/...
package startup_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/startup"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CreateExistsDelete(t *testing.T) {
	task := startup.RecoveryTask(`C:\Windows\System32\cmd.exe`)
	task.Name = "Ghostline Test"
	require.NoError(t, startup.Create(task))
	t.Cleanup(func() { _ = startup.Delete(task.Name) })
	require.True(t, startup.Exists(task.Name))
	require.NoError(t, startup.Delete(task.Name))
	require.False(t, startup.Exists(task.Name))
	require.NoError(t, startup.Delete(task.Name), "deleting a missing task is not an error")
}
