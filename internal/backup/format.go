// Package backup writes and reads Ghostline's settings export file (spec 3 §9).
package backup

import (
	"encoding/json"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
)

const (
	Format        = "ghostline-backup"
	FormatVersion = 1
	MaxSize       = 8 << 20
)

// Section names.
const (
	SecSettings     = "settings"
	SecRules        = "rules"
	SecCustom       = "customServers"
	SecBlacklist    = "dpiBlacklist"
	SecAutoHostlist = "dpiAutoHostlist"
)

// AllSections lists every section in file order.
var AllSections = []string{SecSettings, SecRules, SecCustom, SecBlacklist, SecAutoHostlist}

// File is the export file.
type File struct {
	Format        string    `json:"format"`
	FormatVersion int       `json:"formatVersion"`
	AppVersion    string    `json:"appVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	Sections      Sections  `json:"sections"`
}

// Sections holds the exported data; absent sections were not exported.
type Sections struct {
	Settings        json.RawMessage  `json:"settings,omitempty"`
	Rules           *store.RulesFile `json:"rules,omitempty"`
	CustomServers   []model.Server   `json:"customServers,omitempty"`
	DPIBlacklist    *string          `json:"dpiBlacklist,omitempty"`
	DPIAutoHostlist []string         `json:"dpiAutoHostlist,omitempty"`
}

// Data is everything a backup can hold, as live values.
type Data struct {
	Settings     store.Settings
	Rules        store.RulesFile
	Custom       []model.Server
	Blacklist    string
	AutoHostlist []string
}

// FileName is the suggested name for an export made at now.
func FileName(now time.Time) string {
	return "ghostline-" + now.Format("2006-01-02") + ".ghostline.json"
}
