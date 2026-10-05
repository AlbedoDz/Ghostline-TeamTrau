package certs_test

import (
	"bytes"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/stretchr/testify/require"
)

type xorProt struct{ fail bool }

func (p xorProt) Protect(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[i] ^ 0x5a
	}
	return out, nil
}

func (p xorProt) Unprotect(b []byte) ([]byte, error) {
	if p.fail {
		return nil, errors.New("dpapi: wrong machine")
	}
	return p.Protect(b)
}

func TestLANCA_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "lan-ca.crt"), filepath.Join(dir, "lan-ca.key")
	ca, err := certs.NewLANCA("PC1", t0)
	require.NoError(t, err)
	require.NoError(t, certs.SaveLANCA(cp, kp, ca, xorProt{}))

	raw, err := os.ReadFile(kp)
	require.NoError(t, err)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(ca.Key)
	require.NoError(t, err)
	require.False(t, bytes.Equal(raw, pkcs8), "key must not be stored in clear")

	got, err := certs.LoadLANCA(cp, kp, xorProt{})
	require.NoError(t, err)
	require.Equal(t, ca.DER, got.DER)
	require.True(t, ca.Key.Equal(got.Key))
}

func TestLANCA_UnprotectFails(t *testing.T) {
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "lan-ca.crt"), filepath.Join(dir, "lan-ca.key")
	ca, err := certs.NewLANCA("PC1", t0)
	require.NoError(t, err)
	require.NoError(t, certs.SaveLANCA(cp, kp, ca, xorProt{}))
	_, err = certs.LoadLANCA(cp, kp, xorProt{fail: true})
	require.ErrorIs(t, err, certs.ErrKeyUnreadable)
}

func TestLANCA_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := certs.LoadLANCA(filepath.Join(dir, "a"), filepath.Join(dir, "b"), xorProt{})
	require.ErrorIs(t, err, os.ErrNotExist)
}
