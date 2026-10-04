package app

import (
	"context"
	"time"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

const (
	healthInterval = 30 * time.Second
	degradedAfter  = 15 * time.Second
	dpiSettleDelay = 2 * time.Second
	probeAttempts  = 2
)

// afterConnect starts background work once protected: DPI (if enabled),
// health checks and the post-connect site probe.
func (o *Orchestrator) afterConnect() {
	s := o.d.Settings()
	if s.DPI.Enabled {
		if err := o.startDPI(context.Background(), s); err != nil {
			o.log("dpi", errCode(err))
		}
	}
	if o.d.Ticker != nil {
		ticks, stop := o.d.Ticker(healthInterval)
		ctx, cancel := context.WithCancel(context.Background())
		o.mu.Lock()
		o.healthStop = func() { cancel(); stop() }
		o.mu.Unlock()
		go o.healthLoop(ctx, ticks)
	}
	if o.d.Prober != nil {
		go o.probeBlocked(context.Background())
	}
}

func errCode(err error) string {
	if ae, ok := err.(*AppError); ok {
		return ae.Code
	}
	return CodeInternal
}

// upstreamsFailing reports whether every upstream in use has failed more
// recently than it last succeeded.
func (o *Orchestrator) upstreamsFailing(st engine.Stats) bool {
	if len(st.PerUpstream) == 0 {
		return false
	}
	for _, u := range st.PerUpstream {
		if !u.LastErrAt.After(u.LastOKAt) {
			return false
		}
	}
	return true
}

func (o *Orchestrator) healthLoop(ctx context.Context, ticks <-chan time.Time) {
	var failingSince time.Time
	for {
		var now time.Time
		select {
		case <-ctx.Done():
			return
		case now = <-ticks:
		}
		failing := o.d.Engine.SelfTest(ctx) != nil || o.upstreamsFailing(o.d.Engine.Stats())
		if !failing {
			failingSince = time.Time{}
			continue
		}
		if failingSince.IsZero() {
			failingSince = now
			continue
		}
		if now.Sub(failingSince) >= degradedAfter {
			o.heal(ctx)
			failingSince = time.Time{}
		}
	}
}

// heal marks the connection degraded, picks fresh servers and hot-swaps
// them. DNS keeps pointing at loopback throughout, so nothing leaks.
func (o *Orchestrator) heal(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	o.update(func(s *Snapshot) { s.Status = StatusDegraded })
	o.log("engine", "DEGRADED")
	picked, err := o.d.Picker.Pick(ctx, nil)
	if err != nil {
		o.log("engine", CodeNoServers)
		return
	}
	ups, err := o.buildUpstreams(picked)
	if err != nil {
		return
	}
	if err := o.d.Engine.Swap(ctx, ups); err != nil {
		o.log("engine", "SWAP_FAILED")
		return
	}
	o.mu.Lock()
	o.servers = picked
	o.mu.Unlock()
	o.update(func(s *Snapshot) { s.Status, s.Servers = StatusProtected, serverNames(picked) })
	o.log("ok", "SWAPPED", "servers", len(picked))
}

// OnNetworkChange snapshots and redirects adapters that appeared while
// connected. The snapshot is persisted before the adapter is changed.
func (o *Orchestrator) OnNetworkChange(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	s := o.d.Settings()
	ads, err := o.d.DNS.Select(s.Adapters, s.AdapterGUIDs)
	if err != nil {
		return
	}
	o.mu.Lock()
	known := map[string]bool{}
	for _, sn := range o.snaps {
		known[sn.GUID] = true
	}
	o.mu.Unlock()
	for _, a := range ads {
		if known[a.GUID] {
			continue
		}
		snaps, err := o.d.DNS.Snapshot([]sysdns.Adapter{a})
		if err != nil || len(snaps) == 0 {
			continue
		}
		if err := o.d.States.Update(func(st *store.State) error {
			st.Snapshot = append(st.Snapshot, snaps...)
			return nil
		}); err != nil {
			continue
		}
		o.mu.Lock()
		o.snaps = append(o.snaps, snaps...)
		o.mu.Unlock()
		if err := o.d.DNS.ApplyLoopback(snaps, o.d.System.IPv6Available()); err != nil {
			o.log("system", CodeSetDNSFailed, "adapter", a.Alias)
			continue
		}
		_ = o.d.DNS.Flush()
		o.log("system", "ADAPTER_ADDED", "adapter", a.Alias)
	}
}

// OnResume checks the engine after sleep and heals immediately on failure.
func (o *Orchestrator) OnResume(ctx context.Context) {
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	if o.d.Engine.SelfTest(ctx) != nil || o.upstreamsFailing(o.d.Engine.Stats()) {
		o.heal(ctx)
	}
}

// ProbeNow probes the configured sites once.
func (o *Orchestrator) ProbeNow(ctx context.Context) []probe.Result {
	if o.d.Prober == nil {
		return nil
	}
	return o.d.Prober.ProbeAll(ctx, o.d.Settings().ProbeSites)
}

func (o *Orchestrator) probeBlocked(ctx context.Context) {
	sites := o.d.Settings().ProbeSites
	results := make([][]probe.Result, probeAttempts)
	for i := range results {
		results[i] = o.d.Prober.ProbeAll(ctx, sites)
	}
	blocked := probe.DPIBlocked(results[0], results[1])
	if len(blocked) == 0 {
		return
	}
	o.update(func(s *Snapshot) {
		if s.Status == StatusProtected || s.Status == StatusDegraded {
			s.BlockedSites = blocked
		}
	})
	o.log("dpi", "SITES_BLOCKED", "count", len(blocked))
}
