package servers

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ameshkov/dnsstamps"
	"github.com/hashcott/ghostline/internal/model"
)

// ErrUnencrypted rejects plain DNS servers: Ghostline only speaks encrypted DNS.
var ErrUnencrypted = errors.New("servers: unencrypted DNS is not allowed")

// errSkip marks stamps Ghostline does not use (relays, ODoH).
var errSkip = errors.New("servers: unsupported stamp type")

// FromStamp builds a server from an sdns:// stamp. The stamp itself stays the
// address; IPs and tags are read from it.
func FromStamp(stamp string, src model.Source) (model.Server, error) {
	st, err := dnsstamps.NewServerStampFromString(stamp)
	if err != nil {
		return model.Server{}, fmt.Errorf("servers: bad stamp: %w", err)
	}
	s := model.Server{Address: stamp, Source: src, Provider: st.ProviderName, Name: st.ProviderName}
	switch st.Proto {
	case dnsstamps.StampProtoTypeDNSCrypt:
		s.Protocol = model.ProtoDNSCrypt
	case dnsstamps.StampProtoTypeDoH:
		s.Protocol = model.ProtoDoH
	case dnsstamps.StampProtoTypeTLS:
		s.Protocol = model.ProtoDoT
	case dnsstamps.StampProtoTypeDoQ:
		s.Protocol = model.ProtoDoQ
	case dnsstamps.StampProtoTypePlain:
		return model.Server{}, ErrUnencrypted
	default:
		return model.Server{}, errSkip
	}
	if ip := stampIP(st.ServerAddrStr); ip != "" {
		s.IPs = []string{ip}
	}
	if st.Props&dnsstamps.ServerInformalPropertyNoFilter != 0 {
		s.Tags = append(s.Tags, "no-filter")
	}
	if st.Props&dnsstamps.ServerInformalPropertyNoLog != 0 {
		s.Tags = append(s.Tags, "no-log")
	}
	if st.Props&dnsstamps.ServerInformalPropertyDNSSEC != 0 {
		s.Tags = append(s.Tags, "dnssec")
	}
	return s, nil
}

// stampIP extracts the IP from "ip", "ip:port" or "[ipv6]:port".
func stampIP(addr string) string {
	if addr == "" {
		return ""
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

// FromAddress builds a server from a URL or stamp typed by the user.
func FromAddress(addr string, src model.Source) (model.Server, error) {
	addr = strings.TrimSpace(addr)
	var s model.Server
	switch {
	case strings.HasPrefix(addr, "sdns://"):
		var err error
		if s, err = FromStamp(addr, src); err != nil {
			return model.Server{}, err
		}
	case strings.HasPrefix(addr, "https://"):
		s = model.Server{Protocol: model.ProtoDoH}
	case strings.HasPrefix(addr, "tls://"):
		s = model.Server{Protocol: model.ProtoDoT}
	case strings.HasPrefix(addr, "quic://"):
		s = model.Server{Protocol: model.ProtoDoQ}
	default:
		return model.Server{}, ErrUnencrypted
	}
	s.Address, s.Source = addr, src
	if s.Name == "" {
		s.Name = hostOf(addr)
		s.Provider = s.Name
	}
	if src == model.SourceCustom {
		sum := sha1.Sum([]byte(addr))
		s.ID = "custom:" + hex.EncodeToString(sum[:])[:8]
	}
	return s, nil
}

func hostOf(addr string) string {
	rest := addr[strings.Index(addr, "://")+3:]
	if i := strings.IndexAny(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
