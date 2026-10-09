package dpi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrHashMismatch  = errors.New("dpi: DPI engine files do not match the pinned hashes")
	ErrStartFailed   = errors.New("dpi: DPI engine failed to start")
	ErrBlockedByAV   = errors.New("dpi: DPI engine was blocked (antivirus?)")
	ErrDriverInUse   = errors.New("dpi: the WinDivert driver is in use by another program")
	ErrUnknownEngine = errors.New("dpi: unknown engine")
)

// driverService is the WinDivert 2.x service name. WinDivert 1.x (shipped
// with GoodbyeDPI 0.2.2) used a versioned name ("WinDivert1.4"), so cleanup
// looks for the prefix.
const driverService = "WinDivert"

// List files are copied into the engine directory under these names: the
// engines read argv as ANSI (GoodbyeDPI) or through Cygwin (winws2), so a
// path with Vietnamese letters (C:\Users\Đức…) would be mangled.
const (
	blacklistName    = "blacklist.txt"
	autoHostlistName = "autohostlist.txt"
)

func extractWith(src fs.FS, dir string, pins map[string]string) error {
	for name, want := range pins {
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if fileHash(dst) == want {
			continue
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		if hashOf(b) != want {
			return fmt.Errorf("%w: embedded %s", ErrHashMismatch, name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return verifyWith(dir, pins)
}

func verifyWith(dir string, pins map[string]string) error {
	for name, want := range pins {
		if fileHash(filepath.Join(dir, filepath.FromSlash(name))) != want {
			return fmt.Errorf("%w: %s", ErrHashMismatch, name)
		}
	}
	return nil
}

func hashOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashOf(b)
}

// Process is a started engine process.
type Process interface {
	PID() int
	Exited() bool
	Kill() error
}

// Runner starts processes.
type Runner interface {
	Start(exe string, args []string, dir string) (Process, error)
}

// Services controls Windows services.
type Services interface {
	Find(prefix string) ([]string, error)
	Running(name string) (bool, error)
	// Active reports whether a service exists and is not fully stopped
	// (running, paused, or mid-transition). A missing service is not active.
	Active(name string) (bool, error)
	Stop(name string) error
	Delete(name string) error
}

// Installed pairs an engine with the embedded files it is extracted from.
type Installed struct {
	Engine Engine
	Assets fs.FS
}

// Manager runs at most one engine process. Each engine lives in its own
// directory under binDir.
type Manager struct {
	binDir  string
	engines map[string]Installed
	runner  Runner
	svc     Services
	sleep   func(time.Duration)

	mu      sync.Mutex
	proc    Process
	running string // engine ID of proc
	plan    Plan   // plan proc was started with (absolute paths)
}

// NewManager manages the given engines, extracted under binDir.
func NewManager(binDir string, engines []Installed, r Runner, s Services, sleep func(time.Duration)) *Manager {
	if sleep == nil {
		sleep = time.Sleep
	}
	m := &Manager{binDir: binDir, engines: map[string]Installed{}, runner: r, svc: s, sleep: sleep}
	for _, e := range engines {
		m.engines[e.Engine.ID()] = e
	}
	return m
}

// Get returns an engine by ID.
func (m *Manager) Get(engine string) (Engine, bool) {
	in, ok := m.engines[engine]
	return in.Engine, ok
}

func (m *Manager) dir(engine string) string { return filepath.Join(m.binDir, engine) }

// Start stops whatever runs, removes leftover WinDivert services, verifies
// (re-extracting if needed) and launches the engine. It is running when,
// after 2s, the process is alive and the WinDivert driver is up. p's list
// paths are absolute.
func (m *Manager) Start(ctx context.Context, engine string, p Plan) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.engines[engine]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownEngine, engine)
	}
	if err := m.stopLocked(); err != nil {
		return 0, fmt.Errorf("%w: cleanup: %v", ErrStartFailed, err)
	}
	// A WinDivert service still active after cleanup is held by another DPI
	// tool the user runs outside Ghostline (a standalone GoodbyeDPI or zapret).
	// An in-use kernel driver can't be claimed, so ask the user to close it
	// instead of fighting over it and failing cryptically below.
	if m.driverInUse() {
		return 0, ErrDriverInUse
	}
	dir, pins := m.dir(engine), in.Engine.Files()
	if err := verifyWith(dir, pins); err != nil {
		// Expected on first run; after that it means the files were changed
		// or removed (antivirus quarantine).
		slog.Info("dpi: engine files missing or changed; extracting", "engine", engine, "dir", dir, "err", err)
		if err := extractWith(in.Assets, dir, pins); err != nil {
			// Real-time antivirus refuses the write itself.
			if errors.Is(err, os.ErrPermission) || isAppControlBlock(err) {
				return 0, fmt.Errorf("%w: %v", ErrBlockedByAV, err)
			}
			return 0, err
		}
	}
	rel, err := copyLists(dir, p)
	if err != nil {
		return 0, fmt.Errorf("%w: lists: %v", ErrStartFailed, err)
	}
	args, err := in.Engine.Args(rel)
	if err != nil {
		return 0, err
	}
	proc, err := m.runner.Start(filepath.Join(dir, filepath.FromSlash(in.Engine.Exe())), args, dir)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrNotExist) || isAppControlBlock(err) {
			return 0, fmt.Errorf("%w: %v", ErrBlockedByAV, err)
		}
		return 0, fmt.Errorf("%w: %v", ErrStartFailed, err)
	}
	m.sleep(2 * time.Second)
	if proc.Exited() {
		slog.Warn("dpi: engine exited during startup", "engine", engine, "pid", proc.PID(), "args", args)
		if err := verifyWith(dir, pins); err != nil {
			slog.Warn("dpi: engine files changed after start (quarantined?)", "engine", engine, "err", err)
			return 0, ErrBlockedByAV // files vanished or changed: quarantined
		}
		return 0, ErrStartFailed
	}
	if ok, err := m.svc.Running(driverService); !ok {
		if err != nil {
			slog.Warn("dpi: query WinDivert driver state failed", "err", err, "service", driverService)
		}
		slog.Warn("dpi: WinDivert driver not running after start; killing engine", "engine", engine, "pid", proc.PID())
		if err := proc.Kill(); err != nil {
			slog.Warn("dpi: kill engine failed", "err", err, "engine", engine, "pid", proc.PID())
		}
		return 0, fmt.Errorf("%w: WinDivert driver not running", ErrStartFailed)
	}
	m.proc, m.running, m.plan = proc, engine, p
	return proc.PID(), nil
}

