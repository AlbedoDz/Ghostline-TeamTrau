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
	Sleep   func(time.Duration) // default time.Sleep
	// RestoreSysProxy puts the system proxy back if it is still Ghostline's
	// (sysproxy.Manager.RestoreIfOurs); nil skips it.
	RestoreSysProxy func(ours string, snap store.SysProxySnapshot) (bool, error)
	// DeleteFirewall removes the LAN-sharing firewall rule; nil skips it.
	DeleteFirewall func() error
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
			if d.DeleteFirewall != nil {
				// Idempotent; there is no system proxy snapshot to restore.
				_ = d.DeleteFirewall()
			}
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
			if rerr != nil {
				return rerr // leave the corrupt file: the next layer retries
			}
			return d.States.Write(store.CleanState())
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
		// Order: system proxy, firewall, DNS (spec 2A 6.5).
		perr := restoreProxy(d, st)
		rerr := joinRestore(d.DNS.Restore(stillOurs(d.DNS, st.Snapshot)))
		if st.DPI.Running && d.StopDPI != nil {
			_ = d.StopDPI()
		}
		out = Restored
		if err := errors.Join(perr, rerr); err != nil {
			// Keep the state so a later layer can retry.
			return err
		}
		return d.States.Write(store.CleanState())
	})
	return out, err
}

// restoreProxy undoes the proxy phase: the system proxy (unless never
// applied or taken over by another app) and the firewall rule.
func restoreProxy(d Deps, st store.State) error {
	var errs []error
	// No Set gate: a crash between Apply and recording Set would otherwise
	// leave the proxy behind; RestoreIfOurs checks the value is ours.
	if sp := st.SysProxy; sp != nil && !sp.TakenOver && sp.Snapshot != nil && d.RestoreSysProxy != nil {
		if _, err := d.RestoreSysProxy(sp.Ours, *sp.Snapshot); err != nil {
			errs = append(errs, fmt.Errorf("watchdog: restore system proxy: %w", err))
		}
	}
	if st.Firewall != nil && d.DeleteFirewall != nil {
		if err := d.DeleteFirewall(); err != nil {
			errs = append(errs, fmt.Errorf("watchdog: delete firewall rule: %w", err))
		}
	}
	return errors.Join(errs...)
}

// stillOurs keeps the snapshots of adapters whose DNS still points at
// loopback. An adapter the user re-configured after a crash keeps their
// settings. If the current DNS cannot be read, everything is restored.
func stillOurs(dns Restorer, snaps []model.AdapterSnapshot) []model.AdapterSnapshot {
	ads, err := dns.LoopbackAdapters()
	if err != nil {
		return snaps
	}
	on := make(map[string]bool, len(ads))
	for _, a := range ads {
		on[a.GUID] = true
	}
	var out []model.AdapterSnapshot
	for _, s := range snaps {
		if on[s.GUID] {
			out = append(out, s)
		}
	}
	return out
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
	sleep := d.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for {
		if err := wait(parentPID); err != nil {
			d.log().Warn("waiting for parent failed", "err", err)
		}
		// A wait that returns while the parent (same pid and start time)
		// is still alive was spurious: keep watching.
		if !d.Alive(parentPID, parentStart) {
			break
		}
		sleep(time.Second)
	}
	_, err := RestoreIfOrphaned(d)
	return err
}

// RunRestore is the --restore mode: one recovery attempt.
func RunRestore(d Deps) error {
	_, err := RestoreIfOrphaned(d)
	return err
}
