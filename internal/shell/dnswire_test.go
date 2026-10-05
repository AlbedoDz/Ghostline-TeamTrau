package shell

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type deadUp struct{}

func (deadUp) Exchange(context.Context, *dns.Msg) (*dns.Msg, error) {
	return nil, errors.New("offline")
}
func (deadUp) Address() string { return "dead" }
func (deadUp) Close() error    { return nil }

var _ upstream.Upstream = deadUp{}

func TestDNSWiring_SelfTest(t *testing.T) {
	eng := engine.New(nil)
	require.NoError(t, eng.Start(context.Background(), engine.Config{ListenV4: netip.MustParseAddrPort("127.0.0.1:0"), Upstreams: []upstream.Upstream{deadUp{}}}))
	t.Cleanup(func() { _ = eng.Stop(context.Background()) })

	cw := testCertWiring(t, certstore.NewFake(), t.TempDir())
	ca, err := cw.LANCA(context.Background())
	require.NoError(t, err)
	leaf, err := ca.IssueServer([]string{"dns.ghostline.lan"}, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, time.Hour, time.Now())
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	doh := netip.MustParseAddrPort(ln.Addr().String())
	ln.Close()

	w := &dnsWiring{eng: eng, certs: cw}
	require.Error(t, w.SelfTest(context.Background()), "not serving yet")
	_, err = w.Serve(context.Background(), engine.ServeConfig{DoH: []netip.AddrPort{doh}, Cert: func() *tls.Certificate { return leaf }})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.StopServe(context.Background()) })
	require.NoError(t, w.SelfTest(context.Background()))
}
