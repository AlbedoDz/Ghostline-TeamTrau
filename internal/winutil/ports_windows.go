package winutil

import (
	"errors"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	afInet                   = 2
	afInet6                  = 23
	tcpTableOwnerPIDListener = 3
	udpTableOwnerPID         = 1
	errInsufficientBuffer    = 122
)

// PortOwner is a process holding a local port on a loopback or wildcard address.
type PortOwner struct {
	PID     uint32
	Name    string
	Service string
	Proto   string // "udp" | "tcp"
}

func extendedTable(proc *windows.LazyProc, af, class uint32) ([]byte, error) {
	var size uint32
	r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(af), uintptr(class), 0)
	for attempt := 0; attempt < 5; attempt++ {
		if r != errInsufficientBuffer && r != 0 {
			return nil, windows.Errno(r)
		}
		buf := make([]byte, size+1024)
		size = uint32(len(buf))
		r, _, _ = proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(af), uintptr(class), 0)
		if r == 0 {
			return buf, nil
		}
	}
	return nil, errors.New("winutil: port table kept growing")
}

// PortOwners lists processes listening on port (UDP or TCP) on a loopback
// or wildcard address, IPv4 and IPv6.
func PortOwners(port uint16) ([]PortOwner, error) {
	type src struct {
		proc  *windows.LazyProc
		af    uint32
		class uint32
		parse func([]byte) []portRow
	}
	var rows []portRow
	for _, s := range []src{
		{procGetExtendedUdpTable, afInet, udpTableOwnerPID, parseUDP4Table},
		{procGetExtendedUdpTable, afInet6, udpTableOwnerPID, parseUDP6Table},
		{procGetExtendedTcpTable, afInet, tcpTableOwnerPIDListener, parseTCP4Table},
		{procGetExtendedTcpTable, afInet6, tcpTableOwnerPIDListener, parseTCP6Table},
	} {
		b, err := extendedTable(s.proc, s.af, s.class)
		if err != nil {
			return nil, err
		}
		rows = append(rows, s.parse(b)...)
	}
	seen := map[[2]uint32]bool{}
	var out []PortOwner
	for _, r := range rows {
		if r.port != port || !(r.addr.IsLoopback() || r.addr.IsUnspecified()) {
			continue
		}
		proto := "udp"
		if r.tcp {
			proto = "tcp"
		}
		key := [2]uint32{r.pid, map[bool]uint32{false: 0, true: 1}[r.tcp]}
		if seen[key] {
			continue
		}
		seen[key] = true
		o := PortOwner{PID: r.pid, Proto: proto}
		if name, err := ProcessName(r.pid); err == nil {
			o.Name = filepath.Base(name)
		}
		o.Service, _ = ServiceForPID(r.pid)
		out = append(out, o)
	}
	return out, nil
}
