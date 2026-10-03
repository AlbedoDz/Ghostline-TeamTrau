package servers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/hashcott/ghostline/internal/model"
	minisign "github.com/jedisct1/go-minisign"
)

// ParseDNSCryptMarkdown reads a DNSCrypt resolvers list (v3 markdown). Each
// sdns:// line under a "## name" section becomes one server; unsupported
// stamps (plain DNS, relays) are skipped.
func ParseDNSCryptMarkdown(md []byte) ([]model.Server, error) {
	var out []model.Server
	section, n := "", 0
	sc := bufio.NewScanner(bytes.NewReader(md))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "## "):
			section, n = strings.TrimSpace(line[3:]), 0
		case strings.HasPrefix(line, "sdns://") && section != "":
			s, err := FromStamp(line, model.SourceDNSCrypt)
			if err != nil {
				continue
			}
			n++
			s.ID = "dnscrypt:" + section
			if n > 1 {
				s.ID = fmt.Sprintf("%s#%d", s.ID, n)
			}
			s.Name = section
			out = append(out, s)
		}
	}
	return out, sc.Err()
}

// VerifyMinisign checks data against a .minisig file with the given public key.
func VerifyMinisign(data, sig []byte, pubKey string) error {
	pk, err := minisign.NewPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("servers: bad minisign key: %w", err)
	}
	s, err := minisign.DecodeSignature(string(sig))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadSignature, err)
	}
	ok, err := pk.Verify(data, s)
	if err != nil || !ok {
		return errors.Join(ErrBadSignature, err)
	}
	return nil
}
