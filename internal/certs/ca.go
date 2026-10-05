// Package certs creates Ghostline's two name-constrained root CAs (spec 2B
// section 5) and the server certificates they sign. It is pure Go and has
// no dependency on Windows or on other Ghostline packages.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
	"time"
)

// Subject prefixes; recovery sweeps match on them, so never change them.
const (
	LANPrefix     = "Ghostline LAN CA"
	SessionPrefix = "Ghostline Fake SNI"
)

// LANDomain is the only DNS name the LAN CA may sign (and its subdomains).
const LANDomain = "ghostline.lan"

// LANPermittedCIDRs are the only addresses the LAN CA may sign.
var LANPermittedCIDRs = []string{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
	"169.254.0.0/16", "fc00::/7", "fe80::/10", "::1/128",
}

// MaxSessionDomains caps the session CA's Name Constraints.
const MaxSessionDomains = 1000

const (
	lanLife     = 5 // years
	sessionLife = 30 * 24 * time.Hour
	backdate    = time.Hour
)

// ErrTooManyDomains means more than MaxSessionDomains sni= domains.
var ErrTooManyDomains = errors.New("certs: too many Fake SNI domains")

// CA is a root certificate with its key. A session CA's key lives only in
// this struct: nothing in this package serialises it.
type CA struct {
	Cert *x509.Certificate
	DER  []byte
	Key  *ecdsa.PrivateKey
}

// NewLANCA creates the long-lived LAN CA for host (the computer name).
func NewLANCA(host string, now time.Time) (*CA, error) {
	ips, err := parseCIDRs(LANPermittedCIDRs)
	if err != nil {
		return nil, err
	}
	tmpl := caTemplate(fmt.Sprintf("%s — %s %s", LANPrefix, host, randomTag()), now, now.AddDate(lanLife, 0, 0))
	tmpl.PermittedDNSDomains = []string{LANDomain}
	tmpl.PermittedIPRanges = ips
	return newCA(tmpl)
}

// NewSessionCA creates a Fake SNI CA limited to domains (Name Constraint
// strings from rules.Compiled.SNIDomains) and to no IP address at all.
func NewSessionCA(domains []string, now time.Time) (*CA, error) {
	if len(domains) == 0 {
		return nil, errors.New("certs: a session CA needs at least one domain")
	}
	if len(domains) > MaxSessionDomains {
		return nil, ErrTooManyDomains
	}
	all, err := parseCIDRs([]string{"0.0.0.0/0", "::/0"})
	if err != nil {
		return nil, err
	}
	tmpl := caTemplate(fmt.Sprintf("%s — phiên %s", SessionPrefix, now.Format(time.RFC3339)), now.Add(-backdate), now.Add(sessionLife))
	tmpl.PermittedDNSDomains = append([]string(nil), domains...)
	tmpl.ExcludedIPRanges = all
	return newCA(tmpl)
}

func caTemplate(cn string, notBefore, notAfter time.Time) *x509.Certificate {
	return &x509.Certificate{
		Subject:                     pkix.Name{CommonName: cn, Organization: []string{"Ghostline"}},
		NotBefore:                   notBefore,
		NotAfter:                    notAfter,
		IsCA:                        true,
		BasicConstraintsValid:       true,
		MaxPathLen:                  0,
		MaxPathLenZero:              true,
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:                 []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		PermittedDNSDomainsCritical: true,
	}
}

func newCA(tmpl *x509.Certificate) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if tmpl.SerialNumber, err = serial(); err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("certs: create CA: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, DER: der, Key: key}, nil
}

// Thumbprint is the lower-case hex SHA-1 of the certificate, the key the
// Windows certificate store finds it by.
func (c *CA) Thumbprint() string { return Thumbprint(c.DER) }

// Thumbprint is the lower-case hex SHA-1 of a DER certificate.
func Thumbprint(der []byte) string {
	sum := sha1.Sum(der)
	return fmt.Sprintf("%x", sum[:])
}

// Fingerprint is the SHA-256 shown to users ("AB:CD:…").
func (c *CA) Fingerprint() string {
	sum := sha256.Sum256(c.DER)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// IssueServer signs a server certificate for names and ips valid for life
// from now.
func (c *CA) IssueServer(names []string, ips []netip.Addr, life time.Duration, now time.Time) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	sn, err := serial()
	if err != nil {
		return nil, err
	}
	cn := ""
	if len(names) > 0 {
		cn = names[0]
	}
	tmpl := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-backdate),
		NotAfter:              now.Add(life),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
	}
	for _, ip := range ips {
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip.AsSlice())
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.Cert, &key.PublicKey, c.Key)
	if err != nil {
		return nil, fmt.Errorf("certs: issue: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, c.DER}, PrivateKey: key, Leaf: leaf}, nil
}

func serial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

const tagAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomTag() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = tagAlphabet[int(b[i])%len(tagAlphabet)]
	}
	return string(b)
}
