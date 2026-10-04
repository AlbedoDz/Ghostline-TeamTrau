package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// SettingsBox holds the live settings and persists every change.
type SettingsBox struct {
	mu   sync.Mutex
	path string
	s    store.Settings
}

// NewSettingsBox wraps initial settings stored at path.
func NewSettingsBox(path string, initial store.Settings) *SettingsBox {
	return &SettingsBox{path: path, s: initial}
}

// Get returns the current settings.
func (b *SettingsBox) Get() store.Settings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s
}

// Save persists and adopts s.
func (b *SettingsBox) Save(s store.Settings) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := store.SaveSettings(b.path, s); err != nil {
		return err
	}
	b.s = s
	return nil
}

// AppInfo describes the running build.
type AppInfo struct {
	Version   string `json:"version"`
	Portable  bool   `json:"portable"`
	UpdateTag string `json:"updateTag"`
	UpdateURL string `json:"updateUrl"`
}

// ServerRow is one line of the Servers page.
type ServerRow struct {
	Server model.Server    `json:"server"`
	Result *scanner.Result `json:"result,omitempty"`
	InUse  bool            `json:"inUse"`
	Pinned bool            `json:"pinned"`
}

// ServiceDeps wires the UI service.
type ServiceDeps struct {
	Bus          *Bus
	Paths        store.Paths
	Settings     *SettingsBox
	Catalog      func() []model.Server
	LoadCustom   func() ([]model.Server, error)
	SaveCustom   func([]model.Server) error
	ListAdapters func() ([]sysdns.Adapter, error)
	StopService  func(name string) error
	SetMode      func(mode string)
	RestoreNow   func() error
	Info         func() AppInfo
	// OnSettingsChanged lets the shell react (autostart task, language…).
	OnSettingsChanged func(old, new store.Settings)

	// Phase 2A.
	Rules        *rules.Holder
	RulesPath    string
	Fetcher      *lists.Fetcher
	MaxEntries   int // 0 = DefaultMaxEntries
	FragCache    *store.FragCache
	NetKey       func() string
	Proxy        ProxyQuery
	LANInfo      func() LANInfo
	Protect      func(string) (string, error) // DPAPI
	TestUpstream func(ctx context.Context, id string) error
	// CheckUpdate asks GitHub for the latest release now (manual check).
	CheckUpdate func(ctx context.Context) (UpdateCheck, error)
	// CheckServer re-tests one server and updates the cached scan.
	CheckServer func(ctx context.Context, id string) error
}

// Service is bound to the frontend by Wails; its exported methods are the
// UI's whole API.
type Service struct {
	o *Orchestrator
	x ServiceDeps

	mu         sync.Mutex
	scanCancel context.CancelFunc
	tuneCancel context.CancelFunc
	overrideCh chan bool

	rmu sync.Mutex // guards rf
	rf  store.RulesFile
	bg  sync.WaitGroup
}

// NewService creates the UI service.
func NewService(o *Orchestrator, x ServiceDeps) *Service { return &Service{o: o, x: x} }

// GetSnapshot returns the current state.
func (s *Service) GetSnapshot() Snapshot { return s.o.Snapshot() }

// Connect starts protection. Errors are also reported in the snapshot.
func (s *Service) Connect() error {
	slog.Info("ui: connect requested")
	return s.o.Connect(context.Background())
}

// Disconnect stops protection.
func (s *Service) Disconnect() error { return s.o.Disconnect(context.Background()) }

// CancelConnect aborts a connect in progress.
func (s *Service) CancelConnect() { s.o.Cancel() }

// GetSettings returns the settings.
func (s *Service) GetSettings() store.Settings { return s.x.Settings.Get() }

