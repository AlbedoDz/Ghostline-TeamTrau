package app

import (
	"context"
	"slices"
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func zapretHarness(t *testing.T) *harness {
	h := newHarness(t)
	h.setSettings(func(s *store.Settings) { s.DPI.Engine = store.EngineZapret2; s.DPI.Preset = "medium" })
	require.NoError(t, h.o.Connect(context.Background()))
	return h
}

func logged(h *harness, code string) bool {
	for _, e := range h.sink.events() {
		if e.Code == code {
			return true
		}
	}
	return false
}

func TestStartDPI_Zapret2(t *testing.T) {
	h := zapretHarness(t)
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	st := h.dpi.lastStart()
	require.Equal(t, "zapret2", st.engine)
	require.Equal(t, "z-split", st.plan.Strategy)
	sn := h.o.Snapshot().DPI
	require.Equal(t, DPIStatus{Enabled: true, Running: true, Engine: "zapret2", Preset: "z-split"}, sn)
}

func TestStartDPI_Zapret2BlockedFallsBack(t *testing.T) {
	for _, cause := range []error{dpi.ErrBlockedByAV, dpi.ErrHashMismatch} {
		h := zapretHarness(t)
		h.dpi.failOn = map[string]error{"zapret2": cause}
		require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
		require.Len(t, h.dpi.starts, 2)
		st := h.dpi.lastStart()
		require.Equal(t, "goodbyedpi", st.engine)
		require.Equal(t, "medium", st.plan.Strategy)
		sn := h.o.Snapshot()
		require.Contains(t, sn.Reasons, ReasonDPIFallback)
		require.Equal(t, StatusDegraded, sn.Status)
		require.True(t, sn.DPI.Fallback)
		require.Equal(t, "goodbyedpi", sn.DPI.Engine)
		require.Equal(t, "zapret2", h.getSettings().DPI.Engine, "settings unchanged")
		require.True(t, logged(h, CodeDPIFallback))
	}
}

func TestStartDPI_StartFailedNoFallback(t *testing.T) {
	h := zapretHarness(t)
	h.dpi.failOn = map[string]error{"zapret2": dpi.ErrStartFailed}
	requireCode(t, h.o.SetDPIEnabled(context.Background(), true), CodeDPIStartFailed)
	require.Len(t, h.dpi.starts, 1)
}

func TestStartDPI_FallbackAlsoFails(t *testing.T) {
	h := zapretHarness(t)
	h.dpi.failOn = map[string]error{"zapret2": dpi.ErrBlockedByAV, "goodbyedpi": dpi.ErrStartFailed}
	err := h.o.SetDPIEnabled(context.Background(), true)
	requireCode(t, err, CodeDPIBlockedByAV)
	require.Equal(t, "zapret2", err.(*AppError).Params["engine"])
	require.NotContains(t, h.o.Snapshot().Reasons, ReasonDPIFallback)
}

func TestStartDPI_UnknownStrategyFallsBackToFirst(t *testing.T) { // Review Focus #4
	h := zapretHarness(t)
	h.setSettings(func(s *store.Settings) { s.DPI.Zapret2.Strategy = "gone" })
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.Equal(t, "z-split", h.dpi.lastStart().plan.Strategy)
}

func TestStartDPI_Zapret2AutoHostlistOnlyWhenEnabled(t *testing.T) {
	h := zapretHarness(t)
	h.o.d.AutoHostlistPath = `C:\data\dpi-autohostlist.txt`
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.Equal(t, "", h.dpi.lastStart().plan.AutoHostlist)
	h.setSettings(func(s *store.Settings) { s.DPI.Zapret2.AutoHostlist = true })
	require.NoError(t, h.o.RestartDPI(context.Background()))
	require.Equal(t, `C:\data\dpi-autohostlist.txt`, h.dpi.lastStart().plan.AutoHostlist)
}

func TestRestartDPI_RetriesConfiguredEngine(t *testing.T) {
	h := zapretHarness(t)
	h.dpi.failOn = map[string]error{"zapret2": dpi.ErrBlockedByAV}
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.Contains(t, h.o.Snapshot().Reasons, ReasonDPIFallback)
	h.dpi.failOn = nil
	require.NoError(t, h.o.RestartDPI(context.Background()))
	require.Equal(t, "zapret2", h.dpi.lastStart().engine)
	sn := h.o.Snapshot()
	require.NotContains(t, sn.Reasons, ReasonDPIFallback)
	require.False(t, sn.DPI.Fallback)
	require.Equal(t, StatusProtected, sn.Status)
}

func TestSetDPIEnabled_OffClearsFallback(t *testing.T) {
	h := zapretHarness(t)
	h.dpi.failOn = map[string]error{"zapret2": dpi.ErrBlockedByAV}
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), false))
	require.NotContains(t, h.o.Snapshot().Reasons, ReasonDPIFallback)
}

