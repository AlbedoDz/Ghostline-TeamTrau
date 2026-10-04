package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/hashcott/ghostline/internal/winutil"
)

// runHeadless handles --watchdog and --restore. It never touches Wails.
func runHeadless(mode cli.Mode) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	paths := store.ResolvePaths(exe, os.Getenv("APPDATA"))
	logger := slog.Default()
	if w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3); err == nil {
		defer w.Close()
		logger = slog.New(slog.NewTextHandler(w, nil)).With("mode", modeName(mode.Kind))
	}
	lock, err := winutil.NewNamedMutex(brand.StateMutex)
	if err != nil {
		logger.Error("state mutex", "err", err)
		return 1
	}
	d := watchdog.Deps{
		States:  store.NewStateStore(paths.State, lock),
		DNS:     sysdns.NewManager(sysdns.NewWindowsAPI(), time.Sleep),
		StopDPI: stopDPI(paths),
		Alive:   winutil.ProcessAlive,
		Log:     logger,

		RestoreSysProxy: sysproxy.Manager{API: sysproxy.NewWindowsAPI()}.RestoreIfOurs,
		DeleteFirewall:  winutil.DeleteFirewallRule,
	}
	switch mode.Kind {
	case cli.KindWatchdog:
		err = watchdog.RunWatchdog(mode.ParentPID, mode.ParentStart, winutil.WaitForExit, d)
	default:
		err = watchdog.RunRestore(d)
	}
	if err != nil {
		logger.Error("recovery failed", "err", err)
		return 1
	}
	return 0
}

func modeName(k cli.Kind) string {
	switch k {
	case cli.KindWatchdog:
		return "watchdog"
	case cli.KindRestore:
		return "restore"
	case cli.KindAutostart:
		return "autostart"
	}
	return "ui"
}
