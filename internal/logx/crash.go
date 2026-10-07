package logx

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// crashMax caps the crash file: past it, the next run starts it over.
const crashMax = 1 << 20

// CrashOutput sends the runtime's report of a fatal error or unrecovered
// panic (any goroutine, with stacks) to <dir>/<base>.log as well as stderr,
// which a GUI process does not have.
func CrashOutput(dir, base string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, base+".log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if fi, err := os.Stat(path); err == nil && fi.Size() > crashMax {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	// SetCrashOutput duplicates the handle: f can be closed right away.
	defer f.Close()
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}
