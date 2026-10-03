package app

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/hashcott/ghostline/internal/winutil"
	"github.com/miekg/dns"
)

var errBoom = errors.New("boom")

// rec is a shared, ordered call log; fail makes the named call return errBoom.
type rec struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]bool
}

func (r *rec) add(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
	if r.fail[name] {
		return errBoom
	}
	return nil
}

func (r *rec) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type nopUp struct{}

func (nopUp) Exchange(context.Context, *dns.Msg) (*dns.Msg, error) { return nil, errBoom }
func (nopUp) Address() string                                      { return "nop" }
func (nopUp) Close() error                                         { return nil }

type fEngine struct {
	r     *rec
	saw   bool
	swaps int
	stats engine.Stats
	selfE error
	mu    sync.Mutex
}

func (e *fEngine) Start(context.Context, engine.Config) error { return e.r.add("engine.start") }
func (e *fEngine) Swap(context.Context, []upstream.Upstream) error {
	e.mu.Lock()
	e.swaps++
	e.mu.Unlock()
	return e.r.add("engine.swap")
}
func (e *fEngine) Stop(context.Context) error { return e.r.add("engine.stop") }
func (e *fEngine) SelfTest(context.Context) error {
	if err := e.r.add("engine.selftest"); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.selfE
}
func (e *fEngine) ExpectVerify(string) { _ = e.r.add("engine.expect") }
func (e *fEngine) SawVerify(string) bool {
	if e.r.add("engine.saw") != nil {
		return false
	}
	return e.saw
}
func (e *fEngine) Stats() engine.Stats { e.mu.Lock(); defer e.mu.Unlock(); return e.stats }

type fDNS struct {
	r          *rec
	adapters   []sysdns.Adapter
	restoreErr bool
}

