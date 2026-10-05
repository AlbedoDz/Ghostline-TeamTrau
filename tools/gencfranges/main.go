// Command gencfranges refreshes the Cloudflare IPv4 ranges embedded in
// internal/cfscan (run before each release).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"
)

const source = "https://www.cloudflare.com/ips-v4"

func main() {
	out := flag.String("out", "internal/cfscan/ranges_v4.txt", "output file")
	flag.Parse()
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(source)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("%s: %s", source, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Fatal(err)
	}
	ps, err := parse(body)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, render(ps, time.Now().UTC().Format("2006-01-02")), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d ranges to %s\n", len(ps), *out)
}

// parse reads one IPv4 CIDR per line, rejecting anything else and prefixes
// shorter than /8, and returns them sorted.
func parse(body []byte) ([]netip.Prefix, error) {
	var ps []netip.Prefix
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p, err := netip.ParsePrefix(line)
		if err != nil {
			return nil, fmt.Errorf("bad line %q: %w", line, err)
		}
		if !p.Addr().Is4() || p.Bits() < 8 {
			return nil, fmt.Errorf("unexpected range %q", line)
		}
		ps = append(ps, p.Masked())
	}
	if len(ps) == 0 {
		return nil, errors.New("no ranges")
	}
	slices.SortFunc(ps, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return ps, nil
}

func render(ps []netip.Prefix, date string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# source: %s (fetched %s)\n", source, date)
	for _, p := range ps {
		b.WriteString(p.String() + "\n")
	}
	return b.Bytes()
}
