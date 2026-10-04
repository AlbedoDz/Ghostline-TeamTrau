package dialer_test

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tlsConfig is a server certificate borrowed from httptest.
func tlsConfig(t *testing.T) *tls.Config {
	srv := httptest.NewUnstartedServer(nil)
	srv.StartTLS()
	cfg := srv.TLS.Clone()
	srv.Close()
	return cfg
}

// prefixConn replays already-read bytes before the rest of the stream.
type prefixConn struct {
	net.Conn
	r io.Reader
}

func (c *prefixConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// dpiSim is a TLS echo server behind a fake DPI box: when the first read of
// a connection contains the blocked SNI in one piece, the DPI resets the
// connection (or, when silent, swallows it).
type dpiSim struct {
	ln       net.Listener
	blocked  string
	silent   atomic.Bool
	blockAll atomic.Bool // also block fragmented hellos
	accepts  atomic.Int32
	cfg      *tls.Config
	wg       sync.WaitGroup
}

func newDPISim(t *testing.T, blocked string) *dpiSim {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	d := &dpiSim{ln: ln, blocked: blocked, cfg: tlsConfig(t)}
	d.wg.Add(1)
	go d.serve()
	t.Cleanup(func() { ln.Close(); d.wg.Wait() })
	return d
}

func (d *dpiSim) addr() netip.AddrPort { return d.ln.Addr().(*net.TCPAddr).AddrPort() }

func (d *dpiSim) serve() {
	defer d.wg.Done()
	for {
		c, err := d.ln.Accept()
		if err != nil {
			return
		}
		d.accepts.Add(1)
		go d.handle(c)
	}
}

func (d *dpiSim) handle(c net.Conn) {
	defer c.Close()
	buf := make([]byte, 64<<10)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := c.Read(buf)
	if err != nil {
		return
	}
	first := buf[:n]
	if bytes.Contains(first, []byte(d.blocked)) || d.blockAll.Load() {
		if d.silent.Load() {
			time.Sleep(2 * time.Second)
			return
		}
		_ = c.(*net.TCPConn).SetLinger(0) // RST
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	tc := tls.Server(&prefixConn{Conn: c, r: io.MultiReader(bytes.NewReader(first), c)}, d.cfg)
	if err := tc.Handshake(); err != nil {
		return
	}
	_, _ = io.Copy(tc, tc) // echo
}

// clientHandshake runs a TLS client for host over one end of a pipe and
// returns the proxy-facing end plus a channel with the handshake result.
func clientHandshake(t *testing.T, host string) (net.Conn, *bufio.Reader, <-chan error) {
	app, proxySide := net.Pipe()
	done := make(chan error, 1)
	go func() {
		tc := tls.Client(app, &tls.Config{ServerName: host, InsecureSkipVerify: true})
		err := tc.Handshake()
		if err == nil {
			_, err = tc.Write([]byte("ping"))
			if err == nil {
				b := make([]byte, 4)
				_, err = io.ReadFull(tc, b)
				if err == nil && string(b) != "ping" {
					err = io.ErrUnexpectedEOF
				}
			}
		}
		done <- err
		app.Close()
	}()
	t.Cleanup(func() { app.Close(); proxySide.Close() })
	return proxySide, bufio.NewReaderSize(proxySide, 16<<10), done
}

// relay copies between the proxy-facing client end and the server conn.
func relay(client net.Conn, br *bufio.Reader, server net.Conn, first []byte) {
	if len(first) > 0 {
		_, _ = client.Write(first)
	}
	go func() { _, _ = io.Copy(server, br); server.Close() }()
	go func() { _, _ = io.Copy(client, server); client.Close() }()
}
