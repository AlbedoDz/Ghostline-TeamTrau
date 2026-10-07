// Package sysproxy snapshots, applies and restores the Windows system proxy
// (WinINET per-connection options of the default LAN connection).
package sysproxy

import (
	"errors"
	"log/slog"

	"github.com/hashcott/ghostline/internal/store"
)

// API reads and writes the per-connection proxy options. Set must also
// broadcast the settings change so running browsers pick it up.
type API interface {
	Query() (store.SysProxySnapshot, error)
	Set(store.SysProxySnapshot) error
}

// INTERNET_PER_CONN_FLAGS bits.
const (
	FlagDirect       uint32 = 1
	FlagProxy        uint32 = 2
	FlagAutoProxyURL uint32 = 4
)

// Bypass keeps local and private addresses off the proxy.
const Bypass = "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;[::1]"

// ErrNotApplied means the setting did not read back as written.
var ErrNotApplied = errors.New("sysproxy: setting did not stick")

// Manager applies Ghostline's proxy and restores the previous one.
type Manager struct{ API API }

// Snapshot reads the current configuration.
func (m Manager) Snapshot() (store.SysProxySnapshot, error) { return m.API.Query() }

// Existing reports a proxy server or PAC script another app has enabled.
func (m Manager) Existing(s store.SysProxySnapshot) (server, pac string, has bool) {
	if s.Flags&FlagProxy != 0 && s.Server != "" {
		server = s.Server
	}
	if s.Flags&FlagAutoProxyURL != 0 && s.AutoconfigURL != "" {
		pac = s.AutoconfigURL
	}
	return server, pac, server != "" || pac != ""
}

// Apply points the system proxy at addr and checks it reads back.
func (m Manager) Apply(addr string) error {
	if err := m.API.Set(store.SysProxySnapshot{Flags: FlagDirect | FlagProxy, Server: addr, Bypass: Bypass}); err != nil {
		return err
	}
	ours, err := m.IsOurs(addr)
	if err != nil {
		return err
	}
	if !ours {
		return ErrNotApplied
	}
	return nil
}

// IsOurs reports whether the system proxy is enabled and set to addr.
func (m Manager) IsOurs(addr string) (bool, error) {
	cur, err := m.API.Query()
	if err != nil {
		return false, err
	}
	return cur.Flags&FlagProxy != 0 && cur.Server == addr, nil
}

// RestoreIfOurs puts snap back unless another app or the user replaced
// Ghostline's setting. If the current setting cannot be read, it restores.
func (m Manager) RestoreIfOurs(addr string, snap store.SysProxySnapshot) (bool, error) {
	ours, err := m.IsOurs(addr)
	if err == nil && !ours {
		return false, nil
	}
	if err != nil {
		slog.Warn("sysproxy: read current proxy failed; restoring anyway", "err", err, "ours", addr)
	}
	if err := m.API.Set(snap); err != nil {
		return false, err
	}
	return true, nil
}
