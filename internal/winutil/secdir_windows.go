package winutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// AdminOnlySDDL grants full control to SYSTEM and Administrators only,
// with inheritance from the parent blocked (protected DACL).
const AdminOnlySDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// SecureDir creates dir if needed and replaces its DACL with
// AdminOnlySDDL, so programs running as a normal user can neither read
// nor replace what Ghostline (elevated) keeps there.
func SecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(AdminOnlySDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("secure %s: %w", dir, err)
	}
	return nil
}

// OwnedByAdmins reports whether path is owned by Administrators or SYSTEM.
// Files Ghostline writes while elevated are; a file planted by a normal
// user is not.
func OwnedByAdmins(path string) (bool, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return false, err
	}
	for _, w := range []windows.WELL_KNOWN_SID_TYPE{windows.WinBuiltinAdministratorsSid, windows.WinLocalSystemSid} {
		sid, err := windows.CreateWellKnownSid(w)
		if err == nil && owner.Equals(sid) {
			return true, nil
		}
	}
	return false, nil
}
