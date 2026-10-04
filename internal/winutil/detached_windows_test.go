package winutil

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// The launcher runs in its own process (this test binary re-executed) so it
// can sit inside a kill-on-close job, like Ghostline started from a
// terminal or IDE, without taking the test runner down with it.
const launcherEnv = "GHOSTLINE_TEST_LAUNCHER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(launcherEnv); mode != "" {
		os.Exit(runLauncher(mode == "breakaway"))
	}
	os.Exit(m.Run())
}

// runLauncher joins a kill-on-close job, starts a detached child, prints its
// pid, then closes the job (killing itself and anything left in the job).
func runLauncher(allowBreakaway bool) int {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 2
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if allowBreakaway {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	}
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return 3
	}
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		return 4
	}
	cmd, err := StartDetached("cmd", []string{"/c", "ping -n 30 127.0.0.1"})
	if err != nil {
		fmt.Println("start error:", err)
		return 5
	}
	fmt.Println("child", cmd.Process.Pid)
	_ = os.Stdout.Sync()
	windows.CloseHandle(h) // kills this process and everything still in the job
	time.Sleep(5 * time.Second)
	return 6
}

func launch(t *testing.T, mode string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), launcherEnv+"="+mode)
	out, _ := cmd.Output()
	for _, line := range strings.Split(string(out), "\n") {
		if pid, ok := strings.CutPrefix(strings.TrimSpace(line), "child "); ok {
			n, err := strconv.Atoi(pid)
			require.NoError(t, err)
			return n
		}
	}
	t.Fatalf("launcher printed no child pid: %q", out)
	return 0
}

func kill(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// The watchdog must outlive a terminal/IDE job that kills Ghostline.
func TestStartDetached_SurvivesParentJobClose(t *testing.T) {
	pid := launch(t, "breakaway")
	defer kill(pid)
	time.Sleep(500 * time.Millisecond)
	require.True(t, running(pid), "detached child was killed with the launcher's job")
}

// A job that forbids breakaway must not stop the watchdog from starting.
func TestStartDetached_FallsBackWhenBreakawayDenied(t *testing.T) {
	pid := launch(t, "nobreakaway")
	defer kill(pid)
	require.NotZero(t, pid)
}

func running(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}
