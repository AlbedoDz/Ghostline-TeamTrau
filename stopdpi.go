package main

import (
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/winutil"
)

// stopDPI stops the WinDivert driver service left by GoodbyeDPI.
// Replaced by dpi.Manager.Stop once the dpi package exists (Task 14).
func stopDPI(_ store.Paths) func() error {
	return func() error {
		if err := winutil.StopService("WinDivert", 5*time.Second); err != nil {
			return err
		}
		return winutil.DeleteService("WinDivert")
	}
}
