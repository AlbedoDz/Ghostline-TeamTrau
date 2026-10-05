package proxy_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/testutil"
	"github.com/stretchr/testify/require"
)

// edge is a CDN edge serving youtube.com; it records SNIs and can refuse
// the fake one.
type edge struct {
	srv  *httptest.Server
	mu   sync.Mutex
	snis []string
}

func (e *edge) seen() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.snis...)
}

func newEdge(t *testing.T, pub *testutil.TestCA, refuseFake bool) *edge {
	e := &edge{}
	e.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "edge "+r.Host)
	}))
	e.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pub.Leaf(t, []string{"youtube.com", "www.google.com"}, time.Now().Add(time.Hour))},
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			e.mu.Lock()
			e.snis = append(e.snis, chi.ServerName)
			e.mu.Unlock()
			if refuseFake && chi.ServerName == "www.google.com" {
				return nil, errors.New("refused")
			}
			return nil, nil
		},
	}
	e.srv.StartTLS()
	t.Cleanup(e.srv.Close)
	return e
}

type fakeSNIEnv struct {
	pub    *testutil.TestCA
	ses    *certs.CA
	issuer *certs.Issuer
}

func newFakeSNIEnv(t *testing.T) *fakeSNIEnv {
	ses, err := certs.NewSessionCA([]string{"youtube.com"}, time.Now())
	require.NoError(t, err)
	return &fakeSNIEnv{pub: testutil.NewTestCA(t), ses: ses, issuer: certs.NewIssuer(ses, time.Now)}
}

func (f *fakeSNIEnv) proxy(t *testing.T, active bool) (*proxy.Server, string) {
	d, _ := realDialer(t, mapResolver{"youtube.com": {netip.MustParseAddr("127.0.0.1")}}, "youtube.com sni=www.google.com")
	return startServer(t, proxy.Config{Dialer: d, MITMRoots: f.pub.Pool, MITM: func() mitm.LeafSource {
		if !active {
			return nil
		}
		return f.issuer
	}})
}

func (f *fakeSNIEnv) get(t *testing.T, proxyAddr string, e *edge, trustSession bool) (string, error) {
	pool := x509.NewCertPool()
	pool.AddCert(f.pub.Cert)
	if trustSession {
		pool.AddCert(f.ses.Cert)
	}
	pu, _ := url.Parse("http://" + proxyAddr)
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, err := c.Get(fmt.Sprintf("https://youtube.com:%d/", port(t, e.srv.URL)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func issuerOf(t *testing.T, proxyAddr string, e *edge, roots *x509.CertPool) string {
	pu, _ := url.Parse("http://" + proxyAddr)
	var subject string
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: roots,
		VerifyConnection: func(cs tls.ConnectionState) error { subject = cs.PeerCertificates[0].Issuer.CommonName; return nil }}}}
	resp, err := c.Get(fmt.Sprintf("https://youtube.com:%d/", port(t, e.srv.URL)))
	require.NoError(t, err)
	resp.Body.Close()
	return subject
}

func TestTunnel_FakeSNI(t *testing.T) {
	f := newFakeSNIEnv(t)
	e := newEdge(t, f.pub, false)
	s, addr := f.proxy(t, true)
	body, err := f.get(t, addr, e, true)
	require.NoError(t, err)
	require.Contains(t, body, "edge youtube.com")
	require.Equal(t, []string{"www.google.com"}, e.seen())
	require.Equal(t, uint64(1), s.Stats().ByOutcome["fakesni"])
}

func TestTunnel_FakeSNIFallback(t *testing.T) {
	f := newFakeSNIEnv(t)
	e := newEdge(t, f.pub, true)
	s, addr := f.proxy(t, true)
	pool := x509.NewCertPool()
	pool.AddCert(f.pub.Cert) // the client trusts only the real server
	require.Equal(t, "Test Public Root", issuerOf(t, addr, e, pool))
	require.Equal(t, []string{"www.google.com", "youtube.com"}, e.seen())
	require.Equal(t, uint64(1), s.Stats().ByOutcome["fakesni_fallback"])
}

func TestTunnel_FakeSNIInactive(t *testing.T) {
	f := newFakeSNIEnv(t)
	e := newEdge(t, f.pub, false)
	s, addr := f.proxy(t, false)
	pool := x509.NewCertPool()
	pool.AddCert(f.pub.Cert)
	require.Equal(t, "Test Public Root", issuerOf(t, addr, e, pool))
	require.Equal(t, []string{"youtube.com"}, e.seen())
	require.Zero(t, s.Stats().ByOutcome["fakesni"])
}

func TestTunnel_ClientRejects(t *testing.T) {
	f := newFakeSNIEnv(t)
	e := newEdge(t, f.pub, false)
	s, addr := f.proxy(t, true)
	_, err := f.get(t, addr, e, false)
	require.Error(t, err)
	require.Eventually(t, func() bool { return s.Stats().ByOutcome["fakesni_client_rejected"] == 1 }, 3*time.Second, 20*time.Millisecond)
}

// A rule added before the session CA rotated: the running CA cannot sign
// the host, so the connection takes the 2A path instead of failing.
func TestTunnel_HostNotCoveredByCurrentCA(t *testing.T) {
	f := newFakeSNIEnv(t)
	other, err := certs.NewSessionCA([]string{"example.com"}, time.Now())
	require.NoError(t, err)
	f.issuer = certs.NewIssuer(other, time.Now)
	e := newEdge(t, f.pub, false)
	s, addr := f.proxy(t, true)
	pool := x509.NewCertPool()
	pool.AddCert(f.pub.Cert)
	require.Equal(t, "Test Public Root", issuerOf(t, addr, e, pool))
	require.Zero(t, s.Stats().ByOutcome["fakesni"])
}

// The certificate source is read when the leaf is signed, not when the
// connection starts: a rotation in between uses the new CA.
func TestTunnel_LeafFromCurrentIssuer(t *testing.T) {
	f := newFakeSNIEnv(t)
	newer, err := certs.NewSessionCA([]string{"youtube.com"}, time.Now())
	require.NoError(t, err)
	newIssuer := certs.NewIssuer(newer, time.Now)
	e := newEdge(t, f.pub, false)
	d, _ := realDialer(t, mapResolver{"youtube.com": {netip.MustParseAddr("127.0.0.1")}}, "youtube.com sni=www.google.com")
	calls := 0
	_, addr := startServer(t, proxy.Config{Dialer: d, MITMRoots: f.pub.Pool, MITM: func() mitm.LeafSource {
		calls++
		if calls == 1 {
			return f.issuer
		}
		return newIssuer
	}})
	pool := x509.NewCertPool()
	pool.AddCert(f.pub.Cert)
	pool.AddCert(newer.Cert) // trusts only the newer session CA
	require.Equal(t, newer.Cert.Subject.CommonName, issuerOf(t, addr, e, pool))
}
