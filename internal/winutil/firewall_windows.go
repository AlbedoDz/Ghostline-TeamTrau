package winutil

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func system32(name string) string {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return name
	}
	return filepath.Join(dir, name)
}

func netsh(args []string) ([]byte, error) {
	return HiddenCmd(system32("netsh.exe"), args, "").CombinedOutput()
}

// CurrentNetworkIsPublic reports whether a connected network uses the
// Public firewall profile (LAN devices cannot reach the proxy then).
func CurrentNetworkIsPublic() (bool, error) {
	ps := filepath.Join(system32(""), `WindowsPowerShell\v1.0\powershell.exe`)
	out, err := HiddenCmd(ps, []string{"-NoProfile", "-NonInteractive", "-Command",
		"Get-NetConnectionProfile | ForEach-Object { $_.NetworkCategory.ToString() }"}, "").Output()
	if err != nil {
		return false, err
	}
	return parsePublic(string(out)), nil
}
