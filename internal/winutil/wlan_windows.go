package winutil

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wlanapi                = windows.NewLazySystemDLL("wlanapi.dll")
	procWlanOpenHandle     = wlanapi.NewProc("WlanOpenHandle")
	procWlanCloseHandle    = wlanapi.NewProc("WlanCloseHandle")
	procWlanEnumInterfaces = wlanapi.NewProc("WlanEnumInterfaces")
	procWlanQueryInterface = wlanapi.NewProc("WlanQueryInterface")
	procWlanFreeMemory     = wlanapi.NewProc("WlanFreeMemory")
)

const (
	wlanStateConnected          = 1
	wlanOpcodeCurrentConnection = 7
	// WLAN_INTERFACE_INFO: GUID(16) + WCHAR[256] + state(4).
	wlanInterfaceInfoSize = 16 + 512 + 4
	// WLAN_CONNECTION_ATTRIBUTES: state(4) + mode(4) + WCHAR[256], then
	// DOT11_SSID{ULONG len; UCHAR ssid[32]}.
	wlanSSIDOffset = 4 + 4 + 512
)

// CurrentSSID returns the name of the Wi-Fi network this PC is connected
// to, or "" when it is not on Wi-Fi (or has no WLAN service).
func CurrentSSID() (string, error) {
	if wlanapi.Load() != nil {
		return "", nil
	}
	var negotiated uint32
	var h windows.Handle
	if r, _, _ := procWlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&negotiated)), uintptr(unsafe.Pointer(&h))); r != 0 {
		return "", nil // WLAN AutoConfig not running: no Wi-Fi
	}
	defer procWlanCloseHandle.Call(uintptr(h), 0)
	var list unsafe.Pointer
	if r, _, _ := procWlanEnumInterfaces.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&list))); r != 0 {
		return "", errors.New("wlan: enumerate interfaces failed")
	}
	defer procWlanFreeMemory.Call(uintptr(list))
	n := *(*uint32)(list)
	for i := uint32(0); i < n; i++ {
		info := unsafe.Add(list, 8+uintptr(i)*wlanInterfaceInfoSize)
		if *(*uint32)(unsafe.Add(info, 16+512)) != wlanStateConnected {
			continue
		}
		var size uint32
		var data unsafe.Pointer
		if r, _, _ := procWlanQueryInterface.Call(uintptr(h), uintptr(info), wlanOpcodeCurrentConnection, 0,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0); r != 0 {
			continue
		}
		l := *(*uint32)(unsafe.Add(data, wlanSSIDOffset))
		if l > 32 {
			l = 32
		}
		ssid := string(unsafe.Slice((*byte)(unsafe.Add(data, wlanSSIDOffset+4)), l))
		procWlanFreeMemory.Call(uintptr(data))
		return ssid, nil
	}
	return "", nil
}
