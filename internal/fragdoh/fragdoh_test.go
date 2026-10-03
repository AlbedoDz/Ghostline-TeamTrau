package fragdoh_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/fragdoh"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func captureClientHello(t *testing.T, host string) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer c1.Close()
	go func() {
		_ = tls.Client(c1, &tls.Config{ServerName: host, InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, 64*1024)
	n, err := c2.Read(buf)
	require.NoError(t, err)
	c2.Close()
	return buf[:n]
}

func TestSplitClientHello_SplitsHostnameIntoChunks(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")
	segs := fragdoh.SplitClientHello(rec, 5)
	require.Len(t, segs, 7)
	require.Equal(t, rec, bytes.Join(segs, nil))
	require.Equal(t, "dns.example.test", string(bytes.Join(segs[1:6], nil)))
}

func TestSplitClientHello_NonHandshakePassthrough(t *testing.T) {
	rec := []byte{0x17, 3, 3, 0, 1, 0}
	require.Equal(t, [][]byte{rec}, fragdoh.SplitClientHello(rec, 5))
}

func TestSplitClientHello_TruncatedPassthrough(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")[:40]
	require.Equal(t, [][]byte{rec}, fragdoh.SplitClientHello(rec, 5))
}

// readCounter records the size of every Read on accepted connections.
type readCounter struct {
	net.Listener
	mu    sync.Mutex
	sizes [][]int
}

type countConn struct {
	net.Conn
	rc  *readCounter
	idx int
}

func (c *countConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.rc.mu.Lock()
	c.rc.sizes[c.idx] = append(c.rc.sizes[c.idx], n)
	c.rc.mu.Unlock()
	return n, err
}

func (l *readCounter) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sizes = append(l.sizes, nil)
	return &countConn{Conn: c, rc: l, idx: len(l.sizes) - 1}, nil
}

func dohHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/dns-message", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		var q dns.Msg
		require.NoError(t, q.Unpack(body))
		require.Zero(t, q.Id, "RFC 8484: ID must be 0")
		resp := new(dns.Msg).SetReply(&q)
		resp.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(192, 0, 2, 53)}}
		out, _ := resp.Pack()
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(out)
	})
}

func TestUpstream_ExchangeOverFragmentedTLS(t *testing.T) {
	srv := httptest.NewUnstartedServer(dohHandler(t))
	srv.EnableHTTP2 = true
	rc := &readCounter{}
	srv.StartTLS()
	defer srv.Close()
	// Re-wrap the listener after StartTLS by restarting on a counting listener.
	srv.Listener.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	rc.Listener = ln
	tlsLn := tls.NewListener(rc, srv.TLS)
	go func() { _ = http.Serve(tlsLn, srv.Config.Handler) }()
	defer tlsLn.Close()

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	port := ln.Addr().(*net.TCPAddr).Port
	u, err := fragdoh.New("https://example.com:"+strconv.Itoa(port)+"/dns-query", fragdoh.Options{
		Bootstrap: upstream.StaticResolver{netip.MustParseAddr("127.0.0.1")},
		Chunks:    5, Delay: 20 * time.Millisecond, Timeout: 3 * time.Second, RootCAs: pool,
	})
	require.NoError(t, err)
	defer u.Close()

	q := new(dns.Msg).SetQuestion("example.org.", dns.TypeA)
	q.Id = 4242
	resp, err := u.Exchange(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, uint16(4242), resp.Id)
	require.Equal(t, "192.0.2.53", resp.Answer[0].(*dns.A).A.String())
	require.True(t, strings.HasPrefix(u.Address(), "https://example.com:"))

	rc.mu.Lock()
	defer rc.mu.Unlock()
	require.NotEmpty(t, rc.sizes)
	reads, total := 0, 0
	for _, n := range rc.sizes[0] {
		if total >= 200 {
			break
		}
		total += n
		reads++
	}
	require.GreaterOrEqual(t, reads, 3, "ClientHello should arrive in several segments: %v", rc.sizes[0])
}
