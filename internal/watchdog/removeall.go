package watchdog

import (
	"errors"
	"io/fs"
	"os"

	"github.com/hashcott/ghostline/internal/certstore"
)

// Subject prefixes of Ghostline's roots (certs.SessionPrefix, LANPrefix).
var certPrefixes = []string{"Ghostline Fake SNI", "Ghostline LAN CA"}

// RemoveAllCerts removes every Ghostline root certificate from s and
// deletes files (the LAN CA certificate and key). It is the uninstaller's
// last step, after recovery.
func RemoveAllCerts(s certstore.Store, files ...string) error {
	var errs []error
	for _, p := range certPrefixes {
		if _, err := certstore.Sweep(s, p, nil); err != nil {
			errs = append(errs, err)
		}
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
