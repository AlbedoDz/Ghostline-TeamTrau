package watchdog_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func TestRemoveAllCerts(t *testing.T) {
	s := certstore.NewFake()
	lan, err := certs.NewLANCA("PC", time.Now())
	require.NoError(t, err)
	ses, err := certs.NewSessionCA([]string{"example.com"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, s.Install(lan.DER))
	require.NoError(t, s.Install(ses.DER))
	dir := t.TempDir()
	crt, key := filepath.Join(dir, "lan-ca.crt"), filepath.Join(dir, "lan-ca.key")
	require.NoError(t, os.WriteFile(crt, lan.DER, 0o600))
	require.NoError(t, os.WriteFile(key, []byte("k"), 0o600))

	require.NoError(t, watchdog.RemoveAllCerts(s, crt, key, filepath.Join(dir, "missing")))
	left, _ := s.List("Ghostline")
	require.Empty(t, left)
	_, err = os.Stat(key)
	require.ErrorIs(t, err, os.ErrNotExist)
}
