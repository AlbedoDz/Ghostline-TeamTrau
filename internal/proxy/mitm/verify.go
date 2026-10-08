package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	sysRootsOnce sync.Once
	sysRoots     *x509.CertPool
	sysRootsErr  error
)

func getSystemRoots() (*x509.CertPool, error) {
	sysRootsOnce.Do(func() {
		sysRoots, sysRootsErr = x509.SystemCertPool()
	})
	return sysRoots, sysRootsErr
}

// verifyServer requires a chain to p.Roots (system roots when nil) for
// serverAuth, valid now, for the fake SNI or the real host. With no fake
// SNI only the real host is accepted.
func verifyServer(cs tls.ConnectionState, p Params) error {
	if len(cs.PeerCertificates) == 0 {
		return fmt.Errorf("%w: no certificate", ErrVerifyFailed)
	}
	roots := p.Roots
	if roots == nil {
		var err error
		if roots, err = getSystemRoots(); err != nil {
			return fmt.Errorf("%w: %v", ErrVerifyFailed, err)
		}
	}
	inter := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	names := []string{p.Host}
	if p.FakeSNI != "" {
		names = []string{p.FakeSNI, p.Host}
	}
	var errs []error
	for _, name := range names {
		_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots: roots, Intermediates: inter, DNSName: name, CurrentTime: now,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("%w: %v", ErrVerifyFailed, errors.Join(errs...))
}
