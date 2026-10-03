package servers_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/testutil"
	"github.com/hashcott/ghostline/lists"
	"github.com/stretchr/testify/require"
)

const aaStamp1 = "sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ"
const aaStamp2 = "sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjMADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ"

func TestFromAddress(t *testing.T) {
	cases := map[string]model.Protocol{
		"https://dns.example/dns-query": model.ProtoDoH,
		"tls://dns.example":             model.ProtoDoT,
		"quic://dns.example":            model.ProtoDoQ,
		aaStamp1:                        model.ProtoDoH,
	}
	for addr, proto := range cases {
		s, err := servers.FromAddress(addr, model.SourceCustom)
		require.NoError(t, err, addr)
		require.Equal(t, proto, s.Protocol, addr)
		require.Equal(t, addr, s.Address)
		require.Equal(t, model.SourceCustom, s.Source)
		require.Regexp(t, `^custom:[0-9a-f]{8}$`, s.ID)
	}
	for _, bad := range []string{"udp://1.1.1.1", "1.1.1.1", "tcp://1.1.1.1:53"} {
		_, err := servers.FromAddress(bad, model.SourceCustom)
		require.True(t, errors.Is(err, servers.ErrUnencrypted), bad)
	}
}

func TestFromStamp_TagsAndIPs(t *testing.T) {
	s, err := servers.FromStamp(aaStamp1, model.SourceDNSCrypt)
	require.NoError(t, err)
	require.Equal(t, model.ProtoDoH, s.Protocol)
	require.Equal(t, []string{"217.169.20.22"}, s.IPs)
	require.ElementsMatch(t, []string{"no-filter", "no-log", "dnssec"}, s.Tags)
}

func TestParseDNSCryptMarkdown_OneServerPerStamp(t *testing.T) {
	md := []byte("# public-resolvers\n\nIntro text.\n\n--\n\n## a-and-a\n\nNon-filtering.\n\n" +
		aaStamp1 + "\n" + aaStamp2 + "\n\n## plain-one\n\nPlain.\n\nsdns://AAcAAAAAAAAABzEuMS4xLjE\n")
	got, err := servers.ParseDNSCryptMarkdown(md)
	require.NoError(t, err)
	require.Len(t, got, 2, "plain DNS stamp must be skipped")
	require.Equal(t, "dnscrypt:a-and-a", got[0].ID)
	require.Equal(t, "dnscrypt:a-and-a#2", got[1].ID)
	require.Equal(t, "a-and-a", got[0].Name)
	require.Equal(t, model.SourceDNSCrypt, got[0].Source)
	require.Equal(t, []string{"217.169.20.23"}, got[1].IPs)
}

func TestVerifySigned_RoundTripAndTamper(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	data := []byte(`{"servers":[]}`)
	sig := servers.Sign(data, priv)
	require.NoError(t, servers.VerifySigned(data, sig, pub))
	tampered := append([]byte{}, data...)
	tampered[2] = 'X'
	require.ErrorIs(t, servers.VerifySigned(tampered, sig, pub), servers.ErrBadSignature)
	require.ErrorIs(t, servers.VerifySigned(data, []byte("not base64!"), pub), servers.ErrBadSignature)
}

func TestVerifyMinisign_RoundTripAndTamper(t *testing.T) {
	data := []byte("## a-and-a\n" + aaStamp1 + "\n")
	pubKey, sig := testutil.SignMinisign(t, data)
	require.NoError(t, servers.VerifyMinisign(data, sig, pubKey))
	require.Error(t, servers.VerifyMinisign(append(data, '!'), sig, pubKey))
	otherKey, _ := testutil.SignMinisign(t, data)
	require.Error(t, servers.VerifyMinisign(data, sig, otherKey))
}

func TestParseImport_SkipsBlankAndCommentsReportsBad(t *testing.T) {
	got, bad := servers.ParseImport([]byte("# comment\n\n  https://a.example/dns-query  \r\nudp://1.1.1.1\n"))
	require.Len(t, got, 1)
	require.Equal(t, "https://a.example/dns-query", got[0].Address)
	require.Equal(t, []string{"udp://1.1.1.1"}, bad)
}

func srv(id, addr string, src model.Source, tags ...string) model.Server {
	return model.Server{ID: id, Address: addr, Source: src, Protocol: model.ProtoDoH, Tags: tags}
}

func TestMerge_RemoteOnlyWhenNewerCustomWins(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	builtin := servers.List{GeneratedAt: t0, Servers: []model.Server{srv("b1", "https://b1/dns-query", model.SourceBuiltin)}}
	older := servers.List{GeneratedAt: t0.Add(-time.Hour), Servers: []model.Server{srv("r1", "https://r1/dns-query", model.SourceRemote)}}
	newer := servers.List{GeneratedAt: t0.Add(time.Hour), Servers: []model.Server{srv("r1", "https://r1/dns-query", model.SourceRemote)}}
	dnscrypt := []model.Server{srv("dnscrypt:x", "https://b1/dns-query", model.SourceDNSCrypt), srv("dnscrypt:y", "https://y/dns-query", model.SourceDNSCrypt)}
	custom := []model.Server{srv("custom:1", "https://y/dns-query", model.SourceCustom)}

	ids := func(ss []model.Server) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return out
	}
	require.Equal(t, []string{"b1", "custom:1"}, ids(servers.Merge(builtin, older, dnscrypt, custom)))
	require.Equal(t, []string{"custom:1", "dnscrypt:x", "r1"}, ids(servers.Merge(builtin, newer, dnscrypt, custom)))
}

func TestFilter_IncludeTagsCustomAlwaysKept(t *testing.T) {
	all := []model.Server{
		srv("a", "https://a/q", model.SourceBuiltin, "no-filter"),
		srv("b", "https://b/q", model.SourceBuiltin, "adblock"),
		srv("c", "https://c/q", model.SourceCustom),
	}
	got := servers.Filter(all, []string{"no-filter"})
	require.Len(t, got, 2)
	require.Equal(t, "a", got[0].ID)
	require.Equal(t, "c", got[1].ID)
}

func TestBuiltinListParses(t *testing.T) {
	_, err := servers.ParseList(lists.BuiltinJSON)
	require.NoError(t, err)
}
