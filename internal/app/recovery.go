package app

import (
	"context"
	"errors"

	"github.com/hashcott/ghostline/internal/watchdog"
)

// CodeStateReset warns that state.json was unreadable at startup and
// adapters still on loopback were reset to DHCP.
const CodeStateReset = "STATE_RESET"

// StartupWarnings turns the startup recovery outcome into UI warnings, so a
// failed or lossy recovery is never silent.
func StartupWarnings(out watchdog.Outcome, err error) []AppError {
	var w []AppError
	if err != nil {
		w = append(w, AppError{Code: CodeRestoreFailed, Params: map[string]any{"adapter": ""}})
	}
	if out == watchdog.RestoredFromCorrupt {
		w = append(w, AppError{Code: CodeStateReset})
	}
	return w
}

// RestoreNow is the manual "Restore DNS now" action. While connected (or
// dirty after a failed restore) it disconnects through the normal path;
// otherwise it runs fallback, which restores from state.json or resets
// loopback adapters. It holds opMu so it cannot race Connect.
func (o *Orchestrator) RestoreNow(ctx context.Context, fallback func() error) error {
	o.Cancel()
	o.opMu.Lock()
	defer o.opMu.Unlock()

	o.mu.Lock()
	active := o.dirty || o.snap.Status == StatusProtected || o.snap.Status == StatusDegraded
	o.mu.Unlock()
	if active {
		if errs := o.disconnectLocked(ctx); len(errs) > 0 {
			return errors.Join(errs[0])
		}
		o.update(func(s *Snapshot) {
			s.Status, s.Error, s.Servers, s.BlockedSites, s.LatencyMs, s.Queries = StatusDisconnected, nil, nil, nil, 0, 0
			s.DPI.Running, s.DPI.Engine, s.DPI.Fallback = false, "", false
		})
		o.log("system", "DISCONNECTED")
		return nil
	}
	if fallback != nil {
		if err := fallback(); err != nil {
			return err
		}
		_ = o.d.Safety.DeleteRecoveryTask()
	}
	o.ClearWarning(CodeRestoreFailed)
	o.ClearWarning(CodeStateReset)
	return nil
}
