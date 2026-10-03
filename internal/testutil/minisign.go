// Package testutil holds helpers shared by tests.
package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	minisign "github.com/jedisct1/go-minisign"
)

// SignMinisign creates a fresh minisign key, signs data with it (prehashed
// "ED" mode, like the DNSCrypt resolver lists) and returns the public key
// string and the .minisig file contents.
func SignMinisign(t testing.TB, data []byte) (pubKey string, sigFile []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var sk minisign.PrivateKey
	sk.SignatureAlgorithm = [2]byte{'E', 'd'}
	if _, err := rand.Read(sk.KeyId[:]); err != nil {
		t.Fatal(err)
	}
	copy(sk.SecretKey[:], priv)
	sig, err := sk.Sign(data, minisign.SignOptions{Hashed: true, TrustedComment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte{'E', 'd'}, sk.KeyId[:]...)
	raw = append(raw, pub...)
	return base64.StdEncoding.EncodeToString(raw), sig.Encode()
}