func (d *fDNS) Select(string, []string) ([]sysdns.Adapter, error) {
	return d.adapters, d.r.add("dns.select")
}
func (d *fDNS) Snapshot(ads []sysdns.Adapter) ([]model.AdapterSnapshot, error) {
	var out []model.AdapterSnapshot
	for _, a := range ads {
		out = append(out, model.AdapterSnapshot{GUID: a.GUID, Alias: a.Alias, IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	return out, d.r.add("dns.snapshot")
}
func (d *fDNS) ApplyLoopback(snaps []model.AdapterSnapshot, _ bool) error {
	name := "dns.apply"
	if len(snaps) == 1 && snaps[0].GUID != "{A}" {
		name = "dns.apply:" + snaps[0].GUID
	}
	return d.r.add(name)
}
func (d *fDNS) Restore(s []model.AdapterSnapshot) []sysdns.RestoreError {
	_ = d.r.add("dns.restore")
	if d.restoreErr {
		return []sysdns.RestoreError{{GUID: s[0].GUID, Alias: s[0].Alias, Err: errBoom}}
	}
	return nil
}
func (d *fDNS) Flush() error { return d.r.add("dns.flush") }

type fDPI struct {
	r       *rec
	running bool
	startE  error
	started []string
}

func (p *fDPI) Start(_ context.Context, args []string) (int, error) {
	_ = p.r.add("dpi.start")
	if p.startE != nil {
		return 0, p.startE
	}
	p.running = true
	p.started = args
	return 99, nil
}
func (p *fDPI) Stop() error   { p.running = false; return p.r.add("dpi.stop") }
func (p *fDPI) Running() bool { return p.running }

type fSafety struct{ r *rec }

func (s *fSafety) StartWatchdog(uint32, time.Time) (func() error, error) {
	if err := s.r.add("safety.watchdog"); err != nil {
		return nil, err
	}
	return func() error { return s.r.add("safety.watchdog.stop") }, nil
}
func (s *fSafety) CreateRecoveryTask() error { return s.r.add("safety.task.create") }
func (s *fSafety) DeleteRecoveryTask() error { return s.r.add("safety.task.delete") }

type fSystem struct {
	r      *rec
	admin  bool
	owners []winutil.PortOwner
}

func (s *fSystem) IsAdmin() bool { _ = s.r.add("sys.admin"); return s.admin }
func (s *fSystem) PortOwners(uint16) ([]winutil.PortOwner, error) {
	return s.owners, s.r.add("sys.ports")
}
func (s *fSystem) SelfPID() (uint32, time.Time) { return 1234, time.Unix(100, 0) }
func (s *fSystem) IPv6Available() bool          { return true }

type fPicker struct {
	r     *rec
	block chan struct{}
	err   error
	n     int
}

func (p *fPicker) Pick(ctx context.Context, _ func(done, total int)) ([]model.Server, error) {
	if err := p.r.add("pick"); err != nil {
		return nil, err
	}
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	n := p.n
	if n == 0 {
		n = 1
	}
	var out []model.Server
	for i := 0; i < n; i++ {
		out = append(out, model.Server{ID: "cf", Name: "Cloudflare"})
	}
	return out, nil
}

type fBuilder struct{ r *rec }

func (b *fBuilder) Build(model.Server) (upstream.Upstream, error) { return nopUp{}, b.r.add("build") }

type fResolver struct{ r *rec }

func (f *fResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, f.r.add("resolve")
}

type fStates struct {
	r *rec
	s *store.StateStore
}

type memLock struct{ mu sync.Mutex }

func (l *memLock) Lock() error   { l.mu.Lock(); return nil }
func (l *memLock) Unlock() error { l.mu.Unlock(); return nil }

func (f *fStates) Load() (store.State, error) { return f.s.Load() }
func (f *fStates) Update(fn func(*store.State) error) error {
	var before store.State
	err := f.s.Update(func(st *store.State) error {
		before = *st
		if err := fn(st); err != nil {
			return err
		}
		switch {
		case st.Phase == store.PhaseClean:
			return f.r.add("state.clean")
		case before.Phase == store.PhaseDNSSet && len(st.Snapshot) > len(before.Snapshot):
			return f.r.add("state.append")
		default:
			return f.r.add("state.dns_set")
		}
	})
	return err
}

type fSink struct {
	mu     sync.Mutex
	states []Snapshot
	logs   []LogEvent
}

func (s *fSink) State(sn Snapshot) { s.mu.Lock(); s.states = append(s.states, sn); s.mu.Unlock() }
func (s *fSink) Log(e LogEvent)    { s.mu.Lock(); s.logs = append(s.logs, e); s.mu.Unlock() }

type fProber struct {
	r     *rec
	stage func(site string, call int) probe.Stage
	calls int
	mu    sync.Mutex
}

func (p *fProber) ProbeAll(_ context.Context, sites []string) []probe.Result {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	_ = p.r.add("probe")
	var out []probe.Result
	for _, s := range sites {
		st := probe.StageOK
		if p.stage != nil {
			st = p.stage(s, call)
		}
		out = append(out, probe.Result{Site: s, Stage: st})
	}
	return out
}

type harness struct {
	o        *Orchestrator
	r        *rec
	eng      *fEngine
	dns      *fDNS
	dpi      *fDPI
	sys      *fSystem
	pick     *fPicker
	states   *fStates
	sink     *fSink
	prober   *fProber
	settings store.Settings
	recovers int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	r := &rec{fail: map[string]bool{}}
	h := &harness{r: r,
		eng:      &fEngine{r: r, saw: true},
		dns:      &fDNS{r: r, adapters: []sysdns.Adapter{{GUID: "{A}", Alias: "Wi-Fi", IfType: 71, Up: true, HasGateway: true}}},
		dpi:      &fDPI{r: r},
		sys:      &fSystem{r: r, admin: true},
		pick:     &fPicker{r: r},
		states:   &fStates{r: r, s: store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), &memLock{})},
		sink:     &fSink{},
		prober:   &fProber{r: r},
		settings: store.DefaultSettings(),
	}
	h.o = New(Deps{
		Engine: h.eng, DNS: h.dns, DPI: h.dpi, Safety: &fSafety{r: r}, System: h.sys, Picker: h.pick,
		Builder: &fBuilder{r: r}, Resolver: &fResolver{r: r},
		Recover: func() (watchdog.Outcome, error) { h.recovers++; _ = r.add("recover"); return watchdog.Restored, nil },
		Sink:    h.sink, States: h.states,
		Settings:     func() store.Settings { return h.settings },
		SaveSettings: func(s store.Settings) error { h.settings = s; return nil },
		Now:          time.Now,
		Prober:       h.prober,
		Sleep:        func(time.Duration) {},
	})
	return h
}
