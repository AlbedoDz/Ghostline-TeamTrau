package app

import (
	"context"
	"errors"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/store"
)

// dpiErr maps dpi errors to UI codes.
func dpiErr(err error) *AppError {
	switch {
	case errors.Is(err, dpi.ErrHashMismatch):
		return appErr(CodeDPIHashMismatch, err)
	case errors.Is(err, dpi.ErrBlockedByAV):
		return appErr(CodeDPIBlockedByAV, err)
	}
	return appErr(CodeDPIStartFailed, err)
}

func (o *Orchestrator) dpiArgs(s store.Settings, preset string) ([]string, error) {
	return dpi.Args(dpi.Preset(preset), s.DPI.CustomArgs, dpi.Scope(s.DPI.Scope), o.d.BlacklistPath)
}

func (o *Orchestrator) startDPI(ctx context.Context, s store.Settings) error {
	args, err := o.dpiArgs(s, s.DPI.Preset)
	if err != nil {
		return appErr(CodeDPIStartFailed, err)
	}
	pid, err := o.d.DPI.Start(ctx, args)
	if err != nil {
		return dpiErr(err)
	}
	o.recordDPI(true, pid)
	o.update(func(sn *Snapshot) { sn.DPI = DPIStatus{Enabled: true, Running: true, Preset: s.DPI.Preset} })
	o.log("dpi", "DPI_STARTED", "preset", s.DPI.Preset)
	return nil
}

func (o *Orchestrator) connected() bool {
	st := o.Snapshot().Status
	return st == StatusProtected || st == StatusDegraded
}

// SetDPIEnabled turns GoodbyeDPI on or off. While disconnected it only
// saves the setting; enabled DPI starts on the next connect.
func (o *Orchestrator) SetDPIEnabled(ctx context.Context, on bool) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	s := o.d.Settings()
	if on && o.connected() && !o.d.DPI.Running() {
		if err := o.startDPI(ctx, s); err != nil {
			return err
		}
	}
	if !on && o.d.DPI.Running() {
		_ = o.d.DPI.Stop()
		o.recordDPI(false, 0)
		o.log("dpi", "DPI_STOPPED")
	}
	s.DPI.Enabled = on
	if err := o.d.SaveSettings(s); err != nil {
		return err
	}
	o.update(func(sn *Snapshot) {
		sn.DPI.Enabled, sn.DPI.Preset = on, s.DPI.Preset
		sn.DPI.Running = o.d.DPI.Running()
	})
	return nil
}

// Autotune tries presets lightest first and keeps the first one under
// which every blocked site passes the TLS stage (spec §7.2).
func (o *Orchestrator) Autotune(ctx context.Context, onProgress func(preset string, i, n int)) error {
	// Probing goes through Ghostline's DNS, so it only means something
	// while connected.
	if !o.connected() {
		return appErr(CodeNotConnected, nil)
	}
	ctx, cancel := o.background(ctx)
	defer cancel()
	o.opMu.Lock()
	defer o.opMu.Unlock()
	s := o.d.Settings()
	sites := o.Snapshot().BlockedSites
	if len(sites) == 0 {
		sites = s.ProbeSites
	}
	n := len(dpi.AutotuneOrder)
	for i, p := range dpi.AutotuneOrder {
		if err := ctx.Err(); err != nil {
			_ = o.d.DPI.Stop()
			return err
		}
		if onProgress != nil {
			onProgress(string(p), i+1, n)
		}
		o.log("dpi", "AUTOTUNE_TRY", "preset", string(p))
		if o.d.DPI.Running() {
			_ = o.d.DPI.Stop()
		}
		args, err := o.dpiArgs(s, string(p))
		if err != nil {
			return appErr(CodeDPIStartFailed, err)
		}
		pid, err := o.d.DPI.Start(ctx, args)
		if err != nil {
			ae := dpiErr(err)
			if ae.Code != CodeDPIStartFailed {
				return ae // hash mismatch / AV: further presets will fail the same way
			}
			continue
		}
		o.recordDPI(true, pid)
		o.d.Sleep(dpiSettleDelay)
		results := o.d.Prober.ProbeAll(ctx, sites)
		if err := ctx.Err(); err != nil {
			_ = o.d.DPI.Stop()
			o.recordDPI(false, 0)
			return err
		}
		ok := true
		for _, r := range results {
			if r.Stage == probe.StageTLS {
				ok = false
			}
		}
		if ok {
			s.DPI.Enabled, s.DPI.Preset = true, string(p)
			if err := o.d.SaveSettings(s); err != nil {
				return err
			}
			o.update(func(sn *Snapshot) {
				sn.DPI = DPIStatus{Enabled: true, Running: true, Preset: string(p)}
				sn.BlockedSites = nil
			})
			o.log("ok", "AUTOTUNE_FOUND", "preset", string(p))
			return nil
		}
	}
	_ = o.d.DPI.Stop()
	o.recordDPI(false, 0)
	o.update(func(sn *Snapshot) { sn.DPI.Running = false })
	return appErr(CodeAutotuneNoPreset, nil)
}

// Rescan runs a full scan of all servers.
func (o *Orchestrator) Rescan(ctx context.Context, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error) {
	if o.d.Scans == nil {
		return nil, errors.New("app: no scanner")
	}
	return o.d.Scans.Rescan(ctx, onProgress)
}

// ScanResults returns the last scan for the current network.
func (o *Orchestrator) ScanResults() []scanner.Result {
	if o.d.Scans == nil {
		return nil
	}
	return o.d.Scans.Results()
}
