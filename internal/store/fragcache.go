package store

import (
	"errors"
	"io/fs"
	"sort"
	"sync"
	"time"
)

// FragCache remembers, per network, which hosts needed a fragmented
// ClientHello, until an expiry time (frag-cache.json).
type FragCache struct {
	mu       sync.Mutex
	networks map[string]map[string]time.Time
}

type fragCacheFile struct {
	Version  int                             `json:"version"`
	Networks map[string]map[string]time.Time `json:"networks"`
}

// LoadFragCache reads the cache, dropping expired entries. A missing or
// unreadable file gives an empty cache.
func LoadFragCache(path string, now time.Time) (*FragCache, error) {
	c := &FragCache{networks: map[string]map[string]time.Time{}}
	var f fragCacheFile
	if err := ReadJSON(path, &f); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return c, err
	}
	for k, hosts := range f.Networks {
		for h, until := range hosts {
			if until.After(now) {
				if c.networks[k] == nil {
					c.networks[k] = map[string]time.Time{}
				}
				c.networks[k][h] = until
			}
		}
	}
	return c, nil
}

// Has reports whether host needs fragmenting on network netKey.
func (c *FragCache) Has(netKey, host string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	until, ok := c.networks[netKey][host]
	return ok && until.After(now)
}

// Add remembers host on netKey until the given time.
func (c *FragCache) Add(netKey, host string, until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.networks[netKey] == nil {
		c.networks[netKey] = map[string]time.Time{}
	}
	c.networks[netKey][host] = until
}

// List returns the hosts remembered for netKey, sorted.
func (c *FragCache) List(netKey string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for h := range c.networks[netKey] {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Remove forgets host on netKey; host "" forgets the whole network.
func (c *FragCache) Remove(netKey, host string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if host == "" {
		delete(c.networks, netKey)
		return
	}
	delete(c.networks[netKey], host)
}

// Save writes the cache atomically.
func (c *FragCache) Save(path string) error {
	c.mu.Lock()
	f := fragCacheFile{Version: 1, Networks: map[string]map[string]time.Time{}}
	for k, hosts := range c.networks {
		m := make(map[string]time.Time, len(hosts))
		for h, t := range hosts {
			m[h] = t
		}
		f.Networks[k] = m
	}
	c.mu.Unlock()
	return WriteJSONAtomic(path, f)
}
