package logx_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/logx"
	"github.com/stretchr/testify/require"
)

func TestRotating_RotatesAtMaxAndKeepsN(t *testing.T) {
	dir := t.TempDir()
	w, err := logx.NewRotating(dir, "base", 100, 3)
	require.NoError(t, err)
	line := append(bytes.Repeat([]byte("x"), 49), '\n') // 50 bytes
	for i := 0; i < 9; i++ {                              // 450 bytes
		_, err := w.Write(line)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.FileExists(t, filepath.Join(dir, "base.log"))
	require.FileExists(t, filepath.Join(dir, "base.1.log"))
	require.FileExists(t, filepath.Join(dir, "base.2.log"))
	require.NoFileExists(t, filepath.Join(dir, "base.3.log"))
	fi, _ := os.Stat(filepath.Join(dir, "base.log"))
	require.LessOrEqual(t, fi.Size(), int64(100))
}
