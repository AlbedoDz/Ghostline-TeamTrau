package formats_test

import (
	"os"
	"testing"

	"github.com/hashcott/ghostline/internal/rules/formats"
	"github.com/stretchr/testify/require"
)

func TestDetect_Ghostline(t *testing.T) {
	data, err := os.ReadFile("testdata/ghostline.txt")
	require.NoError(t, err)
	for _, name := range []string{"x.txt", "list.yaml", "x.json", ""} {
		f, err := formats.Detect(name, data)
		require.NoError(t, err, name)
		require.Equal(t, formats.Ghostline, f, name)
	}
}

func TestParse_Ghostline(t *testing.T) {
	data, err := os.ReadFile("testdata/ghostline.txt")
	require.NoError(t, err)
	r, err := formats.Parse(formats.Ghostline, data)
	require.NoError(t, err)
	require.Len(t, r.Entries, 2)
	require.Equal(t, 1, r.Skipped)
	require.Equal(t, 2, r.Counts["domain"])
	e := r.Entries[0]
	require.Equal(t, "youtube.com", e.Pattern.Value)
	require.Equal(t, 3, e.Line)
	require.NotNil(t, e.Action)
	require.Equal(t, "www.google.com", e.Action.SNI)
	require.Equal(t, "www.google.com", e.Action.Connect)
	require.True(t, r.Entries[1].Action.Block)
}
