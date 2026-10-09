//go:build windows

package game

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const (
	sysProfilePath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`
	gamesTaskPath  = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile\Tasks\Games`
	tcpipIntfPath  = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
)

// ApplyWindowsGamingTweaks optimizes or restores Windows network latency registry parameters.
func ApplyWindowsGamingTweaks(enable bool) error {
	var errs []error

	// 1. Multimedia SystemProfile (Network throttling & system responsiveness)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, sysProfilePath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err == nil {
		if enable {
			_ = k.SetDWordValue("NetworkThrottlingIndex", 0xffffffff) // Disables network throttling
			_ = k.SetDWordValue("SystemResponsiveness", 0x00000000)   // 100% priority for gaming tasks
		} else {
			_ = k.SetDWordValue("NetworkThrottlingIndex", 10)         // Windows default (10 packets/ms)
			_ = k.SetDWordValue("SystemResponsiveness", 20)          // Windows default (20% reserved)
		}
		_ = k.Close()
	} else {
		errs = append(errs, err)
	}

	// 2. Multimedia Tasks\Games (High GPU/SFIO priority for games)
	gk, err := registry.OpenKey(registry.LOCAL_MACHINE, gamesTaskPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err == nil {
		if enable {
			_ = gk.SetDWordValue("GPU Priority", 8)
			_ = gk.SetDWordValue("Priority", 6)
			_ = gk.SetStringValue("Scheduling Category", "High")
			_ = gk.SetStringValue("SFIO Priority", "High")
		} else {
			_ = gk.SetDWordValue("GPU Priority", 8)
			_ = gk.SetDWordValue("Priority", 2) // Windows default priority
			_ = gk.SetStringValue("Scheduling Category", "Medium")
			_ = gk.SetStringValue("SFIO Priority", "Normal")
		}
		_ = gk.Close()
	} else {
		errs = append(errs, err)
	}

	// 3. Tcpip interfaces (TcpAckFrequency & TCPNoDelay)
	ik, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpipIntfPath, registry.ENUMERATE_SUB_KEYS|registry.READ)
	if err == nil {
		subkeys, _ := ik.ReadSubKeyNames(-1)
		_ = ik.Close()

		for _, sub := range subkeys {
			subPath := tcpipIntfPath + `\` + sub
			sk, sErr := registry.OpenKey(registry.LOCAL_MACHINE, subPath, registry.SET_VALUE|registry.QUERY_VALUE)
			if sErr == nil {
				if enable {
					_ = sk.SetDWordValue("TcpAckFrequency", 1) // Immediate ACK (no Nagle delay)
					_ = sk.SetDWordValue("TCPNoDelay", 1)
				} else {
					_ = sk.DeleteValue("TcpAckFrequency")
					_ = sk.DeleteValue("TCPNoDelay")
				}
				_ = sk.Close()
			}
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// IsWindowsGamingTweaksApplied checks if low-latency gaming tweaks are active.
func IsWindowsGamingTweaksApplied() (bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, sysProfilePath, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("NetworkThrottlingIndex")
	if err != nil {
		return false, err
	}
	return uint32(val) == 0xffffffff, nil
}
