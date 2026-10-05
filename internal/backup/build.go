package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
)

// Build renders the chosen sections of d as an export file. Machine-bound
// and private values are never written (spec 3 §9.2).
func Build(d Data, sections []string, appVersion string, now time.Time) ([]byte, error) {
	if len(sections) == 0 {
		return nil, errors.New("backup: no section chosen")
	}
	f := File{Format: Format, FormatVersion: FormatVersion, AppVersion: appVersion, CreatedAt: now}
	for _, sec := range sections {
		switch sec {
		case SecSettings:
			raw, err := redactSettings(d.Settings)
			if err != nil {
				return nil, err
			}
			f.Sections.Settings = raw
		case SecRules:
			rf := store.RulesFile{Version: d.Rules.Version, Rules: d.Rules.Rules, Lists: exportLists(d.Rules.Lists)}
			f.Sections.Rules = &rf
		case SecCustom:
			f.Sections.CustomServers = slices.Clone(d.Custom)
		case SecBlacklist:
			b := d.Blacklist
			f.Sections.DPIBlacklist = &b
		case SecAutoHostlist:
			f.Sections.DPIAutoHostlist = slices.Clone(d.AutoHostlist)
		default:
			return nil, fmt.Errorf("backup: unknown section %q", sec)
		}
	}
	return json.MarshalIndent(f, "", "  ")
}

// redactSettings drops machine-bound fields and blanks proxy passwords
// (DPAPI-encrypted for this machine, useless elsewhere).
func redactSettings(s store.Settings) (json.RawMessage, error) {
	s.Proxy.Upstreams = slices.Clone(s.Proxy.Upstreams)
	for i := range s.Proxy.Upstreams {
		s.Proxy.Upstreams[i].PassEnc = ""
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	delete(m, "adapterGuids")
	delete(m, "advancedWindow")
	if ds, ok := m["dnsServer"].(map[string]any); ok {
		delete(ds, "iosSsid")
	}
	return json.Marshal(m)
}

// exportLists keeps URL lists' configuration only. File lists point at a
// local path (often under the user's profile) and are left out.
func exportLists(ls []lists.List) []lists.List {
	out := []lists.List{}
	for _, l := range ls {
		if l.Source == "file" {
			continue
		}
		out = append(out, lists.List{
			ID: l.ID, Name: l.Name, Source: l.Source, URL: l.URL, Format: l.Format, Action: l.Action,
			Enabled: l.Enabled, UpdateHours: l.UpdateHours, TrustedForSNI: l.TrustedForSNI, Signed: l.Signed,
		})
	}
	return out
}
