package app

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// zoneUp answers A queries for the names in zone; other names get no
// answer, like steam.com.
type zoneUp struct {
	zone map[string]string
	down bool
}

func (z zoneUp) Exchange(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	if z.down {
		return nil, context.DeadlineExceeded
	}
	m := new(dns.Msg).SetReply(req)
	if ip, ok := z.zone[req.Question[0].Name]; ok {
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP(ip)})
	}
	return m, nil
}
func (zoneUp) Address() string { return "zone" }
func (zoneUp) Close() error    { return nil }

func domainPicker(up zoneUp) *ScanPicker {
	p, _ := newPicker(nil, store.DefaultSettings())
	p.Checker = scanner.DNSChecker{Build: func(model.Server) (upstream.Upstream, error) { return up, nil }, TestDomain: "www.google.com", Timeout: time.Second}
	return p
}

func TestCheckDomain(t *testing.T) {
	up := zoneUp{zone: map[string]string{"www.google.com.": "142.250.1.1", "store.steampowered.com.": "23.1.1.1"}}
	p := domainPicker(up)
	require.NoError(t, p.CheckDomain(context.Background(), "store.steampowered.com"))
	require.ErrorIs(t, p.CheckDomain(context.Background(), "steam.com"), ErrNoAddress)

	up.down = true // offline: nothing to judge by, so not refused
	require.NoError(t, domainPicker(up).CheckDomain(context.Background(), "steam.com"))
}

func TestSaveSettings_NewTestDomainWithoutAddressIsRefused(t *testing.T) {
	s := newSvc(t)
	var asked []string
	s.svc.x.CheckTestDomain = func(d string) error {
		asked = append(asked, d)
		if d == "steam.com" {
			return ErrNoAddress
		}
		return nil
	}
	st := s.box.Get()
	st.TestDomain = "www.google.com\nsteam.com"
	err := s.svc.SaveSettings(st)
	var ae *AppError
	require.True(t, errors.As(err, &ae))
	require.Equal(t, CodeTestDomainNoAddress, ae.Code)
	require.Equal(t, "TEST_DOMAIN_NO_ADDRESS: steam.com", err.Error())
	require.Equal(t, []string{"steam.com"}, asked, "only new domains are checked")

	st.TestDomain = "www.google.com\nstore.steampowered.com"
	require.NoError(t, s.svc.SaveSettings(st))
}
