package probe_test

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/probe"
	"github.com/stretchr/testify/require"
)

func loopback(context.Context, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

func prober(port int, roots *x509.CertPool) probe.Prober {
	return probe.Prober{Resolve: loopback, Dial: (&net.Dialer{}).DialContext, Timeout: 2 * time.Second, RootCAs: roots, Port: strconv.Itoa(port)}
}

func TestProbe_OK(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	r := prober(srv.Listener.Addr().(*net.TCPAddr).Port, roots).Probe(context.Background(), "example.com")
	require.Equal(t, probe.StageOK, r.Stage, r.Err)
	require.Equal(t, "example.com", r.Site)
}

func TestProbe_DNSFailure(t *testing.T) {
	p := prober(443, nil)
	p.Resolve = func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("nxdomain") }
	require.Equal(t, probe.StageDNS, p.Probe(context.Background(), "example.com").Stage)
}

func TestProbe_TCPRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	require.Equal(t, probe.StageTCP, prober(port, nil).Probe(context.Background(), "example.com").Stage)
}

func TestProbe_TLSReset(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_, _ = c.Read(buf) // read ClientHello, then drop like a DPI box
			if tc, ok := c.(*net.TCPConn); ok {
				_ = tc.SetLinger(0) // RST
			}
			c.Close()
		}
	}()
	r := prober(l.Addr().(*net.TCPAddr).Port, nil).Probe(context.Background(), "example.com")
	require.Equal(t, probe.StageTLS, r.Stage)
}

func TestProbeAll_Concurrent(t *testing.T) {
	p := prober(443, nil)
	p.Resolve = func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("x") }
	rs := p.ProbeAll(context.Background(), []string{"a.example", "b.example"})
	require.Len(t, rs, 2)
	require.Equal(t, "a.example", rs[0].Site)
	require.Equal(t, "b.example", rs[1].Site)
}

func TestDPIBlocked_RequiresBothAttempts(t *testing.T) {
	first := []probe.Result{{Site: "a", Stage: probe.StageTLS}, {Site: "b", Stage: probe.StageTLS}, {Site: "c", Stage: probe.StageTCP}}
	second := []probe.Result{{Site: "a", Stage: probe.StageTLS}, {Site: "b", Stage: probe.StageOK}, {Site: "c", Stage: probe.StageTCP}}
	require.Equal(t, []string{"a"}, probe.DPIBlocked(first, second))
}
