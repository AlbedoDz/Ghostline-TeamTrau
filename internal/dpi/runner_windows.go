package dpi

import (
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/hashcott/ghostline/internal/winutil"
)

type winProc struct {
	cmd    *exec.Cmd
	job    *winutil.Job
	exited atomic.Bool
}

func (p *winProc) PID() int     { return p.cmd.Process.Pid }
func (p *winProc) Exited() bool { return p.exited.Load() }
func (p *winProc) Kill() error {
	err := p.cmd.Process.Kill()
	if p.job != nil {
		_ = p.job.Close()
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
	p := &winProc{cmd: cmd}
	if job, err := winutil.NewKillOnCloseJob(); err == nil {
		if err := job.Assign(cmd.Process); err == nil {
			p.job = job
		} else {
			_ = job.Close()
		}
	}
	go func() { _ = cmd.Wait(); p.exited.Store(true) }()
	return p, nil
}

type winServices struct{}

// NewWindowsServices controls services through the SCM.
func NewWindowsServices() Services { return winServices{} }

func (winServices) Find(prefix string) ([]string, error) { return winutil.FindServices(prefix) }
func (winServices) Running(name string) (bool, error)    { return winutil.ServiceRunning(name) }
func (winServices) Stop(name string) error               { return winutil.StopService(name, 5*time.Second) }
func (winServices) Delete(name string) error             { return winutil.DeleteService(name) }
