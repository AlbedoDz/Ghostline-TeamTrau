package store

import "time"

// Meta records when once-a-day background jobs last ran.
type Meta struct {
	LastUpdateCheck time.Time `json:"lastUpdateCheck"`
	LastServerList  time.Time `json:"lastServerList"`
	LastDNSCrypt    time.Time `json:"lastDnsCrypt"`
	// LatestTag/URL remember the newest release seen, so the notice survives
	// restarts between the daily checks.
	LatestTag string `json:"latestTag,omitempty"`
	LatestURL string `json:"latestUrl,omitempty"`
}

// LoadMeta reads meta.json; a missing or unreadable file is a zero Meta.
func LoadMeta(path string) Meta {
	var m Meta
	_ = ReadJSON(path, &m)
	return m
}

// SaveMeta writes meta.json atomically.
func SaveMeta(path string, m Meta) error { return WriteJSONAtomic(path, m) }
