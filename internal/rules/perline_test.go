package rules_test

import (
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func perLineSet(trusted bool) rules.ListSet {
	return rules.ListSet{ID: "p", TrustedForSNI: trusted, Entries: []rules.Entry{
		{Pattern: rules.Pattern{Kind: rules.KindDomain, Value: "vercel.com"}, Line: 2,
			Action: &rules.Action{SNI: "nextjs.org", Connect: "nextjs.org", Fragment: rules.FragOff}},
		{Pattern: rules.Pattern{Kind: rules.KindDomain, Value: "ads.com"}, Line: 3, Action: &rules.Action{Block: true}},
	}}
}

func TestMatch_ListPerLineActions(t *testing.T) {
	c, err := rules.Compile(nil, []rules.ListSet{perLineSet(true)})
	require.NoError(t, err)
	d := c.Match("app.vercel.com", netip.Addr{})
	require.Equal(t, "nextjs.org", d.SNI)
	require.Equal(t, "nextjs.org", d.Connect)
	require.Equal(t, rules.FragOff, d.Fragment)
	require.Equal(t, 2, d.Source.Line)
	require.False(t, d.SNIIgnored)
	require.True(t, c.Match("ads.com", netip.Addr{}).Block)
}

func TestMatch_UntrustedListDropsSNI(t *testing.T) {
	c, err := rules.Compile(nil, []rules.ListSet{perLineSet(false)})
	require.NoError(t, err)
	d := c.Match("vercel.com", netip.Addr{})
	require.Empty(t, d.SNI)
	require.Empty(t, d.Connect)
	require.True(t, d.SNIIgnored)
	require.Equal(t, rules.FragOff, d.Fragment)
	require.True(t, c.Match("ads.com", netip.Addr{}).Block)
}

func TestSNIDomains_TrustedListsOnly(t *testing.T) {
	untrusted := rules.ListSet{ID: "u", Entries: []rules.Entry{
		{Pattern: rules.Pattern{Kind: rules.KindDomain, Value: "evil.com"}, Line: 1, Action: &rules.Action{SNI: "x.com"}}}}
	c, err := rules.Compile(nil, []rules.ListSet{perLineSet(true), untrusted})
	require.NoError(t, err)
	require.Equal(t, []string{"vercel.com"}, c.SNIDomains())
}
