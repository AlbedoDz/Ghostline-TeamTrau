package certstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/stretchr/testify/require"
)

func session(t *testing.T) *certs.CA {
	t.Helper()
	ca, err := certs.NewSessionCA([]string{"example.com"}, time.Now())
	require.NoError(t, err)
	return ca
}

func TestFake_InstallListRemove(t *testing.T) {
	s := certstore.NewFake()
	ca := session(t)
	require.NoError(t, s.Install(ca.DER))
	got, err := s.List(certs.SessionPrefix)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, ca.Thumbprint(), got[0].Thumbprint)
	require.Equal(t, ca.Cert.Subject.CommonName, got[0].Subject)
	require.NoError(t, s.Remove(ca.Thumbprint()))
	require.NoError(t, s.Remove(ca.Thumbprint()), "removing a missing cert is not an error")
	got, _ = s.List(certs.SessionPrefix)
	require.Empty(t, got)
}

func TestFake_InstallFailure(t *testing.T) {
	s := certstore.NewFake()
	s.FailInstall = errors.New("access denied")
	require.Error(t, s.Install(session(t).DER))
}

func TestSweep_KeepsCurrent(t *testing.T) {
	s := certstore.NewFake()
	a, b, c := session(t), session(t), session(t)
	lan, err := certs.NewLANCA("PC", time.Now())
	require.NoError(t, err)
	for _, ca := range []*certs.CA{a, b, c, lan} {
		require.NoError(t, s.Install(ca.DER))
	}
	removed, err := certstore.Sweep(s, certs.SessionPrefix, []string{b.Thumbprint()})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.Thumbprint(), c.Thumbprint()}, removed)
	left, _ := s.List("Ghostline")
	require.Len(t, left, 2) // b and the LAN CA
}

func TestSweep_RemoveErrorReported(t *testing.T) {
	s := certstore.NewFake()
	require.NoError(t, s.Install(session(t).DER))
	s.FailRemove = errors.New("locked")
	_, err := certstore.Sweep(s, certs.SessionPrefix, nil)
	require.Error(t, err)
}

func TestRemoveIfPrefix_OnlyGhostlineRoots(t *testing.T) {
	s := certstore.NewFake()
	ses := session(t)
	lan, err := certs.NewLANCA("PC", time.Now())
	require.NoError(t, err)
	require.NoError(t, s.Install(ses.DER))
	require.NoError(t, s.Install(lan.DER))

	// A thumbprint from an untrusted file that names another root: kept.
	require.NoError(t, certstore.RemoveIfPrefix(s, lan.Thumbprint(), certs.SessionPrefix))
	require.True(t, s.Has(lan.Thumbprint()))
	require.NoError(t, certstore.RemoveIfPrefix(s, ses.Thumbprint(), certs.SessionPrefix))
	require.False(t, s.Has(ses.Thumbprint()))
	require.NoError(t, certstore.RemoveIfPrefix(s, "00112233445566778899aabbccddeeff00112233", certs.SessionPrefix))
}
