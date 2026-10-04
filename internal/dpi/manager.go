package dpi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Pinned SHA-256 of GoodbyeDPI 0.2.2 x86_64.
var Pinned = map[string]string{
	"goodbyedpi.exe":  "331ac6c1d22ba5a0a217f3f27d0d823051869cafc8b8ef7f2002fa2accebc74e",
	"WinDivert.dll":   "a97859785a2df1d4462e7d48d33ccbd89fedd40dac4970f4afd89e63f59ee1ec",
	"WinDivert64.sys": "53ab28ec00be6e6f8aefa9ee76fc2735e94d7f3f9dbc06eb2b7ac8cd3084a6af",
}

var (
	ErrHashMismatch = errors.New("dpi: GoodbyeDPI files do not match the pinned hashes")
	ErrStartFailed  = errors.New("dpi: GoodbyeDPI failed to start")
	ErrBlockedByAV  = errors.New("dpi: GoodbyeDPI was blocked (antivirus?)")
)

const driverService = "WinDivert"

// Extract writes the binaries from src to dir and verifies them against Pinned.
func Extract(src fs.FS, dir string) error { return extractWith(src, dir, Pinned) }

// Verify checks the files in dir against the pinned hashes.
func Verify(dir string) error { return verifyWith(dir, Pinned) }

func extractWith(src fs.FS, dir string, pins map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, want := range pins {
		dst := filepath.Join(dir, name)
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
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return verifyWith(dir, pins)
}

func verifyWith(dir string, pins map[string]string) error {
	for name, want := range pins {
		if fileHash(filepath.Join(dir, name)) != want {
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

// Process is a started GoodbyeDPI process.
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
	Stop(name string) error
	Delete(name string) error
}

// Manager runs at most one GoodbyeDPI process.
type Manager struct {
	dir    string
	assets fs.FS
	pins   map[string]string
	runner Runner
	svc    Services
	sleep  func(time.Duration)

	mu   sync.Mutex
	proc Process
}

// NewManager manages GoodbyeDPI extracted into dir from assets.
func NewManager(dir string, assets fs.FS, r Runner, s Services, sleep func(time.Duration)) *Manager {
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Manager{dir: dir, assets: assets, pins: Pinned, runner: r, svc: s, sleep: sleep}
}

// Start verifies (re-extracting if needed) and launches GoodbyeDPI. It is
// running when, after 2s, the process is alive and the WinDivert driver is up.
func (m *Manager) Start(ctx context.Context, args []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc != nil && !m.proc.Exited() {
		return 0, errors.New("dpi: already running")
	}
	if err := verifyWith(m.dir, m.pins); err != nil {
		if err := extractWith(m.assets, m.dir, m.pins); err != nil {
			return 0, err
		}
	}
	args, err := m.localBlacklist(args)
	if err != nil {
		return 0, fmt.Errorf("%w: blacklist: %v", ErrStartFailed, err)
	}
	p, err := m.runner.Start(filepath.Join(m.dir, "goodbyedpi.exe"), args, m.dir)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrNotExist) || isAppControlBlock(err) {
			return 0, fmt.Errorf("%w: %v", ErrBlockedByAV, err)
		}
		return 0, fmt.Errorf("%w: %v", ErrStartFailed, err)
	}
	m.sleep(2 * time.Second)
	if p.Exited() {
		if verifyWith(m.dir, m.pins) != nil {
			return 0, ErrBlockedByAV // files vanished or changed: quarantined
		}
		return 0, ErrStartFailed
	}
	if !m.driverRunning() {
		_ = p.Kill()
		return 0, fmt.Errorf("%w: WinDivert driver not running", ErrStartFailed)
	}
	m.proc = p
	return p.PID(), nil
}

func isAppControlBlock(err error) bool {
	return bytes.Contains([]byte(err.Error()), []byte("Application Control")) ||
		bytes.Contains([]byte(err.Error()), []byte("virus"))
}

// Stop kills GoodbyeDPI and removes the WinDivert service it installed.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	if m.proc != nil {
		if !m.proc.Exited() {
			errs = append(errs, m.proc.Kill())
		}
		m.proc = nil
	}
	names, err := m.svc.Find(driverService)
	errs = append(errs, err)
	for _, n := range names {
		errs = append(errs, m.svc.Stop(n), m.svc.Delete(n))
	}
	return errors.Join(errs...)
}

// Running reports whether the managed process is alive.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc != nil && !m.proc.Exited()
}

// localBlacklist copies a --blacklist file into the working directory as
// "blacklist.txt" and passes that relative name. GoodbyeDPI 0.2.2 reads its
// argv as ANSI, so a path with Vietnamese letters (C:\Users\Đức…) would be
// mangled before fopen.
func (m *Manager) localBlacklist(args []string) ([]string, error) {
	out := append([]string(nil), args...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] != "--blacklist" {
			continue
		}
		b, err := os.ReadFile(out[i+1])
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(m.dir, "blacklist.txt"), b, 0o644); err != nil {
			return nil, err
		}
		out[i+1] = "blacklist.txt"
	}
	return out, nil
}

// driverRunning reports whether any WinDivert driver service is running.
// WinDivert 1.x names its service with the version ("WinDivert1.4").
func (m *Manager) driverRunning() bool {
	names, _ := m.svc.Find(driverService)
	for _, n := range names {
		if ok, _ := m.svc.Running(n); ok {
			return true
		}
	}
	return false
}
