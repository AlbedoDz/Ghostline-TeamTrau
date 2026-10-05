package certs_test

import (
	"crypto/x509"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func verify(t *testing.T, ca *certs.CA, names []string, ips []netip.Addr, eku x509.ExtKeyUsage) error {
	t.Helper()
	leaf, err := ca.IssueServer(names, ips, 24*time.Hour, t0)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	name := ""
	if len(names) > 0 {
		name = names[0]
	} else {
		name = ips[0].String()
	}
	_, err = leaf.Leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: name, CurrentTime: t0.Add(time.Hour), KeyUsages: []x509.ExtKeyUsage{eku}})
	return err
}

func TestSessionCA_AllowsListedDomains(t *testing.T) {
	ca, err := certs.NewSessionCA([]string{"youtube.com", "googlevideo.com"}, t0)
	require.NoError(t, err)
	for _, h := range []string{"youtube.com", "m.youtube.com", "r1.googlevideo.com"} {
		require.NoError(t, verify(t, ca, []string{h}, nil, x509.ExtKeyUsageServerAuth), h)
	}
}

func TestSessionCA_RejectsOthers(t *testing.T) {
	ca, err := certs.NewSessionCA([]string{"youtube.com", "googlevideo.com"}, t0)
	require.NoError(t, err)
	for _, h := range []string{"vietcombank.com.vn", "googlevideo.com.evil.net", "notyoutube.com"} {
		require.Error(t, verify(t, ca, []string{h}, nil, x509.ExtKeyUsageServerAuth), h)
	}
	require.Error(t, verify(t, ca, nil, []netip.Addr{netip.MustParseAddr("1.1.1.1")}, x509.ExtKeyUsageServerAuth))
}

func TestSessionCA_TooMany(t *testing.T) {
	d := make([]string, 1001)
	for i := range d {
		d[i] = strings.Repeat("a", 1+i%50) + ".com"
	}
	_, err := certs.NewSessionCA(d, t0)
	require.ErrorIs(t, err, certs.ErrTooManyDomains)
	_, err = certs.NewSessionCA(nil, t0)
	require.Error(t, err)
}

func TestLANCA_Constraints(t *testing.T) {
	ca, err := certs.NewLANCA("PC1", t0)
	require.NoError(t, err)
	for _, ip := range []string{"192.168.1.5", "127.0.0.1", "::1", "10.1.2.3", "fd00::1"} {
		require.NoError(t, verify(t, ca, nil, []netip.Addr{netip.MustParseAddr(ip)}, x509.ExtKeyUsageServerAuth), ip)
	}
	require.NoError(t, verify(t, ca, []string{"dns.ghostline.lan"}, nil, x509.ExtKeyUsageServerAuth))
	require.Error(t, verify(t, ca, nil, []netip.Addr{netip.MustParseAddr("8.8.8.8")}, x509.ExtKeyUsageServerAuth))
	require.Error(t, verify(t, ca, []string{"example.com"}, nil, x509.ExtKeyUsageServerAuth))
}

func TestCA_Basics(t *testing.T) {
	lan, err := certs.NewLANCA("PC1", t0)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(lan.Cert.Subject.CommonName, certs.LANPrefix+" — PC1 "))
	require.Equal(t, t0.AddDate(5, 0, 0), lan.Cert.NotAfter.UTC())
	ses, err := certs.NewSessionCA([]string{"youtube.com"}, t0)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ses.Cert.Subject.CommonName, certs.SessionPrefix+" — "))
	require.Equal(t, t0.Add(-time.Hour), ses.Cert.NotBefore.UTC())
	require.Equal(t, t0.Add(30*24*time.Hour), ses.Cert.NotAfter.UTC())
	for _, ca := range []*certs.CA{lan, ses} {
		c := ca.Cert
		require.True(t, c.IsCA)
		require.True(t, c.MaxPathLenZero)
		require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, c.ExtKeyUsage)
		require.True(t, c.PermittedDNSDomainsCritical)
		require.Len(t, ca.Thumbprint(), 40)
		require.Len(t, ca.Fingerprint(), 32*3-1)
	}
	// A client-auth leaf is refused by the serverAuth-only CA.
	require.Error(t, verify(t, ses, []string{"youtube.com"}, nil, x509.ExtKeyUsageClientAuth))
}

func TestIssuer_CachesAndEvicts(t *testing.T) {
	ca, err := certs.NewSessionCA([]string{"example.com"}, t0)
	require.NoError(t, err)
	is := certs.NewIssuer(ca, func() time.Time { return t0 })
	a, err := is.Leaf("a.example.com")
	require.NoError(t, err)
	b, err := is.Leaf("a.example.com")
	require.NoError(t, err)
	require.Same(t, a, b)
	require.Equal(t, t0.Add(7*24*time.Hour), a.Leaf.NotAfter.UTC())
	for i := 0; i < 1000; i++ {
		_, err := is.Leaf(fmt.Sprintf("h%d.example.com", i))
		require.NoError(t, err)
	}
	c, err := is.Leaf("a.example.com")
	require.NoError(t, err)
	require.NotSame(t, a, c)
	require.Same(t, ca, is.CA())
}
