package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"regexp"
	"slices"
	"strconv"
)

// Settings is the user configuration (spec §9).
type Settings struct {
	Version          int              `json:"version"`
	Language         string           `json:"language"`
	Mode             string           `json:"mode"`
	StartWithWindows bool             `json:"startWithWindows"`
	AutoConnect      bool             `json:"autoConnect"`
	CloseToTray      bool             `json:"closeToTray"`
	Adapters         string           `json:"adapters"` // "auto" | "manual"
	AdapterGUIDs     []string         `json:"adapterGuids,omitempty"`
	TestDomain       string           `json:"testDomain"`
	Bootstrap        []string         `json:"bootstrap"`
	MaxUpstreams     int              `json:"maxUpstreams"`
	IncludeTags      []string         `json:"includeTags"`
	Pinned           []string         `json:"pinned"`
	PinnedOnly       bool             `json:"pinnedOnly"`
	ProbeSites       []string         `json:"probeSites"`
	DPI              DPISettings      `json:"dpi"`
	FragmentDNS      FragmentSettings `json:"fragmentDns"`
	Updates          UpdateSettings   `json:"updates"`
	AdvancedWindow   WindowSize       `json:"advancedWindow"`
	Proxy            ProxySettings    `json:"proxy"`
	DNSBlockMode     string           `json:"dnsBlockMode"` // "zero" | "nxdomain"
}

// ProxySettings configures the local proxy (phase 2A).
type ProxySettings struct {
	Enabled     bool            `json:"enabled"`
	Port        int             `json:"port"`
	SystemProxy bool            `json:"systemProxy"`
	ShareLAN    bool            `json:"shareLan"`
	Fragment    WebFragment     `json:"fragment"`
	Upstreams   []UpstreamProxy `json:"upstreams"`
}

// WebFragment configures ClientHello fragmentation for proxied traffic.
type WebFragment struct {
	Mode          string `json:"mode"`   // auto | always | never
	Method        string `json:"method"` // tcp | record | both
	Chunks        int    `json:"chunks"`
	DelayMs       int    `json:"delayMs"`
	AutoTimeoutMs int    `json:"autoTimeoutMs"`
	CacheDays     int    `json:"cacheDays"`
}

// UpstreamProxy is a proxy rules can send traffic through. PassEnc is the
// DPAPI-protected password, base64.
type UpstreamProxy struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // socks5 | http
	Addr    string `json:"addr"` // host:port
	User    string `json:"user"`
	PassEnc string `json:"passEnc"`
}

var upstreamID = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateProxy checks proxy settings against the spec section 9 ranges.
func ValidateProxy(p ProxySettings) error {
	f := p.Fragment
	switch {
	case p.Port < 1024 || p.Port > 65535:
		return fmt.Errorf("proxy: port must be 1024..65535")
	case f.Mode != "auto" && f.Mode != "always" && f.Mode != "never":
		return fmt.Errorf("proxy: fragment mode must be auto, always or never")
	case f.Method != "tcp" && f.Method != "record" && f.Method != "both":
		return fmt.Errorf("proxy: fragment method must be tcp, record or both")
	case f.Chunks < 2 || f.Chunks > 64:
		return fmt.Errorf("proxy: chunks must be 2..64")
	case f.DelayMs < 0 || f.DelayMs > 100:
		return fmt.Errorf("proxy: delayMs must be 0..100")
	case f.AutoTimeoutMs < 1000 || f.AutoTimeoutMs > 10000:
		return fmt.Errorf("proxy: autoTimeoutMs must be 1000..10000")
	case f.CacheDays < 1 || f.CacheDays > 90:
		return fmt.Errorf("proxy: cacheDays must be 1..90")
	}
	seen := map[string]bool{}
	for _, u := range p.Upstreams {
		if !upstreamID.MatchString(u.ID) || seen[u.ID] {
			return fmt.Errorf("proxy: upstream id %q must be unique lower-case letters, digits or '-'", u.ID)
		}
		seen[u.ID] = true
		if u.Type != "socks5" && u.Type != "http" {
			return fmt.Errorf("proxy: upstream %q type must be socks5 or http", u.ID)
		}
		host, port, err := net.SplitHostPort(u.Addr)
		if n, perr := strconv.Atoi(port); err != nil || perr != nil || host == "" || n < 1 || n > 65535 {
			return fmt.Errorf("proxy: upstream %q address must be host:port", u.ID)
		}
	}
	return nil
}

