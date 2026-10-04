package shell

import (
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/stretchr/testify/require"
)

func TestTrayText_FollowsLanguage(t *testing.T) { // review I10
	vi, en := trayText("vi"), trayText("en")
	require.Equal(t, "Thoát", vi.quit)
	require.Equal(t, "Quit", en.quit)
	require.Equal(t, "Connect", en.connect)
	require.Equal(t, "Disconnect", en.disconnect)
	require.Equal(t, "Protected", en.status[app.StatusProtected])
	require.Equal(t, "Đã bảo vệ", vi.status[app.StatusProtected])
	require.Equal(t, en, trayText("fr"), "unknown languages fall back to English")
	for _, s := range []app.Status{app.StatusDisconnected, app.StatusConnecting, app.StatusProtected, app.StatusDegraded, app.StatusDisconnecting, app.StatusError} {
		require.NotEmpty(t, vi.status[s])
		require.NotEmpty(t, en.status[s])
	}
}

func TestTrayText_UpdateLabel(t *testing.T) {
	require.Equal(t, "Có bản mới v0.1.1 ↗", trayText("vi").updateLabel("v0.1.1"))
	require.Equal(t, "New version v0.1.1 ↗", trayText("en").updateLabel("v0.1.1"))
}
