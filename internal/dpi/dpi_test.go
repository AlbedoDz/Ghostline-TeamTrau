package dpi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

var light = []string{"-p", "-r", "-s", "-m", "-e", "40", "-w", "--native-frag"}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestExtractVerify(t *testing.T) {
	dir := t.TempDir()
	pins := map[string]string{"a.bin": sha("hello")}
	src := fstest.MapFS{"a.bin": {Data: []byte("hello")}}
	require.NoError(t, extractWith(src, dir, pins))
	require.NoError(t, verifyWith(dir, pins))
	fi1, _ := os.Stat(filepath.Join(dir, "a.bin"))

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, extractWith(src, dir, pins))
	fi2, _ := os.Stat(filepath.Join(dir, "a.bin"))
	require.Equal(t, fi1.ModTime(), fi2.ModTime(), "correct files are not rewritten")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bin"), []byte("evil"), 0o644))
	require.ErrorIs(t, verifyWith(dir, pins), ErrHashMismatch)
	require.NoError(t, extractWith(src, dir, pins))
	require.NoError(t, verifyWith(dir, pins))

	require.ErrorIs(t, extractWith(fstest.MapFS{"a.bin": {Data: []byte("tampered")}}, t.TempDir(), pins), ErrHashMismatch)
}

type fakeProc struct {
	pid    int
	exited bool
	killed bool
}

func (p *fakeProc) PID() int     { return p.pid }
func (p *fakeProc) Exited() bool { return p.exited }
func (p *fakeProc) Kill() error  { p.killed = true; p.exited = true; return nil }

type fakeRunner struct {
	proc *fakeProc
	args []string
	err  error
}

func (r *fakeRunner) Start(exe string, args []string, dir string) (Process, error) {
	r.args = args
	if r.err != nil {
		return nil, r.err
	}
	return r.proc, nil
}

type fakeSvc struct {
	running bool
	calls   []string
	names   []string // installed WinDivert services
}

func (s *fakeSvc) Find(prefix string) ([]string, error) {
	if s.names == nil {
		return []string{"WinDivert1.4"}, nil
	}
	return s.names, nil
}

func (s *fakeSvc) Running(name string) (bool, error) { return s.running && name == "WinDivert1.4", nil }
func (s *fakeSvc) Stop(name string) error            { s.calls = append(s.calls, "svc.stop:"+name); return nil }
func (s *fakeSvc) Delete(name string) error {
	s.calls = append(s.calls, "svc.delete:"+name)
	return nil
}

func newTestManager(t *testing.T, r *fakeRunner, s *fakeSvc) *Manager {
	pins := map[string]string{"goodbyedpi.exe": sha("x")}
	m := NewManager(t.TempDir(), fstest.MapFS{"goodbyedpi.exe": {Data: []byte("x")}}, r, s, func(time.Duration) {})
	m.pins = pins
	return m
}

func TestManager_StartSuccess(t *testing.T) {
	r := &fakeRunner{proc: &fakeProc{pid: 8812}}
	m := newTestManager(t, r, &fakeSvc{running: true})
	pid, err := m.Start(context.Background(), light)
	require.NoError(t, err)
	require.Equal(t, 8812, pid)
	require.Equal(t, light, r.args)
	require.True(t, m.Running())
}

func TestManager_ExitedImmediatelyIsStartFailed(t *testing.T) {
	m := newTestManager(t, &fakeRunner{proc: &fakeProc{pid: 1, exited: true}}, &fakeSvc{running: false})
	_, err := m.Start(context.Background(), light)
	require.ErrorIs(t, err, ErrStartFailed)
	require.False(t, m.Running())
}

func TestManager_AccessDeniedIsBlockedByAV(t *testing.T) {
	m := newTestManager(t, &fakeRunner{err: os.ErrPermission}, &fakeSvc{})
	_, err := m.Start(context.Background(), light)
	require.ErrorIs(t, err, ErrBlockedByAV)
}

func TestManager_DriverNotRunningIsStartFailed(t *testing.T) {
	m := newTestManager(t, &fakeRunner{proc: &fakeProc{pid: 1}}, &fakeSvc{running: false})
	_, err := m.Start(context.Background(), light)
	require.ErrorIs(t, err, ErrStartFailed)
}

func TestManager_StopKillsAndRemovesWinDivert(t *testing.T) {
	r := &fakeRunner{proc: &fakeProc{pid: 8812}}
	s := &fakeSvc{running: true}
	m := newTestManager(t, r, s)
	_, err := m.Start(context.Background(), light)
	require.NoError(t, err)
	require.NoError(t, m.Stop())
	require.True(t, r.proc.killed)
	require.Equal(t, []string{"svc.stop:WinDivert1.4", "svc.delete:WinDivert1.4"}, s.calls)
	require.False(t, m.Running())
}

func TestManager_StopWhenNotRunningStillCleansDriver(t *testing.T) {
	s := &fakeSvc{}
	m := newTestManager(t, &fakeRunner{}, s)
	require.NoError(t, m.Stop())
	require.Equal(t, []string{"svc.stop:WinDivert1.4", "svc.delete:WinDivert1.4"}, s.calls)
	require.False(t, errors.Is(nil, ErrStartFailed))
}

func TestManager_BlacklistCopiedToASCIIName(t *testing.T) { // review I7: GoodbyeDPI uses ANSI argv
	src := filepath.Join(t.TempDir(), "Đức Hạnh", "dpi-blacklist.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("youtube.com\n"), 0o644))
	r := &fakeRunner{proc: &fakeProc{pid: 1}}
	m := newTestManager(t, r, &fakeSvc{running: true})
	_, err := m.Start(context.Background(), append(append([]string{}, light...), "--blacklist", src))
	require.NoError(t, err)
	require.Equal(t, "blacklist.txt", r.args[len(r.args)-1])
	b, err := os.ReadFile(filepath.Join(m.dir, "blacklist.txt"))
	require.NoError(t, err)
	require.Equal(t, "youtube.com\n", string(b))
}

// WinDivert 1.x registers a versioned service ("WinDivert1.4"); looking for
// exactly "WinDivert" made every start look failed.
func TestManager_VersionedDriverServiceCountsAsRunning(t *testing.T) {
	m := newTestManager(t, &fakeRunner{proc: &fakeProc{pid: 7}}, &fakeSvc{running: true, names: []string{"WinDivert1.4"}})
	pid, err := m.Start(context.Background(), light)
	require.NoError(t, err)
	require.Equal(t, 7, pid)
}

func TestManager_StopRemovesEveryWinDivertService(t *testing.T) {
	s := &fakeSvc{names: []string{"WinDivert", "WinDivert1.4"}}
	m := newTestManager(t, &fakeRunner{}, s)
	require.NoError(t, m.Stop())
	require.Equal(t, []string{"svc.stop:WinDivert", "svc.delete:WinDivert", "svc.stop:WinDivert1.4", "svc.delete:WinDivert1.4"}, s.calls)
}
