package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/qr"
	"github.com/hashcott/ghostline/internal/store"
)

// LANInfo is what the Proxy page shows for LAN sharing.
type LANInfo struct {
	Addrs  []string `json:"addrs"`
	Public bool     `json:"public"`
}

// ProxyQuery reads live proxy counters (nil when the proxy is not wired).
type ProxyQuery interface {
	Stats() proxy.Stats
}

// GetLANInfo lists the addresses other devices can use and whether the
// network is Public (LAN sharing then does not work).
func (s *Service) GetLANInfo() LANInfo {
	if s.x.LANInfo == nil {
		return LANInfo{Addrs: []string{}}
	}
	return s.x.LANInfo()
}

// GetQR encodes text as a QR module matrix for the UI to draw.
func (s *Service) GetQR(text string) ([][]bool, error) { return qr.Encode(text) }

// SetProxyEnabled turns the proxy on or off (tray menu).
func (s *Service) SetProxyEnabled(on bool) error {
	st := s.x.Settings.Get()
	st.Proxy.Enabled = on
	return s.SaveSettings(st)
}

// RestoreSystemProxy retries a failed system proxy restore
// (SYSPROXY_RESTORE_FAILED).
func (s *Service) RestoreSystemProxy() error { return s.o.RestoreProxyNow() }

// RetryProxy re-runs the proxy phase after an error.
func (s *Service) RetryProxy() error { return s.o.ReapplyProxy(context.Background()) }

// SaveUpstreamProxy adds or replaces an upstream proxy. An empty password
// keeps the stored one; a new one is DPAPI-protected before saving.
func (s *Service) SaveUpstreamProxy(u store.UpstreamProxy, password string) error {
	st := s.x.Settings.Get()
	ups := slices.Clone(st.Proxy.Upstreams)
	i := slices.IndexFunc(ups, func(x store.UpstreamProxy) bool { return x.ID == u.ID })
	switch {
	case password != "":
		if s.x.Protect == nil {
			return errors.New("proxy: password protection unavailable")
		}
		enc, err := s.x.Protect(password)
		if err != nil {
			return err
		}
		u.PassEnc = enc
	case i >= 0:
		u.PassEnc = ups[i].PassEnc
	default:
		u.PassEnc = ""
	}
	if i >= 0 {
		ups[i] = u
	} else {
		ups = append(ups, u)
	}
	st.Proxy.Upstreams = ups
	if err := store.ValidateProxy(st.Proxy); err != nil {
		return err
	}
	return s.x.Settings.Save(st)
}

// DeleteUpstreamProxy removes an upstream no rule or list refers to.
func (s *Service) DeleteUpstreamProxy(id string) error {
	s.rmu.Lock()
	for _, r := range s.rf.Rules {
		if r.Upstream == id {
			s.rmu.Unlock()
			return fmt.Errorf("proxy: upstream %q is used by rule %q", id, r.Pattern)
		}
	}
	for _, l := range s.rf.Lists {
		if l.Action == "upstream="+id {
			s.rmu.Unlock()
			return fmt.Errorf("proxy: upstream %q is used by list %q", id, l.Name)
		}
	}
	s.rmu.Unlock()
	st := s.x.Settings.Get()
	st.Proxy.Upstreams = slices.DeleteFunc(slices.Clone(st.Proxy.Upstreams), func(x store.UpstreamProxy) bool { return x.ID == id })
	return s.x.Settings.Save(st)
}

// TestUpstreamProxy opens a TLS connection to www.google.com:443 through
// the upstream.
func (s *Service) TestUpstreamProxy(id string) error {
	if s.x.TestUpstream == nil {
		return errors.New("proxy: not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.x.TestUpstream(ctx, id)
}

// GetFragCache lists hosts remembered as needing fragmentation on the
// current network.
func (s *Service) GetFragCache() []string {
	if s.x.FragCache == nil || s.x.NetKey == nil {
		return []string{}
	}
	return s.x.FragCache.List(s.x.NetKey())
}

// ClearFragCache forgets host ("" = every host) on the current network.
func (s *Service) ClearFragCache(host string) error {
	if s.x.FragCache == nil || s.x.NetKey == nil {
		return nil
	}
	s.x.FragCache.Remove(s.x.NetKey(), strings.TrimSpace(host))
	return s.x.FragCache.Save(s.x.Paths.FragCache)
}

// GetProxyStats returns the proxy counters.
func (s *Service) GetProxyStats() proxy.Stats {
	if s.x.Proxy == nil {
		return proxy.Stats{ByOutcome: map[string]uint64{}}
	}
	return s.x.Proxy.Stats()
}

// GetProxyConns returns the RAM-only recent connections (only while the
// query view is on).
func (s *Service) GetProxyConns() []proxy.ConnEvent {
	if !s.x.Bus.queryLog.Load() {
		return []proxy.ConnEvent{}
	}
	return s.x.Bus.ProxyConns()
}

// AnswerSysProxyOverride is the user's reply to SYSPROXY_EXISTING.
func (s *Service) AnswerSysProxyOverride(replace bool) {
	s.mu.Lock()
	ch := s.overrideCh
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- replace:
		default:
		}
	}
}

// AskOverride shows SYSPROXY_EXISTING and waits for the user's answer
// (false on timeout). It is a function so Wails does not bind it.
func AskOverride(ctx context.Context, s *Service, server, pac string, timeout time.Duration) bool {
	ch := make(chan bool, 1)
	s.mu.Lock()
	s.overrideCh = ch
	s.mu.Unlock()
	s.o.AddWarning(AppError{Code: CodeSysProxyExisting, Params: map[string]any{"server": server, "pac": pac}})
	defer func() {
		s.o.ClearWarning(CodeSysProxyExisting)
		s.mu.Lock()
		s.overrideCh = nil
		s.mu.Unlock()
	}()
	select {
	case ans := <-ch:
		return ans
	case <-time.After(timeout):
		return false
	case <-ctx.Done():
		return false
	}
}

// proxyPhaseChanged reports settings changes that need the proxy phase
// re-run (rules, fragment and upstream changes are read live).
func proxyPhaseChanged(a, b store.ProxySettings) bool {
	return a.Enabled != b.Enabled || a.Port != b.Port || a.SystemProxy != b.SystemProxy || a.ShareLAN != b.ShareLAN
}
