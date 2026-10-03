// Package watchdog restores DNS when Ghostline's main process dies uncleanly.
// The same logic serves three safety layers: the --watchdog child, startup
// recovery in the app, and the --restore logon task.
package watchdog

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// Restorer is the part of sysdns.Manager recovery needs.
type Restorer interface {
	Restore([]model.AdapterSnapshot) []sysdns.RestoreError
	LoopbackAdapters() ([]sysdns.Adapter, error)
}

// Deps wires recovery to the system.
type Deps struct {
	States  *store.StateStore
	DNS     Restorer
	StopDPI func() error
	Alive   func(pid uint32, start time.Time) bool
	Log     *slog.Logger
}

// Outcome says what RestoreIfOrphaned did.
type Outcome int

const (
	NothingToDo Outcome = iota
	Restored
	RestoredFromCorrupt
	OwnerAlive
)

func (d Deps) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// RestoreIfOrphaned restores DNS if state.json says Ghostline changed it and
// the process that did so (pid + start time) is gone. A corrupt state file
// falls back to resetting every adapter still pointing at loopback to DHCP.
func RestoreIfOrphaned(d Deps) (Outcome, error) {
	var out Outcome
	err := d.States.Locked(func() error {
		st, err := d.States.Load()
		if errors.Is(err, store.ErrStateCorrupt) {
			d.log().Warn("state.json corrupt; resetting loopback adapters to DHCP")
			ads, lerr := d.DNS.LoopbackAdapters()
			if lerr != nil {
				return lerr
			}
			var snaps []model.AdapterSnapshot
			for _, a := range ads {
				snaps = append(snaps, model.AdapterSnapshot{GUID: a.GUID, LUID: a.LUID, IfIndex: a.IfIndex, Alias: a.Alias,
					IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}})
			}
			rerr := joinRestore(d.DNS.Restore(snaps))
			if d.StopDPI != nil {
				_ = d.StopDPI()
			}
			out = RestoredFromCorrupt
			if werr := d.States.Write(store.State{Version: 1, Phase: store.PhaseClean}); werr != nil {
				return werr
			}
			return rerr
		}
		if err != nil {
			return err
		}
		if st.Phase == store.PhaseClean {
			out = NothingToDo
			return nil
		}
		if d.Alive(st.PID, st.PIDStartTime) {
			out = OwnerAlive
			return nil
		}
		rerr := joinRestore(d.DNS.Restore(st.Snapshot))
		if st.DPI.Running && d.StopDPI != nil {
			_ = d.StopDPI()
		}
		out = Restored
		if rerr != nil {
			// Keep the snapshot so a later layer can retry.
			return rerr
		}
		return d.States.Write(store.State{Version: 1, Phase: store.PhaseClean})
	})
	return out, err
}

func joinRestore(errs []sysdns.RestoreError) error {
	var out []error
	for _, e := range errs {
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil
	}
	return fmt.Errorf("watchdog: restore incomplete: %w", errors.Join(out...))
}

// RunWatchdog waits for the parent to exit, then restores if it died without
// cleaning up.
func RunWatchdog(parentPID uint32, parentStart time.Time, wait func(pid uint32) error, d Deps) error {
	if err := wait(parentPID); err != nil {
		d.log().Warn("waiting for parent failed", "err", err)
	}
	_, err := RestoreIfOrphaned(d)
	return err
}

// RunRestore is the --restore mode: one recovery attempt.
func RunRestore(d Deps) error {
	_, err := RestoreIfOrphaned(d)
	return err
}
