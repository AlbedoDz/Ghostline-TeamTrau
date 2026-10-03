package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
)

// Orchestrator owns the connection lifecycle.
type Orchestrator struct {
	d Deps

	opMu sync.Mutex // serialises connect/disconnect/swap/autotune

	mu           sync.Mutex
	snap         Snapshot
	cancel       context.CancelFunc
	snaps        []model.AdapterSnapshot
	stopWatchdog func() error
	servers      []model.Server
	healthStop   func()
}

// New creates an orchestrator in the disconnected state.
func New(d Deps) *Orchestrator {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if !d.ListenV4.IsValid() {
		d.ListenV4 = netip.MustParseAddrPort("127.0.0.1:53")
	}
	if !d.ListenV6.IsValid() {
		d.ListenV6 = netip.MustParseAddrPort("[::1]:53")
	}
	o := &Orchestrator{d: d, snap: Snapshot{Status: StatusDisconnected}}
	s := d.Settings()
	o.snap.DPI = DPIStatus{Enabled: s.DPI.Enabled, Preset: s.DPI.Preset}
	return o
}

// Snapshot returns a copy of the current UI state.
func (o *Orchestrator) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snap.clone()
}

func (o *Orchestrator) emit() {
	o.mu.Lock()
	s := o.snap.clone()
	o.mu.Unlock()
	if o.d.Sink != nil {
		o.d.Sink.State(s)
	}
}

func (o *Orchestrator) update(fn func(*Snapshot)) {
	o.mu.Lock()
	fn(&o.snap)
	o.mu.Unlock()
	o.emit()
}

func (o *Orchestrator) log(source, code string, kv ...any) {
	if o.d.Sink == nil {
		return
	}
	e := LogEvent{Time: o.d.Now(), Source: source, Code: code}
	if len(kv) > 0 {
		e.Params = map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i].(string)] = kv[i+1]
		}
	}
	o.d.Sink.Log(e)
}

// AddWarning adds a persistent warning (deduplicated by code and params).
func (o *Orchestrator) AddWarning(e AppError) {
	o.update(func(s *Snapshot) {
		for _, w := range s.Warnings {
			if w.Code == e.Code && sameParams(w.Params, e.Params) {
				return
			}
		}
		s.Warnings = append(s.Warnings, AppError{Code: e.Code, Params: e.Params})
	})
}

func sameParams(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// ClearWarning removes every warning with code.
func (o *Orchestrator) ClearWarning(code string) {
	o.update(func(s *Snapshot) {
		out := s.Warnings[:0]
		for _, w := range s.Warnings {
			if w.Code != code {
				out = append(out, w)
			}
		}
		s.Warnings = out
	})
}

// Cancel aborts an in-progress Connect.
func (o *Orchestrator) Cancel() {
	o.mu.Lock()
	c := o.cancel
	o.mu.Unlock()
	if c != nil {
		c()
	}
}

// Connect runs the connect saga (spec §5.2). Calling it while not
// disconnected (or in error) does nothing.
func (o *Orchestrator) Connect(ctx context.Context) error {
	o.mu.Lock()
	if o.snap.Status != StatusDisconnected && o.snap.Status != StatusError {
		o.mu.Unlock()
		return nil
	}
	cctx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	o.snap.Status, o.snap.Step, o.snap.Error, o.snap.BlockedSites = StatusConnecting, 0, nil, nil
	o.mu.Unlock()
	o.emit()
	defer func() {
		cancel()
		o.mu.Lock()
		o.cancel = nil
		o.mu.Unlock()
	}()

	o.opMu.Lock()
	defer o.opMu.Unlock()

	err := runSteps(cctx, o.connectSteps(), func(i int) { o.update(func(s *Snapshot) { s.Step = i }) })
	if err != nil {
		if cctx.Err() != nil && ctx.Err() == nil || errors.Is(err, context.Canceled) {
			o.update(func(s *Snapshot) { s.Status, s.Step, s.Error = StatusDisconnected, 0, nil })
			o.log("system", "CONNECT_CANCELLED")
			return err
		}
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeInternal, err, "step", o.Snapshot().Step)
		}
		o.update(func(s *Snapshot) { s.Status, s.Error = StatusError, &AppError{Code: ae.Code, Params: ae.Params} })
		o.log("system", ae.Code, flatten(ae.Params)...)
		return err
	}
	o.update(func(s *Snapshot) {
		s.Status, s.Step, s.Since = StatusProtected, 0, o.d.Now()
		s.Servers = serverNames(o.servers)
	})
	o.log("ok", "CONNECTED", "servers", len(o.servers))
	o.afterConnect()
	return nil
}

