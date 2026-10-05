package lists

import (
	_ "embed"
	"encoding/json"
)

// CatalogItem is a link in the "Quick add" catalog. Ghostline ships only the
// link; the list is downloaded from its own repository at run time.
type CatalogItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"` // English fallback; the UI translates by ID
	Category    string `json:"category"`
	Repo        string `json:"repo"`
	License     string `json:"license"`
	URL         string `json:"url"`
	Format      string `json:"format"`
	Action      string `json:"action"`
	// Signed items are verified with Ghostline's list key; TrustedForSNI
	// lets their sni= and connect= take effect (Fake SNI presets).
	Signed        bool `json:"signed,omitempty"`
	TrustedForSNI bool `json:"trustedForSNI,omitempty"`
}

// Categories are the catalog groups, in display order.
var Categories = []string{"ads", "security", "adult", "gambling", "social", "telemetry", "vietnam", "bypass", "fakesni"}

//go:embed catalog.json
var catalogJSON []byte

// Catalog returns the embedded quick-add catalog.
func Catalog() []CatalogItem {
	var items []CatalogItem
	if err := json.Unmarshal(catalogJSON, &items); err != nil {
		panic("lists: bad catalog.json: " + err.Error())
	}
	return items
}
