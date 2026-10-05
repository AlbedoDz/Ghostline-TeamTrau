package lists_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/formats"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/lists"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedFakeSNIPresets(t *testing.T) {
	names, err := fs.Glob(lists.FakeSNIPresets, "fakesni/*.txt")
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for _, n := range names {
		data, err := fs.ReadFile(lists.FakeSNIPresets, n)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(data), formats.GhostlineHeader), n)
		r, err := formats.Parse(formats.Ghostline, data)
		require.NoError(t, err, n)
		require.Zero(t, r.Skipped, "%s has lines the rule parser rejects: %v", n, r.Samples)
		for _, e := range r.Entries {
			require.NotNil(t, e.Action, n)
			require.NotEmpty(t, e.Action.SNI, "%s line %d: every preset line sets sni=", n, e.Line)
			require.Contains(t, []rules.PatternKind{rules.KindDomain, rules.KindExact, rules.KindSubOnly}, e.Pattern.Kind)
		}
	}
}

// Signatures are checked for releases: the maintainer signs presets with
// the servers.json key (tools/genservers -sign-file). Unsigned presets are
// never used by the app, which verifies before loading.
func TestEmbeddedFakeSNIPresets_Signed(t *testing.T) {
	if os.Getenv("GHOSTLINE_RELEASE") == "" {
		t.Skip("set GHOSTLINE_RELEASE=1 to require signed presets")
	}
	pub, err := hex.DecodeString(brand.ServerListPublicKeyHex)
	require.NoError(t, err)
	names, _ := fs.Glob(lists.FakeSNIPresets, "fakesni/*.txt")
	for _, n := range names {
		data, _ := fs.ReadFile(lists.FakeSNIPresets, n)
		sig, err := fs.ReadFile(lists.FakeSNIPresets, n+".sig")
		require.NoError(t, err, "%s is not signed", n)
		require.NoError(t, servers.VerifySigned(data, sig, ed25519.PublicKey(pub)), n)
	}
}

func TestFakeSNIFallbackByURL(t *testing.T) {
	data, _, ok := lists.FakeSNIFallback("https://raw.githubusercontent.com/hashcott/ghostline/main/lists/fakesni/google.txt")
	require.True(t, ok)
	require.NotEmpty(t, data)
	_, _, ok = lists.FakeSNIFallback("https://example.com/google.txt")
	require.False(t, ok)
}
