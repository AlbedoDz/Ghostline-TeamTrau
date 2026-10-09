//go:build !windows

package game

func ApplyWindowsGamingTweaks(enable bool) error {
	return nil
}

func IsWindowsGamingTweaksApplied() (bool, error) {
	return false, nil
}
