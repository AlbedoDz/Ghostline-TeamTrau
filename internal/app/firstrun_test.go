package app

import (
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMarkNetworkChecked(t *testing.T) {
	h := newTools(t)
	st := h.box.Get()
	st.Simple.Checked = false
	st.Language = "en"
	require.NoError(t, h.box.Save(st))
	require.NoError(t, h.svc.MarkNetworkChecked())
	got, _, err := store.LoadSettings(h.paths.Settings)
	require.NoError(t, err)
	require.True(t, got.Simple.Checked)
	require.Equal(t, "en", got.Language, "nothing else changes")
}
