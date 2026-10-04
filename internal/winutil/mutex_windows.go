package winutil

import (
	"fmt"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

// NamedMutex is a cross-process Win32 mutex. Win32 mutexes are owned by a
// thread, so every Lock/Unlock runs on one locked OS thread. If the owning
// process dies the mutex is abandoned and the next Lock still succeeds.
type NamedMutex struct {
	// local excludes goroutines of this process: the Win32 mutex is
	// recursive for its owning thread, which serves every caller here.
	local sync.Mutex
	reqs  chan mutexReq
}

type mutexReq struct {
	lock bool
	done chan error
}

// NewNamedMutex opens (or creates) the mutex called name.
func NewNamedMutex(name string) (*NamedMutex, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	created := make(chan error, 1)
	m := &NamedMutex{reqs: make(chan mutexReq)}
	go func() {
		runtime.LockOSThread() // never unlocked: the thread owns the mutex
		h, err := windows.CreateMutex(nil, false, p)
		if err != nil && h == 0 {
			created <- err
			return
		}
		created <- nil
		for r := range m.reqs {
			if r.lock {
				ev, err := windows.WaitForSingleObject(h, windows.INFINITE)
				if err == nil && ev != windows.WAIT_OBJECT_0 && ev != windows.WAIT_ABANDONED {
					err = fmt.Errorf("winutil: wait mutex: %#x", ev)
				}
				r.done <- err
			} else {
				r.done <- windows.ReleaseMutex(h)
			}
		}
	}()
	if err := <-created; err != nil {
		return nil, err
	}
	return m, nil
}

func (m *NamedMutex) do(lock bool) error {
	done := make(chan error, 1)
	m.reqs <- mutexReq{lock: lock, done: done}
	return <-done
}

// Lock waits for the mutex.
func (m *NamedMutex) Lock() error {
	m.local.Lock()
	if err := m.do(true); err != nil {
		m.local.Unlock()
		return err
	}
	return nil
}

// Unlock releases the mutex.
func (m *NamedMutex) Unlock() error {
	err := m.do(false)
	m.local.Unlock()
	return err
}
