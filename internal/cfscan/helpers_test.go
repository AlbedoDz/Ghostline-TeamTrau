package cfscan_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

// testPKI is a CA plus a leaf for the given names.
type testPKI struct {
	roots *x509.CertPool
	leaf  tls.Certificate
}

func newPKI(t *testing.T, names ...string) testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, _ := x509.ParseCertificate(caDER)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return testPKI{roots: pool, leaf: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}}
}

const host = "speed.cloudflare.com"

// edge is a fake Cloudflare edge. It records SNI and Host of each request.
type edge struct {
	srv   *httptest.Server
	pki   testPKI
	mu    sync.Mutex
	snis  []string
	hosts []string
}

func newEdge(t *testing.T, h http.HandlerFunc, certNames ...string) *edge {
	t.Helper()
	if len(certNames) == 0 {
		certNames = []string{host}
	}
	e := &edge{pki: newPKI(t, certNames...)}
	e.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.hosts = append(e.hosts, r.Host)
		e.mu.Unlock()
		h(w, r)
	}))
	e.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{e.pki.leaf},
		GetConfigForClient: func(hi *tls.ClientHelloInfo) (*tls.Config, error) {
			e.mu.Lock()
			e.snis = append(e.snis, hi.ServerName)
			e.mu.Unlock()
			return nil, nil
		},
	}
	e.srv.StartTLS()
	t.Cleanup(e.srv.Close)
	return e
}

func trace(colo string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cdn-cgi/trace" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("fl=1\nh=" + host + "\nip=1.2.3.4\ncolo=" + colo + "\n"))
	}
}

// prober dials addr whatever IP it is asked for, counting dials.
func prober(e testPKI, addr string, dials *atomic.Int32) cfscan.Prober {
	return cfscan.Prober{
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if dials != nil {
				dials.Add(1)
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
		Host: host, Timeout: 2 * time.Second, Roots: e.roots,
	}
}
