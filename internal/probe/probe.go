// Package probe checks whether sites are reachable and, when not, at which
// stage they fail. A TLS-stage failure after DNS and TCP succeed is the
// signature of SNI/DPI blocking.
package probe

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// Stage is where a probe stopped.
type Stage string

const (
	StageOK   Stage = "ok"
	StageDNS  Stage = "dns"
	StageTCP  Stage = "tcp"
	StageTLS  Stage = "tls"
	StageHTTP Stage = "http"
)

// Result is one site's probe outcome.
type Result struct {
	Site    string        `json:"site"`
	Stage   Stage         `json:"stage"`
	Latency time.Duration `json:"latency"`
	Err     string        `json:"err,omitempty"`
}

// Prober probes HTTPS sites the way a browser reaches them.
type Prober struct {
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
	Timeout time.Duration
	RootCAs *x509.CertPool
	Port    string // default "443"
}

// Probe runs resolve → TCP → TLS → GET / and reports the first failing stage.
func (p Prober) Probe(ctx context.Context, site string) Result {
	start := time.Now()
	r := Result{Site: site}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	fail := func(s Stage, err error) Result {
		r.Stage, r.Err, r.Latency = s, err.Error(), time.Since(start)
		return r
	}

	ips, err := p.Resolve(ctx, site)
	if err != nil || len(ips) == 0 {
		if err == nil {
			err = fmt.Errorf("no addresses")
		}
		return fail(StageDNS, err)
	}
	port := p.Port
	if port == "" {
		port = "443"
	}
	conn, err := p.Dial(ctx, "tcp", net.JoinHostPort(ips[0].String(), port))
	if err != nil {
		return fail(StageTCP, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tc := tls.Client(conn, &tls.Config{ServerName: site, RootCAs: p.RootCAs, NextProtos: []string{"http/1.1"}})
	if err := tc.HandshakeContext(ctx); err != nil {
		return fail(StageTLS, err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+site+"/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Ghostline-probe")
	req.Header.Set("Connection", "close")
	if err := req.Write(tc); err != nil {
		return fail(StageHTTP, err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(tc), req)
	if err != nil {
		return fail(StageHTTP, err)
	}
	resp.Body.Close()
	r.Stage, r.Latency = StageOK, time.Since(start)
	return r
}

// ProbeAll probes sites concurrently; results keep the input order.
func (p Prober) ProbeAll(ctx context.Context, sites []string) []Result {
	out := make([]Result, len(sites))
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = p.Probe(ctx, s)
		}()
	}
	wg.Wait()
	return out
}

// DPIBlocked lists sites that failed at the TLS stage in both attempts.
func DPIBlocked(first, second []Result) []string {
	tls2 := map[string]bool{}
	for _, r := range second {
		if r.Stage == StageTLS {
			tls2[r.Site] = true
		}
	}
	var out []string
	for _, r := range first {
		if r.Stage == StageTLS && tls2[r.Site] {
			out = append(out, r.Site)
		}
	}
	return out
}
