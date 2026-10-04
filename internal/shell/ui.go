package shell

import (
	"context"
	"image/color"
	"log/slog"
	"net/netip"
	"sync"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/icon"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type (
	upstreamT = upstream.Upstream
	netipAddr = netip.Addr
)

// builderFunc adapts a function to app.Builder.
type builderFunc func(model.Server) (upstream.Upstream, error)

func (f builderFunc) Build(s model.Server) (upstream.Upstream, error) { return f(s) }

// emitter forwards events to Wails and lets the shell watch state changes.
type emitter struct {
	app     *application.App
	onState func(app.Snapshot)
}

func (e *emitter) Emit(name string, data any) {
	if e.app != nil {
		e.app.Event.Emit(name, data)
	}
	if name == app.EventState && e.onState != nil {
		if s, ok := data.(app.Snapshot); ok {
			e.onState(s)
		}
	}
}

const (
	simpleW, simpleH = 380, 580
	minAdvW, minAdvH = 900, 600
)

var statusColour = map[app.Status]color.RGBA{
	app.StatusDisconnected:  {0x3a, 0x4a, 0x44, 0xff},
	app.StatusConnecting:    {0x00, 0xd0, 0xff, 0xff},
	app.StatusDisconnecting: {0x00, 0xd0, 0xff, 0xff},
	app.StatusProtected:     {0x00, 0xff, 0xa3, 0xff},
	app.StatusDegraded:      {0xff, 0xb0, 0x20, 0xff},
	app.StatusError:         {0xff, 0x4d, 0x6d, 0xff},
}

var statusLabel = map[app.Status]string{
	app.StatusDisconnected: "Chưa bảo vệ", app.StatusConnecting: "Đang kết nối", app.StatusProtected: "Đã bảo vệ",
	app.StatusDegraded: "Suy giảm", app.StatusDisconnecting: "Đang ngắt", app.StatusError: "Lỗi",
}

type ui struct {
	app  *application.App
	orch *app.Orchestrator
	box  *app.SettingsBox
	log  *slog.Logger

	mu        sync.Mutex
	win       *application.WebviewWindow
	tray      *application.SystemTray
	connItem  *application.MenuItem
	dpiItem   *application.MenuItem
	lastState app.Status
}

func (u *ui) createWindow(hidden bool) {
	u.win = u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                "main",
		Title:               brand.AppName,
		Width:               simpleW,
		Height:              simpleH,
		Frameless:           true,
		DisableResize:       true,
		MaximiseButtonState: application.ButtonDisabled,
		BackgroundColour:    application.NewRGB(5, 7, 10),
		URL:                 "/",
		Hidden:              hidden,
	})
	u.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if u.box.Get().CloseToTray {
			u.win.Hide()
			e.Cancel()
			return
		}
		u.app.Quit()
	})
	if u.box.Get().Mode == "advanced" {
		u.setMode("advanced")
	}
}

func (u *ui) show() {
	if u.win != nil {
		u.win.Show()
		u.win.Focus()
	}
}

// setMode resizes the window while keeping its centre in place.
func (u *ui) setMode(mode string) {
	if u.win == nil {
		return
	}
	x, y := u.win.Position()
	w0, h0 := u.win.Size()
	cx, cy := x+w0/2, y+h0/2
	s := u.box.Get()
	w, h := simpleW, simpleH
	if mode == "advanced" {
		w, h = max(s.AdvancedWindow.Width, minAdvW), max(s.AdvancedWindow.Height, minAdvH)
		u.win.SetResizable(true)
		u.win.SetMinSize(minAdvW, minAdvH)
	} else {
		if w0 >= minAdvW { // remember the advanced size
			s.AdvancedWindow.Width, s.AdvancedWindow.Height = w0, h0
			_ = u.box.Save(s)
		}
		u.win.SetMinSize(simpleW, simpleH)
		u.win.SetResizable(false)
	}
	u.win.SetSize(w, h)
	u.win.SetPosition(cx-w/2, cy-h/2)
}

func (u *ui) createTray() {
	u.tray = u.app.SystemTray.New()
	u.tray.SetIcon(icon.Ring(statusColour[app.StatusDisconnected], 32))
	u.tray.SetTooltip(brand.AppName + " · " + statusLabel[app.StatusDisconnected])
	menu := application.NewMenu()
	u.connItem = menu.Add("Kết nối").OnClick(func(*application.Context) {
		go func() {
			if st := u.orch.Snapshot().Status; st == app.StatusProtected || st == app.StatusDegraded {
				_ = u.orch.Disconnect(context.Background())
			} else {
				_ = u.orch.Connect(context.Background())
			}
		}()
	})
	u.dpiItem = menu.AddCheckbox("Vượt DPI", u.box.Get().DPI.Enabled)
	u.dpiItem.OnClick(func(c *application.Context) {
		on := c.IsChecked()
		go func() {
			if err := u.orch.SetDPIEnabled(context.Background(), on); err != nil {
				u.dpiItem.SetChecked(!on)
			}
		}()
	})
	menu.AddSeparator()
	menu.Add("Mở Ghostline").OnClick(func(*application.Context) { u.show() })
	menu.Add("Ghostline " + brand.Version).SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Thoát").OnClick(func(*application.Context) { u.app.Quit() })
	u.tray.SetMenu(menu)
	u.tray.OnClick(u.show)
	u.tray.OnRightClick(u.tray.OpenMenu) // works around tray menu issue #6161
}

func (u *ui) onState(s app.Snapshot) {
	u.mu.Lock()
	changed := s.Status != u.lastState
	u.lastState = s.Status
	u.mu.Unlock()
	if !changed || u.tray == nil {
		return
	}
	u.tray.SetIcon(icon.Ring(statusColour[s.Status], 32))
	u.tray.SetTooltip(brand.AppName + " · " + statusLabel[s.Status])
	if s.Status == app.StatusProtected || s.Status == app.StatusDegraded {
		u.connItem.SetLabel("Ngắt kết nối")
	} else {
		u.connItem.SetLabel("Kết nối")
	}
	u.dpiItem.SetChecked(s.DPI.Enabled)
}
