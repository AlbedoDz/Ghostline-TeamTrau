package goodbyedpi_test

import (
	"testing"

	goodbyedpi "github.com/hashcott/ghostline/assets/goodbyedpi"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

// This test binary embeds GoodbyeDPI and WinDivert; Smart App Control may
// refuse to run it. CI runs it on GitHub's Windows runners.
func TestEmbeddedFilesMatchPinnedHashes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, dpi.Extract(goodbyedpi.FS, dir))
	require.NoError(t, dpi.Verify(dir))
}
