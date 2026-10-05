package proxy

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/proxy/wire"

	"github.com/stretchr/testify/require"
)

// On Windows a wildcard listen on network "tcp" opens a dual-stack socket,
// so 0.0.0.0:p followed by [::]:p collides. Each address must pick its own
// family (final review C1).
func TestListenNetwork_PerFamily(t *testing.T) {
	require.Equal(t, "tcp4", listenNetwork(netip.MustParseAddrPort("0.0.0.0:8080")))
	require.Equal(t, "tcp6", listenNetwork(netip.MustParseAddrPort("[::]:8080")))
	require.Equal(t, "tcp4", listenNetwork(netip.MustParseAddrPort("127.0.0.1:8080")))
	require.Equal(t, "tcp6", listenNetwork(netip.MustParseAddrPort("[::1]:8080")))
}

func TestStart_LoopbackBothFamiliesSamePort(t *testing.T) {
	s := New(Config{Listen: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}})
	require.NoError(t, s.Start(context.Background()))
	p := s.Addrs()[0].Port()
	require.NoError(t, s.Stop(context.Background()))
	s = New(Config{Listen: []netip.AddrPort{
		netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), p),
		netip.AddrPortFrom(netip.IPv6Loopback(), p),
	}})
	require.NoError(t, s.Start(context.Background()))
	require.NoError(t, s.Stop(context.Background()))
}

// A listener that dies without Stop must make Alive false so the health
// check can restart the proxy (final review I8).
func TestAlive_FalseWhenListenerDies(t *testing.T) {
	s := New(Config{Listen: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}})
	require.NoError(t, s.Start(context.Background()))
	defer func() { _ = s.Stop(context.Background()) }()
	require.True(t, s.Alive())
	s.mu.Lock()
	ln := s.lns[0]
	s.mu.Unlock()
	ln.Close()
	require.Eventually(t, func() bool { return !s.Alive() }, 2*time.Second, 10*time.Millisecond)
}

// Fake SNI is for browsers on this PC only (spec 2B §2): devices on the
// LAN would not trust the session CA, so they always take the 2A path.
func TestFakeSNI_OnlyForLoopbackClients(t *testing.T) {
	asked := false
	s := New(Config{MITM: func() mitm.LeafSource { asked = true; return nil }})
	handled := s.fakeSNI(nil, nil, netip.MustParseAddr("192.168.1.9"), wire.Target{Host: "youtube.com", Port: 443}, []byte{0x16})
	require.False(t, handled)
	require.False(t, asked, "LAN clients must not reach the MITM decision")
}
