package cfscan

import (
	"cmp"
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
)

// Options control a scan.
type Options struct {
	Concurrency int
	Want        int // stop after this many OK results; 0 = probe every IP
	Limiter     Limiter
	OnProgress  func(tried, ok int, r Result)
}

// ErrNoNetwork means the first noNetworkAfter probes all failed at TCP.
var ErrNoNetwork = errors.New("cfscan: no network")

const noNetworkAfter = 200

// Scan probes ips (only those inside Ranges) and returns every finished
// result, sorted. A cancelled scan returns what finished before.
func Scan(ctx context.Context, ips []netip.Addr, p Prober, o Options) ([]Result, error) {
	if o.Concurrency <= 0 {
		o.Concurrency = 64
	}
	if o.Limiter == nil {
		o.Limiter = NewLimiter(200)
	}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan netip.Addr)
	var (
		mu       sync.Mutex
		out      []Result
		ok       int
		tcpFails int
		noNet    bool
		wg       sync.WaitGroup
	)
	for range o.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				if o.Limiter.Wait(ctx) != nil {
					continue
				}
				r := p.Probe(ctx, ip)
				if ctx.Err() != nil {
					continue // cut short by cancel; not a real result
				}
				mu.Lock()
				out = append(out, r)
				if r.OK {
					ok++
				} else if strings.HasPrefix(r.Reason, "tcp_") && len(out) <= noNetworkAfter {
					tcpFails++
				}
				if len(out) == noNetworkAfter && tcpFails == noNetworkAfter {
					noNet = true
					cancel()
				}
				if o.Want > 0 && ok >= o.Want {
					cancel()
				}
				if o.OnProgress != nil {
					o.OnProgress(len(out), ok, r)
				}
				mu.Unlock()
			}
		}()
	}
	rs := Ranges()
feed:
	for _, ip := range ips {
		if !Contains(rs, ip) {
			continue
		}
		select {
		case jobs <- ip:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if noNet || (parent.Err() == nil && len(out) > 0 && len(out) < noNetworkAfter && tcpFails == len(out)) {
		return out, ErrNoNetwork
	}
	if o.Want > 0 && ok > o.Want {
		out = trimOK(out, o.Want)
	}
	Sort(out)
	return out, nil
}

// trimOK drops OK results past the first want (finished concurrently).
func trimOK(rs []Result, want int) []Result {
	n := 0
	return slices.DeleteFunc(rs, func(r Result) bool {
		if !r.OK {
			return false
		}
		n++
		return n > want
	})
}

// Sort puts OK results first: faster download first when both were measured,
// then lower latency.
func Sort(rs []Result) {
	slices.SortStableFunc(rs, func(a, b Result) int {
		if a.OK != b.OK {
			if a.OK {
				return -1
			}
			return 1
		}
		return cmp.Or(
			cmp.Compare(b.Mbps, a.Mbps),
			cmp.Compare(a.LatencyMs, b.LatencyMs),
			strings.Compare(a.IP, b.IP),
		)
	})
}
