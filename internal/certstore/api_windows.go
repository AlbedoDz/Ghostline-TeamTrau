package certstore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Location selects the Windows system store.
type Location uint32

const (
	// LocalMachine is the machine-wide Root store (production: no prompt
	// when elevated).
	LocalMachine Location = windows.CERT_SYSTEM_STORE_LOCAL_MACHINE
	// CurrentUser is the user's Root store (integration tests only: adding
	// to it shows a Windows confirmation dialog).
	CurrentUser Location = windows.CERT_SYSTEM_STORE_CURRENT_USER
)

const encoding = windows.X509_ASN_ENCODING | windows.PKCS_7_ASN_ENCODING

type winStore struct{ loc Location }

// NewWindows returns the Root store at loc.
func NewWindows(loc Location) Store { return winStore{loc: loc} }

// open opens the Root store; readOnly works without elevation.
func (w winStore) open(readOnly bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString("ROOT")
	if err != nil {
		return 0, err
	}
	flags := uint32(w.loc)
	if readOnly {
		flags |= windows.CERT_STORE_READONLY_FLAG
	}
	h, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0, flags, uintptr(unsafe.Pointer(name)))
	if err != nil {
		return 0, fmt.Errorf("certstore: open Root: %w", err)
	}
	return h, nil
}

// Install implements Store.
func (w winStore) Install(der []byte) error {
	if len(der) == 0 {
		return errors.New("certstore: empty certificate")
	}
	h, err := w.open(false)
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(h, 0)
	ctx, err := windows.CertCreateCertificateContext(encoding, &der[0], uint32(len(der)))
	if err != nil {
		return fmt.Errorf("certstore: parse: %w", err)
	}
	defer windows.CertFreeCertificateContext(ctx)
	if err := windows.CertAddCertificateContextToStore(h, ctx, windows.CERT_STORE_ADD_REPLACE_EXISTING, nil); err != nil {
		return fmt.Errorf("certstore: add: %w", err)
	}
	found, err := find(h, Thumbprint(der))
	if err != nil {
		return err
	}
	if found == nil {
		return ErrNotInstalled
	}
	windows.CertFreeCertificateContext(found)
	return nil
}

// Remove implements Store.
func (w winStore) Remove(thumbprint string) error {
	h, err := w.open(false)
	if err != nil {
		return err
	}
	defer windows.CertCloseStore(h, 0)
	ctx, err := find(h, thumbprint)
	if err != nil || ctx == nil {
		return err
	}
	// CertDeleteCertificateFromStore frees ctx, even on failure.
	if err := windows.CertDeleteCertificateFromStore(ctx); err != nil {
		return fmt.Errorf("certstore: delete %s: %w", thumbprint, err)
	}
	return nil
}

// List implements Store.
func (w winStore) List(prefix string) ([]Cert, error) {
	h, err := w.open(true)
	if err != nil {
		return nil, err
	}
	defer windows.CertCloseStore(h, 0)
	var out []Cert
	var ctx *windows.CertContext
	for {
		ctx, err = windows.CertEnumCertificatesInStore(h, ctx)
		if ctx == nil {
			break
		}
		der := unsafe.Slice(ctx.EncodedCert, ctx.Length)
		c, perr := describe(der)
		if perr == nil && strings.HasPrefix(c.Subject, prefix) {
			out = append(out, c)
		}
	}
	if err != nil && !errors.Is(err, windows.Errno(windows.CRYPT_E_NOT_FOUND)) && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return out, fmt.Errorf("certstore: list: %w", err)
	}
	return out, nil
}

// find returns the context of thumbprint (caller frees) or nil.
func find(h windows.Handle, thumbprint string) (*windows.CertContext, error) {
	raw, err := hex.DecodeString(thumbprint)
	if err != nil || len(raw) != 20 {
		return nil, fmt.Errorf("certstore: bad thumbprint %q", thumbprint)
	}
	blob := windows.CryptHashBlob{Size: uint32(len(raw)), Data: &raw[0]}
	ctx, err := windows.CertFindCertificateInStore(h, encoding, 0, windows.CERT_FIND_SHA1_HASH, unsafe.Pointer(&blob), nil)
	if ctx == nil {
		if err == nil || errors.Is(err, windows.Errno(windows.CRYPT_E_NOT_FOUND)) {
			return nil, nil
		}
		return nil, fmt.Errorf("certstore: find: %w", err)
	}
	return ctx, nil
}
