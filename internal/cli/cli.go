// Package cli parses the run mode from the command line.
package cli

import (
	"fmt"
	"strconv"
	"time"
)

// Kind is the process run mode.
type Kind int

const (
	KindUI Kind = iota
	KindAutostart
	KindWatchdog
	KindRestore
	// KindRemoveCerts is --remove-certs (the uninstaller): restore, then
	// remove every Ghostline root certificate.
	KindRemoveCerts
	// KindExport is --export <file>: write a settings backup and exit.
	KindExport
)

// Mode is the parsed command line.
type Mode struct {
	Kind        Kind
	ParentPID   uint32
	ParentStart time.Time
	ExportPath  string
}

// Parse reads the run mode from args (without the program name).
func Parse(args []string) (Mode, error) {
	m := Mode{Kind: KindUI}
	var hasPID, hasStart bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--autostart":
			m.Kind = KindAutostart
		case "--restore":
			m.Kind = KindRestore
		case "--remove-certs":
			m.Kind = KindRemoveCerts
		case "--watchdog":
			m.Kind = KindWatchdog
		case "--export":
			if i+1 >= len(args) {
				return Mode{}, fmt.Errorf("cli: --export needs a file")
			}
			m.Kind, m.ExportPath = KindExport, args[i+1]
			i++
		case "--parent", "--parent-start":
			if i+1 >= len(args) {
				return Mode{}, fmt.Errorf("cli: %s needs a value", args[i])
			}
			v, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil {
				return Mode{}, fmt.Errorf("cli: bad %s: %w", args[i], err)
			}
			if args[i] == "--parent" {
				m.ParentPID, hasPID = uint32(v), true
			} else {
				m.ParentStart, hasStart = time.Unix(0, v), true
			}
			i++
		default:
			return Mode{}, fmt.Errorf("cli: unknown argument %q", args[i])
		}
	}
	if m.Kind == KindWatchdog && (!hasPID || !hasStart) {
		return Mode{}, fmt.Errorf("cli: --watchdog requires --parent and --parent-start")
	}
	return m, nil
}
