package cfscan

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Result is one IP's outcome.
type Result struct {
	IP        string    `json:"ip"`
	OK        bool      `json:"ok"`
	Reason    string    `json:"reason,omitempty"` // tcp_timeout tcp_refused tls_timeout tls_reset tls_verify http_status bad_trace
	LatencyMs int64     `json:"latencyMs"`
	Colo      string    `json:"colo"`
	Mbps      float64   `json:"mbps"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Prober checks one Cloudflare IP: TCP to port 443, TLS for Host (verified
// against Roots, or the system roots), then GET /cdn-cgi/trace.
type Prober struct {
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
	Host    string
	Timeout time.Duration // whole chain
	Roots   *x509.CertPool
	Now     func() time.Time
}

func (p Prober) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// connect dials ip:443 and completes a verified TLS handshake. The returned
// conn is closed when ctx ends. reason is set when err is.
func (p Prober) connect(ctx context.Context, ip netip.Addr, alpn string) (c *tls.Conn, connected time.Time, reason string, err error) {
	raw, err := p.Dial(ctx, "tcp", netip.AddrPortFrom(ip, 443).String())
	if err != nil {
		if timedOut(ctx, err) {
			return nil, time.Time{}, "tcp_timeout", err
		}
		return nil, time.Time{}, "tcp_refused", err
	}
	connected = time.Now()
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	}
	tc := tls.Client(raw, &tls.Config{
		ServerName: p.Host, RootCAs: p.Roots, MinVersion: tls.VersionTLS12, NextProtos: []string{alpn},
	})
	if err := tc.HandshakeContext(ctx); err != nil {
		stop()
		_ = raw.Close()
		var verr *tls.CertificateVerificationError
		switch {
		case errors.As(err, &verr):
			return nil, time.Time{}, "tls_verify", err
		case timedOut(ctx, err):
			return nil, time.Time{}, "tls_timeout", err
		}
		return nil, time.Time{}, "tls_reset", err
	}
	return tc, connected, "", nil
}

func timedOut(ctx context.Context, err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		(errors.As(err, &ne) && ne.Timeout())
}

// get sends a GET for path over c and returns the response (body unread).
func (p Prober) get(c net.Conn, path string) (*http.Response, error) {
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Ghostline\r\nAccept: */*\r\nConnection: close\r\n\r\n", path, p.Host)
	if _, err := io.WriteString(c, req); err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(c), nil)
}

// Probe checks ip (spec 3 §7.2).
func (p Prober) Probe(ctx context.Context, ip netip.Addr) Result {
	r := Result{IP: ip.String(), CheckedAt: p.now()}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	c, connected, reason, err := p.connect(ctx, ip, "http/1.1")
	if err != nil {
		r.Reason = reason
		return r
	}
	defer c.Close()
	resp, err := p.get(c, "/cdn-cgi/trace")
	if err != nil {
		r.Reason = "bad_trace"
		if timedOut(ctx, err) {
			r.Reason = "tls_timeout"
		}
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.Reason = "http_status"
		return r
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var hasIP bool
	for _, line := range strings.Split(string(body), "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "colo":
			r.Colo = v
		case "ip":
			hasIP = v != ""
		}
	}
	if r.Colo == "" || !hasIP {
		r.Reason = "bad_trace"
		return r
	}
	r.OK = true
	r.LatencyMs = time.Since(connected).Milliseconds()
	return r
}
