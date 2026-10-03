package startup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/winutil"
)

// Create registers (or replaces) the task. Needs elevation for HighestAvailable.
func Create(t Task) error {
	f, err := os.CreateTemp("", "ghostline-task-*.xml")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(EncodeUTF16LE(TaskXML(t))); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	out, err := winutil.HiddenCmd("schtasks", []string{"/Create", "/TN", t.Name, "/XML", filepath.Clean(path), "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Create %q: %w: %s", t.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes the task; a missing task is not an error.
func Delete(name string) error {
	if !Exists(name) {
		return nil
	}
	out, err := winutil.HiddenCmd("schtasks", []string{"/Delete", "/TN", name, "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Delete %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Exists reports whether the task is registered.
func Exists(name string) bool {
	return winutil.HiddenCmd("schtasks", []string{"/Query", "/TN", name}, "").Run() == nil
}
