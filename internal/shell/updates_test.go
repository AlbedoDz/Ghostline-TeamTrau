package shell

import (
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func fetchOK(tag string) (func() (updater.Release, error), *int) {
	n := 0
	return func() (updater.Release, error) {
		n++
		return updater.Release{Tag: tag, URL: "https://example/" + tag}, nil
	}, &n
}

func TestReleaseCheck_FindsNewerAndRemembersIt(t *testing.T) {
	var meta store.Meta
	fetch, _ := fetchOK("v0.1.1")
	r, ok := releaseCheck(&meta, now, "0.1.0", fetch)
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Equal(t, "v0.1.1", meta.LatestTag)
	require.Equal(t, "https://example/v0.1.1", meta.LatestURL)
	require.Equal(t, now, meta.LastUpdateCheck)
}

// Restarting within 24 h must still show the release found earlier.
func TestReleaseCheck_RemembersAcrossRestartWithoutFetching(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Hour), LatestTag: "v0.1.1", LatestURL: "https://example/v0.1.1"}
	fetch, calls := fetchOK("v9.9.9")
	r, ok := releaseCheck(&meta, now, "0.1.0", fetch)
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Zero(t, *calls, "not due yet")
}

// After updating to the remembered version there is nothing to announce.
func TestReleaseCheck_NothingWhenAlreadyCurrent(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Hour), LatestTag: "v0.1.1"}
	fetch, _ := fetchOK("v0.1.1")
	_, ok := releaseCheck(&meta, now, "0.1.1", fetch)
	require.False(t, ok)
}

// A failed check keeps what was known and retries later.
func TestReleaseCheck_FailureKeepsKnownRelease(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-48 * time.Hour), LatestTag: "v0.1.1", LatestURL: "u"}
	r, ok := releaseCheck(&meta, now, "0.1.0", func() (updater.Release, error) { return updater.Release{}, errors.New("offline") })
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Equal(t, now.Add(-48*time.Hour), meta.LastUpdateCheck, "a failure is not a completed check")
}
