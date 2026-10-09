//go:build windows

package game

import (
	"context"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultTargetGames lists the process executables to monitor for VAC safety.
var DefaultTargetGames = []string{
	"cs2.exe",
	"dota2.exe",
	"steam.exe",
}

// FindRunningGames inspects running processes using standard Windows toolhelp snapshot
// without opening handles to or reading memory of the target processes (100% passive & VAC-safe).
func FindRunningGames(targets []string) ([]string, error) {
	if len(targets) == 0 {
		targets = DefaultTargetGames
	}
	targetMap := make(map[string]bool, len(targets))
	for _, t := range targets {
		targetMap[strings.ToLower(strings.TrimSpace(t))] = true
	}

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}

	foundMap := make(map[string]bool)
	for {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if targetMap[name] {
			foundMap[name] = true
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}

	var found []string
	for g := range foundMap {
		found = append(found, g)
	}
	return found, nil
}

// Watcher monitors running games periodically and fires callbacks on state transitions.
type Watcher struct {
	targets  []string
	interval time.Duration
	onStart  func(game string)
	onStop   func(game string)

	mu      sync.Mutex
	running map[string]bool
	cancel  context.CancelFunc
}

// NewWatcher creates a game process watcher.
func NewWatcher(targets []string, interval time.Duration, onStart, onStop func(string)) *Watcher {
	if len(targets) == 0 {
		targets = DefaultTargetGames
	}
	if interval < 500*time.Millisecond {
		interval = 2 * time.Second
	}
	return &Watcher{
		targets:  targets,
		interval: interval,
		onStart:  onStart,
		onStop:   onStop,
		running:  make(map[string]bool),
	}
}

// Start begins periodic process monitoring.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.mu.Unlock()

	// Immediate poll to catch games already running at startup
	w.poll()

	go w.loop(cctx)
}

// Stop terminates the watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

// RunningGames returns the currently observed running games.
func (w *Watcher) RunningGames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for g, active := range w.running {
		if active {
			out = append(out, g)
		}
	}
	return out
}

func (w *Watcher) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	current, err := FindRunningGames(w.targets)
	if err != nil {
		return
	}

	curMap := make(map[string]bool, len(current))
	for _, g := range current {
		curMap[g] = true
	}

	w.mu.Lock()
	// Detect newly started games
	for g := range curMap {
		if !w.running[g] {
			w.running[g] = true
			if w.onStart != nil {
				go w.onStart(g)
			}
		}
	}

	// Detect stopped games
	for g, wasRunning := range w.running {
		if wasRunning && !curMap[g] {
			w.running[g] = false
			if w.onStop != nil {
				go w.onStop(g)
			}
		}
	}
	w.mu.Unlock()
}
