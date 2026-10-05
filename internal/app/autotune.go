package app

import (
	"context"
	"errors"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/store"
)

// Autotune tries the configured engine's strategies lightest first and keeps
// the first one under which every blocked site passes the TLS stage (spec
// §7.2). When zapret2 cannot start at all (antivirus, tampered files) it
// tries GoodbyeDPI's presets instead and, on success, switches the engine:
// the user asked for whatever works (zapret2 spec §8.3).
func (o *Orchestrator) Autotune(ctx context.Context, onProgress func(engine, preset string, i, n int)) error {
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
	engine := s.DPI.Engine
	e, ok := o.d.DPI.Get(engine)
	if !ok {
		return appErr(CodeDPIStartFailed, dpi.ErrUnknownEngine, "engine", engine)
	}
	steps, switched := e.Strategies(), false
	for i := 0; i < len(steps); i++ {
		p := steps[i].ID
		if err := ctx.Err(); err != nil {
			o.stopDPI()
			return err
		}
		if onProgress != nil {
			onProgress(engine, p, i+1, len(steps))
		}
		o.log("dpi", "AUTOTUNE_TRY", "engine", engine, "preset", p)
		if o.d.DPI.Running() {
			o.stopDPI()
		}
		plan := o.planFor(s, engine)
		plan.Strategy, plan.Custom = p, ""
		if err := o.startEngine(ctx, engine, plan); err != nil {
			ae := err.(*AppError)
			switch {
			case ae.Code == CodeDPIStartFailed:
				continue
			case engine == store.EngineZapret2 && i == 0:
				engine, switched = store.EngineGoodbyeDPI, true
				g, _ := o.d.DPI.Get(engine)
				steps, i = g.Strategies(), -1
				continue
			}
			return ae // hash mismatch / AV: further presets will fail the same way
		}
		o.d.Sleep(dpiSettleDelay)
		results := o.d.Prober.ProbeAll(ctx, sites)
		if err := ctx.Err(); err != nil {
			o.stopDPI()
			return err
		}
		ok := true
		for _, r := range results {
			if r.Stage == probe.StageTLS {
				ok = false
			}
		}
		if !ok {
			continue
		}
		s.DPI.Enabled, s.DPI.Engine = true, engine
		if engine == store.EngineZapret2 {
			s.DPI.Zapret2.Strategy = p
		} else {
			s.DPI.Preset = p
		}
		if err := o.d.SaveSettings(s); err != nil {
			return err
		}
		o.update(func(sn *Snapshot) {
			sn.DPI = DPIStatus{Enabled: true, Running: true, Engine: engine, Preset: p}
			sn.BlockedSites = nil
		})
		o.clearReason(ReasonDPIFallback)
		if switched {
			o.log("dpi", CodeAutotuneEngineSwitched, "from", store.EngineZapret2, "to", engine, "preset", p)
		}
		o.log("ok", "AUTOTUNE_FOUND", "engine", engine, "preset", p)
		return nil
	}
	o.stopDPI()
	o.update(func(sn *Snapshot) { sn.DPI.Running, sn.DPI.Engine, sn.DPI.Fallback = false, "", false })
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
