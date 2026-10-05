// Package proxy is Ghostline's local HTTP/HTTPS/SOCKS proxy: one port,
// protocol sniffed from the first byte, outbound side delegated to a dialer.
package proxy

import (
	"bufio"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/dialer"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
)

// Opener opens outbound connections (*dialer.Dialer).
type Opener interface {
	Decide(t wire.Target) error
	Open(ctx context.Context, client netip.Addr, t wire.Target, hello []byte) (dialer.Result, error)
}

// Limits are the spec 5.1/5.2/5.7 limits; zero fields take the defaults.
type Limits struct {
	PerIP     int           // 64
	Total     int           // 1024
	Handshake time.Duration // 10 s
	Hello     time.Duration // 5 s
	Idle      time.Duration // 5 min
	Drain     time.Duration // 2 s
}

func (l Limits) withDefaults() Limits {
	def := func(v *time.Duration, d time.Duration) {
		if *v == 0 {
			*v = d
		}
	}
	if l.PerIP == 0 {
		l.PerIP = 64
	}
	if l.Total == 0 {
		l.Total = 1024
	}
	def(&l.Handshake, 10*time.Second)
	def(&l.Hello, 5*time.Second)
	def(&l.Idle, 5*time.Minute)
	def(&l.Drain, 2*time.Second)
	return l
}

// Config describes one proxy run.
type Config struct {
	Listen   []netip.AddrPort // loopback, or 0.0.0.0/[::] when sharing on the LAN
	ShareLAN bool
	Dialer   Opener
	OnConn   func(ConnEvent) // nil: not recorded (RAM-only live view)
	Limits   Limits
	// MITM returns the Fake SNI certificate source; nil (or a nil func)
	// means Fake SNI is inactive and every connection takes the 2A path.
	MITM func() mitm.LeafSource
	// MITMRoots verifies real servers during Fake SNI; nil = system roots.
	MITMRoots *x509.CertPool
}

// fakeSNIOpener is implemented by *dialer.Dialer.
type fakeSNIOpener interface {
	Plan(t wire.Target, hello []byte) (rules.Decision, string, bool)
	OpenRaw(ctx context.Context, client netip.Addr, t wire.Target, dec rules.Decision) (net.Conn, error)
}

// ConnEvent describes one finished handshake for the live view.
type ConnEvent struct {
	Time    time.Time    `json:"time"`
	Client  string       `json:"client"`
	Target  string       `json:"target"`
	Outcome string       `json:"outcome"`
	Source  rules.Source `json:"source"`
}

// Server is a running proxy.
type Server struct {
	cfg Config
	lim Limits

	mu      sync.Mutex
	lns     []net.Listener
	conns   map[net.Conn]struct{}
	perIP   map[netip.Addr]int
	running bool
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	stats   stats
}

// New creates a stopped server.
func New(c Config) *Server {
	return &Server{cfg: c, lim: c.Limits.withDefaults(), conns: map[net.Conn]struct{}{}, perIP: map[netip.Addr]int{}}
}

// Start binds every listen address. A bind error is returned as is (a
// *net.OpError) so callers can report PROXY_PORT_BUSY.
func (s *Server) Start(ctx context.Context) error {
	var lns []net.Listener
	for _, a := range s.cfg.Listen {
		ln, err := net.Listen(listenNetwork(a), a.String())
		if err != nil {
			for _, l := range lns {
				l.Close()
			}
			return err
		}
		lns = append(lns, ln)
	}
	s.mu.Lock()
	s.lns, s.running = lns, true
	s.ctx, s.cancel = context.WithCancel(context.WithoutCancel(ctx))
	s.mu.Unlock()
	for _, ln := range lns {
		s.wg.Add(1)
		go s.accept(ln)
	}
	return nil
}

// listenNetwork picks tcp4/tcp6 by address family: on Windows a wildcard
// address on "tcp" opens a dual-stack socket, so 0.0.0.0:p and [::]:p
// would collide.
func listenNetwork(a netip.AddrPort) string {
	if a.Addr().Unmap().Is4() {
		return "tcp4"
	}
	return "tcp6"
}

