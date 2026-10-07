package main

import (
	goodbyedpi "github.com/hashcott/ghostline/assets/goodbyedpi"
	zapret2 "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/shell"
	"github.com/hashcott/ghostline/internal/store"
)

func newDPIManager(paths store.Paths) *dpi.Manager { // headless only
	return shell.NewDPIManager(paths, goodbyedpi.FS, zapret2.FS, nil)
}

// stopDPI removes the WinDivert services left behind. An engine process
// owned by a dead Ghostline is already gone: it lived in a kill-on-close job.
func stopDPI(paths store.Paths) func() error {
	return newDPIManager(paths).Stop
}
