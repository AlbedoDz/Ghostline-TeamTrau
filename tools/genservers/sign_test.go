package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/servers"
	"github.com/stretchr/testify/require"
)

func TestSignFile(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "strategies.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":1}`), 0o644))
	require.NoError(t, signFile(p, base64.StdEncoding.EncodeToString(priv)))
	sig, err := os.ReadFile(p + ".sig")
	require.NoError(t, err)
	require.NoError(t, servers.VerifySigned([]byte(`{"version":1}`), sig, pub))
	require.Error(t, signFile(p, "not-a-key"))
}
