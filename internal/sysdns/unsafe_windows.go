package sysdns

import "unsafe"

// unsafePtr smuggles a small integer id through the PVOID callback context.
// The value is never dereferenced.
func unsafePtr(id uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&id))
}
