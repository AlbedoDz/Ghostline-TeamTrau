package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFindRunningGames(t *testing.T) {
	// Should execute without panicking and return a slice (even if empty)
	games, err := FindRunningGames([]string{"non_existent_game_xyz.exe"})
	require.NoError(t, err)
	require.Empty(t, games)
}

func TestWatcher_StartStop(t *testing.T) {
	w := NewWatcher([]string{"cs2.exe"}, 100*time.Millisecond, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)
	require.NotNil(t, w.RunningGames())
	w.Stop()
}