// copyLists copies p's list files into dir and returns p with relative names.
func copyLists(dir string, p Plan) (Plan, error) {
	if p.Scope != ScopeBlacklist {
		p.Blacklist = ""
	}
	if p.Blacklist != "" {
		b, err := os.ReadFile(p.Blacklist)
		if err != nil {
			return p, err
		}
		if err := os.WriteFile(filepath.Join(dir, blacklistName), b, 0o644); err != nil {
			return p, err
		}
		p.Blacklist = blacklistName
	}
	if p.AutoHostlist != "" {
		b, err := os.ReadFile(p.AutoHostlist)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return p, err
		}
		if err := os.WriteFile(filepath.Join(dir, autoHostlistName), b, 0o644); err != nil {
			return p, err
		}
		p.AutoHostlist = autoHostlistName
	}
	return p, nil
}

// RefreshLists copies the blacklist into the running engine's directory, for
// engines that re-read it by themselves.
func (m *Manager) RefreshLists(p Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == "" || p.Blacklist == "" {
		return nil
	}
	b, err := os.ReadFile(p.Blacklist)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir(m.running), blacklistName), b, 0o644)
}

func isAppControlBlock(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "application control") ||
		strings.Contains(s, "virus") ||
		strings.Contains(s, "potentially unwanted") ||
		strings.Contains(s, "applocker") ||
		strings.Contains(s, "smartscreen") ||
		strings.Contains(s, "windows defender")
}

// Stop kills the engine and removes every WinDivert service.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	var errs []error
	if m.proc != nil {
		if m.plan.AutoHostlist != "" {
			// Keep what the engine learned; it only lives in its directory.
			src := filepath.Join(m.dir(m.running), autoHostlistName)
			if b, err := os.ReadFile(src); err == nil {
				errs = append(errs, os.WriteFile(m.plan.AutoHostlist, b, 0o644))
			} else if !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("dpi: read learned auto-hostlist failed", "err", err, "path", src)
			}
		}
		if !m.proc.Exited() {
			errs = append(errs, m.proc.Kill())
		}
		m.proc, m.running, m.plan = nil, "", Plan{}
	}
	names, err := m.svc.Find(driverService)
	errs = append(errs, err)
	for _, n := range names {
		errs = append(errs, m.svc.Stop(n), m.svc.Delete(n))
	}
	return errors.Join(errs...)
}

// driverInUse reports whether a WinDivert service is still active after
// stopLocked tried to remove it. stopLocked waits out our own just-killed
// driver, so anything still active here is held by another live process.
func (m *Manager) driverInUse() bool {
	names, err := m.svc.Find(driverService)
	if err != nil {
		slog.Warn("dpi: list WinDivert services failed", "err", err)
	}
	for _, n := range names {
		ok, err := m.svc.Active(n)
		if err != nil {
			slog.Warn("dpi: query WinDivert service state failed", "err", err, "service", n)
		}
		if ok {
			slog.Warn("dpi: WinDivert service still active after cleanup; held by another program", "service", n)
			return true
		}
	}
	return false
}

// Running reports whether the managed process is alive.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc != nil && !m.proc.Exited()
}

// Engine returns the ID of the running engine, "" when none runs.
func (m *Manager) Engine() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil || m.proc.Exited() {
		return ""
	}
	return m.running
}
