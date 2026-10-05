package proxy

import (
	"bufio"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/dialer"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
)

// fakeSNI intercepts the connection when a rule asks for sni= and Fake
// SNI is active (spec 2B 8.3). It reports whether it handled the
// connection; false means the caller continues on the 2A path with the
// untouched hello (nothing has been sent to the client yet).
func (s *Server) fakeSNI(c net.Conn, br *bufio.Reader, ip netip.Addr, t wire.Target, hello []byte) bool {
	if s.cfg.MITM == nil {
		return false
	}
	leaf := s.cfg.MITM()
	op, ok := s.cfg.Dialer.(fakeSNIOpener)
	if leaf == nil || !ok {
		return false
	}
	dec, host, ok := op.Plan(t, hello)
	if !ok {
		return false
	}
	fake := dec.SNI
	if fake == rules.SNINone {
		fake = ""
	}
	p := mitm.Params{Host: host, FakeSNI: fake, Hello: hello, Roots: s.cfg.MITMRoots, Timeout: s.lim.Handshake, Now: time.Now}
	raw, err := op.OpenRaw(s.ctx, ip, t, dec)
	if err != nil {
		s.event(ip, t, dialer.OutcomeFakeSNIFallback, dec.Source)
		return false
	}
	server, err := mitm.DialServer(s.ctx, raw, p)
	if err != nil {
		raw.Close()
		if errors.Is(err, mitm.ErrVerifyFailed) {
			s.event(ip, t, dialer.OutcomeFakeSNIVerifyFailed, dec.Source)
		}
		s.event(ip, t, dialer.OutcomeFakeSNIFallback, dec.Source)
		return false
	}
	defer server.Close()
	s.trackConn(server)
	defer s.untrackConn(server)
	client, err := mitm.AcceptClient(s.ctx, c, br, p, leaf, server.ConnectionState().NegotiatedProtocol)
	if err != nil {
		s.event(ip, t, dialer.OutcomeFakeSNIClientRejected, dec.Source)
		return true
	}
	s.event(ip, t, dialer.OutcomeFakeSNI, dec.Source)
	s.relay(client, bufio.NewReader(client), server)
	return true
}
