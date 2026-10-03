// Package servers loads, verifies and merges DNS server lists.
package servers

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// List is the on-disk server list format (lists/servers.json).
type List struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Servers     []model.Server `json:"servers"`
}

// ParseList decodes a server list.
func ParseList(b []byte) (List, error) {
	var l List
	err := json.Unmarshal(b, &l)
	return l, err
}

// ErrBadSignature means a signed list failed verification.
var ErrBadSignature = errors.New("servers: bad signature")

// Sign returns the standard-base64 ed25519 signature of data.
func Sign(data []byte, priv ed25519.PrivateKey) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)))
}

// VerifySigned checks a signature produced by Sign.
func VerifySigned(data, sig []byte, pub ed25519.PublicKey) error {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, data, raw) {
		return ErrBadSignature
	}
	return nil
}
