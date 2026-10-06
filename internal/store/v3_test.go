package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func loadRaw(t *testing.T, raw string) store.Settings {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.False(t, recovered)
	return s
}

func TestLoadSettings_V2KeepsGoodbyeDPI(t *testing.T) { // Review Focus #3
	s := loadRaw(t, `{"version":2,"dpi":{"enabled":true,"preset":"high","scope":"blacklist"}}`)
	require.Equal(t, 5, s.Version)
	require.Equal(t, "goodbyedpi", s.DPI.Engine)
	require.Equal(t, "high", s.DPI.Preset)
	require.Equal(t, "blacklist", s.DPI.Scope)
	require.True(t, s.DPI.Enabled)
	require.Equal(t, "z-split", s.DPI.Zapret2.Strategy)
}

func TestLoadSettings_V1FileKeepsGoodbyeDPI(t *testing.T) {
	s := loadRaw(t, `{"dpi":{"preset":"light"}}`)
	require.Equal(t, "goodbyedpi", s.DPI.Engine)
}

func TestLoadSettings_NoFileIsZapret2(t *testing.T) {
	s, _, err := store.LoadSettings(filepath.Join(t.TempDir(), "settings.json"))
	require.NoError(t, err)
	require.Equal(t, 5, s.Version)
	require.Equal(t, "zapret2", s.DPI.Engine)
	require.Equal(t, store.Zapret2Settings{Strategy: "z-split"}, s.DPI.Zapret2)
}

func TestLoadSettings_V3KeepsChoice(t *testing.T) {
	require.Equal(t, "zapret2", loadRaw(t, `{"version":3,"dpi":{"engine":"zapret2"}}`).DPI.Engine)
	s := loadRaw(t, `{"version":3,"dpi":{"engine":"goodbyedpi","zapret2":{"strategy":"z-fake","autoHostlist":true}}}`)
	require.Equal(t, "goodbyedpi", s.DPI.Engine)
	require.Equal(t, "z-fake", s.DPI.Zapret2.Strategy)
	require.True(t, s.DPI.Zapret2.AutoHostlist)
}

func TestLoadSettings_UnknownEngine(t *testing.T) {
	require.Equal(t, "zapret2", loadRaw(t, `{"version":3,"dpi":{"engine":"x"}}`).DPI.Engine)
}

func TestPaths_DPIFiles(t *testing.T) {
	p := store.ResolvePaths(`C:\exe\ghostline.exe`, `C:\Users\u\AppData\Roaming`)
	require.Equal(t, filepath.Join(p.DataDir, "dpi-strategies.json"), p.DPIStrategies)
	require.Equal(t, filepath.Join(p.DataDir, "dpi-strategies.json.sig"), p.DPIStrategiesSig)
	require.Equal(t, filepath.Join(p.DataDir, "dpi-autohostlist.txt"), p.DPIAutoHostlist)
}
