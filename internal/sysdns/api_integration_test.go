//go:build windows && integration

// Run from an elevated terminal: go test -tags integration ./internal/sysdns/...
package sysdns

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegration_ApplyAndRestoreRealAdapter(t *testing.T) {
	m := NewManager(NewWindowsAPI(), time.Sleep)
	ads, err := m.Select("auto", nil)
	require.NoError(t, err)
	require.NotEmpty(t, ads, "needs at least one connected Ethernet/Wi-Fi adapter")
	snaps, err := m.Snapshot(ads[:1])
	require.NoError(t, err)
	t.Cleanup(func() { require.Empty(t, m.Restore(snaps)) })

	require.NoError(t, m.ApplyLoopback(snaps, true))
	got, err := NewWindowsAPI().GetDNS(snaps[0].GUID, false)
	require.NoError(t, err)
	require.Equal(t, []string{"127.0.0.1"}, got)

	require.Empty(t, m.Restore(snaps))
	got, err = NewWindowsAPI().GetDNS(snaps[0].GUID, false)
	require.NoError(t, err)
	require.Equal(t, snaps[0].IPv4.Servers, nilIfEmpty(got))
}

func TestIntegration_SetEmptyRevertsToDHCP(t *testing.T) {
	api := NewWindowsAPI()
	m := NewManager(api, time.Sleep)
	ads, err := m.Select("auto", nil)
	require.NoError(t, err)
	require.NotEmpty(t, ads)
	snaps, err := m.Snapshot(ads[:1])
	require.NoError(t, err)
	t.Cleanup(func() { m.Restore(snaps) })

	require.NoError(t, api.SetDNS(ads[0].GUID, false, []string{"9.9.9.9"}))
	require.NoError(t, api.SetDNS(ads[0].GUID, false, nil))
	got, err := api.GetDNS(ads[0].GUID, false)
	require.NoError(t, err)
	require.Empty(t, got, "empty NameServer must mean DHCP")
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