// Addrs returns the bound addresses.
func (s *Server) Addrs() []netip.AddrPort {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []netip.AddrPort
	for _, ln := range s.lns {
		out = append(out, ln.Addr().(*net.TCPAddr).AddrPort())
	}
	return out
}

// Alive reports whether the listeners are up.
func (s *Server) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Stop closes the listeners, lets connections finish for up to the drain
// time, then closes the rest.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	for _, ln := range s.lns {
		ln.Close()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(s.lim.Drain):
	case <-ctx.Done():
	}
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.cancel()
	<-done
	return nil
}

func (s *Server) accept(ln net.Listener) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("proxy: accept loop panicked", "panic", fmt.Sprint(r))
		}
		// A listener that ends without Stop leaves the proxy dead: report it
		// so the health check restarts the proxy phase.
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go s.serve(c)
	}
}

// track registers c, enforcing source and connection limits.
func (s *Server) track(c net.Conn, ip netip.Addr) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || len(s.conns) >= s.lim.Total || s.perIP[ip] >= s.lim.PerIP {
		return false
	}
	s.conns[c] = struct{}{}
	s.perIP[ip]++
	s.stats.open(ip)
	return true
}

func (s *Server) trackConn(c net.Conn) {
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) untrackConn(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
}

func (s *Server) untrack(c net.Conn, ip netip.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c)
	if s.perIP[ip]--; s.perIP[ip] <= 0 {
		delete(s.perIP, ip)
	}
	s.stats.close(ip)
}

