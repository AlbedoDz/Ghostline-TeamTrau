package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	for args, kind := range map[string]cli.Kind{"": cli.KindUI, "--autostart": cli.KindAutostart, "--restore": cli.KindRestore} {
		m, err := cli.Parse(strings.Fields(args))
		require.NoError(t, err)
		require.Equal(t, kind, m.Kind)
	}
	m, err := cli.Parse([]string{"--watchdog", "--parent", "4242", "--parent-start", "1759561331512000000"})
	require.NoError(t, err)
	require.Equal(t, cli.KindWatchdog, m.Kind)
	require.Equal(t, uint32(4242), m.ParentPID)
	require.True(t, m.ParentStart.Equal(time.Unix(0, 1759561331512000000)))
}

func TestParse_WatchdogWithoutParentFails(t *testing.T) {
	_, err := cli.Parse([]string{"--watchdog"})
	require.Error(t, err)
}

func TestParse_UnknownFlagFails(t *testing.T) {
	_, err := cli.Parse([]string{"--bogus"})
	require.Error(t, err)
}

func TestParse_RemoveCerts(t *testing.T) {
	m, err := cli.Parse([]string{"--remove-certs"})
	require.NoError(t, err)
	require.Equal(t, cli.KindRemoveCerts, m.Kind)
}
