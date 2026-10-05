package cfscan_test

import (
	"math/rand/v2"
	"net/netip"
	"testing"
	"testing/quick"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

func TestRanges_Parse(t *testing.T) {
	rs := cfscan.Ranges()
	require.NotEmpty(t, rs)
	for i, p := range rs {
		require.True(t, p.Addr().Is4(), p)
		require.GreaterOrEqual(t, p.Bits(), 8)
		for _, q := range rs[i+1:] {
			require.False(t, p.Overlaps(q), "%s overlaps %s", p, q)
		}
	}
	require.True(t, cfscan.Contains(rs, netip.MustParseAddr("104.16.1.1")))
	require.False(t, cfscan.Contains(rs, netip.MustParseAddr("8.8.8.8")))
}

var small = []netip.Prefix{netip.MustParsePrefix("10.1.0.0/22"), netip.MustParsePrefix("10.9.9.0/24")}

func TestSample_OnePer24(t *testing.T) {
	got := cfscan.Sample(small, 100, rand.New(rand.NewPCG(1, 2)))
	require.Len(t, got, 5) // four /24s in the /22 plus one
	seen := map[netip.Prefix]bool{}
	for _, a := range got {
		p, _ := a.Prefix(24)
		require.False(t, seen[p], "two samples in %s", p)
		seen[p] = true
		last := a.As4()[3]
		require.NotEqual(t, byte(0), last)
		require.NotEqual(t, byte(255), last)
	}
}

func TestSample_Shuffled(t *testing.T) {
	a := cfscan.Sample(cfscan.Ranges(), 50, rand.New(rand.NewPCG(1, 2)))
	b := cfscan.Sample(cfscan.Ranges(), 50, rand.New(rand.NewPCG(3, 4)))
	require.NotEqual(t, a, b)
}

func TestSample_AlwaysInRanges(t *testing.T) {
	rs := cfscan.Ranges()
	f := func(s1, s2 uint64) bool {
		for _, a := range cfscan.Sample(rs, 300, rand.New(rand.NewPCG(s1, s2))) {
			if !cfscan.Contains(rs, a) {
				return false
			}
		}
		return true
	}
	require.NoError(t, quick.Check(f, &quick.Config{MaxCount: 30}))
}

func TestSample_Max(t *testing.T) {
	require.Len(t, cfscan.Sample(cfscan.Ranges(), 200, rand.New(rand.NewPCG(1, 1))), 200)
	require.Len(t, cfscan.Sample(small, 2, rand.New(rand.NewPCG(1, 1))), 2)
}
