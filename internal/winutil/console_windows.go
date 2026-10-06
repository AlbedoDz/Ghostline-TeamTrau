package winutil

import (
	"os"

	"golang.org/x/sys/windows"
)

var procAttachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

// attachParentProcess is ATTACH_PARENT_PROCESS ((DWORD)-1).
const attachParentProcess = ^uint32(0)

// AttachParentConsole points stdout and stderr at the console of the
// process that started Ghostline (cmd, PowerShell), so a command-line mode
// of this GUI-subsystem exe can print. Streams that already work (pipes,
// redirects) are kept. It reports whether a console was attached.
func AttachParentConsole() bool {
	if r, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); r == 0 {
		return false
	}
	con, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	if !usable(os.Stdout) {
		os.Stdout = con
	}
	if !usable(os.Stderr) {
		os.Stderr = con
	}
	return true
}

// usable reports whether f is an open handle (a GUI exe starts without
// standard handles unless they were redirected).
func usable(f *os.File) bool {
	if f == nil {
		return false
	}
	_, err := f.Stat()
	return err == nil
}
