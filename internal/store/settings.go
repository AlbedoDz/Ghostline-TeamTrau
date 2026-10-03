package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
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
}

type DPISettings struct {
	Enabled    bool   `json:"enabled"`
	Preset     string `json:"preset"`
	CustomArgs string `json:"customArgs"`
	Scope      string `json:"scope"`
}

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
		Version:        1,
		Language:       "vi",
		Mode:           "simple",
		CloseToTray:    true,
		Adapters:       "auto",
		TestDomain:     "www.google.com",
		Bootstrap:      []string{"1.1.1.1:53", "8.8.8.8:53"},
		MaxUpstreams:   5,
		IncludeTags:    []string{"no-filter"},
		Pinned:         []string{},
		ProbeSites:     []string{"youtube.com", "discord.com", "telegram.org", "x.com"},
		DPI:            DPISettings{Preset: "light", Scope: "all"},
		FragmentDNS:    FragmentSettings{Chunks: 5, DelayMs: 5},
		Updates:        UpdateSettings{CheckApp: true, UpdateServerList: true},
		AdvancedWindow: WindowSize{Width: 1000, Height: 660},
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
	if jerr := json.Unmarshal(b, &s); jerr != nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return DefaultSettings(), true, err
		}
		return DefaultSettings(), true, nil
	}
	return s, false, nil
}

// SaveSettings writes settings atomically.
func SaveSettings(path string, s Settings) error {
	return WriteJSONAtomic(path, s)
}
