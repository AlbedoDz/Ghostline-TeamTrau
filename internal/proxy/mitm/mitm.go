// Package mitm performs the Fake SNI handshake pair (spec 2B 8.3): TLS to
// the server with a fake (or no) SNI and strict certificate checks, then
// TLS to the client with a certificate from the session CA. The caller
// relays bytes between the two connections; nothing here reads or keeps
// the decrypted content.
package mitm

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/hashcott/ghostline/internal/tlsfrag"
)

// LeafSource signs certificates for intercepted hosts (*certs.Issuer).
type LeafSource interface {
	Leaf(host string) (*tls.Certificate, error)
}

// Params describe one interception.
type Params struct {
	Host    string         // real host: the ClientHello SNI
	FakeSNI string         // SNI sent to the server; "" sends none
	Hello   []byte         // the client's ClientHello, already read
	Roots   *x509.CertPool // nil = system roots
	Timeout time.Duration  // per handshake; 0 = 10 s
	Now     func() time.Time
}

var (
	ErrServerRejected = errors.New("mitm: server refused the fake SNI")
	ErrVerifyFailed   = errors.New("mitm: server certificate not valid for the fake SNI or host")
	ErrClientRejected = errors.New("mitm: client refused the local certificate")
)

const defaultTimeout = 10 * time.Second

func (p Params) timeout() time.Duration {
	if p.Timeout <= 0 {
		return defaultTimeout
	}
	return p.Timeout
}

// DialServer completes TLS over raw (connected, nothing written yet) with
// the fake SNI and the ALPN protocols the client offered. Errors are
// ErrVerifyFailed or ErrServerRejected; raw is not closed.
func DialServer(ctx context.Context, raw net.Conn, p Params) (*tls.Conn, error) {
	cfg := &tls.Config{
		ServerName: p.FakeSNI,
		NextProtos: tlsfrag.ALPN(p.Hello),
		MinVersion: tls.VersionTLS12,
		// Verification is done by verifyServer, which accepts the fake
		// SNI or the real host; it is never skipped.
		InsecureSkipVerify: true,
		VerifyConnection:   func(cs tls.ConnectionState) error { return verifyServer(cs, p) },
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	c := tls.Client(raw, cfg)
	if err := c.HandshakeContext(ctx); err != nil {
		if errors.Is(err, ErrVerifyFailed) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrServerRejected, err)
	}
	return c, nil
}

// AcceptClient completes TLS with the client. The ClientHello the proxy
// already consumed is replayed first, then the rest of br. Only the
// protocol the server negotiated is offered, so both sides speak the same
// one.
func AcceptClient(ctx context.Context, client net.Conn, br *bufio.Reader, p Params, leaf LeafSource, negotiated string) (*tls.Conn, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return leaf.Leaf(p.Host)
		},
	}
	if negotiated != "" {
		cfg.NextProtos = []string{negotiated}
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	c := tls.Server(&replayConn{Conn: client, prefix: p.Hello, br: br}, cfg)
	if err := c.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClientRejected, err)
	}
	return c, nil
}

// replayConn reads prefix, then br, and writes to the underlying conn.
type replayConn struct {
	net.Conn
	prefix []byte
	br     *bufio.Reader
}

func (r *replayConn) Read(b []byte) (int, error) {
	if len(r.prefix) > 0 {
		n := copy(b, r.prefix)
		r.prefix = r.prefix[n:]
		return n, nil
	}
	if r.br != nil {
		return r.br.Read(b)
	}
	return r.Conn.Read(b)
}

// CloseWrite lets the relay half-close the client side.
func (r *replayConn) CloseWrite() error {
	if cw, ok := r.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
