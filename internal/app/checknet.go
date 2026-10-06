package app

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/lookup"
	"github.com/hashcott/ghostline/internal/probe"
)

// SiteCheck is one test site in the first-run network check.
type SiteCheck struct {
	Site      string `json:"site"`
	Poisoned  bool   `json:"poisoned"`  // the ISP's DNS gave a fake answer
	ISPAnswer string `json:"ispAnswer"` // first address or rcode from the ISP
	Blocked   bool   `json:"blocked"`   // TLS to the real address failed twice
	Stage     string `json:"stage"`     // where the HTTPS probe stopped
}

// NetworkCheck is the first-run check's result and suggested level.
type NetworkCheck struct {
	Sites     []SiteCheck `json:"sites"`
	Poisoned  int         `json:"poisoned"`
	Blocked   int         `json:"blocked"`
	Recommend string      `json:"recommend"` // "dns" | "dpi"
	ISP       string      `json:"isp"`       // the ISP resolver asked (unencrypted), "" if none
	Server    string      `json:"server"`    // the encrypted server used
}

// checkTimeout bounds the whole check.
const checkTimeout = 20 * time.Second

// CheckNetwork looks at this network as it is, before Ghostline changes
// anything: for each test site it compares the ISP's DNS answer with an
// encrypted server's (poisoning) and tries HTTPS to the real address
// without DPI bypass (blocking). It runs only while disconnected.
func (s *Service) CheckNetwork() (NetworkCheck, error) {
	if st := s.o.Snapshot().Status; st != StatusDisconnected && st != StatusError {
		return NetworkCheck{}, appErr(CodeCheckWhileConnected, nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	var enc upstream.Upstream
	var r NetworkCheck
	for _, src := range s.DefaultLookupSources() {
		if src.Kind != "server" {
			continue
		}
		if u, got, err := s.lookupUpstream(src); err == nil {
			enc, r.Server = u, got.Label
			break
		}
	}
	if enc == nil {
		return NetworkCheck{}, errors.New("check: no encrypted DNS server available")
	}
	defer enc.Close()
	var isp upstream.Upstream
	if ips := s.ISPResolvers(); len(ips) > 0 {
		if u, err := s.x.PlainUpstream(ips[0]); err == nil {
			isp, r.ISP = u, ips[0]
			defer isp.Close()
		}
	}

	sites := s.x.Settings.Get().ProbeSites
	if len(sites) > 5 {
		sites = sites[:5]
	}
	r.Sites = make([]SiteCheck, len(sites))
	var wg sync.WaitGroup
	for i, site := range sites {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Sites[i] = s.checkSite(ctx, enc, isp, r.ISP, site)
		}()
	}
	wg.Wait()
	for _, sc := range r.Sites {
		if sc.Poisoned {
			r.Poisoned++
		}
		if sc.Blocked {
			r.Blocked++
		}
	}
	r.Recommend = "dns"
	if r.Blocked > 0 {
		r.Recommend = "dpi"
	}
	return r, nil
}

func (s *Service) checkSite(ctx context.Context, enc, isp upstream.Upstream, ispIP, site string) SiteCheck {
	sc := SiteCheck{Site: site}
	encA := lookup.Query(ctx, enc, lookup.Source{Kind: "server"}, site, "A")
	if isp != nil {
		ispA := lookup.Query(ctx, isp, lookup.Source{Kind: "isp", Ref: ispIP}, site, "A")
		_, per := lookup.Compare("A", []lookup.Answer{encA, ispA})
		sc.Poisoned = per[1] == lookup.VerdictPoisoned
		sc.ISPAnswer = ispA.Rcode
		if len(ispA.Records) > 0 {
			sc.ISPAnswer = ispA.Records[0].Data
		}
	}
	var addrs []netip.Addr
	for _, rec := range encA.Records {
		if a, err := netip.ParseAddr(rec.Data); err == nil && rec.Type == "A" {
			addrs = append(addrs, a)
		}
	}
	p := probe.Prober{
		Resolve: func(context.Context, string) ([]netip.Addr, error) {
			if len(addrs) == 0 {
				return nil, errors.New("no address")
			}
			return addrs, nil
		},
		Dial:    s.x.DialDirect,
		Timeout: 5 * time.Second,
		RootCAs: s.checkRoots,
	}
	// Blocked only when TLS fails twice, like the connected check (one
	// failure can be a fluke).
	first := p.Probe(ctx, site)
	sc.Stage = string(first.Stage)
	if first.Stage == probe.StageTLS {
		sc.Blocked = p.Probe(ctx, site).Stage == probe.StageTLS
	}
	return sc
}

// MarkNetworkChecked records that the first-run check was answered, so it
// does not run on open again.
func (s *Service) MarkNetworkChecked() error {
	st := s.x.Settings.Get()
	st.Simple.Checked = true
	return s.x.Settings.Save(st)
}