func (s *Server) serve(c net.Conn) {
	defer s.wg.Done()
	defer c.Close()
	ap, _ := netip.ParseAddrPort(c.RemoteAddr().String())
	ip := ap.Addr().Unmap()
	if !AllowedSource(ip, s.cfg.ShareLAN) || !s.track(c, ip) {
		return
	}
	defer s.untrack(c, ip)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("proxy: connection handler panicked", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()

	_ = c.SetDeadline(time.Now().Add(s.lim.Handshake))
	br := bufio.NewReaderSize(c, 16<<10)
	req, err := wire.ReadRequest(br, c)
	if err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	if req.Proto == wire.ProtoHTTPForward {
		s.forward(c, br, ip, req)
		return
	}
	s.tunnel(c, br, ip, req)
}

func (s *Server) event(client netip.Addr, t wire.Target, o dialer.Outcome, src rules.Source) {
	s.mu.Lock()
	s.stats.outcome(string(o))
	s.mu.Unlock()
	if s.cfg.OnConn != nil {
		s.cfg.OnConn(ConnEvent{Time: time.Now(), Client: client.String(), Target: t.String(), Outcome: string(o), Source: src})
	}
}

func replyFor(err error) wire.Reply {
	switch {
	case errors.Is(err, dialer.ErrBlocked), errors.Is(err, dialer.ErrForbidden):
		return wire.ReplyBlocked
	}
	return wire.ReplyUnreachable
}

func (s *Server) tunnel(c net.Conn, br *bufio.Reader, ip netip.Addr, req wire.Request) {
	if err := s.cfg.Dialer.Decide(req.Target); err != nil {
		_ = wire.WriteReply(c, req.Proto, replyFor(err))
		s.event(ip, req.Target, dialer.OutcomeBlocked, rules.Source{})
		return
	}
	if err := wire.WriteReply(c, req.Proto, wire.ReplyOK); err != nil {
		return
	}
	// The client speaks first for TLS; for server-first protocols the hello
	// is empty after the timeout.
	_ = c.SetReadDeadline(time.Now().Add(s.lim.Hello))
	hello, err := dialer.ReadHello(br)
	if err != nil && !isTimeout(err) {
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	if s.fakeSNI(c, br, ip, req.Target, hello) {
		return
	}
	res, err := s.cfg.Dialer.Open(s.ctx, ip, req.Target, hello)
	s.event(ip, req.Target, res.Outcome, res.Source)
	if err != nil {
		return
	}
	defer res.Conn.Close()
	s.trackConn(res.Conn)
	defer s.untrackConn(res.Conn)
	if len(res.FirstServerBytes) > 0 {
		if _, err := c.Write(res.FirstServerBytes); err != nil {
			return
		}
		s.addBytes(0, uint64(len(res.FirstServerBytes)))
	}
	s.addBytes(uint64(len(hello)), 0)
	s.relay(c, br, res.Conn)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// forward serves HTTP forward requests on one keep-alive client connection,
// reconnecting whenever the target host changes.
func (s *Server) forward(c net.Conn, br *bufio.Reader, ip netip.Addr, req wire.Request) {
	var up net.Conn
	var upBR *bufio.Reader
	var cur wire.Target
	closeUp := func() {
		if up != nil {
			s.untrackConn(up)
			up.Close()
			up = nil
		}
	}
	defer closeUp()
	r, t := req.HTTP, req.Target
	for {
		if up == nil || t != cur {
			closeUp()
			if err := s.cfg.Dialer.Decide(t); err != nil {
				_ = wire.WriteReply(c, wire.ProtoHTTPForward, replyFor(err))
				s.event(ip, t, dialer.OutcomeBlocked, rules.Source{})
				return
			}
			res, err := s.cfg.Dialer.Open(s.ctx, ip, t, nil)
			s.event(ip, t, res.Outcome, res.Source)
			if err != nil {
				_ = wire.WriteReply(c, wire.ProtoHTTPForward, replyFor(err))
				return
			}
			up, upBR, cur = res.Conn, bufio.NewReader(res.Conn), t
			s.trackConn(up) // Stop must be able to close it
		}
		cw := &countWriter{w: up}
		if err := r.Write(cw); err != nil {
			return
		}
		cc := &countWriter{w: c}
		resp, err := http.ReadResponse(upBR, r)
		// Pass interim 1xx responses (100 Continue…) through and wait for
		// the final one; 101 is final for HTTP/1.1 upgrades.
		for err == nil && resp.StatusCode >= 100 && resp.StatusCode < 200 && resp.StatusCode != http.StatusSwitchingProtocols {
			if _, werr := fmt.Fprintf(cc, "HTTP/%d.%d %s\r\n\r\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status); werr != nil {
				return
			}
			resp, err = http.ReadResponse(upBR, r)
		}
		if err != nil {
			_ = wire.WriteReply(c, wire.ProtoHTTPForward, wire.ReplyUnreachable)
			return
		}
		err = resp.Write(cc)
		resp.Body.Close()
		s.addBytes(cw.n, cc.n)
		if err != nil || resp.Close || r.Close {
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(s.lim.Idle))
		r, t, err = wire.ReadForward(br)
		if err != nil {
			return
		}
		_ = c.SetReadDeadline(time.Time{})
	}
}

type countWriter struct {
	w io.Writer
	n uint64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += uint64(n)
	return n, err
}

// SelfTest CONNECTs through the proxy to a temporary loopback echo server.
func (s *Server) SelfTest(ctx context.Context) error {
	addrs := s.Addrs()
	if len(addrs) == 0 {
		return errors.New("proxy: not running")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()
	pa := addrs[0]
	if pa.Addr().IsUnspecified() {
		pa = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), pa.Port())
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", pa.String())
	if err != nil {
		return err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	echo := ln.Addr().(*net.TCPAddr).AddrPort()
	a := echo.Addr().As4()
	msg := append([]byte{5, 1, 0, 5, 1, 0, 1}, a[:]...)
	msg = append(msg, byte(echo.Port()>>8), byte(echo.Port()))
	if _, err := c.Write(append(msg, "ghostline-selftest"...)); err != nil {
		return err
	}
	reply := make([]byte, 12+len("ghostline-selftest"))
	if _, err := io.ReadFull(c, reply); err != nil {
		return fmt.Errorf("proxy: self test: %w", err)
	}
	if reply[1] != 0 || reply[3] != 0 || string(reply[12:]) != "ghostline-selftest" {
		return errors.New("proxy: self test: unexpected reply")
	}
	return nil
}
