package updater_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/testutil"
	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

func TestNewer(t *testing.T) {
	require.True(t, updater.Newer("0.1.0", "v0.2.0"))
	require.True(t, updater.Newer("v0.1.0", "v0.1.1"))
	require.False(t, updater.Newer("0.2.0", "v0.2.0"))
	require.False(t, updater.Newer("0.3.0", "v0.2.0"))
	require.False(t, updater.Newer("dev", "v9.0.0"))
	require.False(t, updater.Newer("0.1.0", "garbage"))
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	require.True(t, updater.Due(time.Time{}, now))
	require.True(t, updater.Due(now.Add(-24*time.Hour), now))
	require.False(t, updater.Due(now.Add(-23*time.Hour), now))
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://x/r"}`))
	}))
	defer srv.Close()
	r, err := updater.Latest(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, updater.Release{Tag: "v0.2.0", URL: "https://x/r"}, r)
}

func TestLatest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	_, err := updater.Latest(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
}

func serveFiles(t *testing.T, files map[string][]byte, fail map[string]bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail[r.URL.Path] {
			w.WriteHeader(500)
			return
		}
		b, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchServerList(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	data, _ := json.Marshal(servers.List{GeneratedAt: time.Unix(1, 0).UTC()})
	srv := serveFiles(t, map[string][]byte{"/s.json": data, "/s.json.sig": servers.Sign(data, priv)}, nil)
	l, raw, sig, err := updater.FetchServerList(context.Background(), srv.Client(), srv.URL+"/s.json", srv.URL+"/s.json.sig", pub)
	require.NoError(t, err)
	require.Equal(t, data, raw)
	require.NotEmpty(t, sig)
	require.True(t, l.GeneratedAt.Equal(time.Unix(1, 0)))
}

func TestFetchServerList_BadSignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	data := []byte(`{"servers":[]}`)
	srv := serveFiles(t, map[string][]byte{"/s.json": data, "/s.json.sig": servers.Sign(data, other)}, nil)
	_, _, _, err := updater.FetchServerList(context.Background(), srv.Client(), srv.URL+"/s.json", srv.URL+"/s.json.sig", pub)
	require.ErrorIs(t, err, servers.ErrBadSignature)
}

func TestFetchDNSCrypt_FallsBackToSecondURL(t *testing.T) {
	md := []byte("## x\nsdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ\n")
	key, sig := testutil.SignMinisign(t, md)
	srv := serveFiles(t, map[string][]byte{"/b/list.md": md, "/b/list.md.minisig": sig}, map[string]bool{"/a/list.md": true})
	gotMD, gotSig, err := updater.FetchDNSCrypt(context.Background(), srv.Client(), []string{srv.URL + "/a/list.md", srv.URL + "/b/list.md"}, key)
	require.NoError(t, err)
	require.Equal(t, md, gotMD)
	require.Equal(t, sig, gotSig)
}

func TestFetchDNSCrypt_BadSignatureEverywhere(t *testing.T) {
	md := []byte("## x\n")
	key, _ := testutil.SignMinisign(t, md)
	_, badSig := testutil.SignMinisign(t, md)
	srv := serveFiles(t, map[string][]byte{"/a/l.md": md, "/a/l.md.minisig": badSig}, nil)
	_, _, err := updater.FetchDNSCrypt(context.Background(), srv.Client(), []string{srv.URL + "/a/l.md"}, key)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "signature"))
}
