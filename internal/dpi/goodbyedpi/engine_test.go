package goodbyedpi

import (
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

var light = []string{"-p", "-r", "-s", "-m", "-e", "40", "-w", "--native-frag"}

func args(t *testing.T, strategy string) []string {
	t.Helper()
	a, err := New().Args(dpi.Plan{Strategy: strategy, Scope: dpi.ScopeAll})
	require.NoError(t, err)
	return a
}

func TestArgs_Presets(t *testing.T) {
	require.Equal(t, light, args(t, "light"))
	require.Equal(t, append(append([]string{}, light...), "--auto-ttl", "1-4-10", "--min-ttl", "3"), args(t, "medium"))
	require.Equal(t, append(append([]string{}, light...), "--auto-ttl", "1-4-10", "--min-ttl", "3", "--wrong-seq"), args(t, "high"))
	require.Equal(t, []string{"-p", "-r", "-s", "-m", "-f", "2", "-e", "40", "-w", "--auto-ttl", "1-4-10", "--min-ttl", "3",
		"--native-frag", "--wrong-chksum", "--wrong-seq", "--max-payload"}, args(t, "extreme"))
	require.Equal(t, []string{"-3"}, args(t, "mode3"))
	for _, p := range []string{"light", "medium", "high", "extreme", "mode1", "mode6"} {
		require.NotContains(t, args(t, p), "--dns-addr")
	}
	_, err := New().Args(dpi.Plan{Strategy: "bogus"})
	require.Error(t, err)
}

func TestArgs_BlacklistIsSingleArg(t *testing.T) {
	a, err := New().Args(dpi.Plan{Strategy: "light", Scope: dpi.ScopeBlacklist, Blacklist: "blacklist.txt"})
	require.NoError(t, err)
	require.Equal(t, []string{"--blacklist", "blacklist.txt"}, a[len(a)-2:])
}

func TestArgs_Custom(t *testing.T) {
	a, err := New().Args(dpi.Plan{Strategy: "custom", Custom: `-p -e 40 --auto-ttl 1-4-10`})
	require.NoError(t, err)
	require.Equal(t, []string{"-p", "-e", "40", "--auto-ttl", "1-4-10"}, a)
	_, err = New().Args(dpi.Plan{Strategy: "custom", Custom: `--dns-addr 1.1.1.1`})
	require.ErrorIs(t, err, dpi.ErrForbiddenFlag)
}

func TestValidateCustom(t *testing.T) {
	got, err := validateCustom(`-p -e 40 --auto-ttl 1-4-10 --auto-ttl --wrong-seq --ip-id 7`)
	require.NoError(t, err)
	require.Equal(t, []string{"-p", "-e", "40", "--auto-ttl", "1-4-10", "--auto-ttl", "--wrong-seq", "--ip-id", "7"}, got)
	for _, bad := range []string{"--dns-addr 1.1.1.1", "--dnsv6-port 53", "--blacklist x", "--evil", "-e", "-e abc", "rm -rf", `"-p`} {
		require.Error(t, New().ValidateCustom(bad), bad)
	}
	require.ErrorIs(t, New().ValidateCustom("--dns-addr 1.1.1.1"), dpi.ErrForbiddenFlag)
}

func TestValidateCustom_FakeWithSNI(t *testing.T) {
	e := New()
	require.NoError(t, e.ValidateCustom(`--fake-with-sni www.w3.org --fake-gen 5`))
	require.Error(t, e.ValidateCustom(`--fake-with-sni`))
	require.Error(t, e.ValidateCustom(`--fake-with-sni "a b"`))
	require.Error(t, e.ValidateCustom(`--fake-with-sni -p`))
	require.Error(t, e.ValidateCustom(`--fake-gen 31`))
	require.Error(t, e.ValidateCustom(`--fake-gen 0`))
}

func TestStrategies_AutotuneOrder(t *testing.T) {
	ids := []string{}
	for _, s := range New().Strategies() {
		ids = append(ids, s.ID)
		require.NotEmpty(t, s.Name["vi"])
		require.NotEmpty(t, s.Name["en"])
	}
	require.Equal(t, []string{"light", "medium", "high", "extreme"}, ids)
}

func TestEngineIdentity(t *testing.T) {
	e := New()
	require.Equal(t, "goodbyedpi", e.ID())
	require.Equal(t, "goodbyedpi.exe", e.Exe())
	require.False(t, e.HotReloadsLists())
	require.Equal(t, "8d412b094bb9c137ff25ba9a794d1122ecc84bb776debff6c249723a13cc31cd", e.Files()["goodbyedpi.exe"])
	require.Equal(t, "6110bfa44667405179c3e15e12af1b62037e447ed59b054b19042032995e6c7e", e.Files()["WinDivert.dll"])
	require.Equal(t, "e69b5ba3f0cd6cfb2983e442636e7f0b342b61b15264b0328317d4559c82cf50", e.Files()["WinDivert64.sys"])
}
