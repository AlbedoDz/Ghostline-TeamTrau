package winutil

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirewallArgs(t *testing.T) {
	exe := `C:\Program Files\Ghostline Đức\ghostline.exe`
	require.Equal(t, []string{"advfirewall", "firewall", "add", "rule", "name=Ghostline Proxy", "dir=in", "action=allow",
		"protocol=TCP", "localport=8080", "program=" + exe, "profile=private", "remoteip=localsubnet"}, FirewallAddArgs(8080, exe))
	require.Equal(t, []string{"advfirewall", "firewall", "delete", "rule", "name=Ghostline Proxy"}, FirewallDeleteArgs())
}

type fakeNetsh struct {
	calls [][]string
	fail  map[string]bool // by verb: add, delete, show
}

func (f *fakeNetsh) run(args []string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.fail[args[2]] {
		return []byte("localized failure text"), errors.New("exit status 1")
	}
	return []byte("Ok."), nil
}

func withNetsh(t *testing.T, f *fakeNetsh) {
	old := runNetsh
	runNetsh = f.run
	t.Cleanup(func() { runNetsh = old })
}

func TestDeleteFirewallRule_NoMatchIsNil(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"delete": true, "show": true}}
	withNetsh(t, f)
	require.NoError(t, DeleteFirewallRule())
	require.Equal(t, "show", f.calls[1][2])
}

func TestDeleteFirewallRule_RealFailure(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"delete": true}} // rule exists but cannot be deleted
	withNetsh(t, f)
	require.Error(t, DeleteFirewallRule())
}

func TestAddFirewallRule_ReplacesExisting(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"show": true}}
	withNetsh(t, f)
	require.NoError(t, AddFirewallRule(8080, `C:\g.exe`))
	require.Equal(t, "delete", f.calls[0][2])
	require.Equal(t, "add", f.calls[len(f.calls)-1][2])

	f = &fakeNetsh{fail: map[string]bool{"add": true}}
	withNetsh(t, f)
	require.Error(t, AddFirewallRule(8080, `C:\g.exe`))
}

func TestParseCategories(t *testing.T) {
	require.True(t, parsePublic("Private\r\nPublic\r\n"))
	require.False(t, parsePublic("Private\r\nDomainAuthenticated\r\n"))
	require.False(t, parsePublic(""))
}

func TestLANAddrs(t *testing.T) {
	ifs := []net.Interface{
		{Index: 1, Name: "Wi-Fi", Flags: net.FlagUp},
		{Index: 2, Name: "Loopback", Flags: net.FlagUp | net.FlagLoopback},
		{Index: 3, Name: "Down", Flags: 0},
	}
	addrs := map[int][]net.Addr{
		1: {mustCIDR("192.168.1.5/24"), mustCIDR("8.8.8.8/32"), mustCIDR("fe80::1/64"), mustCIDR("fd00::5/64")},
		2: {mustCIDR("127.0.0.1/8")},
		3: {mustCIDR("10.0.0.9/8")},
	}
	got := LANAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return addrs[i.Index], nil })
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("fd00::5")}, got)
}

func mustCIDR(s string) net.Addr {
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	n.IP = ip
	return n
}

// Final review I6: the SSRF guard needs every address of this machine,
// including public IPv4 and global IPv6, not only the private ones.
func TestUnicastAddrs_IncludesPublicAndGlobal(t *testing.T) {
	ifs := []net.Interface{{Index: 1, Name: "Wi-Fi", Flags: net.FlagUp}, {Index: 2, Name: "Down", Flags: 0}}
	addrs := map[int][]net.Addr{
		1: {mustCIDR("192.168.1.5/24"), mustCIDR("203.0.113.7/24"), mustCIDR("2405:4800::5/64"), mustCIDR("fe80::1/64"), mustCIDR("100.64.1.2/10")},
		2: {mustCIDR("10.0.0.9/8")},
	}
	got := UnicastAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return addrs[i.Index], nil })
	require.ElementsMatch(t, []netip.Addr{
		netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("2405:4800::5"),
		netip.MustParseAddr("fe80::1"), netip.MustParseAddr("100.64.1.2"),
	}, got)
}

func TestIsPrivateOrLocal(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.5", "fd00::1", "169.254.1.1", "fe80::1"} {
		require.True(t, IsPrivateOrLocal(netip.MustParseAddr(s)), s)
	}
	for _, s := range []string{"8.8.8.8", "172.32.0.1", "2001:4860::8888", "100.64.0.1"} {
		require.False(t, IsPrivateOrLocal(netip.MustParseAddr(s)), s)
	}
	require.True(t, IsPrivateOrLocal(netip.MustParseAddr("::ffff:192.168.1.5")))
}

func TestFirewallRuleArgs(t *testing.T) {
	exe := `C:\g.exe`
	tail := []string{"program=" + exe, "profile=private", "remoteip=localsubnet"}
	want := func(name, proto, ports string) []string {
		return append([]string{"advfirewall", "firewall", "add", "rule", "name=" + name, "dir=in", "action=allow",
			"protocol=" + proto, "localport=" + ports}, tail...)
	}
	require.Equal(t, want("Ghostline DNS (TCP)", "TCP", "53,443"), FirewallRuleArgs(FirewallRule{Name: RuleDNSTCP, Protocol: "TCP", Ports: []int{53, 443}}, exe))
	require.Equal(t, want("Ghostline DNS (UDP)", "UDP", "53"), FirewallRuleArgs(FirewallRule{Name: RuleDNSUDP, Protocol: "UDP", Ports: []int{53}}, exe))
	require.Equal(t, want("Ghostline Setup", "TCP", "8053"), FirewallRuleArgs(FirewallRule{Name: RuleSetup, Protocol: "TCP", Ports: []int{8053}}, exe))
}

func TestNamedRule_AddDelete(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"show": true}}
	withNetsh(t, f)
	require.NoError(t, AddNamedRule(FirewallRule{Name: RuleDNSUDP, Protocol: "UDP", Ports: []int{53}}, `C:\g.exe`))
	require.Equal(t, []string{"advfirewall", "firewall", "delete", "rule", "name=Ghostline DNS (UDP)"}, f.calls[0])
	require.Equal(t, "add", f.calls[len(f.calls)-1][2])

	f = &fakeNetsh{fail: map[string]bool{"delete": true, "show": true}}
	withNetsh(t, f)
	require.NoError(t, DeleteNamedRule(RuleSetup))
	require.Equal(t, []string{"advfirewall", "firewall", "show", "rule", "name=Ghostline Setup"}, f.calls[1])
}

func TestAllRuleNames(t *testing.T) {
	require.Equal(t, []string{"Ghostline Proxy", "Ghostline DNS (TCP)", "Ghostline DNS (UDP)", "Ghostline Setup"}, AllRuleNames)
}