// DPISettings configures the DPI bypass engine. Preset and CustomArgs are
// GoodbyeDPI's (kept at this level for older files); zapret2 has its own.
type DPISettings struct {
	Enabled        bool            `json:"enabled"`
	Engine         string          `json:"engine"` // "goodbyedpi" | "zapret2"
	Preset         string          `json:"preset"`
	CustomArgs     string          `json:"customArgs"`
	Scope          string          `json:"scope"`
	Zapret2        Zapret2Settings `json:"zapret2"`
	HideEngineHint bool            `json:"hideEngineHint"`
}

// Zapret2Settings is the zapret2 engine's part of DPISettings.
type Zapret2Settings struct {
	Strategy     string `json:"strategy"`
	CustomArgs   string `json:"customArgs"`
	AutoHostlist bool   `json:"autoHostlist"`
}

// DPI engine IDs.
const (
	EngineGoodbyeDPI = "goodbyedpi"
	EngineZapret2    = "zapret2"
)

type FragmentSettings struct {
	Enabled bool `json:"enabled"`
	Chunks  int  `json:"chunks"`
	DelayMs int  `json:"delayMs"`
}

type UpdateSettings struct {
	CheckApp         bool `json:"checkApp"`
	UpdateServerList bool `json:"updateServerList"`
}

type WindowSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DefaultSettings returns the spec §9 defaults.
func DefaultSettings() Settings {
	return Settings{
		Version:        3,
		Language:       "vi",
		Mode:           "simple",
		CloseToTray:    true,
		Adapters:       "auto",
		TestDomain:     "www.google.com",
		Bootstrap:      []string{"1.1.1.1:53", "8.8.8.8:53"},
		MaxUpstreams:   5,
		IncludeTags:    []string{"no-filter"},
		Pinned:         []string{},
		ProbeSites:     []string{"youtube.com", "discord.com", "x.com"},
		DPI:            DPISettings{Engine: EngineZapret2, Preset: "light", Scope: "all", Zapret2: Zapret2Settings{Strategy: "z-split"}},
		FragmentDNS:    FragmentSettings{Chunks: 5, DelayMs: 5},
		Updates:        UpdateSettings{CheckApp: true, UpdateServerList: true},
		AdvancedWindow: WindowSize{Width: 1000, Height: 660},
		Proxy: ProxySettings{
			Port:      8080,
			Fragment:  WebFragment{Mode: "auto", Method: "both", Chunks: 5, DelayMs: 5, AutoTimeoutMs: 3000, CacheDays: 7},
			Upstreams: []UpstreamProxy{},
		},
		DNSBlockMode: "zero",
	}
}

// LoadSettings reads settings, filling missing fields with defaults. A file
// that cannot be parsed is renamed to settings.json.bak and defaults are
// returned with recovered=true.
func LoadSettings(path string) (s Settings, recovered bool, err error) {
	s = DefaultSettings()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // tolerate a UTF-8 BOM (Notepad)
	if jerr := json.Unmarshal(b, &s); jerr != nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return DefaultSettings(), true, err
		}
		return DefaultSettings(), true, nil
	}
	// v1 files gain the v2 defaults through the pre-filled struct. Files
	// older than v3 predate zapret2: their users keep GoodbyeDPI.
	var head struct{ Version int }
	_ = json.Unmarshal(b, &head)
	if head.Version < 3 {
		s.DPI.Engine = EngineGoodbyeDPI
	}
	if s.DPI.Engine != EngineGoodbyeDPI && s.DPI.Engine != EngineZapret2 {
		s.DPI.Engine = EngineZapret2
	}
	s.Version = 3
	if s.Proxy.Upstreams == nil {
		s.Proxy.Upstreams = []UpstreamProxy{}
	}
	if slices.Equal(s.ProbeSites, oldProbeSites) {
		s.ProbeSites = DefaultSettings().ProbeSites
	}
	return s, false, nil
}

// oldProbeSites is the default test-site list before v0.2.5. Files that
// still hold it untouched move to the current default; edited lists stay.
var oldProbeSites = []string{"youtube.com", "discord.com", "telegram.org", "x.com"}

// SaveSettings writes settings atomically.
func SaveSettings(path string, s Settings) error {
	return WriteJSONAtomic(path, s)
}
