package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// nameUp answers A queries from a name → IP map (NXDOMAIN otherwise).
type nameUp map[string]string

func (u nameUp) Exchange(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	name := strings.TrimSuffix(req.Question[0].Name, ".")
	ip, ok := u[name]
	if !ok {
		return new(dns.Msg).SetRcode(req, dns.RcodeNameError), nil
	}
	m := new(dns.Msg).SetReply(req)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP(ip)}}
	return m, nil
}
func (nameUp) Address() string { return "names" }
func (nameUp) Close() error    { return nil }

// httpsSites serves HTTPS for names with a test CA.
func httpsSites(t *testing.T, names ...string) (string, *x509.CertPool) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	caDER, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: names, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return srv.Listener.Addr().String(), pool
}

var probeSites = []string{"youtube.com", "discord.com", "x.com"}

// newCheck: the encrypted server answers 1.0.0.x; addresses in blockedIPs
// get their TLS handshake cut (DPI), the rest reach a real HTTPS server.
func newCheck(t *testing.T, isp nameUp, blockedIPs ...string) *toolsHarness {
	h := newTools(t)
	st := h.box.Get()
	st.ProbeSites = probeSites
	require.NoError(t, h.box.Save(st))
	addr, roots := httpsSites(t, probeSites...)
	h.svc.checkRoots = roots
	h.svc.x.BuildUpstream = func(model.Server) (upstream.Upstream, error) {
		return nameUp{"youtube.com": "1.0.0.1", "discord.com": "1.0.0.2", "x.com": "1.0.0.3"}, nil
	}
	h.svc.x.PlainUpstream = func(string) (upstream.Upstream, error) { return isp, nil }
	h.svc.x.DialDirect = func(ctx context.Context, network, to string) (net.Conn, error) {
		for _, ip := range blockedIPs {
			if strings.HasPrefix(to, ip+":") {
				a, b := net.Pipe()
				_ = b.Close() // reset during the ClientHello, as DPI does
				return a, nil
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	return h
}

func TestCheckNetwork_PoisonedAndBlockedRecommendsDPI(t *testing.T) {
	isp := nameUp{"youtube.com": "10.10.34.35", "discord.com": "1.0.0.2", "x.com": "1.0.0.3"}
	h := newCheck(t, isp, "1.0.0.2")
	r, err := h.svc.CheckNetwork()
	require.NoError(t, err)
	require.Equal(t, 1, r.Poisoned)
	require.Equal(t, 1, r.Blocked)
	require.Equal(t, "dpi", r.Recommend)
	require.Equal(t, "203.162.4.191", r.ISP)
	require.Len(t, r.Sites, 3)
	require.True(t, r.Sites[0].Poisoned)
	require.False(t, r.Sites[0].Blocked)
	require.True(t, r.Sites[1].Blocked)
	require.False(t, r.Sites[2].Poisoned || r.Sites[2].Blocked)
}

func TestCheckNetwork_AllClearRecommendsDNS(t *testing.T) {
	isp := nameUp{"youtube.com": "1.0.0.1", "discord.com": "1.0.0.2", "x.com": "1.0.0.3"}
	h := newCheck(t, isp)
	r, err := h.svc.CheckNetwork()
	require.NoError(t, err)
	require.Zero(t, r.Poisoned)
	require.Zero(t, r.Blocked)
	require.Equal(t, "dns", r.Recommend)
}

func TestCheckNetwork_RefusedWhileConnected(t *testing.T) {
	h := newCheck(t, nameUp{})
	h.o.update(func(s *Snapshot) { s.Status = StatusProtected })
	_, err := h.svc.CheckNetwork()
	require.Equal(t, CodeCheckWhileConnected, code(t, err))
}

func TestCheckNetwork_NoISPResolverStillChecksDPI(t *testing.T) {
	h := newCheck(t, nameUp{}, "1.0.0.1")
	h.svc.x.ISPResolvers = func() []string { return nil }
	r, err := h.svc.CheckNetwork()
	require.NoError(t, err)
	require.Equal(t, "", r.ISP)
	require.Zero(t, r.Poisoned)
	require.Equal(t, 1, r.Blocked)
	require.Equal(t, "dpi", r.Recommend)
}

func TestMarkNetworkChecked(t *testing.T) {
	h := newTools(t)
	st := h.box.Get()
	st.Simple.Checked = false
	st.Language = "en"
	require.NoError(t, h.box.Save(st))
	require.NoError(t, h.svc.MarkNetworkChecked())
	got, _, err := store.LoadSettings(h.paths.Settings)
	require.NoError(t, err)
	require.True(t, got.Simple.Checked)
	require.Equal(t, "en", got.Language, "nothing else changes")
}
