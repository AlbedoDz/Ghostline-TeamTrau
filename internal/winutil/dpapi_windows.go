package winutil

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProtectString encrypts s with DPAPI for the current user and returns it
// base64-encoded (for upstream proxy passwords in settings.json).
func ProtectString(s string) (string, error) {
	in := []byte(s)
	var inBlob windows.DataBlob
	if len(in) > 0 {
		inBlob = windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("dpapi: protect: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return base64.StdEncoding.EncodeToString(unsafe.Slice(out.Data, out.Size)), nil
}

// UnprotectString reverses ProtectString. It fails for data protected by
// another user or machine.
func UnprotectString(b64 string) (string, error) {
	in, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("dpapi: %w", err)
	}
	if len(in) == 0 {
		return "", fmt.Errorf("dpapi: empty blob")
	}
	inBlob := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("dpapi: unprotect: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return string(unsafe.Slice(out.Data, out.Size)), nil
}

// ProtectMachine encrypts b with DPAPI for this computer (any account on
// it can decrypt, so files holding the result must be ACL-protected).
// Ghostline runs elevated, possibly as another admin than the one logged
// on, so per-user DPAPI would not survive a change of elevating account.
func ProtectMachine(b []byte) ([]byte, error) {
	return dpapi(b, true, windows.CRYPTPROTECT_UI_FORBIDDEN|windows.CRYPTPROTECT_LOCAL_MACHINE)
}

// UnprotectMachine reverses ProtectMachine.
func UnprotectMachine(b []byte) ([]byte, error) {
	return dpapi(b, false, windows.CRYPTPROTECT_UI_FORBIDDEN)
}

func dpapi(in []byte, protect bool, flags uint32) ([]byte, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("dpapi: empty input")
	}
	inBlob := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&inBlob, nil, nil, 0, nil, flags, &out)
	} else {
		err = windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, flags, &out)
	}
	if err != nil {
		return nil, fmt.Errorf("dpapi: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
