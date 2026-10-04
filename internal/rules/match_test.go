package rules_test

import (
	"fmt"
	"math/rand"
	"net/netip"
	"sync"
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func compile(t testing.TB, text string, lists ...rules.ListSet) *rules.Compiled {
	t.Helper()
	rs, errs := rules.ParseText(text, []string{"corp", "tor"})
	require.Empty(t, errs)
	c, err := rules.Compile(rs, lists)
	require.NoError(t, err)
	return c
}

func entry(t testing.TB, pat string) rules.Entry {
	p, err := rules.ParsePattern(pat)
	require.NoError(t, err)
	return rules.Entry{Pattern: p}
}

var noIP netip.Addr

func TestMatch_PatternKinds(t *testing.T) {
	c := compile(t, `ads.com block
=exact.com block
*.sub.com block
~adservice block
/^ad[0-9]+\./ block
10.0.0.0/8 block
`)
	blocked := func(host string, ip netip.Addr) bool { return c.Match(host, ip).Block }
	require.True(t, blocked("ads.com", noIP))
	require.True(t, blocked("x.ads.com", noIP))
	require.False(t, blocked("badads.com", noIP))
	require.True(t, blocked("exact.com", noIP))
	require.False(t, blocked("x.exact.com", noIP))
	require.False(t, blocked("sub.com", noIP))
	require.True(t, blocked("a.sub.com", noIP))
	require.True(t, blocked("pagead.adservice.google.com", noIP))
	require.True(t, blocked("ad12.x.com", noIP))
	require.False(t, blocked("adx.x.com", noIP))
	require.True(t, blocked("", netip.MustParseAddr("10.1.2.3")))
	require.False(t, blocked("", netip.MustParseAddr("11.1.2.3")))
	require.False(t, blocked("other.org", noIP))
}

func TestMatch_Order(t *testing.T) {
	list1 := rules.ListSet{ID: "l1", Action: rules.Action{Block: true}, Entries: []rules.Entry{entry(t, "bank.vn"), entry(t, "both.com")}}
	list2 := rules.ListSet{ID: "l2", Action: rules.Action{Fragment: rules.FragOn}, Entries: []rules.Entry{entry(t, "both.com"), entry(t, "two.com")}}
	c := compile(t, `x.com fragment=off
other.com block
y.com allow
x.com block
x.com fragment=on
bank.vn allow
`, list1, list2)
	d := c.Match("x.com", noIP)
	require.Equal(t, rules.Source{Kind: "rule", Index: 0}, d.Source)
	require.Equal(t, rules.FragOff, d.Fragment)
	d = c.Match("bank.vn", noIP)
	require.True(t, d.Allow)
	require.False(t, d.Block)
	d = c.Match("both.com", noIP)
	require.True(t, d.Block)
	require.Equal(t, "l1", d.Source.ListID)
	d = c.Match("two.com", noIP)
	require.Equal(t, rules.FragOn, d.Fragment)
	require.Equal(t, "l2", d.Source.ListID)
	require.Equal(t, "", c.Match("none.com", noIP).Source.Kind)
}

func TestMatch_DisabledRuleIgnored(t *testing.T) {
	c := compile(t, "#! x.com block\n")
	require.False(t, c.Match("x.com", noIP).Block)
}

func TestMatch_ListException(t *testing.T) {
	exc := entry(t, "ok.x.com")
	exc.Except = true
	l1 := rules.ListSet{ID: "l1", Action: rules.Action{Block: true}, Entries: []rules.Entry{entry(t, "x.com"), exc}}
	l2 := rules.ListSet{ID: "l2", Action: rules.Action{Fragment: rules.FragOn}, Entries: []rules.Entry{entry(t, "x.com")}}
	c := compile(t, "", l1, l2)
	require.True(t, c.Match("a.x.com", noIP).Block)
	d := c.Match("ok.x.com", noIP)
	require.False(t, d.Block)
	require.Equal(t, "l2", d.Source.ListID)
}

func TestMatch_FromFileIPs(t *testing.T) {
	e := entry(t, "=site.com")
	e.IPs = []netip.Addr{netip.MustParseAddr("1.2.3.4")}
	l := rules.ListSet{ID: "h", FromFile: true, Entries: []rules.Entry{e, entry(t, "=ads.com")}}
	c := compile(t, "", l)
	require.Equal(t, e.IPs, c.Match("site.com", noIP).IPs)
	require.True(t, c.Match("ads.com", noIP).Block)
}

func TestMatch_CIDRFillsUnset(t *testing.T) {
	c := compile(t, `youtube.com fragment=on
blocked.com block
142.250.0.0/15 upstream=corp fragment=off
`)
	d := c.Match("youtube.com", netip.MustParseAddr("142.250.1.1"))
	require.Equal(t, rules.FragOn, d.Fragment)
	require.Equal(t, "corp", d.Upstream)
	require.Equal(t, 0, d.Source.Index)
	d = c.Match("blocked.com", netip.MustParseAddr("142.250.1.1"))
	require.True(t, d.Block)
	require.Empty(t, d.Upstream)
	d = c.Match("plain.com", netip.MustParseAddr("142.250.1.1"))
	require.Equal(t, "corp", d.Upstream)
	require.Equal(t, rules.FragOff, d.Fragment)
	require.Equal(t, 2, d.Source.Index)
}

func TestMatch_CIDRv6(t *testing.T) {
	c := compile(t, "2001:db8::/32 block\n")
	require.True(t, c.Match("", netip.MustParseAddr("2001:db8::1")).Block)
	require.False(t, c.Match("", netip.MustParseAddr("2001:db9::1")).Block)
	require.False(t, c.Match("", netip.MustParseAddr("1.2.3.4")).Block)
}

func TestMatch_IDNAndCaseAndTrailingDot(t *testing.T) {
	c := compile(t, "Bánh.VN. block\n")
	require.True(t, c.Match("xn--bnh-ela.vn", noIP).Block)
	require.True(t, c.Match("WWW.xn--bnh-ela.vn", noIP).Block)
	require.True(t, c.Match("www.bánh.vn.", noIP).Block)
	require.True(t, c.Explain("BÁNH.vn").Block)
}

func TestExplain(t *testing.T) {
	l := rules.ListSet{ID: "hagezi", Action: rules.Action{Block: true}, Entries: []rules.Entry{{Pattern: entry(t, "ads.com").Pattern, Line: 120}}}
	c := compile(t, "", l)
	d := c.Explain("x.ads.com")
	require.True(t, d.Block)
	require.Equal(t, rules.Source{Kind: "list", ListID: "hagezi", Line: 120}, d.Source)
	require.Equal(t, 1, c.Count())
}

func TestHolder_ConcurrentSwap(t *testing.T) {
	var h rules.Holder
	require.NotNil(t, h.Load())
	require.False(t, h.Load().Match("x.com", noIP).Block)
	a := compile(t, "x.com block\n")
	b := compile(t, "x.com fragment=on\n")
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = h.Load().Match("x.com", noIP)
				}
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		if i%2 == 0 {
			h.Store(a)
		} else {
			h.Store(b)
		}
	}
	close(stop)
	wg.Wait()
	require.Equal(t, rules.FragOn, h.Load().Match("x.com", noIP).Fragment)
}

func bigList(t testing.TB, n int) (rules.ListSet, []string) {
	r := rand.New(rand.NewSource(1))
	hosts := make([]string, n)
	es := make([]rules.Entry, n)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("h%x.d%x.example", r.Int63(), i)
		es[i] = rules.Entry{Pattern: rules.Pattern{Kind: rules.KindDomain, Value: hosts[i]}}
	}
	return rules.ListSet{ID: "big", Action: rules.Action{Block: true}, Entries: es}, hosts
}

func TestMatch_NoAllocs(t *testing.T) {
	l, hosts := bigList(t, 1000)
	c := compile(t, "youtube.com fragment=on\n10.0.0.0/8 block\n", l)
	ip := netip.MustParseAddr("10.1.1.1")
	miss, hit := "www.a.b.c.example", "www."+hosts[7]
	allocs := testing.AllocsPerRun(100, func() {
		_ = c.Match(miss, ip)
		_ = c.Match(hit, noIP)
	})
	require.Zero(t, allocs)
}

func BenchmarkMatch2M(b *testing.B) {
	l, hosts := bigList(b, 2_000_000)
	c, err := rules.Compile(nil, []rules.ListSet{l})
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Match("www."+hosts[i%len(hosts)], noIP)
	}
}
