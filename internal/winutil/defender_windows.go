package winutil

import (
	"path/filepath"
	"strings"
)

// IsPathExcludedFromDefender checks whether dir is covered by an active
// Windows Defender exclusion path.
func IsPathExcludedFromDefender(dir string) (bool, error) {
	cleanDir := strings.ToLower(filepath.Clean(dir))
	ps := filepath.Join(system32(""), `WindowsPowerShell\v1.0\powershell.exe`)
	out, err := HiddenCmd(ps, []string{"-NoProfile", "-NonInteractive", "-Command",
		"Get-MpPreference -ErrorAction SilentlyContinue | Select-Object -ExpandProperty ExclusionPath"}, "").Output()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.ToLower(strings.TrimSpace(line))
		if p == "" {
			continue
		}
		// Direct match or parent directory exclusion
		if p == cleanDir || strings.HasPrefix(cleanDir, strings.TrimSuffix(p, `\`)+`\`) {
			return true, nil
		}
	}
	return false, nil
}

// AddDefenderExclusion prompts for elevation (UAC) to whitelist dir in
// Windows Defender exclusions.
func AddDefenderExclusion(dir string) error {
	cleanDir := filepath.Clean(dir)
	ps := filepath.Join(system32(""), `WindowsPowerShell\v1.0\powershell.exe`)
	cmd := HiddenCmd(ps, []string{
		"-NoProfile", "-NonInteractive", "-Command",
		`Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile -NonInteractive -Command Add-MpPreference -ExclusionPath "'` + cleanDir + `'"'`,
	}, "")
	return cmd.Run()
}
