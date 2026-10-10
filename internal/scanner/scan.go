// Package scanner checks DNS servers in parallel and caches results per network.
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/miekg/dns"
)

// Result is the outcome of checking one server.
type Result struct {
	ServerID  string        `json:"serverId"`
	OK        bool          `json:"ok"`
	Latency   time.Duration `json:"latency"`
	Jitter    time.Duration `json:"jitter,omitempty"`
	Reason    string        `json:"reason,omitempty"` // "" | timeout | poisoned | rcode:<NAME> | empty | error
	CheckedAt time.Time     `json:"checkedAt"`
}

// Checker checks one server.
type Checker interface {
	Check(ctx context.Context, s model.Server) Result
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"), netip.MustParsePrefix("2001:db8::/32"),
}

// IsPublicIP reports whether a is a globally routable unicast address.
// Poisoned DNS answers typically point at private or reserved ranges.
func IsPublicIP(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	return a.IsValid()
}

// DNSChecker checks a server by resolving TestDomain twice; the second
// query's duration is the latency (the first pays for the TLS handshake).
type DNSChecker struct {
	Build      func(model.Server) (upstream.Upstream, error)
	TestDomain string
	// Domains, when set, gives the test domains at each check (the setting
	// can change while the app runs); TestDomain is used otherwise. Every
	// one must pass; the first is asked twice and timed.
	Domains func() []string
	Timeout time.Duration
	Now     func() time.Time
}

// Check implements Checker.
func (c DNSChecker) Check(ctx context.Context, s model.Server) Result {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	r := Result{ServerID: s.ID, CheckedAt: now()}
	u, err := c.Build(s)
	if err != nil {
		r.Reason = "error"
		return r
	}
	defer u.Close()
	var lat time.Duration
	// One budget covers both queries (spec: 3s per server).
	domains := []string{c.TestDomain}
	if c.Domains != nil {
		if ds := c.Domains(); len(ds) > 0 {
			domains = ds
		}
	}
	// The first domain twice (the second is timed), then one query for each
	// other; each other domain adds a second to the budget.
	qctx, cancel := context.WithTimeout(ctx, c.Timeout+time.Duration(len(domains)-1)*time.Second)
	defer cancel()
	queries := append([]string{domains[0]}, domains...)
	var firstLat time.Duration
	for i, domain := range queries {
		fail := func(reason string) Result {
			if i >= 2 {
				reason = domain + ": " + reason
			}
			r.Reason = reason
			return r
		}
		resp, d, err := Exchange(qctx, u, domain, dns.TypeA, false)
		if i == 0 {
			firstLat = d
		} else if i == 1 {
			lat = d
			if firstLat > d {
				r.Jitter = firstLat - d
			} else {
				r.Jitter = d - firstLat
			}
		}
		if err != nil {
			return fail(Classify(err, qctx.Err()))
		}
		if resp.Rcode != dns.RcodeSuccess {
			return fail("rcode:" + dns.RcodeToString[resp.Rcode])
		}
		var any bool
		for _, rr := range resp.Answer {
			a, ok := rr.(*dns.A)
			if !ok {
				continue
			}
			any = true
			ip, _ := netip.AddrFromSlice(a.A.To4())
			if !IsPublicIP(ip) {
				return fail("poisoned")
			}
		}
		if !any {
			return fail("empty")
		}
		if i == 1 {
			r.Latency = lat // kept when another domain fails (see ForgiveBrokenDomains)
		}
	}
	r.OK = true
	return r
}

// ForgiveBrokenDomains finds the other test domains (after the first) that
// failed on at least half of the servers which answered the first one: the
// domain is at fault then (no address, a typo), not the servers. Those
// servers count as passing. It returns the results and the broken domains.
func ForgiveBrokenDomains(rs []Result, domains []string) ([]Result, []string) {
	if len(domains) < 2 {
		return rs, nil
	}
	failedOn := func(r Result, d string) bool { return strings.HasPrefix(r.Reason, d+": ") }
	answeredFirst, fails := 0, map[string]int{}
	for _, r := range rs {
		ok := r.OK
		for _, d := range domains[1:] {
			if failedOn(r, d) {
				fails[d]++
				ok = true
			}
		}
		if ok {
			answeredFirst++
		}
	}
	var broken []string
	for _, d := range domains[1:] {
		if answeredFirst >= 3 && fails[d]*2 >= answeredFirst {
			broken = append(broken, d)
		}
	}
	if len(broken) == 0 {
		return rs, nil
	}
	out := slices.Clone(rs)
	for i, r := range out {
		for _, d := range broken {
			if failedOn(r, d) {
				out[i].OK, out[i].Reason = true, ""
			}
		}
	}
	return out, broken
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return (errors.As(err, &t) && t.Timeout()) || strings.Contains(err.Error(), "timeout")
}

// Options controls a scan.
type Options struct {
	Workers    int           // parallel checks (default 16)
	Want       int           // stop after this many OK results; 0 = check all
	Budget     time.Duration // stop after this long; 0 = no limit
	OnProgress func(done, total int, r Result)
}

