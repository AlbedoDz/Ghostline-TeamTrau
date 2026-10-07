package dpi

import (
	"log/slog"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/hashcott/ghostline/internal/winutil"
)

type winProc struct {
	cmd    *exec.Cmd
	job    *winutil.Job
	exited atomic.Bool
	killed atomic.Bool
}

func (p *winProc) PID() int     { return p.cmd.Process.Pid }
func (p *winProc) Exited() bool { return p.exited.Load() }
func (p *winProc) Kill() error {
	p.killed.Store(true)
	err := p.cmd.Process.Kill()
	if p.job != nil {
		if cerr := p.job.Close(); cerr != nil {
			slog.Warn("dpi: close engine job object failed", "err", cerr, "pid", p.PID())
		}
	}
	return err
}

type winRunner struct{}

// NewWindowsRunner starts hidden processes bound to a kill-on-close job, so
// GoodbyeDPI dies with Ghostline.
func NewWindowsRunner() Runner { return winRunner{} }

func (winRunner) Start(exe string, args []string, dir string) (Process, error) {
	cmd := winutil.HiddenCmd(exe, args, dir)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	p := &winProc{cmd: cmd}
	// Without the job the engine outlives a crashed Ghostline.
	if job, err := winutil.NewKillOnCloseJob(); err == nil {
		if err := job.Assign(cmd.Process); err == nil {
			p.job = job
		} else {
			slog.Warn("dpi: assign engine to kill-on-close job failed", "err", err, "pid", pid, "exe", exe)
			if cerr := job.Close(); cerr != nil {
				slog.Warn("dpi: close engine job object failed", "err", cerr, "pid", pid)
			}
		}
	} else {
		slog.Warn("dpi: create kill-on-close job failed", "err", err, "pid", pid, "exe", exe)
	}
	go func() {
		err := cmd.Wait()
		p.exited.Store(true)
		if p.killed.Load() {
			slog.Info("dpi: engine process stopped", "pid", pid, "exe", exe)
		} else {
			// Includes the exit status: why the engine died on its own.
			slog.Warn("dpi: engine process exited", "err", err, "pid", pid, "exe", exe)
		}
	}()
	return p, nil
}

type winServices struct{}

// NewWindowsServices controls services through the SCM.
func NewWindowsServices() Services { return winServices{} }

func (winServices) Find(prefix string) ([]string, error) { return winutil.FindServices(prefix) }
func (winServices) Running(name string) (bool, error)    { return winutil.ServiceRunning(name) }
func (winServices) Active(name string) (bool, error)     { return winutil.ServiceActive(name) }
func (winServices) Stop(name string) error               { return winutil.StopService(name, 5*time.Second) }
func (winServices) Delete(name string) error             { return winutil.DeleteService(name) }
