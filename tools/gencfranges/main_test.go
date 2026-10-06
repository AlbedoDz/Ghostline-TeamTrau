package main

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	got, err := parse([]byte("173.245.48.0/20\r\n104.16.0.0/13\n\n103.21.244.0/22\n"))
	require.NoError(t, err)
	require.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("103.21.244.0/22"),
		netip.MustParsePrefix("104.16.0.0/13"),
		netip.MustParsePrefix("173.245.48.0/20"),
	}, got)

	for _, bad := range []string{"", "\n", "2606:4700::/32\n", "hello\n", "1.0.0.0/4\n", "1.2.3.4\n"} {
		_, err := parse([]byte(bad))
		require.Error(t, err, bad)
	}
}

func TestRender(t *testing.T) {
	out := render([]netip.Prefix{netip.MustParsePrefix("104.16.0.0/13")}, "2026-10-06")
	require.Equal(t, "# source: https://www.cloudflare.com/ips-v4 (fetched 2026-10-06)\n104.16.0.0/13\n", string(out))
}
