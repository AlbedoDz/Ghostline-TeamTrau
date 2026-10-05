package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func defaultTools() store.ToolsSettings {
	return store.ToolsSettings{
		Scanner: store.ScannerTool{Rounds: 5, Workers: 8, TimeoutMs: 3000},
		CFScan: store.CFScanTool{Host: "speed.cloudflare.com", MaxIPs: 2000, Want: 50, Concurrency: 64,
			TimeoutMs: 2000, SpeedTest: true, SpeedBytes: 1048576},
	}
}

func TestSettingsV4ToV5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":4,"language":"en","dpi":{"engine":"zapret2","preset":"medium"},"dnsServer":{"dohPort":8443}}`), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Equal(t, 5, s.Version)
	require.Equal(t, defaultTools(), s.Tools)
	require.Equal(t, "en", s.Language)
	require.Equal(t, "medium", s.DPI.Preset)
	require.Equal(t, 8443, s.DNSServer.DoHPort)
	require.Equal(t, 5, store.DefaultSettings().Version)
	require.Equal(t, defaultTools(), store.DefaultSettings().Tools)
}

func TestSettingsV5_KeepsTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := store.DefaultSettings()
	s.Tools.Scanner.Rounds = 9
	s.Tools.CFScan.Host = "example.com"
	s.Tools.CFScan.SpeedTest = false
	require.NoError(t, store.SaveSettings(path, s))
	got, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, s.Tools, got.Tools)
}

func TestMigrateSettings_V1ToV5(t *testing.T) {
	s, err := store.MigrateSettings([]byte("\xEF\xBB\xBF" + `{"version":1,"language":"en"}`))
	require.NoError(t, err)
	require.Equal(t, 5, s.Version)
	require.Equal(t, store.EngineGoodbyeDPI, s.DPI.Engine)
	require.Equal(t, defaultTools(), s.Tools)
}

func TestMigrateSettings_BadJSON(t *testing.T) {
	_, err := store.MigrateSettings([]byte(`{"version":`))
	require.Error(t, err)
}

func TestValidateTools(t *testing.T) {
	require.NoError(t, store.ValidateTools(defaultTools()))
	bad := []func(*store.ToolsSettings){
		func(t *store.ToolsSettings) { t.Scanner.Rounds = 2 },
		func(t *store.ToolsSettings) { t.Scanner.Rounds = 21 },
		func(t *store.ToolsSettings) { t.Scanner.Workers = 3 },
		func(t *store.ToolsSettings) { t.Scanner.Workers = 33 },
		func(t *store.ToolsSettings) { t.Scanner.TimeoutMs = 999 },
		func(t *store.ToolsSettings) { t.Scanner.TimeoutMs = 10001 },
		func(t *store.ToolsSettings) { t.CFScan.MaxIPs = 199 },
		func(t *store.ToolsSettings) { t.CFScan.MaxIPs = 10001 },
		func(t *store.ToolsSettings) { t.CFScan.Want = -1 },
		func(t *store.ToolsSettings) { t.CFScan.Want = 1001 },
		func(t *store.ToolsSettings) { t.CFScan.Concurrency = 7 },
		func(t *store.ToolsSettings) { t.CFScan.Concurrency = 129 },
		func(t *store.ToolsSettings) { t.CFScan.TimeoutMs = 999 },
		func(t *store.ToolsSettings) { t.CFScan.TimeoutMs = 5001 },
		func(t *store.ToolsSettings) { t.CFScan.SpeedBytes = 102399 },
		func(t *store.ToolsSettings) { t.CFScan.SpeedBytes = 26214401 },
		func(t *store.ToolsSettings) { t.CFScan.Host = "" },
		func(t *store.ToolsSettings) { t.CFScan.Host = "1.1.1.1" },
		func(t *store.ToolsSettings) { t.CFScan.Host = "a..b" },
		func(t *store.ToolsSettings) { t.CFScan.Host = "-a.com" },
		func(t *store.ToolsSettings) { t.CFScan.Host = strings.Repeat("a.", 127) + "co" },
	}
	for i, f := range bad {
		ts := defaultTools()
		f(&ts)
		require.Error(t, store.ValidateTools(ts), i)
	}
}

func TestPaths_CFScanCache(t *testing.T) {
	p := store.ResolvePaths(`C:\x\ghostline.exe`, `C:\AppData`)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline", "cfscan-cache.json"), p.CFScanCache)
}
