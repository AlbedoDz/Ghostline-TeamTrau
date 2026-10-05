package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/stretchr/testify/require"
)

func TestSignFile(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	old := brand.ServerListPublicKeyHex
	brand.ServerListPublicKeyHex = hex.EncodeToString(pub)
	t.Cleanup(func() { brand.ServerListPublicKeyHex = old })
	p := filepath.Join(t.TempDir(), "strategies.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":1}`), 0o644))
	require.NoError(t, signFile(p, base64.StdEncoding.EncodeToString(priv)))
	sig, err := os.ReadFile(p + ".sig")
	require.NoError(t, err)
	require.NoError(t, servers.VerifySigned([]byte(`{"version":1}`), sig, pub))
	require.Error(t, signFile(p, "not-a-key"))
	require.Error(t, signFile(p, ""))
}

func TestParseKeyTolerantInput(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	old := brand.ServerListPublicKeyHex
	brand.ServerListPublicKeyHex = hex.EncodeToString(pub)
	t.Cleanup(func() { brand.ServerListPublicKeyHex = old })
	b64 := base64.StdEncoding.EncodeToString(priv)
	for _, in := range []string{b64, " " + b64 + "\r\n", "private=" + b64 + "\n"} {
		got, err := parseKey(in)
		require.NoError(t, err, "%q", in)
		require.Equal(t, priv, got)
	}
}

func TestParseKeyRejectsForeignKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = parseKey(base64.StdEncoding.EncodeToString(priv))
	require.ErrorContains(t, err, "ServerListPublicKeyHex")
}
