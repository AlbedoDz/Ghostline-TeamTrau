package startup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
)

// Create registers (or replaces) the task. Needs elevation for HighestAvailable.
func Create(t Task) error { return createXML(t.Name, TaskXML(t)) }

// CreateGuard writes guard.ps1 into dir, made admin-only first because the
// task runs it as SYSTEM, and registers the guard task for the current
// user's state.json. Needs elevation.
func CreateGuard(dir, state string) error {
	if err := winutil.SecureDir(dir); err != nil {
		return err
	}
	script := filepath.Join(dir, "guard.ps1")
	if err := os.WriteFile(script, GuardScript, 0o600); err != nil {
		return err
	}
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("startup: current user: %w", err)
	}
	return createXML(brand.TaskGuard, GuardTaskXML(Guard{
		PowerShell: filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		Script:     script,
		State:      state,
		UserSID:    tu.User.Sid.String(),
		Log:        filepath.Join(dir, "guard.log"),
	}))
}

func createXML(name, xml string) error {
	f, err := os.CreateTemp("", "ghostline-task-*.xml")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(EncodeUTF16LE(xml)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := winutil.HiddenCmdContext(cmdCtx, "schtasks", []string{"/Create", "/TN", name, "/XML", filepath.Clean(path), "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Create %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes the task; a missing task is not an error.
func Delete(name string) error {
	if !Exists(name) {
		return nil
	}
	cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := winutil.HiddenCmdContext(cmdCtx, "schtasks", []string{"/Delete", "/TN", name, "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Delete %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Exists reports whether the task is registered.
func Exists(name string) bool {
	cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := winutil.HiddenCmdContext(cmdCtx, "schtasks", []string{"/Query", "/TN", name}, "").Run()
	// A non-zero exit means "no such task"; anything else means schtasks
	// itself could not run, and the answer is a guess.
	if err != nil && !errors.As(err, new(*exec.ExitError)) {
		slog.Warn("startup: schtasks /Query failed to run", "err", err, "task", name)
	}
	return err == nil
}
