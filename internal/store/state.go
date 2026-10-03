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
}

func cleanState() State { return State{Version: 1, Phase: PhaseClean} }

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
	defer s.lock.Unlock()
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
	defer s.lock.Unlock()
	return WriteJSONAtomic(s.path, cleanState())
}
