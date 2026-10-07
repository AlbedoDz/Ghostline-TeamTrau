// Package stamps reads and writes DNS stamps (sdns://) as plain fields.
package stamps

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/ameshkov/dnsstamps"
)

// Fields is a stamp as editable fields.
type Fields struct {
	Proto        string   `json:"proto"`        // doh dot doq dnscrypt plain odoh-target odoh-relay dnscrypt-relay
	Addr         string   `json:"addr"`         // "ip" or "ip:port"; may be empty for doh/dot/doq
	Host         string   `json:"host"`         // doh/dot/doq/odoh hostname (may include :port)
	Path         string   `json:"path"`         // doh/odoh
	ProviderName string   `json:"providerName"` // dnscrypt
	PublicKey    string   `json:"publicKey"`    // dnscrypt, hex
	Hashes       []string `json:"hashes"`       // hex SHA-256 of TBS certificates
	DNSSEC       bool     `json:"dnssec"`
	NoLog        bool     `json:"noLog"`
	NoFilter     bool     `json:"noFilter"`
	Stamp        string   `json:"stamp"`  // normalised sdns:// string (output only)
	Usable       bool     `json:"usable"` // Ghostline can add it as a server
}

// ErrInvalid marks a stamp or field that cannot be used; the wrapping error
// names the field.
var ErrInvalid = errors.New("stamps: invalid")

func invalid(field string, why string) error {
	if why == "" {
		return fmt.Errorf("%w: %s", ErrInvalid, field)
	}
	return fmt.Errorf("%w: %s: %s", ErrInvalid, field, why)
}

const prefix = "sdns://"

// Stamp type bytes the dnsstamps library does not read.
const (
	typeODoHTarget    = 0x05
	typeDNSCryptRelay = 0x81
	typeODoHRelay     = 0x85
)

// defaultPorts are the ports dnsstamps appends to a bare IP when it decodes
// (and drops again when it encodes).
var defaultPorts = map[dnsstamps.StampProtoType]string{
	dnsstamps.StampProtoTypeDNSCrypt: ":443",
	dnsstamps.StampProtoTypeDoH:      ":443",
	dnsstamps.StampProtoTypeTLS:      ":843",
	dnsstamps.StampProtoTypeDoQ:      ":784",
	dnsstamps.StampProtoTypePlain:    ":53",
}

var protoNames = map[dnsstamps.StampProtoType]string{
	dnsstamps.StampProtoTypeDNSCrypt: "dnscrypt",
	dnsstamps.StampProtoTypeDoH:      "doh",
	dnsstamps.StampProtoTypeTLS:      "dot",
	dnsstamps.StampProtoTypeDoQ:      "doq",
	dnsstamps.StampProtoTypePlain:    "plain",
}

// Decode reads any stamp type, including relays and ODoH.
func Decode(s string) (f Fields, err error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, prefix) {
		return Fields{}, invalid("stamp", "must start with sdns://")
	}
	bin, err := base64.RawURLEncoding.DecodeString(s[len(prefix):])
	if err != nil || len(bin) == 0 {
		return Fields{}, invalid("stamp", "not base64")
	}
	switch bin[0] {
	case typeODoHTarget, typeDNSCryptRelay, typeODoHRelay:
		f, err = decodeOther(bin)
		if err != nil {
			return Fields{}, err
		}
		f.Stamp = s
		return f, nil
	}
	defer func() { // the library indexes past short input on some malformed stamps
		if r := recover(); r != nil {
			// Expected on some malformed input; the stack shows where.
			slog.Warn("stamps: decoder panicked on a malformed stamp", "panic", r, "stack", string(debug.Stack()))
			f, err = Fields{}, invalid("stamp", "malformed")
		}
	}()
	st, err := dnsstamps.NewServerStampFromString(s)
	if err != nil {
		return Fields{}, invalid("stamp", err.Error())
	}
	name, ok := protoNames[st.Proto]
	if !ok {
		return Fields{}, invalid("stamp", "unknown type")
	}
	f = Fields{Proto: name, Path: st.Path, Stamp: st.String()}
	f.Addr = strings.TrimSuffix(st.ServerAddrStr, defaultPorts[st.Proto])
	if st.Proto == dnsstamps.StampProtoTypeDNSCrypt {
		f.ProviderName = st.ProviderName
		f.PublicKey = hex.EncodeToString(st.ServerPk)
	} else {
		f.Host = st.ProviderName
	}
	for _, h := range st.Hashes {
		f.Hashes = append(f.Hashes, hex.EncodeToString(h))
	}
	setProps(&f, uint64(st.Props))
	f.Usable = name != "plain"
	return f, nil
}

func setProps(f *Fields, p uint64) {
	f.DNSSEC = p&uint64(dnsstamps.ServerInformalPropertyDNSSEC) != 0
	f.NoLog = p&uint64(dnsstamps.ServerInformalPropertyNoLog) != 0
	f.NoFilter = p&uint64(dnsstamps.ServerInformalPropertyNoFilter) != 0
}

// reader walks the length-prefixed fields of a stamp.
type reader struct {
	b   []byte
	pos int
	bad bool
}

func (r *reader) lp() string {
	if r.pos >= len(r.b) || r.pos+1+int(r.b[r.pos]) > len(r.b) {
		r.bad = true
		return ""
	}
	n := int(r.b[r.pos])
	s := string(r.b[r.pos+1 : r.pos+1+n])
	r.pos += 1 + n
	return s
}

// vlp reads a set of values whose length bytes carry 0x80 while more follow.
func (r *reader) vlp() []string {
	var out []string
	for !r.bad {
		if r.pos >= len(r.b) {
			r.bad = true
			return nil
		}
		v := r.b[r.pos]
		n := int(v &^ 0x80)
		if r.pos+1+n > len(r.b) {
			r.bad = true
			return nil
		}
		if n > 0 {
			out = append(out, string(r.b[r.pos+1:r.pos+1+n]))
		}
		r.pos += 1 + n
		if v&0x80 == 0 {
			break
		}
	}
	return out
}

func (r *reader) props() uint64 {
	if r.pos+8 > len(r.b) {
		r.bad = true
		return 0
	}
	p := binary.LittleEndian.Uint64(r.b[r.pos:])
	r.pos += 8
	return p
}

func decodeOther(bin []byte) (Fields, error) {
	r := &reader{b: bin, pos: 1}
	var f Fields
	switch bin[0] {
	case typeDNSCryptRelay:
		f = Fields{Proto: "dnscrypt-relay", Addr: r.lp()}
	case typeODoHTarget:
		f.Proto = "odoh-target"
		setProps(&f, r.props())
		f.Host, f.Path = r.lp(), r.lp()
	case typeODoHRelay:
		f.Proto = "odoh-relay"
		setProps(&f, r.props())
		f.Addr = r.lp()
		for _, h := range r.vlp() {
			f.Hashes = append(f.Hashes, hex.EncodeToString([]byte(h)))
		}
		f.Host, f.Path = r.lp(), r.lp()
		if !r.bad && r.pos < len(bin) {
			r.vlp() // optional bootstrap IPs
		}
	}
	if r.bad || r.pos != len(bin) {
		return Fields{}, invalid("stamp", "malformed")
	}
	return f, nil
}
