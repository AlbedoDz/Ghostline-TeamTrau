package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	clientLogMu.Lock()
	clientLogCount = 0
	clientLogMu.Unlock()
	return &buf
}

func TestLogClientError_WritesAndKeepsOnlyCodeOfBindingErrors(t *testing.T) {
	buf := captureSlog(t)
	s := &Service{}
	s.LogClientError("error", "TypeError: x is undefined", "at App (App.tsx:10)")
	s.LogClientError("rejection", "LOOKUP_BAD_NAME: bad name secret.example", "stack")
	out := buf.String()
	require.Contains(t, out, `msg="ui: error"`)
	require.Contains(t, out, "TypeError: x is undefined")
	require.Contains(t, out, "App.tsx:10")
	require.Contains(t, out, "msg=LOOKUP_BAD_NAME")
	require.NotContains(t, out, "secret.example")
}

func TestLogClientError_MutesAfterBurst(t *testing.T) {
	buf := captureSlog(t)
	s := &Service{}
	for i := 0; i < clientLogBurst+10; i++ {
		s.LogClientError("error", "boom", "")
	}
	out := buf.String()
	require.Equal(t, clientLogBurst, strings.Count(out, "msg=boom"))
	require.Equal(t, 1, strings.Count(out, "too many errors"))
}

func TestTruncate_KeepsValidUTF8(t *testing.T) {
	require.Equal(t, "ab", truncate("ab", 5))
	require.Equal(t, "l…", truncate("lỗi", 2))
}
