package store_test

import (
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func TestWithMachineDir(t *testing.T) {
	p := store.ResolvePaths(`C:\Program Files\Ghostline\ghostline.exe`, `C:\Users\u\AppData\Roaming`)
	m := store.WithMachineDir(p, `C:\ProgramData\Ghostline`)
	require.Equal(t, filepath.Join(`C:\ProgramData\Ghostline`, "lan-ca.crt"), m.LANCACert)
	require.Equal(t, filepath.Join(`C:\ProgramData\Ghostline`, "lan-ca.key"), m.LANCAKey)
	require.Equal(t, `C:\ProgramData\Ghostline`, m.MachineDir)
	require.Equal(t, p.State, m.State)
}
