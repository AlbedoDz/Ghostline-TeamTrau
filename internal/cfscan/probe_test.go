package cfscan_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

var ip = netip.MustParseAddr("104.16.1.1")

func TestProbe_OK(t *testing.T) {
	e := newEdge(t, trace("SIN"))
	r := prober(e.pki, e.srv.Listener.Addr().String(), nil).Probe(context.Background(), ip)
	require.True(t, r.OK, r.Reason)
	require.Equal(t, "104.16.1.1", r.IP)
	require.Equal(t, "SIN", r.Colo)
	require.False(t, r.CheckedAt.IsZero())
	require.Equal(t, []string{host}, e.snis)
	require.Equal(t, []string{host}, e.hosts)
}

func TestProbe_DialsPort443(t *testing.T) {
	var got string
	p := cfscan.Prober{Host: host, Timeout: time.Second, Dial: func(_ context.Context, _, addr string) (net.Conn, error) {
		got = addr
		return nil, syscall.ECONNREFUSED
	}}
	p.Probe(context.Background(), ip)
	require.Equal(t, "104.16.1.1:443", got)
}

func TestProbe_Reasons(t *testing.T) {
	status := newEdge(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	noColo := newEdge(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ip=1.2.3.4\n")) })
	wrongName := newEdge(t, trace("SIN"), "other.com")

	require.Equal(t, "http_status", prober(status.pki, status.srv.Listener.Addr().String(), nil).Probe(context.Background(), ip).Reason)
	require.Equal(t, "bad_trace", prober(noColo.pki, noColo.srv.Listener.Addr().String(), nil).Probe(context.Background(), ip).Reason)
	require.Equal(t, "tls_verify", prober(wrongName.pki, wrongName.srv.Listener.Addr().String(), nil).Probe(context.Background(), ip).Reason)
	// Right name, but not signed by a trusted root.
	ok := newEdge(t, trace("SIN"))
	require.Equal(t, "tls_verify", prober(newPKI(t, host), ok.srv.Listener.Addr().String(), nil).Probe(context.Background(), ip).Reason)

	// Closes right after accept.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	require.Equal(t, "tls_reset", prober(ok.pki, ln.Addr().String(), nil).Probe(context.Background(), ip).Reason)

	// Accepts but never answers the ClientHello.
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = silent.Close() })
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	p := prober(ok.pki, silent.Addr().String(), nil)
	p.Timeout = 300 * time.Millisecond
	start := time.Now()
	require.Equal(t, "tls_timeout", p.Probe(context.Background(), ip).Reason)
	require.Less(t, time.Since(start), time.Second)

	refused := cfscan.Prober{Host: host, Timeout: time.Second, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	}}
	require.Equal(t, "tcp_refused", refused.Probe(context.Background(), ip).Reason)

	blocked := cfscan.Prober{Host: host, Timeout: 200 * time.Millisecond, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, errors.Join(ctx.Err(), errors.New("dial timeout"))
	}}
	require.Equal(t, "tcp_timeout", blocked.Probe(context.Background(), ip).Reason)
}
