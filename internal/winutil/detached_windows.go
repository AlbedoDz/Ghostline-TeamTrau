package winutil

import (
	"errors"
	"os/exec"

	"golang.org/x/sys/windows"
)

// StartDetached starts a hidden process meant to outlive this one. When
// Ghostline was started from a terminal or IDE it sits in that host's job
// object, and children inherit the job: closing the host would kill the
// watchdog together with us. CREATE_BREAKAWAY_FROM_JOB takes it out; if
// the job forbids breakaway (access denied), it starts inside the job.
func StartDetached(exe string, args []string) (*exec.Cmd, error) {
	cmd := HiddenCmd(exe, args, "")
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_BREAKAWAY_FROM_JOB
	err := cmd.Start()
	if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return cmd, err
	}
	cmd = HiddenCmd(exe, args, "")
	return cmd, cmd.Start()
}
