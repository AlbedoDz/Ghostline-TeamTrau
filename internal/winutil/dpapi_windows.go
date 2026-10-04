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