// Scan checks servers with a shared work queue and returns results sorted
// OK first, then by latency. Servers not reached before Want/Budget are
// omitted.
func Scan(ctx context.Context, list []model.Server, c Checker, o Options) []Result {
	if o.Workers <= 0 {
		o.Workers = 16
	}
	if o.Budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Budget)
		defer cancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan model.Server)
	var mu sync.Mutex
	var out []Result
	okCount, done := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < o.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				r := c.Check(ctx, s)
				mu.Lock()
				if ctx.Err() == nil || r.OK {
					out = append(out, r)
					done++
					if r.OK {
						okCount++
					}
					if o.OnProgress != nil {
						o.OnProgress(done, len(list), r)
					}
					if o.Want > 0 && okCount >= o.Want {
						cancel()
					}
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, s := range list {
		select {
		case jobs <- s:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	slices.SortStableFunc(out, func(a, b Result) int {
		switch {
		case a.OK != b.OK:
			if a.OK {
				return -1
			}
			return 1
		case a.OK:
			scoreA := a.Latency + a.Jitter/2
			scoreB := b.Latency + b.Jitter/2
			if scoreA != scoreB {
				if scoreA < scoreB {
					return -1
				}
				return 1
			}
		}
		return strings.Compare(a.ServerID, b.ServerID)
	})
	if o.Want > 0 {
		// Keep exactly Want OK results when more finished concurrently.
		n := 0
		trimmed := out[:0]
		for _, r := range out {
			if r.OK {
				if n >= o.Want {
					continue
				}
				n++
			}
			trimmed = append(trimmed, r)
		}
		out = trimmed
	}
	return out
}

// Order puts previously good servers first (stable by ID) and shuffles the rest.
func Order(list []model.Server, everOK map[string]bool, r *rand.Rand) []model.Server {
	var good, rest []model.Server
	for _, s := range list {
		if everOK[s.ID] {
			good = append(good, s)
		} else {
			rest = append(rest, s)
		}
	}
	slices.SortStableFunc(good, func(a, b model.Server) int { return strings.Compare(a.ID, b.ID) })
	r.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	return append(good, rest...)
}

// CacheEntry is one network's last scan.
type CacheEntry struct {
	ScannedAt time.Time `json:"scannedAt"`
	// FullAt is when a scan of the whole list last finished (zero: never).
	FullAt  time.Time `json:"fullAt,omitempty"`
	Results []Result  `json:"results"`
}

// Cache maps network keys to their last scan.
type Cache struct {
	Entries map[string]CacheEntry `json:"entries"`
}

// NetworkKey identifies the current network by its default gateway.
func NetworkKey(gatewayIP, gatewayMAC string) string {
	sum := sha256.Sum256([]byte(gatewayIP + "|" + gatewayMAC))
	return hex.EncodeToString(sum[:])
}

// Fresh returns the network's results checked within ttl (by each result's
// CheckedAt; results without one use the entry's scan time). ok is false
// when none is fresh.
func (c *Cache) Fresh(key string, now time.Time, ttl time.Duration) ([]Result, bool) {
	e, ok := c.Entries[key]
	if !ok {
		return nil, false
	}
	var out []Result
	for _, r := range e.Results {
		at := r.CheckedAt
		if at.IsZero() {
			at = e.ScannedAt
		}
		if now.Sub(at) <= ttl {
			out = append(out, r)
		}
	}
	return out, len(out) > 0
}

// Put replaces the network's results (a full, completed scan).
func (c *Cache) Put(key string, now time.Time, rs []Result) {
	if c.Entries == nil {
		c.Entries = map[string]CacheEntry{}
	}
	c.Entries[key] = CacheEntry{ScannedAt: now, Results: rs}
}

// Merge records the results of a partial scan (quick scan, pinned servers,
// a single re-check): each server's result is replaced, the others kept.
func (c *Cache) Merge(key string, now time.Time, rs []Result) {
	if c.Entries == nil {
		c.Entries = map[string]CacheEntry{}
	}
	e := c.Entries[key]
	idx := make(map[string]int, len(e.Results))
	for i, r := range e.Results {
		idx[r.ServerID] = i
	}
	for _, r := range rs {
		if r.CheckedAt.IsZero() {
			r.CheckedAt = now
		}
		if i, ok := idx[r.ServerID]; ok {
			e.Results[i] = r
		} else {
			idx[r.ServerID] = len(e.Results)
			e.Results = append(e.Results, r)
		}
	}
	e.ScannedAt = now
	c.Entries[key] = e
}

// MarkFull records that a scan of the whole list finished now.
func (c *Cache) MarkFull(key string, now time.Time) {
	if e, ok := c.Entries[key]; ok {
		e.FullAt = now
		c.Entries[key] = e
	}
}

// EverOK lists servers that were OK on any network.
func (c *Cache) EverOK() map[string]bool {
	out := map[string]bool{}
	for _, e := range c.Entries {
		for _, r := range e.Results {
			if r.OK {
				out[r.ServerID] = true
			}
		}
	}
	return out
}

// LoadCache reads the cache; a missing or corrupt file gives an empty cache.
func LoadCache(path string) (*Cache, error) {
	c := &Cache{Entries: map[string]CacheEntry{}}
	if err := store.ReadJSON(path, c); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("scanner: loading the scan cache failed; starting empty", "file", filepath.Base(path), "err", err)
		}
		return &Cache{Entries: map[string]CacheEntry{}}, nil
	}
	if c.Entries == nil {
		c.Entries = map[string]CacheEntry{}
	}
	return c, nil
}

// SaveCache writes the cache atomically.
func SaveCache(path string, c *Cache) error {
	err := store.WriteJSONAtomic(path, c)
	if err != nil {
		slog.Warn("scanner: saving the scan cache failed", "file", filepath.Base(path), "err", err)
	}
	return err
}
