package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetPinnedMany(t *testing.T) {
	sh := newSvc(t)
	require.NoError(t, sh.svc.SetPinned("a", true))
	require.NoError(t, sh.svc.SetPinnedMany([]string{"b", "a", "c", "b"}, true))
	require.Equal(t, []string{"a", "b", "c"}, sh.svc.GetSettings().Pinned) // no duplicates, order kept
	require.NoError(t, sh.svc.SetPinnedMany([]string{"a", "c", "zzz"}, false))
	require.Equal(t, []string{"b"}, sh.svc.GetSettings().Pinned)
	require.NoError(t, sh.svc.SetPinnedMany(nil, true))
	require.Equal(t, []string{"b"}, sh.svc.GetSettings().Pinned)
}
