package app

import (
	"context"
	"net/netip"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/hashcott/ghostline/internal/winutil"
)

// Engine is the loopback DNS server.
type Engine interface {
	Start(context.Context, engine.Config) error
	Swap(context.Context, []upstream.Upstream) error
	Stop(context.Context) error
	SelfTest(context.Context) error
	ExpectVerify(string)
	SawVerify(string) bool
	Stats() engine.Stats
}

// DNS changes adapter DNS settings.
type DNS interface {
	Select(mode string, guids []string) ([]sysdns.Adapter, error)
	Snapshot([]sysdns.Adapter) ([]model.AdapterSnapshot, error)
	ApplyLoopback([]model.AdapterSnapshot, bool) error
	Restore([]model.AdapterSnapshot) []sysdns.RestoreError
	Flush() error
}

// DPI runs GoodbyeDPI.
type DPI interface {
	Start(context.Context, []string) (int, error)
	Stop() error
	Running() bool
}

// Safety starts the watchdog and the logon recovery task.
type Safety interface {
	StartWatchdog(pid uint32, start time.Time) (stop func() error, err error)
	CreateRecoveryTask() error
	DeleteRecoveryTask() error
}

// System answers questions about the machine.
type System interface {
	IsAdmin() bool
	PortOwners(uint16) ([]winutil.PortOwner, error)
	SelfPID() (uint32, time.Time)
	IPv6Available() bool
}

// Picker chooses the upstream servers to use.
type Picker interface {
	Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error)
}

// Builder turns servers into upstreams.
type Builder interface {
	Build(model.Server) (upstream.Upstream, error)
}

// Resolver is the system resolver, used only for leak verification.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Prober probes sites.
type Prober interface {
	ProbeAll(ctx context.Context, sites []string) []probe.Result
}

// States persists the write-ahead connection state.
type States interface {
	Load() (store.State, error)
	Update(func(*store.State) error) error
}

// Sink receives UI events.
type Sink interface {
	State(Snapshot)
	Log(LogEvent)
}

// Deps wires the orchestrator.
type Deps struct {
	Engine       Engine
	DNS          DNS
	DPI          DPI
	Safety       Safety
	System       System
	Picker       Picker
	Builder      Builder
	Resolver     Resolver
	Prober       Prober
	Recover      func() (watchdog.Outcome, error)
	Sink         Sink
	States       States
	Settings     func() store.Settings
	SaveSettings func(store.Settings) error
	Now          func() time.Time
	Sleep        func(time.Duration)
	// Ticker returns a tick channel and a stop func (health checks).
	Ticker        func(time.Duration) (<-chan time.Time, func())
	BlacklistPath string
	ListenV4      netip.AddrPort // default 127.0.0.1:53
	ListenV6      netip.AddrPort // default [::1]:53
}
