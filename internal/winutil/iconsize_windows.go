package winutil

import "golang.org/x/sys/windows"

var procGetSystemMetrics = windows.NewLazySystemDLL("user32.dll").NewProc("GetSystemMetrics")

const smCxSmIcon = 49

// SmallIconSize is the size Windows draws small icons at (the tray), in
// pixels for the current scale: 16 at 100 %, 20 at 125 %, 24 at 150 %.
// Drawing the tray icon at exactly this size keeps Windows from blurring it.
func SmallIconSize() int {
	n, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	if n == 0 {
		return 16
	}
	return int(n)
}
