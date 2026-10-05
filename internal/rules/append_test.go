package rules_test

import (
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func ipRule(p string, ips ...string) rules.Rule {
	r := rules.Rule{Pattern: p, Enabled: true}
	for _, s := range ips {
		r.IPs = append(r.IPs, netip.MustParseAddr(s))
	}
	return r
}

func TestAppendUserRules_AppendsInOrder(t *testing.T) {
	old := []rules.Rule{ipRule("a.com", "1.1.1.1")}
	out, n, errs := rules.AppendUserRules(old, []rules.Rule{ipRule("b.com", "2.2.2.2"), ipRule("*.c.com", "3.3.3.3")}, nil)
	require.Empty(t, errs)
	require.Equal(t, 2, n)
	require.Equal(t, []string{"a.com", "b.com", "*.c.com"}, []string{out[0].Pattern, out[1].Pattern, out[2].Pattern})
}

func TestAppendUserRules_RejectsBadAndDedups(t *testing.T) {
	old := []rules.Rule{ipRule("a.com", "1.1.1.1")}
	// Any invalid rule rejects the whole batch.
	out, n, errs := rules.AppendUserRules(old, []rules.Rule{
		ipRule("b.com", "2.2.2.2"),
		ipRule("bad..com", "2.2.2.2"),
		{Pattern: "c.com", Enabled: true}, // no action
		{Pattern: "d.com", Action: rules.Action{Upstream: "nope"}, Enabled: true},
	}, []string{"corp"})
	require.Equal(t, 0, n)
	require.Equal(t, old, out)
	require.Equal(t, []int{2, 3, 4}, []int{errs[0].Line, errs[1].Line, errs[2].Line})

	// Duplicates (same pattern and action) are skipped; a different action is kept.
	out, n, errs = rules.AppendUserRules(old, []rules.Rule{
		ipRule("a.com", "1.1.1.1"),
		ipRule("a.com", "2.2.2.2"),
		ipRule("a.com", "2.2.2.2"),
	}, nil)
	require.Empty(t, errs)
	require.Equal(t, 1, n)
	require.Len(t, out, 2)
	require.Equal(t, "2.2.2.2", out[1].IPs[0].String())
}

func TestAppendUserRules_Cap(t *testing.T) {
	old := make([]rules.Rule, rules.MaxUserRules)
	for i := range old {
		old[i] = ipRule("a.com", "1.1.1.1")
	}
	out, n, errs := rules.AppendUserRules(old, []rules.Rule{ipRule("b.com", "2.2.2.2")}, nil)
	require.Equal(t, 0, n)
	require.Len(t, out, rules.MaxUserRules)
	require.Equal(t, []rules.LineError{{Line: 0, Msg: "too many rules"}}, errs)
}