// SaveSettings validates and stores settings.
func (s *Service) SaveSettings(n store.Settings) error {
	if n.Language != "vi" && n.Language != "en" {
		return fmt.Errorf("settings: language must be vi or en")
	}
	if n.MaxUpstreams < 1 || n.MaxUpstreams > 10 {
		return fmt.Errorf("settings: maxUpstreams must be 1..10")
	}
	if len(n.Bootstrap) == 0 {
		return fmt.Errorf("settings: bootstrap list is empty")
	}
	for _, b := range n.Bootstrap {
		if _, err := netip.ParseAddrPort(b); err != nil {
			return fmt.Errorf("settings: bootstrap %q must be ip:port", b)
		}
	}
	if err := store.ValidateProxy(n.Proxy); err != nil {
		return err
	}
	if n.DNSBlockMode != "zero" && n.DNSBlockMode != "nxdomain" {
		return fmt.Errorf("settings: dnsBlockMode must be zero or nxdomain")
	}
	if n.DPI.Preset == "custom" || n.DPI.CustomArgs != "" {
		if _, err := dpi.ValidateCustom(n.DPI.CustomArgs); err != nil {
			return err
		}
	}
	old := s.x.Settings.Get()
	// Upstream proxies change only through SaveUpstreamProxy/
	// DeleteUpstreamProxy: the UI's copy may be stale or carry masked
	// passwords, so it never overwrites them. Pins likewise change only
	// through SetPinned/SetPinnedMany.
	n.Proxy.Upstreams = old.Proxy.Upstreams
	n.Pinned = old.Pinned
	if n.PinnedOnly && len(n.Pinned) == 0 {
		n.PinnedOnly = false // nothing to use: would only fail to connect
	}
	if err := s.x.Settings.Save(n); err != nil {
		return err
	}
	if s.x.OnSettingsChanged != nil {
		s.x.OnSettingsChanged(old, n)
	}
	if proxyPhaseChanged(old.Proxy, n.Proxy) {
		// May wait for the SYSPROXY_EXISTING answer: do not block the UI call.
		s.background(func(ctx context.Context) { _ = s.o.ReapplyProxy(ctx) })
	}
	return nil
}

// SetMode switches simple/advanced and resizes the window.
func (s *Service) SetMode(mode string) error {
	if mode != "simple" && mode != "advanced" {
		return fmt.Errorf("mode must be simple or advanced")
	}
	st := s.x.Settings.Get()
	st.Mode = mode
	if err := s.x.Settings.Save(st); err != nil {
		return err
	}
	if s.x.SetMode != nil {
		s.x.SetMode(mode)
	}
	return nil
}

// ListServers returns every server with its last result.
func (s *Service) ListServers() []ServerRow {
	results := map[string]scanner.Result{}
	for _, r := range s.o.ScanResults() {
		results[r.ServerID] = r
	}
	st := s.x.Settings.Get()
	s.o.mu.Lock()
	inUse := map[string]bool{}
	for _, sv := range s.o.servers {
		inUse[sv.ID] = true
	}
	s.o.mu.Unlock()
	var rows []ServerRow
	for _, sv := range s.x.Catalog() {
		row := ServerRow{Server: sv, InUse: inUse[sv.ID], Pinned: slices.Contains(st.Pinned, sv.ID)}
		if r, ok := results[sv.ID]; ok {
			row.Result = &r
		}
		rows = append(rows, row)
	}
	return rows
}

// ScanAll starts a full scan in the background; progress arrives as events.
func (s *Service) ScanAll() error {
	s.mu.Lock()
	if s.scanCancel != nil {
		s.mu.Unlock()
		return errors.New("scan already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.scanCancel = nil
			s.mu.Unlock()
			cancel()
		}()
		_, _ = s.o.Rescan(ctx, func(done, total int, r scanner.Result) {
			s.x.Bus.Emit(EventScan, ScanProgress{Done: done, Total: total, Result: &r, Running: true})
		})
		s.x.Bus.Emit(EventScan, ScanProgress{Running: false})
	}()
	return nil
}

