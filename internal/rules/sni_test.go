package rules_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func TestParseText_SNIConnect(t *testing.T) {
	rs, errs := rules.ParseText("youtube.com sni=www.google.com connect=www.google.com\nexample.org sni=none", nil)
	require.Empty(t, errs)
	require.Equal(t, "www.google.com", rs[0].SNI)
	require.Equal(t, "www.google.com", rs[0].Connect)
	require.Equal(t, rules.SNINone, rs[1].SNI)
}

func TestParseText_SNIRejectsNonDomainPatterns(t *testing.T) {
	for _, l := range []string{"~tube sni=a.com", "/yt/ sni=a.com", "10.0.0.0/8 sni=a.com", "~tube connect=a.com"} {
		_, errs := rules.ParseText(l, nil)
		require.Len(t, errs, 1, l)
		require.Contains(t, errs[0].Msg, "only with domain patterns", l)
	}
}

func TestParseText_ConnectWithIPRejected(t *testing.T) {
	_, errs := rules.ParseText("a.com connect=b.com ip=1.2.3.4", nil)
	require.Len(t, errs, 1)
}

func TestParseText_ConnectAloneIsAnAction(t *testing.T) {
	rs, errs := rules.ParseText("a.com connect=b.com", nil)
	require.Empty(t, errs)
	require.Equal(t, "b.com", rs[0].Connect)
}

func TestFormatText_RoundTripSNIConnect(t *testing.T) {
	rs, errs := rules.ParseText("youtube.com sni=www.google.com connect=www.google.com\nexample.org sni=none", nil)
	require.Empty(t, errs)
	back, errs := rules.ParseText(rules.FormatText(rs), nil)
	require.Empty(t, errs)
	require.Equal(t, rs, back)
}

func TestValidateRule_SNIKeyword(t *testing.T) {
	err := rules.ValidateRule(rules.Rule{Pattern: "~tube", Action: rules.Action{SNI: "a.com"}, Enabled: true}, nil)
	require.ErrorContains(t, err, "only with domain patterns")
}

func TestSNIDomains(t *testing.T) {
	user := []rules.Rule{
		{Pattern: "youtube.com", Action: rules.Action{SNI: "www.google.com"}, Enabled: true},
		{Pattern: "*.googlevideo.com", Action: rules.Action{SNI: "www.google.com"}, Enabled: true},
		{Pattern: "=example.com", Action: rules.Action{SNI: rules.SNINone}, Enabled: true},
		{Pattern: "youtube.com", Action: rules.Action{SNI: "x.com"}, Enabled: true},
		{Pattern: "off.com", Action: rules.Action{SNI: "x.com"}, Enabled: false},
		{Pattern: "plain.com", Action: rules.Action{Block: true}, Enabled: true},
	}
	c, err := rules.Compile(user, nil)
	require.NoError(t, err)
	require.Equal(t, []string{".googlevideo.com", "example.com", "youtube.com"}, c.SNIDomains())
}
