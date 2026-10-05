package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
)

// FakeSNIView is the Fake SNI page.
type FakeSNIView struct {
	Ack     bool         `json:"ack"`
	Enabled bool         `json:"enabled"`
	Rules   []rules.Rule `json:"rules"` // user rules with sni=
	Lists   []lists.List `json:"lists"` // lists that can carry sni= (presets and trusted lists)
	Stats   proxy.Stats  `json:"stats"`
}

// AckFakeSNIWarning records that the user read the current warning.
func (s *Service) AckFakeSNIWarning() error {
	st := s.x.Settings.Get()
	st.FakeSNI.AckVersion = FakeSNIWarningVersion
	return s.saveSettings(st, true)
}

// SetFakeSNI turns Fake SNI on or off. Turning it on needs the warning
// read first.
func (s *Service) SetFakeSNI(on bool) error {
	st := s.x.Settings.Get()
	if on && st.FakeSNI.AckVersion < FakeSNIWarningVersion {
		return appErr(CodeFakeSNINotAcked, nil)
	}
	st.FakeSNI.Enabled = on
	return s.saveSettings(st, true)
}

// RetryFakeSNI re-runs the Fake SNI phase after an error.
func (s *Service) RetryFakeSNI() error { return s.o.ReapplyFakeSNI(context.Background()) }

// GetFakeSNIView returns the Fake SNI page content.
func (s *Service) GetFakeSNIView() FakeSNIView {
	st := s.x.Settings.Get()
	v := FakeSNIView{Ack: st.FakeSNI.AckVersion >= FakeSNIWarningVersion, Enabled: st.FakeSNI.Enabled,
		Rules: []rules.Rule{}, Lists: []lists.List{}}
	s.rmu.Lock()
	for _, r := range s.rf.Rules {
		if r.SNI != "" {
			v.Rules = append(v.Rules, r)
		}
	}
	for _, l := range s.rf.Lists {
		if l.TrustedForSNI || l.Action == "perLine" {
			v.Lists = append(v.Lists, l)
		}
	}
	s.rmu.Unlock()
	if s.x.Proxy != nil {
		v.Stats = s.x.Proxy.Stats()
	}
	return v
}

// SetListTrustedForSNI lets (or stops) a list's sni= and connect= take
// effect. The UI asks for confirmation before trusting a list.
func (s *Service) SetListTrustedForSNI(id string, trusted bool) error {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	i := slices.IndexFunc(s.rf.Lists, func(l lists.List) bool { return l.ID == id })
	if i < 0 {
		return fmt.Errorf("lists: no list %q", id)
	}
	s.rf.Lists[i].TrustedForSNI = trusted
	if err := s.saveRulesLocked(); err != nil {
		return err
	}
	s.recompileLocked(id)
	return nil
}

// ListCerts lists Ghostline's certificates in the system store.
func (s *Service) ListCerts() ([]certstore.Cert, error) { return s.o.listCerts() }

// RemoveAllCerts turns the DNS server and Fake SNI off and removes every
// Ghostline certificate from the system store.
func (s *Service) RemoveAllCerts() error {
	st := s.x.Settings.Get()
	st.DNSServer.Enabled, st.FakeSNI.Enabled = false, false
	if err := s.x.Settings.Save(st); err != nil {
		return err
	}
	return s.o.RemoveAllCerts(context.Background())
}

// RetryCertRemoval retries removing session CAs after CERT_REMOVE_FAILED.
func (s *Service) RetryCertRemoval() error { return s.o.RetryCertRemoval() }
