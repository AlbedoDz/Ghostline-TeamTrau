package backup_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/stretchr/testify/require"
)

func read(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func TestApply_WritesAll(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.txt")
	require.NoError(t, backup.Apply([]backup.Write{{Path: a, Data: []byte("A")}, {Path: b, Data: []byte("B")}}))
	require.Equal(t, "A", read(t, a))
	require.Equal(t, "B", read(t, b))
}

func TestApply_BackupKeptOnSuccess(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(a, []byte("old"), 0o644))
	require.NoError(t, backup.Apply([]backup.Write{{Path: a, Data: []byte("new")}, {Path: b, Data: []byte("B")}}))
	require.Equal(t, "old", read(t, a+".bak-import"))
	_, err := os.Stat(b + ".bak-import")
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestApply_RollbackRestores(t *testing.T) {
	dir := t.TempDir()
	a, bad := filepath.Join(dir, "a.json"), filepath.Join(dir, "sub")
	require.NoError(t, os.WriteFile(a, []byte("old"), 0o644))
	require.NoError(t, os.Mkdir(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "x"), nil, 0o644))
	err := backup.Apply([]backup.Write{{Path: a, Data: []byte("new")}, {Path: bad, Data: []byte("B")}})
	var we *backup.WriteError
	require.ErrorAs(t, err, &we)
	require.Equal(t, bad, we.Path)
	require.Equal(t, "old", read(t, a))
	_, err = os.Stat(a + ".bak-import")
	require.True(t, errors.Is(err, os.ErrNotExist), "rollback leaves no backup behind")
}

func TestApply_RollbackRemovesNewFiles(t *testing.T) {
	dir := t.TempDir()
	a, bad := filepath.Join(dir, "rules.json"), filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "x"), nil, 0o644))
	require.Error(t, backup.Apply([]backup.Write{{Path: a, Data: []byte("new")}, {Path: bad, Data: []byte("B")}}))
	_, err := os.Stat(a)
	require.True(t, errors.Is(err, os.ErrNotExist), "a file the import created is removed again")
}
