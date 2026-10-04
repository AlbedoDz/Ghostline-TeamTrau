package proxy

import (
	"context"
	"net/netip"
	"testing"
	"time"

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
