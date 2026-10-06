package backup_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)

func sampleData() backup.Data {
	s := store.DefaultSettings()
	s.AdapterGUIDs = []string{"{GUID-1}"}
	s.FullWindow = store.WindowSize{Width: 1234, Height: 777}
	s.DNSServer.IOSSSID = "HomeWiFi"
	s.Pinned = []string{"cf"}
	s.Proxy.Upstreams = []store.UpstreamProxy{{ID: "corp", Type: "socks5", Addr: "10.0.0.1:1080", User: "me", PassEnc: "secret"}}
	return backup.Data{
		Settings: s,
		Rules: store.RulesFile{Version: 1, Rules: []rules.Rule{{Pattern: "a.com", Action: rules.Action{Block: true}, Enabled: true}},
			Lists: []lists.List{
				{ID: "ads", Name: "Ads", Source: "url", URL: "https://x/ads.txt", Format: "auto", Action: "block", Enabled: true, UpdateHours: 24,
					LastUpdated: now, ETag: "e", LastModified: "m", Detected: "hosts", Counts: map[string]int{"domain": 3}, Skipped: 2,
					SkippedSamples: []string{"z"}, LastError: "boom", SignatureOK: true},
				{ID: "mine", Name: "Mine", Source: "file", Path: `C:\Users\ducha\mine.txt`, Format: "auto", Action: "block", Enabled: true},
			}},
		Custom:       []model.Server{{ID: "c1", Name: "C", Protocol: model.ProtoDoH, Address: "https://c.example/dns-query", Source: model.SourceCustom}},
		Blacklist:    "youtube.com\n",
		AutoHostlist: []string{"discord.com"},
	}
}

func TestBuild_Redacts(t *testing.T) {
	b, err := backup.Build(sampleData(), backup.AllSections, "0.5.0", now)
	require.NoError(t, err)
	s := string(b)
	for _, banned := range []string{"adapterGuids", "fullWindow", "advancedWindow", "HomeWiFi", "iosSsid", "secret", "GUID-1", "1234", `ducha`} {
		require.NotContains(t, s, banned)
	}
	var f backup.File
	require.NoError(t, json.Unmarshal(b, &f))
	require.Equal(t, "ghostline-backup", f.Format)
	require.Equal(t, 1, f.FormatVersion)
	require.Equal(t, "0.5.0", f.AppVersion)
	require.Equal(t, now, f.CreatedAt)
	require.Contains(t, s, `"passEnc": ""`)
	require.Contains(t, s, `"user": "me"`)
	require.Contains(t, s, `"pinned": [`)
}

func TestBuild_ListsWithoutRuntimeState(t *testing.T) {
	b, err := backup.Build(sampleData(), []string{"rules"}, "0.5.0", now)
	require.NoError(t, err)
	var f backup.File
	require.NoError(t, json.Unmarshal(b, &f))
	require.Len(t, f.Sections.Rules.Lists, 1, "file lists name local paths and are not exported")
	l := f.Sections.Rules.Lists[0]
	require.Equal(t, lists.List{ID: "ads", Name: "Ads", Source: "url", URL: "https://x/ads.txt", Format: "auto", Action: "block", Enabled: true, UpdateHours: 24}, l)
	require.Len(t, f.Sections.Rules.Rules, 1)
}

func TestBuild_PickSections(t *testing.T) {
	b, err := backup.Build(sampleData(), []string{"rules"}, "0.5.0", now)
	require.NoError(t, err)
	s := string(b)
	require.Contains(t, s, `"rules"`)
	for _, k := range []string{`"settings"`, `"customServers"`, `"dpiBlacklist"`, `"dpiAutoHostlist"`} {
		require.NotContains(t, s, k)
	}
	b, err = backup.Build(sampleData(), backup.AllSections, "0.5.0", now)
	require.NoError(t, err)
	for _, k := range []string{`"settings"`, `"rules"`, `"customServers"`, `"dpiBlacklist"`, `"dpiAutoHostlist"`} {
		require.True(t, strings.Contains(string(b), k), k)
	}
}

func TestBuild_UnknownSection(t *testing.T) {
	_, err := backup.Build(sampleData(), []string{"state"}, "0.5.0", now)
	require.Error(t, err)
	_, err = backup.Build(sampleData(), nil, "0.5.0", now)
	require.Error(t, err)
}

func TestFileName(t *testing.T) {
	require.Equal(t, "ghostline-2026-10-06.ghostline.json", backup.FileName(now))
}
