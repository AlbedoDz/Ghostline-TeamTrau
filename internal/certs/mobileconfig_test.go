package certs_test

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/stretchr/testify/require"
)

func wellFormed(t *testing.T, b []byte) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		require.NoError(t, err)
	}
}

func TestMobileConfig(t *testing.T) {
	ca, err := certs.NewLANCA("PC1", t0)
	require.NoError(t, err)
	in := certs.ProfileInput{CA: ca, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("fd00::5")}, Port: 443, SSID: "Nhà a&b<c"}
	b, err := certs.MobileConfig(in)
	require.NoError(t, err)
	wellFormed(t, b)
	for _, want := range []string{
		"com.apple.security.root", "com.apple.dnsSettings.managed", "<string>HTTPS</string>",
		"<string>https://dns.ghostline.lan/dns-query</string>",
		"<string>192.168.1.5</string>", "<string>fd00::5</string>",
		"<key>OnDemandRules</key>", "<key>SSIDMatch</key>",
		"<string>Nhà a&amp;b&lt;c</string>", "<string>Disconnect</string>",
		base64.StdEncoding.EncodeToString(ca.DER)[:60],
	} {
		require.Contains(t, string(b), want)
	}

	in.Port = 8443
	b2, err := certs.MobileConfig(in)
	require.NoError(t, err)
	require.Contains(t, string(b2), "https://dns.ghostline.lan:8443/dns-query")

	// Same CA → same payload UUIDs, so re-installing replaces the profile.
	b3, err := certs.MobileConfig(certs.ProfileInput{CA: ca, Addrs: in.Addrs, Port: 443, SSID: "other"})
	require.NoError(t, err)
	require.Equal(t, uuids(b), uuids(b3))

	_, err = certs.MobileConfig(certs.ProfileInput{CA: ca, Addrs: in.Addrs, Port: 443})
	require.ErrorIs(t, err, certs.ErrNoSSID)
}

func uuids(b []byte) []string {
	var out []string
	s := string(b)
	for {
		i := bytes.Index([]byte(s), []byte("<key>PayloadUUID</key>"))
		if i < 0 {
			return out
		}
		s = s[i+len("<key>PayloadUUID</key>"):]
		j := bytes.Index([]byte(s), []byte("</string>"))
		out = append(out, s[:j])
	}
}
