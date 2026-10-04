package formats_test

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/formats"
	"github.com/stretchr/testify/require"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

func TestDetect_Testdata(t *testing.T) {
	cases := map[string]formats.Format{
		"hosts.txt":    formats.Hosts,
		"domains.txt":  formats.Domains,
		"adblock.txt":  formats.Adblock,
		"dnsmasq.conf": formats.Dnsmasq,
		"unbound.conf": formats.Unbound,
		"rpz.txt":      formats.RPZ,
		"clash.yaml":   formats.Clash,
		"clash.list":   formats.Clash,
		"v2fly-google": formats.V2fly,
		"singbox.json": formats.Singbox,
		"cidr.txt":     formats.CIDR,
	}
	for name, want := range cases {
		got, err := formats.Detect(name, read(t, name))
		require.NoError(t, err, name)
		require.Equal(t, want, got, name)
	}
}

func TestDetect_Unsupported(t *testing.T) {
	_, err := formats.Detect("geosite.dat", []byte("\x0a\x05\x00\x01binary\x00stuff"))
	require.ErrorIs(t, err, formats.ErrUnsupported)
	_, err = formats.Detect("readme.txt", []byte("This is a readme.\nIt explains things.\nNothing to block here.\n"))
	require.ErrorIs(t, err, formats.ErrUnsupported)
}

type want struct {
	pat    string
	kind   rules.PatternKind
	except bool
	ips    string
}

func check(t *testing.T, r formats.Result, ws []want) {
	t.Helper()
	got := make([]want, 0, len(r.Entries))
	for _, e := range r.Entries {
		w := want{kind: e.Pattern.Kind, except: e.Except}
		if e.Pattern.Kind == rules.KindCIDR {
			w.pat = e.Pattern.Prefix.String()
		} else {
			w.pat = e.Pattern.Value
		}
		var ips []string
		for _, ip := range e.IPs {
			ips = append(ips, ip.String())
		}
		w.ips = strings.Join(ips, ",")
		require.Positive(t, e.Line)
		got = append(got, w)
	}
	require.Equal(t, ws, got)
}

func parse(t *testing.T, f formats.Format, name string) formats.Result {
	t.Helper()
	r, err := formats.Parse(f, read(t, name))
	require.NoError(t, err)
	require.LessOrEqual(t, len(r.Samples), 5)
	require.Equal(t, f, r.Format)
	return r
}

func TestParse_Hosts(t *testing.T) {
	r := parse(t, formats.Hosts, "hosts.txt")
	check(t, r, []want{
		{"ads.example", rules.KindExact, false, ""},
		{"tracker.example", rules.KindExact, false, ""},
		{"metrics.example", rules.KindExact, false, ""},
		{"v6sink.example", rules.KindExact, false, ""},
		{"mirror.example", rules.KindExact, false, "1.2.3.4"},
		{"a.example", rules.KindExact, false, ""},
		{"b.example", rules.KindExact, false, ""},
	})
	require.Equal(t, 7, r.Counts["exact"])
	require.Zero(t, r.Skipped)
}

