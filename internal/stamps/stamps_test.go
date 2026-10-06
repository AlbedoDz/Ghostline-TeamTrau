package stamps_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/stamps"
	"github.com/stretchr/testify/require"
)

var (
	hash32 = strings.Repeat("ab", 32)
	key32  = strings.Repeat("cd", 32)
)

func TestRoundTrip(t *testing.T) {
	for _, f := range []stamps.Fields{
		{Proto: "doh", Addr: "8.8.8.8", Host: "dns.google", Path: "/dns-query", Hashes: []string{hash32}, DNSSEC: true, NoLog: true},
		{Proto: "doh", Host: "dns.example:8443", Path: "/q", NoFilter: true},
		{Proto: "dot", Addr: "1.1.1.1", Host: "one.one.one.one"},
		{Proto: "doq", Addr: "[2a10:50c0::ad1:ff]:853", Host: "dns.adguard-dns.com"},
		{Proto: "dnscrypt", Addr: "9.9.9.9:8443", ProviderName: "2.dnscrypt-cert.example", PublicKey: key32, DNSSEC: true},
		{Proto: "plain", Addr: "1.1.1.1"},
	} {
		s, err := stamps.Encode(f)
		require.NoError(t, err, f.Proto)
		require.True(t, strings.HasPrefix(s, "sdns://"))
		got, err := stamps.Decode(s)
		require.NoError(t, err)
		require.Equal(t, s, got.Stamp)
		require.Equal(t, f.Proto != "plain", got.Usable, f.Proto)
		got.Stamp, got.Usable = "", false
		if got.Hashes == nil {
			got.Hashes = f.Hashes
		}
		require.Equal(t, f, got, f.Proto)
	}
}

func TestDecode_KnownStamps(t *testing.T) {
	for _, s := range []string{
		"sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ",
		"sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjMADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ",
		"sdns://AAcAAAAAAAAABzEuMS4xLjE",
	} {
		f, err := stamps.Decode(s)
		require.NoError(t, err, s)
		require.NotEmpty(t, f.Proto)
	}
	f, _ := stamps.Decode("sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ")
	require.Equal(t, "doh", f.Proto)
	require.Equal(t, "217.169.20.22", f.Addr)
	require.Equal(t, "dns.aa.net.uk", f.Host)
	require.Equal(t, "/dns-query", f.Path)
	require.True(t, f.DNSSEC && f.NoLog && f.NoFilter)
}

func lp(s string) []byte { return append([]byte{byte(len(s))}, s...) }

func raw(b ...[]byte) string {
	var all []byte
	for _, x := range b {
		all = append(all, x...)
	}
	return "sdns://" + base64.RawURLEncoding.EncodeToString(all)
}

func TestDecode_RelayNotUsable(t *testing.T) {
	props := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	f, err := stamps.Decode(raw([]byte{0x81}, lp("1.2.3.4:443")))
	require.NoError(t, err)
	require.Equal(t, "dnscrypt-relay", f.Proto)
	require.Equal(t, "1.2.3.4:443", f.Addr)
	require.False(t, f.Usable)

	f, err = stamps.Decode(raw([]byte{0x05}, props, lp("odoh.example"), lp("/dns-query")))
	require.NoError(t, err)
	require.Equal(t, "odoh-target", f.Proto)
	require.Equal(t, "odoh.example", f.Host)
	require.Equal(t, "/dns-query", f.Path)
	require.True(t, f.DNSSEC)
	require.False(t, f.Usable)

	f, err = stamps.Decode(raw([]byte{0x85}, props, lp("5.6.7.8"), []byte{0}, lp("relay.example"), lp("/proxy")))
	require.NoError(t, err)
	require.Equal(t, "odoh-relay", f.Proto)
	require.Equal(t, "5.6.7.8", f.Addr)
	require.Equal(t, "relay.example", f.Host)
	require.Equal(t, "/proxy", f.Path)
	require.False(t, f.Usable)
}

func TestDecode_Rejects(t *testing.T) {
	for _, s := range []string{"", "https://x", "sdns://", "sdns://!!", "sdns://" + base64.RawURLEncoding.EncodeToString([]byte{0x77})} {
		_, err := stamps.Decode(s)
		require.ErrorIs(t, err, stamps.ErrInvalid, s)
	}
}

func TestEncode_Rejects(t *testing.T) {
	cases := map[string]stamps.Fields{
		"hashes":       {Proto: "doh", Host: "a.com", Path: "/q", Hashes: []string{strings.Repeat("ab", 31)}},
		"publicKey":    {Proto: "dnscrypt", Addr: "1.1.1.1", ProviderName: "2.dnscrypt-cert.x", PublicKey: strings.Repeat("cd", 31)},
		"providerName": {Proto: "dnscrypt", Addr: "1.1.1.1", ProviderName: "x.example", PublicKey: key32},
		"addr":         {Proto: "dot", Addr: "300.1.1.1", Host: "a.com"},
		"host":         {Proto: "doh", Path: "/q"},
		"proto":        {Proto: "odoh-relay", Host: "a.com"},
	}
	for field, f := range cases {
		_, err := stamps.Encode(f)
		require.True(t, errors.Is(err, stamps.ErrInvalid), field)
		require.Contains(t, err.Error(), field)
	}
}

func TestFromURL(t *testing.T) {
	f, err := stamps.FromURL("https://dns.google/dns-query", "8.8.8.8")
	require.NoError(t, err)
	require.Equal(t, stamps.Fields{Proto: "doh", Addr: "8.8.8.8", Host: "dns.google", Path: "/dns-query"}, f)
	f, err = stamps.FromURL("tls://one.one.one.one", "")
	require.NoError(t, err)
	require.Equal(t, stamps.Fields{Proto: "dot", Host: "one.one.one.one"}, f)
	f, err = stamps.FromURL("quic://dns.adguard-dns.com:853", "")
	require.NoError(t, err)
	require.Equal(t, stamps.Fields{Proto: "doq", Host: "dns.adguard-dns.com:853"}, f)
	f, err = stamps.FromURL("https://dns.example", "")
	require.NoError(t, err)
	require.Equal(t, "/dns-query", f.Path)
	for _, bad := range []string{"http://x", "ftp://x", "https://", "tls://a.com", "x"} {
		ip := ""
		if bad == "tls://a.com" {
			ip = "nope"
		}
		_, err := stamps.FromURL(bad, ip)
		require.ErrorIs(t, err, stamps.ErrInvalid, bad)
	}
}
