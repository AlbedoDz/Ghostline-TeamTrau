package main

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
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
	roots := certstore.NewWindows(certstore.LocalMachine)
	d := watchdog.Deps{
		States:  store.NewStateStore(paths.State, lock),
		DNS:     sysdns.NewManager(sysdns.NewWindowsAPI(), time.Sleep),
		StopDPI: stopDPI(paths),
		Alive:   winutil.ProcessAlive,
		Log:     logger,

		RestoreSysProxy: sysproxy.Manager{API: sysproxy.NewWindowsAPI()}.RestoreIfOurs,
		DeleteRule:      winutil.DeleteNamedRule,
		RemoveCert:      roots.Remove,
		SweepSession:    sweepSession(roots),
	}
	switch mode.Kind {
	case cli.KindWatchdog:
		err = watchdog.RunWatchdog(mode.ParentPID, mode.ParentStart, winutil.WaitForExit, d)
	case cli.KindRemoveCerts:
		// The uninstaller: undo whatever a run left, then remove every
		// Ghostline root and the LAN CA files.
		err = errors.Join(watchdog.RunRestore(d), watchdog.RemoveAllCerts(roots, paths.LANCACert, paths.LANCAKey))
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
	case cli.KindRemoveCerts:
		return "remove-certs"
	case cli.KindAutostart:
		return "autostart"
	}
	return "ui"
}

// sweepSession removes Fake SNI roots not in keep.
func sweepSession(s certstore.Store) func(keep []string) error {
	return func(keep []string) error {
		_, err := certstore.Sweep(s, certs.SessionPrefix, keep)
		return err
	}
}
