package shell

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/winutil"
)

// certWiring implements app.Certs over the Windows Root store and the LAN
// CA files in the data directory.
type certWiring struct {
	paths store.Paths
	store certstore.Store
	prot  certs.Protector
	host  func() string
	now   func() time.Time

	mu  sync.Mutex
	lan *certs.CA
}

// machineProtector is DPAPI with machine scope (winutil.ProtectMachine).
type machineProtector struct{}

func (machineProtector) Protect(b []byte) ([]byte, error)   { return winutil.ProtectMachine(b) }
func (machineProtector) Unprotect(b []byte) ([]byte, error) { return winutil.UnprotectMachine(b) }

func newCertWiring(paths store.Paths) *certWiring {
	return &certWiring{paths: paths, store: certstore.NewWindows(certstore.LocalMachine), prot: machineProtector{},
		host: func() string { h, _ := os.Hostname(); return h }, now: time.Now}
}

// loadLocked returns the LAN CA from memory or disk; nil when none exists.
func (c *certWiring) loadLocked() (*certs.CA, error) {
	if c.lan != nil {
		return c.lan, nil
	}
	ca, err := certs.LoadLANCA(c.paths.LANCACert, c.paths.LANCAKey, c.prot)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.lan = ca
	return ca, nil
}

// LANCA implements app.Certs: load or create the LAN CA, and (re)install
// it in Root (adding an identical certificate is a no-op).
func (c *certWiring) LANCA(context.Context) (*certs.CA, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ca, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	if ca == nil {
		if ca, err = certs.NewLANCA(c.host(), c.now()); err != nil {
			return nil, err
		}
		if err := certs.SaveLANCA(c.paths.LANCACert, c.paths.LANCAKey, ca, c.prot); err != nil {
			return nil, fmt.Errorf("save LAN CA: %w", err)
		}
		c.lan = ca
	}
	if err := c.store.Install(ca.DER); err != nil {
		return nil, err
	}
	return ca, nil
}

// removeLocked removes the LAN CA from Root and deletes its files. An
// unreadable key still lets the certificate file be removed.
func (c *certWiring) removeLocked() error {
	var errs []error
	if der, err := os.ReadFile(c.paths.LANCACert); err == nil {
		errs = append(errs, c.store.Remove(certstore.Thumbprint(der)))
	}
	for _, p := range []string{c.paths.LANCACert, c.paths.LANCAKey} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	c.lan = nil
	return errors.Join(errs...)
}

// ResetLANCA implements app.Certs.
func (c *certWiring) ResetLANCA(ctx context.Context) (*certs.CA, error) {
	c.mu.Lock()
	err := c.removeLocked()
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return c.LANCA(ctx)
}

// RemoveLANCA implements app.Certs.
func (c *certWiring) RemoveLANCA(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeLocked()
}

// InstallSession implements app.Certs.
func (c *certWiring) InstallSession(der []byte) error { return c.store.Install(der) }

// RemoveSession implements app.Certs.
func (c *certWiring) RemoveSession(thumb string) error { return c.store.Remove(thumb) }

// List implements app.Certs.
func (c *certWiring) List() ([]certstore.Cert, error) { return c.store.List("Ghostline") }

// mitmBox holds the Fake SNI certificate source the proxy reads.
type mitmBox struct{ p atomic.Pointer[leafRef] }

type leafRef struct{ l mitm.LeafSource }

func (b *mitmBox) set(l mitm.LeafSource) {
	if l == nil {
		b.p.Store(nil)
		return
	}
	b.p.Store(&leafRef{l: l})
}

func (b *mitmBox) get() mitm.LeafSource {
	if r := b.p.Load(); r != nil {
		return r.l
	}
	return nil
}

// selfTest checks that Windows itself trusts a certificate from the
// session CA: the CA really is in Root and its Name Constraints are
// accepted (Verify with nil roots uses the system verifier on Windows).
func (b *mitmBox) selfTest(context.Context) error {
	is, ok := b.get().(*certs.Issuer)
	if !ok {
		return errors.New("fake SNI: no session CA")
	}
	domains := is.CA().Cert.PermittedDNSDomains
	if len(domains) == 0 {
		return errors.New("fake SNI: session CA has no domain")
	}
	leaf, err := is.Leaf(domains[0])
	if err != nil {
		return err
	}
	inter := x509.NewCertPool()
	_, err = leaf.Leaf.Verify(x509.VerifyOptions{DNSName: domains[0], Intermediates: inter,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return fmt.Errorf("fake SNI: the system does not trust the session CA: %w", err)
	}
	return nil
}
