// Package app orchestrates Ghostline: connect, disconnect, recovery, health
// and the service bound to the UI.
package app

import "time"

// Status is the connection state shown to the user.
type Status string

const (
	StatusDisconnected  Status = "disconnected"
	StatusConnecting    Status = "connecting"
	StatusProtected     Status = "protected"
	StatusDegraded      Status = "degraded"
	StatusDisconnecting Status = "disconnecting"
	StatusError         Status = "error"
)

// DPIStatus summarises GoodbyeDPI for the UI.
type DPIStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Preset  string `json:"preset"`
}

// ProxyStatus summarises the local proxy for the UI.
type ProxyStatus struct {
	Running     bool      `json:"running"`
	Addr        string    `json:"addr"`
	SystemProxy bool      `json:"systemProxy"`
	ShareLAN    bool      `json:"shareLan"`
	Error       *AppError `json:"error,omitempty"`
}

// Snapshot is the UI-facing state, emitted on every change.
type Snapshot struct {
	Status       Status     `json:"status"`
	Step         int        `json:"step"` // 1..7 while connecting
	Error        *AppError  `json:"error,omitempty"`
	Warnings     []AppError `json:"warnings"`
	Servers      []string   `json:"servers"`
	Since        time.Time  `json:"since"`
	LatencyMs    int        `json:"latencyMs"`
	Queries      uint64     `json:"queries"`
	DPI          DPIStatus  `json:"dpi"`
	BlockedSites []string   `json:"blockedSites"`
	// Reasons lists why the connection is degraded: "upstreams", "proxy".
	Reasons []string    `json:"reasons"`
	Proxy   ProxyStatus `json:"proxy"`
}

func (s Snapshot) clone() Snapshot {
	c := s
	c.Warnings = append([]AppError(nil), s.Warnings...)
	c.Servers = append([]string(nil), s.Servers...)
	c.BlockedSites = append([]string(nil), s.BlockedSites...)
	c.Reasons = append([]string(nil), s.Reasons...)
	if s.Proxy.Error != nil {
		e := *s.Proxy.Error
		c.Proxy.Error = &e
	}
	if s.Error != nil {
		e := *s.Error
		c.Error = &e
	}
	return c
}

// LogEvent is a structured, translatable log line for the UI.
type LogEvent struct {
	Time   time.Time      `json:"time"`
	Source string         `json:"source"` // engine | dpi | system | ok
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}
