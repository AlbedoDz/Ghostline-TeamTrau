package winutil

import (
	"errors"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func openSCM(access uint32) (windows.Handle, error) {
	return windows.OpenSCManager(nil, nil, access)
}

// ServiceForPID returns the name of the Win32 service running in pid, or "".
func ServiceForPID(pid uint32) (string, error) {
	scm, err := openSCM(windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return "", err
	}
	defer windows.CloseServiceHandle(scm)
	var needed, count, resume uint32
	_ = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32,
		windows.SERVICE_ACTIVE, nil, 0, &needed, &count, &resume, nil)
	if needed == 0 {
		return "", nil
	}
	buf := make([]byte, needed)
	resume = 0
	if err := windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32,
		windows.SERVICE_ACTIVE, &buf[0], needed, &needed, &count, &resume, nil); err != nil {
		return "", err
	}
	entries := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), count)
	for _, e := range entries {
		if e.ServiceStatusProcess.ProcessId == pid {
			return windows.UTF16PtrToString(e.ServiceName), nil
		}
	}
	return "", nil
}

// ServiceRunning reports whether the named service exists and is running.
// It needs only query rights, so it works without elevation.
func ServiceRunning(name string) (bool, error) {
	scm, err := openSCM(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(scm)
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenService(scm, p, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(h)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(h, &st); err != nil {
		return false, err
	}
	return st.CurrentState == windows.SERVICE_RUNNING, nil
}

// ServiceActive reports whether the named service exists and is not fully
// stopped (running, paused, or mid-transition). A missing service is not
// active. It needs only query rights, so it works without elevation.
func ServiceActive(name string) (bool, error) {
	scm, err := openSCM(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(scm)
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenService(scm, p, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(h)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(h, &st); err != nil {
		return false, err
	}
	return st.CurrentState != windows.SERVICE_STOPPED, nil
}

// StopService stops a service and waits up to wait for it to stop. A missing
// or already stopped service is not an error. A service mid-transition is
// waited out rather than failed. Needs elevation.
func StopService(name string, wait time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	st, err := s.Control(svc.Stop)
	// ERROR_SERVICE_NOT_ACTIVE: already stopped. ERROR_SERVICE_CANNOT_ACCEPT_CTRL:
	// the service is mid-transition — typically a leftover WinDivert driver
	// auto-unloading (STOP_PENDING) after its process was killed. s.Control
	// still returns the current state, so wait it out in the loop below instead
	// of failing the engine start with "cannot accept control messages".
	if err != nil &&
		!errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) &&
		!errors.Is(err, windows.ERROR_SERVICE_CANNOT_ACCEPT_CTRL) {
		return err
	}
	deadline := time.Now().Add(wait)
	for st.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
}

// DeleteService marks a service for deletion. A missing service is not an
// error. Needs elevation.
func DeleteService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return err
	}
	return nil
}

// FindServices lists installed services and drivers (any state) whose name
// starts with prefix, case-insensitively.
func FindServices(prefix string) ([]string, error) {
	scm, err := openSCM(windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, err
	}
	defer windows.CloseServiceHandle(scm)
	const kinds = windows.SERVICE_DRIVER | windows.SERVICE_WIN32
	var needed, count, resume uint32
	_ = windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, kinds, windows.SERVICE_STATE_ALL, nil, 0, &needed, &count, &resume, nil)
	if needed == 0 {
		return nil, nil
	}
	buf := make([]byte, needed+4096)
	resume = 0
	if err := windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, kinds, windows.SERVICE_STATE_ALL,
		&buf[0], uint32(len(buf)), &needed, &count, &resume, nil); err != nil {
		return nil, err
	}
	var out []string
	p := strings.ToLower(prefix)
	for _, e := range unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), count) {
		name := windows.UTF16PtrToString(e.ServiceName)
		if strings.HasPrefix(strings.ToLower(name), p) {
			out = append(out, name)
		}
	}
	return out, nil
}
