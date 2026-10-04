package app

import (
	"context"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

func TestConnect_NoPinnedServersError(t *testing.T) {
	h := newHarness(t)
	h.pick.err = &NoPinnedError{Checked: 2}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodeNoPinnedServers, e.Code)
	require.Equal(t, 2, e.Params["checked"])
}

func TestUnpinAll_TurnsPinnedOnlyOff(t *testing.T) {
	sh := newSvc(t)
	require.NoError(t, sh.svc.SetPinnedMany([]string{"a"}, true))
	st := sh.svc.GetSettings()
	st.PinnedOnly = true
	require.NoError(t, sh.svc.SaveSettings(st))
	require.True(t, sh.svc.GetSettings().PinnedOnly)
	require.NoError(t, sh.svc.SetPinnedMany([]string{"a"}, false))
	require.False(t, sh.svc.GetSettings().PinnedOnly)
	// Saving pinned-only with nothing pinned is normalised to off.
	st = sh.svc.GetSettings()
	st.PinnedOnly = true
	require.NoError(t, sh.svc.SaveSettings(st))
	require.False(t, sh.svc.GetSettings().PinnedOnly)
}

func TestServerNames_DisambiguatesDuplicates(t *testing.T) {
	got := serverNames([]model.Server{
		{Name: "dnscry.pt-hanoi"},
		{Name: "cloudflare", IPs: []string{"104.16.133.229"}},
		{Name: "cloudflare", Address: "https://104.16.249.249/dns-query"},
		{Name: "cloudflare", Address: "sdns://AgcAAAAAAAAA"},
	})
	require.Equal(t, []string{"dnscry.pt-hanoi", "cloudflare · 104.16.133.229", "cloudflare · 104.16.249.249", "cloudflare · #4"}, got)
}

func TestCheckServer_ReturnsUpdatedRow(t *testing.T) {
	sh := newSvc(t)
	sh.svc.x.CheckServer = func(_ context.Context, id string) error {
		require.Equal(t, "cf", id)
		return nil
	}
	row, err := sh.svc.CheckServer("cf")
	require.NoError(t, err)
	require.Equal(t, "cf", row.Server.ID)
	require.NotNil(t, row.Result)
}

// The UI's settings copy does not track pins (they change through
// SetPinned), so a save from the UI must not wipe them.
func TestSaveSettings_KeepsStoredPins(t *testing.T) {
	sh := newSvc(t)
	ui := sh.svc.GetSettings() // taken before pinning: Pinned is empty here
	require.NoError(t, sh.svc.SetPinned("cf", true))
	ui.PinnedOnly = true
	require.NoError(t, sh.svc.SaveSettings(ui))
	got := sh.svc.GetSettings()
	require.Equal(t, []string{"cf"}, got.Pinned)
	require.True(t, got.PinnedOnly)
}

func TestUseOnlyServer(t *testing.T) {
	sh := newSvc(t)
	require.NoError(t, sh.svc.SetPinnedMany([]string{"a", "b"}, true))
	require.NoError(t, sh.svc.UseOnlyServer("cf"))
	got := sh.svc.GetSettings()
	require.Equal(t, []string{"cf"}, got.Pinned)
	require.True(t, got.PinnedOnly)
	require.Error(t, sh.svc.UseOnlyServer(""))
}
