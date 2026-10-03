// Package store owns Ghostline's on-disk data: paths, settings and state.
package store

import (
	"os"
	"path/filepath"

	"github.com/hashcott/ghostline/internal/brand"
)

// Paths lists every file Ghostline keeps in its data directory.
type Paths struct {
	DataDir            string
	Settings           string
	State              string
	Meta               string
	ScanCache          string
	ServersRemote      string
	ServersRemoteSig   string
	ServersDNSCrypt    string
	ServersDNSCryptSig string
	ServersCustom      string
	DPIBlacklist       string
	LogDir             string
	BinDir             string
	Portable           bool
}

// ResolvePaths picks the data directory: <exe dir>\data when a "portable"
// marker sits next to the executable, otherwise %APPDATA%\Ghostline.
func ResolvePaths(exePath, appData string) Paths {
	exeDir := filepath.Dir(exePath)
	p := Paths{}
	if _, err := os.Stat(filepath.Join(exeDir, "portable")); err == nil {
		p.Portable = true
		p.DataDir = filepath.Join(exeDir, "data")
	} else {
		p.DataDir = filepath.Join(appData, brand.AppName)
	}
	j := func(name string) string { return filepath.Join(p.DataDir, name) }
	p.Settings = j("settings.json")
	p.State = j("state.json")
	p.Meta = j("meta.json")
	p.ScanCache = j("scan-cache.json")
	p.ServersRemote = j("servers-remote.json")
	p.ServersRemoteSig = j("servers-remote.json.sig")
	p.ServersDNSCrypt = j("servers-dnscrypt.md")
	p.ServersDNSCryptSig = j("servers-dnscrypt.md.minisig")
	p.ServersCustom = j("servers-custom.json")
	p.DPIBlacklist = j("dpi-blacklist.txt")
	p.LogDir = j("logs")
	p.BinDir = j("bin")
	return p
}
