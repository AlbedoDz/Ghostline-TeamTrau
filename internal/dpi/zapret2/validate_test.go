package zapret2

import (
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

func TestValidateArgs(t *testing.T) {
	ok := []string{
		"--lua-desync=multisplit:pos=1,midsld",
		"--lua-desync=fake:blob=fake_default_tls:badsum",
		"--lua-desync=fake:blob=fake_default_tls:ip_autottl=-2,3-20",
		"--lua-desync=multidisorder:pos=1,sniext+1:seqovl=midsld-1",
		"--lua-desync=fake:blob=0x1603",
		"--lua-desync=fakeddisorder:pos=2:seqovl_pattern=fake_default_tls",
		"--lua-desync=pass",
		"--lua-desync=fake:blob=fake_default_tls:ipfrag=ipfrag2",
		"--payload=tls_client_hello",
		"--payload=tls_client_hello,quic_initial",
		"--out-range=-n3",
		"--in-range=a1<d10",
	}
	for _, a := range ok {
		require.NoError(t, ValidateArgs([]string{a}), a)
	}
	bad := []string{
		"--lua-desync=luaexec:code=os.execute",
		// Keys whose value names a Lua global the library calls (_G[v]) or loads.
		"--lua-desync=fake:blob=fake_default_tls:fool=luaexec",
		"--lua-desync=fake:fool=error",
		"--lua-desync=multisplit:ipfrag=luaexec",
		"--lua-desync=multisplit:ipfrag=",
		"--lua-desync=fake:hostkey=luaexec",
		"--lua-desync=fake:iff=luaexec",
		"--lua-desync=fake:cond=luaexec",
		"--lua-desync=fake:failure_detector=luaexec",
		"--lua-desync=fake:success_detector=luaexec",
		"--lua-desync=fake:code=x",
		"--lua-desync=fake:cond_code=x",
		"--lua-desync=pktdebug",
		"--lua-desync=",
		"--lua-desync=fake:blob=@C:/x.bin",
		"--lua-desync=fake:blob=evil_blob",
		"--lua-desync=fake:blob=0x",
		"--lua-desync=fake:blob=0x" + strings.Repeat("ab", 1025),
		"--lua-desync=multisplit:pos=1/2",
		"--lua-desync=multisplit:pos=a:b=c=d",
		"--lua-desync=multisplit:Pos=1",
		"--lua-desync=fake:k=" + strings.Repeat("a", 65),
		"--lua-desync=fake:k=a b",
		"--lua-desync=fake:k=a\\b",
		"--lua-init=@x.lua",
		"--blob=x:@f",
		"--writable",
		"--debug=@log",
		"--wf-raw=@f",
		"@config.txt",
		"--new",
		"--filter-tcp=1",
		"--hostlist=x",
		"--ipset=x",
		"--pidfile=x",
		"--chdir",
		"--daemon",
		"--intercept=0",
		"--payload=@x",
		"--payload=",
		"--out-range=@x",
		"--lua-desync multisplit",
		"",
	}
	for _, a := range bad {
		err := ValidateArgs([]string{a})
		require.Error(t, err, a)
		require.ErrorIs(t, err, dpi.ErrForbiddenFlag, a)
	}
}

func TestValidateCustom(t *testing.T) {
	got, err := ValidateCustom(`--lua-desync=fake:blob=fake_default_tls:badsum  --lua-desync=multidisorder:pos=1,midsld`)
	require.NoError(t, err)
	require.Equal(t, []string{"--lua-desync=fake:blob=fake_default_tls:badsum", "--lua-desync=multidisorder:pos=1,midsld"}, got)
	_, err = ValidateCustom(`--lua-desync=fake --lua-init=@x.lua`)
	require.ErrorIs(t, err, dpi.ErrForbiddenFlag)
	_, err = ValidateCustom(`"--lua-desync=fake`)
	require.Error(t, err)
	got, err = ValidateCustom("")
	require.NoError(t, err)
	require.Empty(t, got)
}

func FuzzValidateCustom(f *testing.F) {
	for _, s := range []string{"--lua-desync=fake:blob=fake_default_tls", `"--lua-init=@a b"`, "--payload=x,y --out-range=-n3", "@cfg",
		"--lua-desync=fake:fool=luaexec", "--lua-desync=multisplit:ipfrag=ipfrag2"} {
		f.Add(s)
	}
	allowed := []string{"--lua-desync=", "--payload=", "--out-range=", "--in-range="}
	f.Fuzz(func(t *testing.T, s string) {
		args, err := ValidateCustom(s)
		if err != nil {
			return
		}
		for _, a := range args {
			okPrefix := false
			for _, p := range allowed {
				okPrefix = okPrefix || strings.HasPrefix(a, p)
			}
			fnRef := false
			for _, k := range []string{":fool=", ":hostkey=", ":iff=", ":cond=", ":code=", ":cond_code=", ":failure_detector=", ":success_detector="} {
				fnRef = fnRef || strings.Contains(a, k)
			}
			if i := strings.Index(a, ":ipfrag="); i >= 0 && !strings.HasPrefix(a[i:], ":ipfrag=ipfrag2") {
				fnRef = true
			}
			if !okPrefix || fnRef || strings.ContainsAny(a, "@/\\ \t\r\n\x00") {
				t.Fatalf("accepted %q from %q", a, s)
			}
		}
	})
}