func TestAutotune_SwitchesEngineWhenZapret2Blocked(t *testing.T) {
	h := zapretHarness(t)
	h.dpi.failOn = map[string]error{"zapret2": dpi.ErrBlockedByAV}
	h.prober.stage = func(string, int) probe.Stage {
		if h.dpi.Engine() == "goodbyedpi" {
			return probe.StageOK
		}
		return probe.StageTLS
	}
	var seen []string
	require.NoError(t, h.o.Autotune(context.Background(), func(engine, p string, i, n int) { seen = append(seen, engine+":"+p) }))
	require.Equal(t, []string{"zapret2:z-split", "goodbyedpi:light"}, seen)
	s := h.getSettings()
	require.Equal(t, "goodbyedpi", s.DPI.Engine)
	require.Equal(t, "light", s.DPI.Preset)
	require.True(t, s.DPI.Enabled)
	require.True(t, logged(h, CodeAutotuneEngineSwitched))
}

func TestAutotune_Zapret2SavesStrategy(t *testing.T) {
	h := zapretHarness(t)
	h.prober.stage = func(string, int) probe.Stage {
		if h.dpi.Running() && h.dpi.lastStart().plan.Strategy == "z-disorder" {
			return probe.StageOK
		}
		return probe.StageTLS
	}
	var seen []string
	require.NoError(t, h.o.Autotune(context.Background(), func(engine, p string, i, n int) {
		seen = append(seen, p)
		require.Equal(t, 4, n)
	}))
	require.Equal(t, []string{"z-split", "z-disorder"}, seen)
	s := h.getSettings()
	require.Equal(t, "zapret2", s.DPI.Engine)
	require.Equal(t, "z-disorder", s.DPI.Zapret2.Strategy)
	require.Equal(t, "medium", s.DPI.Preset, "GoodbyeDPI preset untouched")
	require.Equal(t, "z-disorder", h.o.Snapshot().DPI.Preset)
}

func TestSaveSettings_EngineChangeRestarts(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.o.Connect(context.Background()))
	require.NoError(t, s.o.SetDPIEnabled(context.Background(), true))
	require.Equal(t, "goodbyedpi", s.dpi.lastStart().engine)
	st := s.box.Get()
	st.DPI.Engine = store.EngineZapret2
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, "zapret2", s.dpi.lastStart().engine)
	st.DPI.Zapret2.Strategy = "z-fake"
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, "z-fake", s.dpi.lastStart().plan.Strategy)
}

func TestSaveSettings_BadEngineRejected(t *testing.T) {
	s := newSvc(t)
	st := s.box.Get()
	st.DPI.Engine = "x"
	require.Error(t, s.svc.SaveSettings(st))
}

func TestSaveSettings_Zapret2CustomRejected(t *testing.T) {
	s := newSvc(t)
	st := s.box.Get()
	st.DPI.Zapret2.Strategy = "custom"
	st.DPI.Zapret2.CustomArgs = "--lua-init=@x.lua"
	err := s.svc.SaveSettings(st)
	requireCode(t, err, CodeDPICustomRejected)
	st.DPI.Zapret2.CustomArgs = "--lua-desync=multisplit:pos=2"
	require.NoError(t, s.svc.SaveSettings(st))
}

func TestSaveSettings_GoodbyeDPICustomRejected(t *testing.T) {
	s := newSvc(t)
	st := s.box.Get()
	st.DPI.Preset = "custom"
	st.DPI.CustomArgs = "--dns-addr 1.1.1.1"
	requireCode(t, s.svc.SaveSettings(st), CodeDPICustomRejected)
}

func TestSaveDPIBlacklist_Zapret2NoRestart(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	st := s.box.Get()
	st.DPI.Engine, st.DPI.Scope = store.EngineZapret2, "blacklist"
	require.NoError(t, s.box.Save(st))
	require.NoError(t, s.o.Connect(context.Background()))
	require.NoError(t, s.o.SetDPIEnabled(context.Background(), true))
	starts := countOf(s.r.list(), "dpi.start")
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\ndiscord.com\n"))
	require.Equal(t, starts, countOf(s.r.list(), "dpi.start"))
	require.Contains(t, s.r.list(), "dpi.refresh")
}

func TestService_DPIStrategies(t *testing.T) {
	s := newSvc(t)
	z, err := s.svc.DPIStrategies("zapret2")
	require.NoError(t, err)
	require.Equal(t, "z-split", z[0].ID)
	g, err := s.svc.DPIStrategies("goodbyedpi")
	require.NoError(t, err)
	require.Equal(t, "light", g[0].ID)
	_, err = s.svc.DPIStrategies("x")
	require.Error(t, err)
}

func TestService_PreviewDPIArgs(t *testing.T) {
	s := newSvc(t)
	a, err := s.svc.PreviewDPIArgs("zapret2", "z-split", "", "all", false)
	require.NoError(t, err)
	require.Contains(t, a, "--lua-desync=multisplit:pos=1,midsld")
	a, err = s.svc.PreviewDPIArgs("goodbyedpi", "light", "", "blacklist", false)
	require.NoError(t, err)
	require.Equal(t, s.paths.DPIBlacklist, a[len(a)-1])
}

func TestService_AutoHostlistRoundTrip(t *testing.T) {
	s := newSvc(t)
	s.o.d.AutoHostlistPath = s.paths.DPIAutoHostlist
	got, err := s.svc.GetDPIAutoHostlist()
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, s.svc.SaveDPIAutoHostlist([]string{"b.com", " a.com ", "", "b.com"}))
	got, err = s.svc.GetDPIAutoHostlist()
	require.NoError(t, err)
	require.Equal(t, []string{"a.com", "b.com"}, got)
	require.True(t, slices.IsSorted(got))
}
