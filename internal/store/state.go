package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// Phase is the connection phase persisted in state.json.
type Phase string

const (
	PhaseClean  Phase = "clean"
	PhaseDNSSet Phase = "dns_set"
)

// ErrStateCorrupt means state.json exists but cannot be parsed.
var ErrStateCorrupt = errors.New("store: state.json is corrupt")

// DPIState records a running GoodbyeDPI process.
type DPIState struct {
	Running bool `json:"running"`
	PID     int  `json:"pid"`
}

// State is the write-ahead record of what Ghostline changed on the system.
type State struct {
	Version      int                     `json:"version"`
	Phase        Phase                   `json:"phase"`
	PID          uint32                  `json:"pid"`
	PIDStartTime time.Time               `json:"pidStartTime"`
	StartedAt    time.Time               `json:"startedAt"`
	Snapshot     []model.AdapterSnapshot `json:"snapshot"`
	DPI          DPIState                `json:"dpi"`
	SysProxy     *SysProxyState          `json:"sysproxy,omitempty"`
	Firewall     *FirewallState          `json:"firewall,omitempty"`
}

// SysProxySnapshot is the WinINET per-connection proxy configuration.
type SysProxySnapshot struct {
	Flags         uint32 `json:"flags"`
	Server        string `json:"server"`
	Bypass        string `json:"bypass"`
	AutoconfigURL string `json:"autoconfigUrl"`
}

// SysProxyState records Ghostline's change to the system proxy. Set is true
// once Ghostline applied Ours; TakenOver once another app replaced it.
type SysProxyState struct {
	Set       bool              `json:"set"`
	TakenOver bool              `json:"takenOver"`
	Ours      string            `json:"ours"`
	Snapshot  *SysProxySnapshot `json:"snapshot"`
}

// FirewallState records the inbound rule created for LAN sharing.
type FirewallState struct {
	Rule string `json:"rule"`
}

// CleanState is the state with nothing to restore.
func CleanState() State { return State{Version: 2, Phase: PhaseClean} }

func cleanState() State { return CleanState() }

// Locker serialises access to state.json across processes.
type Locker interface {
	Lock() error
	Unlock() error
}

// StateStore reads and writes state.json under a cross-process lock.
type StateStore struct {
	path string
	lock Locker
}

func NewStateStore(path string, l Locker) *StateStore { return &StateStore{path: path, lock: l} }

// Path is the state file location.
func (s *StateStore) Path() string { return s.path }

// Locked runs fn while holding the cross-process lock, for read-decide-write
// sequences that span more than one Update (e.g. restore after corruption).
func (s *StateStore) Locked(fn func() error) error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	return fn()
}

// Write replaces state.json; callers must hold the lock (see Locked).
func (s *StateStore) Write(st State) error { return WriteJSONAtomic(s.path, st) }

// Load reads state.json; a missing file is a clean state.
func (s *StateStore) Load() (State, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return cleanState(), nil
	}
	if err != nil {
		return State{}, err
	}
	st := cleanState()
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	return st, nil
}

// Update loads the state, applies fn and writes it back, all under the lock.
// If fn returns an error nothing is written.
func (s *StateStore) Update(fn func(*State) error) error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	st, err := s.Load()
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	return WriteJSONAtomic(s.path, st)
}

// Reset overwrites state.json with a clean state, under the lock. It is the
// way out of a corrupt state file.
func (s *StateStore) Reset() error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	return WriteJSONAtomic(s.path, cleanState())
}
