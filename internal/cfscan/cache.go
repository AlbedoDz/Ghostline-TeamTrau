package cfscan

import (
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/store"
)

// cacheKeep is how many good IPs are kept per network.
const cacheKeep = 100

// CacheEntry is one network's last scan.
type CacheEntry struct {
	ScannedAt time.Time `json:"scannedAt"`
	Host      string    `json:"host"`
	Results   []Result  `json:"results"`
}

// Cache maps network keys (scanner.NetworkKey) to their last scan.
type Cache struct {
	Entries map[string]CacheEntry `json:"entries"`
}

// Put stores the best cacheKeep OK results of a scan for key.
func (c *Cache) Put(key string, now time.Time, host string, rs []Result) {
	ok := slices.DeleteFunc(slices.Clone(rs), func(r Result) bool { return !r.OK })
	Sort(ok)
	if len(ok) > cacheKeep {
		ok = ok[:cacheKeep]
	}
	if c.Entries == nil {
		c.Entries = map[string]CacheEntry{}
	}
	c.Entries[key] = CacheEntry{ScannedAt: now, Host: host, Results: ok}
}

// Get returns key's last scan.
func (c *Cache) Get(key string) (CacheEntry, bool) {
	e, ok := c.Entries[key]
	return e, ok
}

// LoadCache reads the cache; a missing or corrupt file gives an empty one.
func LoadCache(path string) *Cache {
	c := &Cache{}
	if err := store.ReadJSON(path, c); err != nil || c.Entries == nil {
		return &Cache{Entries: map[string]CacheEntry{}}
	}
	return c
}

// SaveCache writes the cache atomically.
func SaveCache(path string, c *Cache) error { return store.WriteJSONAtomic(path, c) }
