package winutil

import (
	"errors"
	"slices"
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

var (
	procWlanGetProfileList          = wlanapi.NewProc("WlanGetProfileList")
	procWlanGetAvailableNetworkList = wlanapi.NewProc("WlanGetAvailableNetworkList")
)

const (
	// WLAN_PROFILE_INFO: WCHAR strProfileName[256] + DWORD dwFlags.
	wlanProfileInfoSize = 512 + 4
	// WLAN_AVAILABLE_NETWORK: WCHAR[256], DOT11_SSID{ULONG; UCHAR[32]},
	// then 20 DWORD-sized fields (phy types array included).
	wlanAvailableNetworkSize = 628
)

// WifiNames lists Wi-Fi network names this PC knows: saved profiles and
// networks in range, sorted, without duplicates or hidden (empty) names.
// It is empty on a PC without a Wi-Fi adapter.
func WifiNames() ([]string, error) {
	if wlanapi.Load() != nil {
		return nil, nil
	}
	var negotiated uint32
	var h windows.Handle
	if r, _, _ := procWlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&negotiated)), uintptr(unsafe.Pointer(&h))); r != 0 {
		return nil, nil
	}
	defer procWlanCloseHandle.Call(uintptr(h), 0)
	var ifaces unsafe.Pointer
	if r, _, _ := procWlanEnumInterfaces.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&ifaces))); r != 0 {
		return nil, errors.New("wlan: enumerate interfaces failed")
	}
	defer procWlanFreeMemory.Call(uintptr(ifaces))
	seen := map[string]bool{}
	n := *(*uint32)(ifaces)
	for i := uint32(0); i < n; i++ {
		guid := unsafe.Add(ifaces, 8+uintptr(i)*wlanInterfaceInfoSize)
		var profiles unsafe.Pointer
		if r, _, _ := procWlanGetProfileList.Call(uintptr(h), uintptr(guid), 0, uintptr(unsafe.Pointer(&profiles))); r == 0 {
			for j := uint32(0); j < *(*uint32)(profiles); j++ {
				name := unsafe.Slice((*uint16)(unsafe.Add(profiles, 8+uintptr(j)*wlanProfileInfoSize)), 256)
				seen[windows.UTF16ToString(name)] = true
			}
			procWlanFreeMemory.Call(uintptr(profiles))
		}
		var nets unsafe.Pointer
		if r, _, _ := procWlanGetAvailableNetworkList.Call(uintptr(h), uintptr(guid), 0, 0, uintptr(unsafe.Pointer(&nets))); r == 0 {
			for j := uint32(0); j < *(*uint32)(nets); j++ {
				e := unsafe.Add(nets, 8+uintptr(j)*wlanAvailableNetworkSize)
				l := min(*(*uint32)(unsafe.Add(e, 512)), 32)
				seen[string(unsafe.Slice((*byte)(unsafe.Add(e, 516)), l))] = true
			}
			procWlanFreeMemory.Call(uintptr(nets))
		}
	}
	delete(seen, "")
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	slices.Sort(out)
	return out, nil
}