// CancelScan stops a running scan.
func (s *Service) CancelScan() {
	s.mu.Lock()
	c := s.scanCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// AddServers imports URLs/stamps (one per line). It returns how many were
// added and the lines that were rejected.
func (s *Service) AddServers(text string) (int, []string) {
	add, bad := servers.ParseImport([]byte(text))
	cur, err := s.x.LoadCustom()
	if err != nil {
		return 0, append(bad, err.Error())
	}
	seen := map[string]bool{}
	for _, c := range cur {
		seen[c.Address] = true
	}
	n := 0
	for _, a := range add {
		if seen[a.Address] {
			continue
		}
		seen[a.Address] = true
		cur = append(cur, a)
		n++
	}
	if err := s.x.SaveCustom(cur); err != nil {
		return 0, append(bad, err.Error())
	}
	if bad == nil {
		bad = []string{}
	}
	return n, bad
}

// RemoveCustomServer deletes a user-added server.
func (s *Service) RemoveCustomServer(id string) error {
	cur, err := s.x.LoadCustom()
	if err != nil {
		return err
	}
	cur = slices.DeleteFunc(cur, func(m model.Server) bool { return m.ID == id })
	return s.x.SaveCustom(cur)
}

// SetPinned pins or unpins a server.
func (s *Service) SetPinned(id string, pinned bool) error {
	st := s.x.Settings.Get()
	st.Pinned = slices.DeleteFunc(slices.Clone(st.Pinned), func(p string) bool { return p == id })
	if pinned {
		st.Pinned = append(st.Pinned, id)
	}
	return s.x.Settings.Save(st)
}

// SetPinnedMany pins or unpins several servers with one save (bulk pin
// from search results, "unpin all"). Order is kept and duplicates dropped.
func (s *Service) SetPinnedMany(ids []string, pinned bool) error {
	st := s.x.Settings.Get()
	cur := slices.Clone(st.Pinned)
	if pinned {
		for _, id := range ids {
			if !slices.Contains(cur, id) {
				cur = append(cur, id)
			}
		}
	} else {
		cur = slices.DeleteFunc(cur, func(p string) bool { return slices.Contains(ids, p) })
	}
	if cur == nil {
		cur = []string{}
	}
	st.Pinned = cur
	if len(cur) == 0 {
		st.PinnedOnly = false
	}
	return s.x.Settings.Save(st)
}

// UseOnlyServer makes id the only pinned server and turns "use pinned
// servers only" on, in one save.
func (s *Service) UseOnlyServer(id string) error {
	if id == "" {
		return errors.New("app: empty server id")
	}
	st := s.x.Settings.Get()
	st.Pinned, st.PinnedOnly = []string{id}, true
	return s.x.Settings.Save(st)
}

// CheckServer re-tests one server and returns its updated row.
func (s *Service) CheckServer(id string) (ServerRow, error) {
	if s.x.CheckServer == nil {
		return ServerRow{}, errors.New("app: server check unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.x.CheckServer(ctx, id); err != nil {
		return ServerRow{}, err
	}
	for _, r := range s.ListServers() {
		if r.Server.ID == id {
			return r, nil
		}
	}
	return ServerRow{}, fmt.Errorf("app: no server %q", id)
}

// SetDPIEnabled turns GoodbyeDPI on or off.
func (s *Service) SetDPIEnabled(on bool) error { return s.o.SetDPIEnabled(context.Background(), on) }

// StartAutotune runs DPI autotune in the background with progress events.
func (s *Service) StartAutotune() error {
	s.mu.Lock()
	if s.tuneCancel != nil {
		s.mu.Unlock()
		return errors.New("autotune already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tuneCancel = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.tuneCancel = nil
			s.mu.Unlock()
			cancel()
		}()
		err := s.o.Autotune(ctx, func(p string, i, n int) {
			s.x.Bus.Emit(EventAutotune, AutotuneProgress{Preset: p, Index: i, Total: n, Running: true})
		})
		final := AutotuneProgress{Running: false}
		var ae *AppError
		if errors.As(err, &ae) {
			final.Error = &AppError{Code: ae.Code, Params: ae.Params}
		}
		s.x.Bus.Emit(EventAutotune, final)
	}()
	return nil
}

// CancelAutotune stops autotune.
func (s *Service) CancelAutotune() {
	s.mu.Lock()
	c := s.tuneCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// ProbeNow probes the configured sites once.
func (s *Service) ProbeNow() []probe.Result { return s.o.ProbeNow(context.Background()) }

// GetDPIBlacklist reads the blacklist file ("" when missing).
func (s *Service) GetDPIBlacklist() (string, error) {
	b, err := os.ReadFile(s.x.Paths.DPIBlacklist)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// SaveDPIBlacklist writes the blacklist file.
func (s *Service) SaveDPIBlacklist(text string) error {
	if err := os.MkdirAll(s.x.Paths.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.x.Paths.DPIBlacklist, []byte(text), 0o644)
}

// PreviewDPIArgs shows the GoodbyeDPI command line for the given options.
func (s *Service) PreviewDPIArgs(preset, custom, scope string) ([]string, error) {
	return dpi.Args(dpi.Preset(preset), custom, dpi.Scope(scope), s.x.Paths.DPIBlacklist)
}

// DismissWarning hides a warning. RESTORE_FAILED cannot be dismissed: only
// a successful restore clears it.
func (s *Service) DismissWarning(code string) {
	if code == CodeRestoreFailed {
		return
	}
	s.o.ClearWarning(code)
}

// RestoreDNSNow forces a DNS restore.
func (s *Service) RestoreDNSNow() error {
	return s.o.RestoreNow(context.Background(), s.x.RestoreNow)
}

// StopConflictingService stops a Windows service holding port 53. The UI
// calls it only after the user confirmed in-page.
func (s *Service) StopConflictingService(name string) error { return s.x.StopService(name) }

// ListAdapters lists network adapters for manual selection.
func (s *Service) ListAdapters() []sysdns.Adapter {
	ads, _ := s.x.ListAdapters()
	return ads
}

// CheckUpdateNow checks for a newer release right away, ignoring the
// schedule and the "notify about new versions" setting.
func (s *Service) CheckUpdateNow() (UpdateCheck, error) {
	if s.x.CheckUpdate == nil {
		return UpdateCheck{}, errors.New(CodeUpdateCheckFailed)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := s.x.CheckUpdate(ctx)
	if err != nil {
		return UpdateCheck{}, fmt.Errorf("%s: %w", CodeUpdateCheckFailed, err)
	}
	return r, nil
}

// AppInfo returns version and update information.
func (s *Service) AppInfo() AppInfo { return s.x.Info() }

// SetQueryLog toggles the RAM-only live query view.
func (s *Service) SetQueryLog(on bool) {
	s.x.Bus.queryLog.Store(on)
	if !on {
		s.x.Bus.queries.Reset()
		s.x.Bus.conns.Reset()
	}
}

// GetQueries returns the RAM-only query view.
func (s *Service) GetQueries() []engine.QueryEvent { return s.x.Bus.queries.All() }

// GetLogs returns the in-memory log.
func (s *Service) GetLogs() []LogEvent { return s.x.Bus.logs.All() }

// RunStats emits StatsEvent on every tick while connected. It is a
// function, not a method, so Wails does not bind it.
func RunStats(s *Service, ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		if !s.o.connected() {
			continue
		}
		st := s.o.d.Engine.Stats()
		ev := StatsEvent{Queries: st.Queries, LatencyMs: int(st.AvgLatency / time.Millisecond)}
		s.o.update(func(sn *Snapshot) { sn.Queries, sn.LatencyMs = ev.Queries, ev.LatencyMs })
		s.x.Bus.Emit(EventStats, ev)
	}
}
