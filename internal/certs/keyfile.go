package certs

import (
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Protector encrypts the LAN CA key at rest (DPAPI in production).
type Protector interface {
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

// ErrKeyUnreadable means the LAN CA key file exists but cannot be
// decrypted or parsed (another machine, a damaged file).
var ErrKeyUnreadable = errors.New("certs: LAN CA key unreadable")

// SaveLANCA writes the certificate (DER) and the protected PKCS#8 key.
func SaveLANCA(certPath, keyPath string, ca *CA, p Protector) error {
	pkcs8, err := x509.MarshalPKCS8PrivateKey(ca.Key)
	if err != nil {
		return err
	}
	enc, err := p.Protect(pkcs8)
	if err != nil {
		return fmt.Errorf("certs: protect LAN CA key: %w", err)
	}
	if err := writeFile(keyPath, enc); err != nil {
		return err
	}
	return writeFile(certPath, ca.DER)
}

// LoadLANCA reads what SaveLANCA wrote. A missing file returns an error
// wrapping os.ErrNotExist.
func LoadLANCA(certPath, keyPath string, p Protector) (*CA, error) {
	der, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	enc, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyUnreadable, err)
	}
	pkcs8, err := p.Unprotect(enc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyUnreadable, err)
	}
	k, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyUnreadable, err)
	}
	key, ok := k.(*ecdsa.PrivateKey)
	if !ok || !key.PublicKey.Equal(cert.PublicKey) {
		return nil, fmt.Errorf("%w: key does not match the certificate", ErrKeyUnreadable)
	}
	if err := checkLANShape(cert); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyUnreadable, err)
	}
	return &CA{Cert: cert, DER: der, Key: key}, nil
}

// checkLANShape accepts only a certificate NewLANCA could have made: a
// CA limited to serverAuth, the private ranges and ghostline.lan. A file
// replaced by another program is therefore never installed in Root.
func checkLANShape(c *x509.Certificate) error {
	want, err := parseCIDRs(LANPermittedCIDRs)
	if err != nil {
		return err
	}
	switch {
	case !strings.HasPrefix(c.Subject.CommonName, LANPrefix+" "):
		return errors.New("not a Ghostline LAN CA")
	case !c.IsCA || !c.BasicConstraintsValid || !c.MaxPathLenZero:
		return errors.New("bad basic constraints")
	case !slices.Equal(c.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) || len(c.UnknownExtKeyUsage) > 0:
		return errors.New("extended key usage must be serverAuth only")
	case !c.PermittedDNSDomainsCritical || !slices.Equal(c.PermittedDNSDomains, []string{LANDomain}):
		return errors.New("bad DNS name constraints")
	case len(c.ExcludedDNSDomains)+len(c.ExcludedIPRanges)+len(c.PermittedEmailAddresses)+len(c.ExcludedEmailAddresses)+
		len(c.PermittedURIDomains)+len(c.ExcludedURIDomains) > 0:
		return errors.New("unexpected name constraints")
	case len(c.PermittedIPRanges) != len(want):
		return errors.New("bad IP name constraints")
	}
	for i, n := range c.PermittedIPRanges {
		if n.String() != want[i].String() {
			return errors.New("bad IP name constraints")
		}
	}
	return nil
}

// writeFile replaces path atomically, readable only by its owner.
func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
