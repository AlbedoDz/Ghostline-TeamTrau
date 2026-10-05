package cli_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestParse_Export(t *testing.T) {
	m, err := cli.Parse([]string{"--export", `C:\tmp\out.json`})
	require.NoError(t, err)
	require.Equal(t, cli.KindExport, m.Kind)
	require.Equal(t, `C:\tmp\out.json`, m.ExportPath)
	_, err = cli.Parse([]string{"--export"})
	require.Error(t, err)
}
