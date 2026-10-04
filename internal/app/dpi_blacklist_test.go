package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var ae *AppError
	require.True(t, errors.As(err, &ae), "want AppError %s, got %v", code, err)
	require.Equal(t, code, ae.Code)
}

func countOf(list []string, name string) int {
	n := 0
	for _, s := range list {
		if s == name {
			n++
		}
	}
	return n
}

func TestSaveDPIBlacklist_RejectsEmpty(t *testing.T) {
	s := newSvc(t)
	requireCode(t, s.svc.SaveDPIBlacklist(" \n# only a comment\n\n"), CodeDPIBlacklistEmpty)
	_, err := os.Stat(s.paths.DPIBlacklist)
	require.True(t, errors.Is(err, os.ErrNotExist), "nothing written")
}

func TestSaveDPIBlacklist_RestartsRunningDPI(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	st := s.box.Get()
	st.DPI.Scope = "blacklist"
	require.NoError(t, s.box.Save(st))
	require.NoError(t, s.o.Connect(context.Background()))
	require.NoError(t, s.o.SetDPIEnabled(context.Background(), true))
	starts := countOf(s.r.list(), "dpi.start")

	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\ndiscord.com\n"))
	require.Equal(t, starts+1, countOf(s.r.list(), "dpi.start"), "restarted so the new list applies")
	require.True(t, s.o.Snapshot().DPI.Running)
	require.Contains(t, s.dpi.startedArgs(), s.paths.DPIBlacklist)
}

func TestSaveDPIBlacklist_DoesNotStartStoppedDPI(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	require.NotContains(t, s.r.list(), "dpi.start")
}

func TestSaveSettings_BlacklistScopeNeedsEntries(t *testing.T) {
	s := newSvc(t)
	st := s.box.Get()
	st.DPI.Scope = "blacklist"
	requireCode(t, s.svc.SaveSettings(st), CodeDPIBlacklistEmpty)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, "blacklist", s.box.Get().DPI.Scope)
}

func TestSaveSettings_DPIOptionChangeRestartsRunningDPI(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	require.NoError(t, s.o.Connect(context.Background()))
	require.NoError(t, s.o.SetDPIEnabled(context.Background(), true))
	starts := countOf(s.r.list(), "dpi.start")

	st := s.box.Get()
	st.TestDomain = "example.com" // unrelated change: no restart
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, starts, countOf(s.r.list(), "dpi.start"))

	st.DPI.Scope = "blacklist"
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, starts+1, countOf(s.r.list(), "dpi.start"))
	require.Contains(t, s.dpi.startedArgs(), "--blacklist")
}

func TestSaveSettings_ExistingBlacklistScopeStillSaves(t *testing.T) {
	s := newSvc(t)
	st := s.box.Get()
	st.DPI.Scope = "blacklist" // stored by an older version, file still empty
	require.NoError(t, s.box.Save(st))
	st.TestDomain = "example.com"
	require.NoError(t, s.svc.SaveSettings(st), "only switching to blacklist checks the list")
}
