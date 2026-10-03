package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/stretchr/testify/require"
)

func TestGenerate_FillsIPsSetsTimeAndSorts(t *testing.T) {
	seed := []model.Server{
		{ID: "b", Protocol: model.ProtoDoH, Address: "https://b.example/dns-query", Tags: []string{"no-filter"}},
		{ID: "a", Protocol: model.ProtoDoT, Address: "tls://a.example", Tags: []string{"no-filter"}},
	}
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	var asked []string
	l, err := Generate(seed, func(h string) ([]string, error) { asked = append(asked, h); return []string{"192.0.2.10"}, nil }, now)
	require.NoError(t, err)
	require.Equal(t, now, l.GeneratedAt)
	require.Equal(t, "a", l.Servers[0].ID)
	require.Equal(t, []string{"192.0.2.10"}, l.Servers[1].IPs)
	require.Equal(t, model.SourceBuiltin, l.Servers[0].Source)
	require.ElementsMatch(t, []string{"a.example", "b.example"}, asked)
}

func TestGenerate_KeepsStampIPsWithoutResolving(t *testing.T) {
	seed := []model.Server{{ID: "s", Address: "sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ"}}
	l, err := Generate(seed, func(string) ([]string, error) { t.Fatal("must not resolve stamps"); return nil, nil }, time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"217.169.20.22"}, l.Servers[0].IPs)
	require.Equal(t, model.ProtoDoH, l.Servers[0].Protocol)
}

func TestGenerate_RejectsUnencrypted(t *testing.T) {
	_, err := Generate([]model.Server{{ID: "x", Address: "udp://1.1.1.1"}}, func(string) ([]string, error) { return nil, nil }, time.Now())
	require.Error(t, err)
}

func TestGenerate_ResolveFailureIsError(t *testing.T) {
	_, err := Generate([]model.Server{{ID: "x", Address: "https://x.example/dns-query"}},
		func(string) ([]string, error) { return nil, errors.New("nxdomain") }, time.Now())
	require.Error(t, err)
}

func TestCommittedListSignatureMatchesBrandKey(t *testing.T) {
	data, err := os.ReadFile("../../lists/servers.json")
	require.NoError(t, err)
	sig, err := os.ReadFile("../../lists/servers.json.sig")
	require.NoError(t, err)
	pub, err := hex.DecodeString(brand.ServerListPublicKeyHex)
	require.NoError(t, err)
	require.NoError(t, servers.VerifySigned(data, sig, ed25519.PublicKey(pub)))
}
