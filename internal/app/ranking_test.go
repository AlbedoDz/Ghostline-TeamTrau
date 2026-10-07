package app

import (
	"context"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

// rankPicker picks the servers in ids and reports whether its ranking is old.
type rankPicker struct {
	*fPicker
	stale bool
	ids   []string
}

func (p *rankPicker) Stale() bool { return p.stale }

func (p *rankPicker) Pick(ctx context.Context, f func(done, total int)) ([]model.Server, error) {
	if len(p.ids) == 0 {
		return p.fPicker.Pick(ctx, f)
	}
	var out []model.Server
	for _, id := range p.ids {
		out = append(out, model.Server{ID: id, Name: id})
	}
	return out, nil
}

func TestConnect_OldRankingIsRefreshedInTheBackground(t *testing.T) {
	for _, stale := range []bool{true, false} {
		h := newHarness(t)
		h.o.d.Picker = &rankPicker{fPicker: h.pick, stale: stale}
		refreshed := make(chan struct{}, 1)
		h.o.refresh = func() { refreshed <- struct{}{} }
		require.NoError(t, h.o.Connect(context.Background()))
		if stale {
			require.Eventually(t, func() bool { return len(refreshed) == 1 }, time.Second, 5*time.Millisecond)
		} else {
			require.Never(t, func() bool { return len(refreshed) == 1 }, 50*time.Millisecond, 5*time.Millisecond)
		}
		require.NoError(t, h.o.Disconnect(context.Background()))
	}
}

func TestApplyBest_SwapsOnlyWhenTheBestChanged(t *testing.T) {
	h := newHarness(t)
	rp := &rankPicker{fPicker: h.pick}
	h.o.d.Picker = rp
	require.NoError(t, h.o.Connect(context.Background()))

	rp.ids = []string{"cf"} // same as now
	h.o.ApplyBest(context.Background())
	require.Zero(t, h.eng.swaps)

	rp.ids = []string{"q9", "cf"}
	h.o.ApplyBest(context.Background())
	require.Equal(t, 1, h.eng.swaps)
	require.Equal(t, []string{"q9", "cf"}, h.o.Snapshot().Servers)
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)

	require.NoError(t, h.o.Disconnect(context.Background()))
	rp.ids = []string{"x"}
	h.o.ApplyBest(context.Background()) // disconnected: nothing to swap
	require.Equal(t, 1, h.eng.swaps)
}

// The UI waits for Probed before deciding to auto-tune: it is set after
// DPI bypass started, even when nothing is blocked.
func TestPostConnectProbe_MarksProbed(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.Eventually(t, func() bool { return h.o.Snapshot().Probed }, time.Second, 5*time.Millisecond)
	require.Empty(t, h.o.Snapshot().BlockedSites)
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.Disconnect(context.Background()))
}
