// Package updater checks for new Ghostline releases and fetches signed
// server lists.
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/servers"
	"golang.org/x/mod/semver"
)

// Release is the latest published version.
type Release struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

// Latest reads tag_name and html_url from the GitHub releases API.
func Latest(ctx context.Context, c *http.Client, apiURL string) (Release, error) {
	b, err := get(ctx, c, apiURL, 1<<20)
	if err != nil {
		return Release{}, err
	}
	var v struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return Release{}, err
	}
	return Release{Tag: v.Tag, URL: v.URL}, nil
}

func canon(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Newer reports whether tag is a newer semver than current. Dev builds and
// unparsable tags never report an update.
func Newer(current, tag string) bool {
	c, t := canon(current), canon(tag)
	if !semver.IsValid(c) || !semver.IsValid(t) {
		return false
	}
	return semver.Compare(t, c) > 0
}

// Due reports whether a daily job last run at last should run again.
func Due(last, now time.Time) bool { return now.Sub(last) >= 24*time.Hour }

func get(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Ghostline")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// FetchServerList downloads a list and its ed25519 signature and verifies it.
func FetchServerList(ctx context.Context, c *http.Client, url, sigURL string, pub ed25519.PublicKey) (servers.List, []byte, []byte, error) {
	raw, err := get(ctx, c, url, 8<<20)
	if err != nil {
		return servers.List{}, nil, nil, err
	}
	sig, err := get(ctx, c, sigURL, 4096)
	if err != nil {
		return servers.List{}, nil, nil, err
	}
	if err := servers.VerifySigned(raw, sig, pub); err != nil {
		return servers.List{}, nil, nil, err
	}
	l, err := servers.ParseList(raw)
	if err != nil {
		return servers.List{}, nil, nil, err
	}
	return l, raw, sig, nil
}

// FetchDNSCrypt tries each URL until one serves a list whose .minisig
// verifies with minisignKey.
func FetchDNSCrypt(ctx context.Context, c *http.Client, urls []string, minisignKey string) (md, sig []byte, err error) {
	var errs []error
	for _, u := range urls {
		md, err := get(ctx, c, u, 16<<20)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		sig, err := get(ctx, c, u+".minisig", 4096)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := servers.VerifyMinisign(md, sig, minisignKey); err != nil {
			errs = append(errs, fmt.Errorf("%s: bad signature: %w", u, err))
			continue
		}
		return md, sig, nil
	}
	return nil, nil, errors.Join(errs...)
}
