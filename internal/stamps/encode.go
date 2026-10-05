package stamps

import (
	"encoding/hex"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ameshkov/dnsstamps"
)

var protoTypes = map[string]dnsstamps.StampProtoType{
	"dnscrypt": dnsstamps.StampProtoTypeDNSCrypt,
	"doh":      dnsstamps.StampProtoTypeDoH,
	"dot":      dnsstamps.StampProtoTypeTLS,
	"doq":      dnsstamps.StampProtoTypeDoQ,
	"plain":    dnsstamps.StampProtoTypePlain,
}

// Encode builds a stamp from f (doh, dot, doq, dnscrypt or plain). The
// result is decoded again and compared with f before it is returned.
func Encode(f Fields) (string, error) {
	pt, ok := protoTypes[f.Proto]
	if !ok {
		return "", invalid("proto", "only doh, dot, doq, dnscrypt and plain can be built")
	}
	needAddr := f.Proto == "dnscrypt" || f.Proto == "plain"
	if (needAddr || f.Addr != "") && !validAddr(f.Addr) {
		return "", invalid("addr", "must be ip or ip:port")
	}
	st := dnsstamps.ServerStamp{Proto: pt, ServerAddrStr: f.Addr}
	switch f.Proto {
	case "dnscrypt":
		if !strings.HasPrefix(f.ProviderName, "2.dnscrypt-cert.") || len(f.ProviderName) <= len("2.dnscrypt-cert.") {
			return "", invalid("providerName", "must start with 2.dnscrypt-cert.")
		}
		pk, err := hex.DecodeString(f.PublicKey)
		if err != nil || len(pk) != 32 {
			return "", invalid("publicKey", "must be 32 bytes in hex")
		}
		st.ProviderName, st.ServerPk = f.ProviderName, pk
	case "doh", "dot", "doq":
		if !validHost(f.Host) {
			return "", invalid("host", "must be a hostname")
		}
		st.ProviderName = f.Host
		if f.Proto == "doh" {
			if !strings.HasPrefix(f.Path, "/") {
				return "", invalid("path", "must start with /")
			}
			st.Path = f.Path
		}
		for _, h := range f.Hashes {
			b, err := hex.DecodeString(h)
			if err != nil || len(b) != 32 {
				return "", invalid("hashes", "each must be 32 bytes in hex")
			}
			st.Hashes = append(st.Hashes, b)
		}
	}
	if f.DNSSEC {
		st.Props |= dnsstamps.ServerInformalPropertyDNSSEC
	}
	if f.NoLog {
		st.Props |= dnsstamps.ServerInformalPropertyNoLog
	}
	if f.NoFilter {
		st.Props |= dnsstamps.ServerInformalPropertyNoFilter
	}
	s := st.String()
	back, err := Decode(s)
	want := f
	want.Stamp, want.Usable = back.Stamp, back.Usable
	if len(want.Hashes) == 0 {
		want.Hashes = nil
	}
	if f.Proto != "doh" {
		want.Path = ""
	}
	if err != nil || !reflect.DeepEqual(want, back) {
		return "", invalid("stamp", "round trip mismatch")
	}
	return s, nil
}

// FromURL fills fields from https:// (DoH), tls:// (DoT) or quic:// (DoQ)
// and an optional IP.
func FromURL(raw, ip string) (Fields, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || !validHost(u.Host) {
		return Fields{}, invalid("url", "must be https://, tls:// or quic:// with a host")
	}
	f := Fields{Host: u.Host}
	switch u.Scheme {
	case "https":
		f.Proto, f.Path = "doh", u.Path
		if f.Path == "" {
			f.Path = "/dns-query"
		}
	case "tls":
		f.Proto = "dot"
	case "quic":
		f.Proto = "doq"
	default:
		return Fields{}, invalid("url", "must be https://, tls:// or quic://")
	}
	if ip = strings.TrimSpace(ip); ip != "" {
		a, err := netip.ParseAddr(ip)
		if err != nil {
			return Fields{}, invalid("addr", "must be an IP")
		}
		f.Addr = a.String()
		if a.Is6() {
			f.Addr = "[" + f.Addr + "]"
		}
	}
	return f, nil
}

// validAddr accepts "ip", "[ipv6]", "ip:port" and "[ipv6]:port".
func validAddr(s string) bool {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Is4()
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		a, err := netip.ParseAddr(s[1 : len(s)-1])
		return err == nil && a.Is6()
	}
	_, err := netip.ParseAddrPort(s)
	return err == nil && !strings.HasSuffix(s, ":0")
}

var label = regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9_])?$`)

// validHost accepts a hostname with an optional :port.
func validHost(s string) bool {
	host := s
	if h, p, err := net.SplitHostPort(s); err == nil {
		n, perr := strconv.Atoi(p)
		if perr != nil || n < 1 || n > 65535 {
			return false
		}
		host = h
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, l := range strings.Split(host, ".") {
		if !label.MatchString(l) {
			return false
		}
	}
	return true
}
