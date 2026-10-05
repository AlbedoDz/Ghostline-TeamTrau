package certs_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
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

// A LAN CA file replaced by another program (here: an unconstrained CA
// with a matching key) must never be loaded, so it is never installed.
func TestLANCA_RejectsUnconstrainedReplacement(t *testing.T) {
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "lan-ca.crt"), filepath.Join(dir, "lan-ca.key")
	evil := unconstrainedCA(t)
	require.NoError(t, certs.SaveLANCA(cp, kp, evil, xorProt{}))
	_, err := certs.LoadLANCA(cp, kp, xorProt{})
	require.ErrorIs(t, err, certs.ErrKeyUnreadable)
}

func TestLANCA_RejectsSessionCA(t *testing.T) {
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "lan-ca.crt"), filepath.Join(dir, "lan-ca.key")
	ses, err := certs.NewSessionCA([]string{"example.com"}, t0)
	require.NoError(t, err)
	require.NoError(t, certs.SaveLANCA(cp, kp, ses, xorProt{}))
	_, err = certs.LoadLANCA(cp, kp, xorProt{})
	require.ErrorIs(t, err, certs.ErrKeyUnreadable)
}

func unconstrainedCA(t *testing.T) *certs.CA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: certs.LANPrefix + " — PC EVIL"},
		NotBefore: t0, NotAfter: t0.AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	c, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &certs.CA{Cert: c, DER: der, Key: key}
}
