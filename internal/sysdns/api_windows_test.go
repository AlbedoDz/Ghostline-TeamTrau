package sysdns

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestSettingsStructSize(t *testing.T) {
	require.Equal(t, uintptr(64), unsafe.Sizeof(dnsInterfaceSettings{}))
}

func TestWindowsAPI_AdaptersHaveGUIDs(t *testing.T) {
	ads, err := NewWindowsAPI().Adapters()
	require.NoError(t, err)
	require.NotEmpty(t, ads)
	for _, a := range ads {
		_, err := windows.GUIDFromString(a.GUID)
		require.NoError(t, err, a.GUID)
		require.NotZero(t, a.IfIndex)
	}
}

func TestWindowsAPI_GetDNSReadOnly(t *testing.T) {
	api := NewWindowsAPI()
	ads, err := api.Adapters()
	require.NoError(t, err)
	for _, a := range ads {
		_, err := api.GetDNS(a.GUID, false)
		require.NoError(t, err, a.Alias)
	}
}

func TestSplitNameServers(t *testing.T) {
	require.Equal(t, []string{"1.1.1.1", "8.8.8.8"}, splitNameServers("1.1.1.1,8.8.8.8"))
	require.Equal(t, []string{"1.1.1.1", "8.8.8.8"}, splitNameServers(" 1.1.1.1 8.8.8.8 "))
	require.Empty(t, splitNameServers(""))
}

func TestNetshArgs(t *testing.T) {
	require.Equal(t, []string{"interface", "ipv4", "set", "dnsservers", "name=12", "source=static", "address=127.0.0.1", "register=primary", "validate=no"},
		netshArgs(12, false, []string{"127.0.0.1"}))
	require.Equal(t, []string{"interface", "ipv6", "set", "dnsservers", "name=12", "source=dhcp"}, netshArgs(12, true, nil))
}

func TestWatch_StartsAndStops(t *testing.T) {
	stop, err := Watch(func() {})
	require.NoError(t, err)
	stop()
}