func flatten(m map[string]any) []any {
	var out []any
	for k, v := range m {
		out = append(out, k, v)
	}
	return out
}

func serverNames(ss []model.Server) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Name)
	}
	return out
}

func (o *Orchestrator) buildUpstreams(ss []model.Server) ([]upstream.Upstream, error) {
	var ups []upstream.Upstream
	for _, s := range ss {
		u, err := o.d.Builder.Build(s)
		if err != nil {
			for _, x := range ups {
				_ = x.Close()
			}
			return nil, err
		}
		ups = append(ups, u)
	}
	return ups, nil
}

func (o *Orchestrator) connectSteps() []step {
	var picked []model.Server
	var snaps []model.AdapterSnapshot
	var stopWD func() error
	v6 := o.d.System.IPv6Available()
	return []step{
		{name: "preflight", do: func(ctx context.Context) error {
			if !o.d.System.IsAdmin() {
				return appErr(CodeNotAdmin, nil)
			}
			if st, err := o.d.States.Load(); err != nil || st.Phase != store.PhaseClean {
				if o.d.Recover != nil {
					if _, rerr := o.d.Recover(); rerr != nil {
						return appErr(CodeRestoreFailed, rerr)
					}
				}
			}
			owners, err := o.d.System.PortOwners(53)
			if err != nil {
				return appErr(CodeInternal, err, "step", 1)
			}
			if len(owners) > 0 {
				w := owners[0]
				return appErr(CodePort53Busy, nil, "pid", w.PID, "name", w.Name, "service", w.Service)
			}
			return nil
		}},
		{name: "pick", do: func(ctx context.Context) error {
			ss, err := o.d.Picker.Pick(ctx, nil)
			if err != nil {
				var ns *NoServersError
				if errors.As(err, &ns) {
					return appErr(CodeNoServers, err, "checked", ns.Checked, "elapsed", int64(ns.Elapsed/time.Second))
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return appErr(CodeNoServers, err, "checked", 0, "elapsed", int64(0))
			}
			picked = ss
			return nil
		}},
		{name: "engine", do: func(ctx context.Context) error {
			ups, err := o.buildUpstreams(picked)
			if err != nil {
				return appErr(CodeEngineSelfTest, err)
			}
			cfg := engine.Config{ListenV4: o.d.ListenV4, Upstreams: ups, CacheEnabled: true}
			if v6 {
				cfg.ListenV6 = o.d.ListenV6
			}
			if err := o.d.Engine.Start(ctx, cfg); err != nil {
				return appErr(CodeEngineSelfTest, err)
			}
			if err := o.d.Engine.SelfTest(ctx); err != nil {
				_ = o.d.Engine.Stop(context.WithoutCancel(ctx))
				return appErr(CodeEngineSelfTest, err)
			}
			o.mu.Lock()
			o.servers = picked
			o.mu.Unlock()
			return nil
		}, undo: func(ctx context.Context) error { return o.d.Engine.Stop(ctx) }},
		{name: "snapshot", do: func(ctx context.Context) error {
			s := o.d.Settings()
			ads, err := o.d.DNS.Select(s.Adapters, s.AdapterGUIDs)
			if err != nil {
				return appErr(CodeSetDNSFailed, err, "adapter", "")
			}
			if len(ads) == 0 {
				return appErr(CodeSetDNSFailed, errors.New("no connected adapters"), "adapter", "")
			}
			snaps, err = o.d.DNS.Snapshot(ads)
			if err != nil {
				return appErr(CodeSetDNSFailed, err, "adapter", ads[0].Alias)
			}
			pid, start := o.d.System.SelfPID()
			return o.d.States.Update(func(st *store.State) error {
				st.Version, st.Phase, st.PID, st.PIDStartTime, st.StartedAt = 1, store.PhaseDNSSet, pid, start, o.d.Now()
				st.Snapshot = snaps
				return nil
			})
		}, undo: func(ctx context.Context) error {
			return o.d.States.Update(func(st *store.State) error {
				*st = store.State{Version: 1, Phase: store.PhaseClean}
				return nil
			})
		}},
		{name: "safety", do: func(ctx context.Context) error {
			pid, start := o.d.System.SelfPID()
			stop, err := o.d.Safety.StartWatchdog(pid, start)
			if err != nil {
				return appErr(CodeInternal, err, "step", 5)
			}
			if err := o.d.Safety.CreateRecoveryTask(); err != nil {
				_ = stop()
				return appErr(CodeInternal, err, "step", 5)
			}
			stopWD = stop
			return nil
		}, undo: func(ctx context.Context) error {
			err := o.d.Safety.DeleteRecoveryTask()
			if stopWD != nil {
				err = errors.Join(err, stopWD())
			}
			return err
		}},
		{name: "apply", do: func(ctx context.Context) error {
			if err := o.d.DNS.ApplyLoopback(snaps, v6); err != nil {
				return appErr(CodeSetDNSFailed, err, "adapter", snaps[0].Alias)
			}
			if err := o.d.DNS.Flush(); err != nil {
				return appErr(CodeSetDNSFailed, err, "adapter", snaps[0].Alias)
			}
			o.mu.Lock()
			o.snaps, o.stopWatchdog = snaps, stopWD
			o.mu.Unlock()
			return nil
		}, undo: func(ctx context.Context) error {
			errs := o.d.DNS.Restore(snaps)
			ferr := o.d.DNS.Flush()
			if len(errs) > 0 {
				return errors.Join(errs[0], ferr)
			}
			return ferr
		}},
		{name: "verify", do: func(ctx context.Context) error {
			nonce := randomHex(8)
			o.d.Engine.ExpectVerify(nonce)
			ips, err := o.d.Resolver.LookupNetIP(ctx, "ip4", nonce+".verify.ghostline.test")
			hit := false
			for _, ip := range ips {
				if ip == engine.VerifyAnswer {
					hit = true
				}
			}
			if err != nil || !hit || !o.d.Engine.SawVerify(nonce) {
				return appErr(CodeVerifyLeak, err)
			}
			return nil
		}},
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Disconnect restores DNS first, then stops GoodbyeDPI and the engine
// (spec §5.3). If any adapter cannot be restored the state stays dirty and
// the watchdog and recovery task stay armed so later layers retry.
func (o *Orchestrator) Disconnect(ctx context.Context) error {
	o.Cancel()
	o.opMu.Lock()
	defer o.opMu.Unlock()

	o.mu.Lock()
	st := o.snap.Status
	if st != StatusProtected && st != StatusDegraded {
		o.mu.Unlock()
		return nil
	}
	o.snap.Status = StatusDisconnecting
	snaps, stopWD, healthStop := o.snaps, o.stopWatchdog, o.healthStop
	o.healthStop = nil
	o.mu.Unlock()
	o.emit()
	if healthStop != nil {
		healthStop()
	}

	errs := o.d.DNS.Restore(snaps)
	_ = o.d.DNS.Flush()
	for _, e := range errs {
		o.AddWarning(AppError{Code: CodeRestoreFailed, Params: map[string]any{"adapter": e.Alias}})
	}
	if o.d.DPI.Running() {
		_ = o.d.DPI.Stop()
	}
	_ = o.d.Engine.Stop(ctx)
	if len(errs) == 0 {
		_ = o.d.States.Update(func(s *store.State) error {
			*s = store.State{Version: 1, Phase: store.PhaseClean}
			return nil
		})
		if stopWD != nil {
			_ = stopWD()
		}
		_ = o.d.Safety.DeleteRecoveryTask()
	}
	o.mu.Lock()
	o.snaps, o.stopWatchdog, o.servers = nil, nil, nil
	o.mu.Unlock()
	o.update(func(s *Snapshot) {
		s.Status, s.Since, s.Servers, s.BlockedSites, s.LatencyMs, s.Queries = StatusDisconnected, time.Time{}, nil, nil, 0, 0
		s.DPI.Running = false
	})
	o.log("system", "DISCONNECTED")
	return nil
}

// afterConnect is extended by health/probing (Task 18).
func (o *Orchestrator) afterConnect() {}
