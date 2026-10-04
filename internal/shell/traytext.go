package shell

import (
	"fmt"

	"github.com/hashcott/ghostline/internal/app"
)

// trayStrings is the tray menu copy for one language (the tray lives in Go,
// outside the React i18n).
type trayStrings struct {
	connect, disconnect, dpi, open, quit, update string // update: "%s" is the tag
	proxyOn, proxyOff                            string
	status                                       map[app.Status]string
}

var trayLangs = map[string]trayStrings{
	"vi": {
		connect: "Kết nối", disconnect: "Ngắt kết nối", dpi: "Vượt DPI", open: "Mở Ghostline", quit: "Thoát",
		update: "Có bản mới %s ↗", proxyOn: "Proxy: bật", proxyOff: "Proxy: tắt",
		status: map[app.Status]string{
			app.StatusDisconnected: "Chưa bảo vệ", app.StatusConnecting: "Đang kết nối", app.StatusProtected: "Đã bảo vệ",
			app.StatusDegraded: "Suy giảm", app.StatusDisconnecting: "Đang ngắt", app.StatusError: "Lỗi",
		},
	},
	"en": {
		connect: "Connect", disconnect: "Disconnect", dpi: "DPI bypass", open: "Open Ghostline", quit: "Quit",
		update: "New version %s ↗", proxyOn: "Proxy: on", proxyOff: "Proxy: off",
		status: map[app.Status]string{
			app.StatusDisconnected: "Unprotected", app.StatusConnecting: "Connecting", app.StatusProtected: "Protected",
			app.StatusDegraded: "Degraded", app.StatusDisconnecting: "Disconnecting", app.StatusError: "Error",
		},
	},
}

func trayText(lang string) trayStrings {
	if t, ok := trayLangs[lang]; ok {
		return t
	}
	return trayLangs["en"]
}

func (t trayStrings) updateLabel(tag string) string { return fmt.Sprintf(t.update, tag) }

func (t trayStrings) proxyLabel(on bool) string {
	if on {
		return t.proxyOn
	}
	return t.proxyOff
}
