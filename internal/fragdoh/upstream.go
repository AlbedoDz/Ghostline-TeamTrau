package fragdoh

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// Options configures a fragmenting DoH upstream.
type Options struct {
	// Bootstrap resolves the DoH host. Required: never the system resolver.
	Bootstrap upstream.Resolver
	Chunks    int
	Delay     time.Duration
	Timeout   time.Duration
	// RootCAs overrides the system roots (tests only).
	RootCAs *x509.CertPool
}

// Upstream implements upstream.Upstream over HTTPS with a fragmented ClientHello.
type Upstream struct {
	address string
	url     *url.URL
	client  *http.Client
	tr      *http.Transport
	timeout time.Duration
}

var _ upstream.Upstream = (*Upstream)(nil)

// New creates a fragmenting DoH upstream for an https:// address.
func New(address string, o Options) (*Upstream, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("fragdoh: bad address %q", address)
	}
	if o.Bootstrap == nil {
		return nil, errors.New("fragdoh: bootstrap resolver is required")
	}
	if o.Timeout <= 0 {
		o.Timeout = 3 * time.Second
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "443"
	}
	dialer := &net.Dialer{Timeout: o.Timeout}
	tr := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   &tls.Config{ServerName: host, RootCAs: o.RootCAs, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			ips, err := o.Bootstrap.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("fragdoh: bootstrap %s: %w", host, err)
			}
			var lastErr error = fmt.Errorf("fragdoh: no addresses for %s", host)
			for _, ip := range ips {
				c, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
				if err != nil {
					lastErr = err
					continue
				}
				return &fragConn{Conn: c, chunks: o.Chunks, delay: o.Delay}, nil
			}
			return nil, lastErr
		},
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: o.Timeout,
	}
	return &Upstream{address: address, url: u, tr: tr, client: &http.Client{Transport: tr}, timeout: o.Timeout}, nil
}

// Address implements upstream.Upstream.
func (u *Upstream) Address() string { return u.address }

// Exchange sends req as an RFC 8484 POST.
func (u *Upstream) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	q := req.Copy()
	q.Id = 0
	body, err := q.Pack()
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/dns-message")
	hreq.Header.Set("Accept", "application/dns-message")
	resp, err := u.client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fragdoh: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65535))
	if err != nil {
		return nil, err
	}
	out := new(dns.Msg)
	if err := out.Unpack(raw); err != nil {
		return nil, err
	}
	out.Id = req.Id
	return out, nil
}

// Close implements upstream.Upstream.
func (u *Upstream) Close() error {
	u.tr.CloseIdleConnections()
	return nil
}
