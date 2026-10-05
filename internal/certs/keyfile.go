package certs

import (
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
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
	return &CA{Cert: cert, DER: der, Key: key}, nil
}

// writeFile replaces path atomically, readable only by its owner.
func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
