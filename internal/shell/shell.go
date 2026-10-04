// Package shell wires Ghostline together and owns the Wails window and tray.
package shell

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/upstreams"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/hashcott/ghostline/internal/winutil"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Options carries what main provides.
type Options struct {
	Mode       cli.Mode
	Assets     fs.FS
	DPIAssets  fs.FS
	Executable string
}

// Run starts the UI process.
func Run(o Options) error {
	if !webView2Installed() {
		messageBox(brand.AppName, "Ghostline cần Microsoft Edge WebView2 Runtime.\nGhostline needs the Microsoft Edge WebView2 Runtime.\n\nhttps://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return errors.New("webview2 missing")
	}
	paths := store.ResolvePaths(o.Executable, os.Getenv("APPDATA"))
	if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
		fatalBox(err)
		return err
	}
	logw, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3)
	if err != nil {
		fatalBox(err)
		return err
	}
	defer logw.Close()
	log := slog.New(slog.NewTextHandler(logw, nil))
	slog.SetDefault(log)
	log.Info("start", "version", brand.Version, "portable", paths.Portable, "mode", o.Mode.Kind)

	initial, settingsReset, err := store.LoadSettings(paths.Settings)
	if err != nil {
		log.Warn("settings", "err", err)
	}
	box := app.NewSettingsBox(paths.Settings, initial)

	lock, err := winutil.NewNamedMutex(brand.StateMutex)
	if err != nil {
		fatalBox(err)
		return err
	}
	states := store.NewStateStore(paths.State, lock)
	dnsMgr := sysdns.NewManager(sysdns.NewWindowsAPI(), time.Sleep)
	dpiMgr := dpi.NewManager(filepath.Join(paths.BinDir, "goodbyedpi"), o.DPIAssets, dpi.NewWindowsRunner(), dpi.NewWindowsServices(), time.Sleep)
	recoverDeps := watchdog.Deps{States: states, DNS: dnsMgr, StopDPI: dpiMgr.Stop, Alive: winutil.ProcessAlive, Log: log}

	// Safety layer 3: restore whatever a dead previous run left behind.
	startOut, startErr := watchdog.RestoreIfOrphaned(recoverDeps)
	if startErr != nil {
		log.Error("startup restore", "err", startErr)
	} else if startOut != watchdog.NothingToDo {
		log.Info("startup restore", "outcome", startOut)
	}

	cat := newCatalog(paths)
	cache, _ := scanner.LoadCache(paths.ScanCache)
	factoryFor := func() (*upstreams.Factory, error) {
		s := box.Get()
		opts := upstreams.Options{Bootstrap: s.Bootstrap, Timeout: 3 * time.Second}
		if s.FragmentDNS.Enabled {
			opts.Fragment = &upstreams.FragmentOptions{Chunks: s.FragmentDNS.Chunks, Delay: time.Duration(s.FragmentDNS.DelayMs) * time.Millisecond}
		}
		return upstreams.NewFactory(opts)
	}
	build := builderFunc(func(sv model.Server) (upstreamT, error) {
		f, err := factoryFor()
		if err != nil {
			return nil, err
		}
		return f.Build(sv)
	})
	picker := &app.ScanPicker{
		Catalog: cat.get,
		Checker: scanner.DNSChecker{Build: build.Build, TestDomain: box.Get().TestDomain, Timeout: 3 * time.Second},
		Cache:   cache,
		SaveCache: func(c *scanner.Cache) error {
			return scanner.SaveCache(paths.ScanCache, c)
		},
		NetKey:   networkKey,
		Settings: box.Get,
		Now:      time.Now,
		Rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	var wapp *application.App
	em := &emitter{}
	bus := app.NewBus(em)
	eng := engine.New(bus.Query)
	orch := app.New(app.Deps{
		Engine: eng, DNS: dnsMgr, DPI: dpiMgr, Safety: safety{exe: o.Executable}, System: system{},
		Picker: picker, Scans: picker, Builder: build, Resolver: net.DefaultResolver,
		Prober: probe.Prober{
			Resolve: func(ctx context.Context, host string) ([]netipAddr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
			},
			Dial: (&net.Dialer{}).DialContext, Timeout: 5 * time.Second,
		},
		Recover:      func() (watchdog.Outcome, error) { return watchdog.RestoreIfOrphaned(recoverDeps) },
		Sink:         bus,
		States:       states,
		Settings:     box.Get,
		SaveSettings: box.Save,
		Now:          time.Now,
		Sleep:        time.Sleep,
		Ticker: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		},
		BlacklistPath: paths.DPIBlacklist,
	})
	if settingsReset {
		orch.AddWarning(app.AppError{Code: app.CodeSettingsReset})
	}
	for _, w := range app.StartupWarnings(startOut, startErr) {
		orch.AddWarning(w)
	}

	ui := &ui{orch: orch, box: box, log: log}
	update := &updateState{}
	svc := app.NewService(orch, app.ServiceDeps{
		Bus: bus, Paths: paths, Settings: box, Catalog: cat.get,
		LoadCustom: cat.loadCustom, SaveCustom: cat.saveCustom,
		ListAdapters: func() ([]sysdns.Adapter, error) { return sysdns.NewWindowsAPI().Adapters() },
		StopService:  func(name string) error { return winutil.StopService(name, 10*time.Second) },
		SetMode:      ui.setMode,
		RestoreNow:   func() error { return restoreNow(states, dnsMgr) },
		Info: func() app.AppInfo {
			tag, url := update.get()
			return app.AppInfo{Version: brand.Version, Portable: paths.Portable, UpdateTag: tag, UpdateURL: url}
		},
		OnSettingsChanged: func(old, n store.Settings) {
			if old.StartWithWindows != n.StartWithWindows {
				var err error
				if n.StartWithWindows {
					err = startup.Create(startup.AutostartTask(o.Executable))
				} else {
					err = startup.Delete(brand.TaskAutostart)
				}
				if err != nil {
					log.Error("autostart task", "err", err)
				}
			}
			if old.Language != n.Language {
				ui.onLanguage()
			}
			if !slices.Equal(old.Bootstrap, n.Bootstrap) || old.TestDomain != n.TestDomain {
				picker.Checker = scanner.DNSChecker{Build: build.Build, TestDomain: n.TestDomain, Timeout: 3 * time.Second}
			}
		},
	})

	wapp = application.New(application.Options{
		Name:        brand.AppName,
		Description: "Secure DNS client",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(o.Assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               brand.SingleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { ui.show() },
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WndProcInterceptor: func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
				switch classify(msg, wParam) {
				case wmEndSession:
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = orch.Disconnect(ctx)
					cancel()
					if msg == wmQueryEndSession {
						return 1, true
					}
				case wmResume:
					go orch.OnResume(context.Background())
				}
				return 0, false
			},
		},
		OnShutdown: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = orch.Disconnect(ctx)
		},
	})
	em.app = wapp
	ui.app = wapp
	ui.createWindow(o.Mode.Kind == cli.KindAutostart)
	ui.createTray()
	em.onState = ui.onState

	stopWatch, err := sysdns.Watch(func() { orch.OnNetworkChange(context.Background()) })
	if err != nil {
		log.Warn("network watch", "err", err)
	} else {
		defer stopWatch()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	statTick := time.NewTicker(time.Second)
	defer statTick.Stop()
	go app.RunStats(svc, ctx, statTick.C)
	go runUpdates(ctx, paths, box, cat, update, bus, log, ui.onUpdate)

	if o.Mode.Kind == cli.KindAutostart && box.Get().AutoConnect {
		go func() { _ = orch.Connect(context.Background()) }()
	}
	if err := wapp.Run(); err != nil {
		fatalBox(err)
		return fmt.Errorf("shell: %w", err)
	}
	return nil
}

// restoreNow puts DNS back from state.json, or resets loopback adapters to
// DHCP when there is no usable snapshot.
func restoreNow(states *store.StateStore, mgr *sysdns.Manager) error {
	st, err := states.Load()
	if err == nil && len(st.Snapshot) > 0 {
		if errs := mgr.Restore(st.Snapshot); len(errs) > 0 {
			return errs[0]
		}
		return states.Reset()
	}
	ads, err := mgr.LoopbackAdapters()
	if err != nil {
		return err
	}
	var snaps []model.AdapterSnapshot
	for _, a := range ads {
		snaps = append(snaps, model.AdapterSnapshot{GUID: a.GUID, IfIndex: a.IfIndex, Alias: a.Alias,
			IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	if errs := mgr.Restore(snaps); len(errs) > 0 {
		return errs[0]
	}
	return states.Reset()
}
