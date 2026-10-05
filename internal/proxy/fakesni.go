package proxy

import (
	"bufio"
	"crypto/tls"
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
	// Only browsers on this PC trust the session CA; LAN devices sharing
	// the proxy always take the 2A path (spec 2B §2).
	if s.cfg.MITM == nil || !ip.Unmap().IsLoopback() {
		return false
	}
	leaf := s.cfg.MITM()
	op, ok := s.cfg.Dialer.(fakeSNIOpener)
	if leaf == nil || !ok {
		return false
	}
	dec, host, ok := op.Plan(t, hello)
	if !ok || !covers(leaf, host) {
		// No rule, or a rule newer than the running session CA (it rotates
		// a moment later): take the 2A path rather than fail the client.
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
	client, err := mitm.AcceptClient(s.ctx, c, br, p, currentLeaf{s}, server.ConnectionState().NegotiatedProtocol)
	if err != nil {
		s.event(ip, t, dialer.OutcomeFakeSNIClientRejected, dec.Source)
		return true
	}
	s.event(ip, t, dialer.OutcomeFakeSNI, dec.Source)
	s.relay(client, bufio.NewReader(client), server)
	return true
}

// covers asks the certificate source whether it may sign host; sources
// that cannot say are trusted to.
func covers(l mitm.LeafSource, host string) bool {
	if c, ok := l.(interface{ Covers(string) bool }); ok {
		return c.Covers(host)
	}
	return true
}

// currentLeaf signs with the certificate source in use when the client
// handshake needs it, so a CA rotated (and removed) during the server
// handshake is never used.
type currentLeaf struct{ s *Server }

func (c currentLeaf) Leaf(host string) (*tls.Certificate, error) {
	l := c.s.cfg.MITM()
	if l == nil || !covers(l, host) {
		return nil, errors.New("fake SNI: no certificate source for " + host)
	}
	return l.Leaf(host)
}
