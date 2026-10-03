package sysdns

import (
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var (
	watchMu  sync.Mutex
	watchFns = map[uintptr]func(){}
	watchSeq uintptr
	// One callback for the process: windows.NewCallback slots are limited.
	watchCallback = windows.NewCallback(func(ctx uintptr, _ uintptr, _ uint32) uintptr {
		watchMu.Lock()
		f := watchFns[ctx]
		watchMu.Unlock()
		if f != nil {
			f()
		}
		return 0
	})
)

// Watch calls onChange (debounced by 2s) whenever an IP interface changes.
func Watch(onChange func()) (stop func(), err error) {
	trigger, stopDebounce := Debounce(2*time.Second, onChange)
	watchMu.Lock()
	watchSeq++
	id := watchSeq
	watchFns[id] = trigger
	watchMu.Unlock()

	var h windows.Handle
	// ctx is passed back to the callback as an opaque value.
	if err := windows.NotifyIpInterfaceChange(windows.AF_UNSPEC, watchCallback, unsafePtr(id), false, &h); err != nil {
		watchMu.Lock()
		delete(watchFns, id)
		watchMu.Unlock()
		return nil, err
	}
	return func() {
		// Never call CancelMibChangeNotify2 from inside the callback.
		_ = windows.CancelMibChangeNotify2(h)
		stopDebounce()
		watchMu.Lock()
		delete(watchFns, id)
		watchMu.Unlock()
	}, nil
}
