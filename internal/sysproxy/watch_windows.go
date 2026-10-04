package sysproxy

import (
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Watch calls onChange (debounced by 500 ms) whenever the Internet Settings
// key or its Connections subkey changes, until stop is called.
func Watch(onChange func()) (stop func(), err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.NOTIFY)
	if err != nil {
		return nil, err
	}
	changed, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		k.Close()
		return nil, err
	}
	quit, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		k.Close()
		windows.CloseHandle(changed)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer k.Close()
		defer windows.CloseHandle(changed)
		for {
			if err := windows.RegNotifyChangeKeyValue(windows.Handle(k), true,
				windows.REG_NOTIFY_CHANGE_LAST_SET|windows.REG_NOTIFY_CHANGE_NAME, changed, true); err != nil {
				return
			}
			ev, err := windows.WaitForMultipleObjects([]windows.Handle{changed, quit}, false, windows.INFINITE)
			if err != nil || ev != windows.WAIT_OBJECT_0 {
				return
			}
			time.Sleep(500 * time.Millisecond) // let a burst of writes settle
			onChange()
		}
	}()
	return func() {
		_ = windows.SetEvent(quit)
		<-done
		windows.CloseHandle(quit)
	}, nil
}
