package rules_test

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

var ids = []string{"corp", "tor"}

func TestParseText_Valid(t *testing.T) {
	text := `# comment line

youtube.com          fragment=on        # youtube.com and subdomains
=example.com         block
*.doubleclick.net    block
~adservice           block
/^ad[0-9]+\./        block
bank.vn              allow
myrouter.lan         ip=192.168.1.1
example.org          ip=1.2.3.4 ip=2001:db8::1 fragment=off
10.0.0.0/8           upstream=corp
*.onion              upstream=tor
`
	rs, errs := rules.ParseText(text, ids)
	require.Empty(t, errs)
	require.Len(t, rs, 10)
	require.Equal(t, rules.Rule{Pattern: "youtube.com", Action: rules.Action{Fragment: "on"}, Enabled: true, Comment: "youtube.com and subdomains"}, rs[0])
	require.Equal(t, "=example.com", rs[1].Pattern)
	require.True(t, rs[1].Block)
	require.Equal(t, "/^ad[0-9]+\\./", rs[4].Pattern)
	require.True(t, rs[5].Allow)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.168.1.1")}, rs[6].IPs)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("1.2.3.4"), netip.MustParseAddr("2001:db8::1")}, rs[7].IPs)
	require.Equal(t, rules.Frag("off"), rs[7].Fragment)
	require.Equal(t, "corp", rs[8].Upstream)
	require.Equal(t, "tor", rs[9].Upstream)
}

func TestParseText_Errors(t *testing.T) {
	cases := []string{
		"a.com block allow",
		"a.com block fragment=on",
		"a.com ip=abc",
		"a.com fragment=maybe",
		"a.com upstream=nope",
		"/[a-/ block",
		"10.0.0.0/33 block",
		"a.com",
		"a.com colour=red",
		"=*.a.com block",
	}
	for _, c := range cases {
		_, errs := rules.ParseText("ok.com block\n"+c, ids)
		require.Len(t, errs, 1, c)
		require.Equal(t, 2, errs[0].Line, c)
		require.NotEmpty(t, errs[0].Msg, c)
	}
}

func TestParseText_TooManyRules(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= rules.MaxUserRules; i++ {
		fmt.Fprintf(&b, "d%d.com block\n", i)
	}
	_, errs := rules.ParseText(b.String(), nil)
	require.Len(t, errs, 1)
	require.Equal(t, rules.MaxUserRules+1, errs[0].Line)
}

func TestParseText_DisabledRoundTrip(t *testing.T) {
	rs, errs := rules.ParseText("#! youtube.com fragment=on\n", nil)
	require.Empty(t, errs)
	require.Len(t, rs, 1)
	require.False(t, rs[0].Enabled)

	mixed, errs := rules.ParseText(`youtube.com fragment=on # yt
#! =example.com block
~ads block
/^x\d+\./ allow
example.org ip=1.2.3.4 ip=2001:db8::1 fragment=off
10.0.0.0/8 upstream=corp
x.com sni=y.com
`, ids)
	require.Empty(t, errs)
	again, errs := rules.ParseText(rules.FormatText(mixed), ids)
	require.Empty(t, errs)
	require.Equal(t, mixed, again)
}

func TestParseText_SNIAccepted(t *testing.T) {
	rs, errs := rules.ParseText("x.com sni=y.com", nil)
	require.Empty(t, errs)
	require.Equal(t, "y.com", rs[0].SNI)
}

func TestNormalizeHost(t *testing.T) {
	h, err := rules.NormalizeHost("Bánh.VN.")
	require.NoError(t, err)
	require.Equal(t, "xn--bnh-ela.vn", h)
	h, err = rules.NormalizeHost("EXAMPLE.com")
	require.NoError(t, err)
	require.Equal(t, "example.com", h)
	_, err = rules.NormalizeHost("")
	require.Error(t, err)
}

func TestParsePattern(t *testing.T) {
	p, err := rules.ParsePattern("*.Ads.COM")
	require.NoError(t, err)
	require.Equal(t, rules.KindSubOnly, p.Kind)
	require.Equal(t, "ads.com", p.Value)
	p, err = rules.ParsePattern("2001:db8::/32")
	require.NoError(t, err)
	require.Equal(t, rules.KindCIDR, p.Kind)
	require.Equal(t, netip.MustParsePrefix("2001:db8::/32"), p.Prefix)
	p, err = rules.ParsePattern("1.2.3.4")
	require.NoError(t, err)
	require.Equal(t, rules.KindCIDR, p.Kind)
	require.Equal(t, netip.MustParsePrefix("1.2.3.4/32"), p.Prefix)
}
