package lists_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/rules/formats"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/stretchr/testify/require"
)

func TestToListSet_PerLine(t *testing.T) {
	s, err := lists.ToListSet(lists.List{ID: "p", Action: "perLine", TrustedForSNI: true}, lists.Result{Format: formats.Ghostline})
	require.NoError(t, err)
	require.True(t, s.TrustedForSNI)
	_, err = lists.ToListSet(lists.List{ID: "d", Action: "perLine"}, lists.Result{Format: formats.Domains})
	require.Error(t, err)
}
