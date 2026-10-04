package main

import (
	"time"

	goodbyedpi "github.com/hashcott/ghostline/assets/goodbyedpi"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/store"
)

func newDPIManager(paths store.Paths) *dpi.Manager { // headless only
	return dpi.NewManager(paths.BinDir+`\goodbyedpi`, goodbyedpi.FS, dpi.NewWindowsRunner(), dpi.NewWindowsServices(), time.Sleep)
}

// stopDPI removes the WinDivert service left behind. A GoodbyeDPI process
// owned by a dead Ghostline is already gone: it lived in a kill-on-close job.
func stopDPI(paths store.Paths) func() error {
	return newDPIManager(paths).Stop
}
