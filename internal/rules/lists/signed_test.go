package lists_test

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/stretchr/testify/require"
)

const presetV1 = "# ghostline-rules v1\nyoutube.com sni=www.google.com connect=www.google.com\n"
const presetV2 = "# ghostline-rules v1\nvercel.com sni=nextjs.org connect=nextjs.org\n"

// signedServer serves /p.txt and /p.txt.sig; body and sig can be swapped.
type signedServer struct {
	mu        sync.Mutex
	body, sig []byte
	noSig     bool
}

func (s *signedServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, ".sig"):
		if s.noSig {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(s.sig)
	default:
		_, _ = w.Write(s.body)
	}
}

func (s *signedServer) set(body, sig []byte, noSig bool) {
	s.mu.Lock()
	s.body, s.sig, s.noSig = body, sig, noSig
	s.mu.Unlock()
}

func signedSetup(t *testing.T) (*lists.Fetcher, *signedServer, ed25519.PrivateKey, *lists.List) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	ss := &signedServer{}
	srv := httptest.NewTLSServer(http.HandlerFunc(ss.handler))
	t.Cleanup(srv.Close)
	f := newFetcher(t, routed(srv, nil))
	f.SigKey = pub
	l := &lists.List{ID: "fakesni-google", Source: "url", URL: "https://example.com/p.txt", Format: "auto",
		Action: "perLine", Enabled: true, Signed: true, TrustedForSNI: true}
	return f, ss, priv, l
}

func TestSignedList_ValidReplaces(t *testing.T) {
	f, ss, priv, l := signedSetup(t)
	ss.set([]byte(presetV1), servers.Sign([]byte(presetV1), priv), false)
	r, err := f.Fetch(context.Background(), l)
	require.NoError(t, err)
	require.True(t, l.SignatureOK)
	require.Equal(t, "youtube.com", r.Entries[0].Pattern.Value)
	_, err = os.Stat(filepath.Join(f.Dir, "fakesni-google.txt.sig"))
	require.NoError(t, err)

	ss.set([]byte(presetV2), servers.Sign([]byte(presetV2), priv), false)
	r, err = f.Fetch(context.Background(), l)
	require.NoError(t, err)
	require.Equal(t, "vercel.com", r.Entries[0].Pattern.Value)
}

func TestSignedList_BadSigKeepsOld(t *testing.T) {
	f, ss, priv, l := signedSetup(t)
	ss.set([]byte(presetV1), servers.Sign([]byte(presetV1), priv), false)
	_, err := f.Fetch(context.Background(), l)
	require.NoError(t, err)

	ss.set([]byte(presetV2), servers.Sign([]byte(presetV1), priv), false) // sig of the old body
	_, err = f.Fetch(context.Background(), l)
	require.ErrorIs(t, err, lists.ErrSignatureInvalid)
	require.Equal(t, lists.CodeSignatureInvalid, l.LastError)
	r, err := f.LoadCached(*l)
	require.NoError(t, err)
	require.Equal(t, "youtube.com", r.Entries[0].Pattern.Value)
}

func TestSignedList_MissingSigKeepsOld(t *testing.T) {
	f, ss, priv, l := signedSetup(t)
	ss.set([]byte(presetV1), servers.Sign([]byte(presetV1), priv), false)
	_, err := f.Fetch(context.Background(), l)
	require.NoError(t, err)

	ss.set([]byte(presetV2), nil, true)
	_, err = f.Fetch(context.Background(), l)
	require.ErrorIs(t, err, lists.ErrSignatureInvalid)
	r, err := f.LoadCached(*l)
	require.NoError(t, err)
	require.Equal(t, "youtube.com", r.Entries[0].Pattern.Value)
}

func TestSignedList_TamperedCacheRejected(t *testing.T) {
	f, ss, priv, l := signedSetup(t)
	ss.set([]byte(presetV1), servers.Sign([]byte(presetV1), priv), false)
	_, err := f.Fetch(context.Background(), l)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.Dir, "fakesni-google.txt"), []byte(presetV2), 0o644))
	_, err = f.LoadCached(*l)
	require.ErrorIs(t, err, lists.ErrSignatureInvalid)
}

func TestCatalog_FakeSNIGroup(t *testing.T) {
	n := 0
	for _, it := range lists.Catalog() {
		if it.Category != "fakesni" {
			continue
		}
		n++
		require.True(t, it.Signed, it.ID)
		require.True(t, it.TrustedForSNI, it.ID)
		require.Equal(t, "perLine", it.Action, it.ID)
		require.True(t, strings.HasPrefix(it.URL, "https://raw.githubusercontent.com/hashcott/ghostline/main/lists/fakesni/"), it.URL)
	}
	require.GreaterOrEqual(t, n, 1)
	require.Contains(t, lists.Categories, "fakesni")
}