func TestParse_Domains(t *testing.T) {
	r := parse(t, formats.Domains, "domains.txt")
	check(t, r, []want{
		{"ads.example", rules.KindDomain, false, ""},
		{"tracker.example", rules.KindDomain, false, ""},
		{"wild.example", rules.KindSubOnly, false, ""},
		{"metrics.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 1, r.Skipped)
	require.Equal(t, []string{"not a domain line"}, r.Samples)
	require.Equal(t, 3, r.Counts["domain"])
	require.Equal(t, 1, r.Counts["suffix"])
}

func TestParse_AdblockModifiers(t *testing.T) {
	r := parse(t, formats.Adblock, "adblock.txt")
	check(t, r, []want{
		{"ads.example", rules.KindDomain, false, ""},
		{"tracker.example", rules.KindDomain, false, ""},
		{"ok.ads.example", rules.KindDomain, true, ""},
		{"metrics.example", rules.KindDomain, false, ""},
		{"all.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 2, r.Skipped) // $third-party and ##cosmetic
}

func TestParse_Dnsmasq(t *testing.T) {
	r := parse(t, formats.Dnsmasq, "dnsmasq.conf")
	check(t, r, []want{
		{"ads.example", rules.KindDomain, false, ""},
		{"real.example", rules.KindDomain, false, "1.2.3.4"},
		{"tracker.example", rules.KindDomain, false, ""},
		{"metrics.example", rules.KindDomain, false, ""},
		{"multi1.example", rules.KindDomain, false, ""},
		{"multi2.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 1, r.Skipped) // forwarding server=/fwd/9.9.9.9
}

func TestParse_Unbound(t *testing.T) {
	r := parse(t, formats.Unbound, "unbound.conf")
	check(t, r, []want{
		{"ads.example", rules.KindDomain, false, ""},
		{"tracker.example", rules.KindDomain, false, ""},
		{"metrics.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 1, r.Skipped) // transparent
}

func TestParse_RPZ(t *testing.T) {
	r := parse(t, formats.RPZ, "rpz.txt")
	check(t, r, []want{
		{"ads.example", rules.KindExact, false, ""},
		{"ads.example", rules.KindSubOnly, false, ""},
		{"tracker.example", rules.KindExact, false, ""},
	})
	require.Equal(t, 1, r.Skipped) // passthru
}

func TestParse_ClashSkipsUnknown(t *testing.T) {
	r := parse(t, formats.Clash, "clash.yaml")
	check(t, r, []want{
		{"google.example", rules.KindDomain, false, ""},
		{"exact.example", rules.KindExact, false, ""},
		{"adservice", rules.KindKeyword, false, ""},
		{"1.0.0.0/8", rules.KindCIDR, false, ""},
		{"plus.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 1, r.Skipped)
	r = parse(t, formats.Clash, "clash.list")
	check(t, r, []want{
		{"google.example", rules.KindDomain, false, ""},
		{"exact.example", rules.KindExact, false, ""},
		{`^ad[0-9]+\.`, rules.KindRegexp, false, ""},
		{"2001:db8::/32", rules.KindCIDR, false, ""},
		{"yt.example", rules.KindDomain, false, ""},
	})
	require.Equal(t, 1, r.Skipped)
}

func TestParse_V2flyIncludeAndAttrs(t *testing.T) {
	r := parse(t, formats.V2fly, "v2fly-google")
	check(t, r, []want{
		{"google.example", rules.KindDomain, false, ""},
		{"g.example", rules.KindDomain, false, ""},
		{"www.full.example", rules.KindExact, false, ""},
		{"gstatic", rules.KindKeyword, false, ""},
		{`^gg[0-9]+\.example$`, rules.KindRegexp, false, ""},
	})
	require.Equal(t, []string{"google-ads"}, r.Includes)
}

func TestParse_Singbox(t *testing.T) {
	r, err := formats.Parse(formats.Singbox, read(t, "singbox.json"))
	require.NoError(t, err)
	var kinds []rules.PatternKind
	for _, e := range r.Entries {
		kinds = append(kinds, e.Pattern.Kind)
	}
	require.Equal(t, []rules.PatternKind{rules.KindExact, rules.KindDomain, rules.KindSubOnly, rules.KindKeyword, rules.KindRegexp, rules.KindCIDR}, kinds)
	require.Equal(t, 1, r.Skipped) // logical rule
}

func TestParse_CIDR(t *testing.T) {
	r := parse(t, formats.CIDR, "cidr.txt")
	require.Len(t, r.Entries, 4)
	require.Equal(t, netip.MustParsePrefix("1.2.3.4/32"), r.Entries[3].Pattern.Prefix)
}

func TestParse_RegexpCap(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= formats.MaxRegexpPerList; i++ {
		fmt.Fprintf(&b, "regexp:^r%d\\.example$\n", i)
	}
	r, err := formats.Parse(formats.V2fly, []byte(b.String()))
	require.NoError(t, err)
	require.Len(t, r.Entries, formats.MaxRegexpPerList)
	require.Equal(t, 1, r.Skipped)
}

func TestFormats_BOMCRLFAndInlineComments(t *testing.T) {
	data := []byte("\xef\xbb\xbf# list\r\nads.com # tracker\r\n\r\n")
	f, err := formats.Detect("list.txt", data)
	require.NoError(t, err)
	require.Equal(t, formats.Domains, f)
	r, err := formats.Parse(f, data)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)
	require.Equal(t, "ads.com", r.Entries[0].Pattern.Value)
	require.Equal(t, 2, r.Entries[0].Line)
	require.Zero(t, r.Skipped)
}
