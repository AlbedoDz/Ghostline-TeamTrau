package dialer_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/dialer"
	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/stretchr/testify/require"
)

type fakeResolver struct {
	mu    sync.Mutex
	m     map[string][]netip.Addr
	calls []string
}

func (f *fakeResolver) Resolve(_ context.Context, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, host)
	if a, ok := f.m[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

type fakeCache struct {
	mu   sync.Mutex
	m    map[string]bool
	adds []string
}

func (c *fakeCache) Has(h string) bool { c.mu.Lock(); defer c.mu.Unlock(); return c.m[h] }
func (c *fakeCache) Add(h string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]bool{}
	}
	c.m[h] = true
	c.adds = append(c.adds, h)
}

var loopback = netip.MustParseAddr("127.0.0.1")
var lanClient = netip.MustParseAddr("192.168.1.9")

type env struct {
	res   *fakeResolver
	cache *fakeCache
	frag  dialer.FragConfig
	rules string
	ups   map[string]dialer.Upstream
	dial  func(ctx context.Context, ap netip.AddrPort) (net.Conn, error)
	self  []netip.Addr
	forb  []netip.AddrPort
}

func newEnv() *env {
	return &env{
		res:   &fakeResolver{m: map[string][]netip.Addr{"blocked.test": {loopback}, "ok.test": {loopback}}},
		cache: &fakeCache{},
		frag:  dialer.FragConfig{Mode: "auto", Method: tlsfrag.MethodTCP, Chunks: 4, Delay: 20 * time.Millisecond, AutoTimeout: 500 * time.Millisecond},
	}
}

func (e *env) dialer(t *testing.T) *dialer.Dialer {
	rs, errs := rules.ParseText(e.rules, []string{"up5", "uph"})
	require.Empty(t, errs)
	c, err := rules.Compile(rs, nil)
	require.NoError(t, err)
	return dialer.New(dialer.Config{
		Resolver:  e.res,
		Matcher:   func() dialer.Matcher { return c },
		Cache:     e.cache,
		Frag:      func() dialer.FragConfig { return e.frag },
		Upstreams: func(id string) (dialer.Upstream, bool) { u, ok := e.ups[id]; return u, ok },
		SelfAddrs: func() []netip.Addr { return e.self },
		Forbidden: e.forb,
		Dial:      e.dial,
	})
}

// openTLS runs a TLS client for host through d to sim and reports the result.
func openTLS(t *testing.T, d *dialer.Dialer, host string, sim *dpiSim) (dialer.Result, <-chan error, error) {
	t.Helper()
	side, br, done := clientHandshake(t, host)
	_ = side.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := dialer.ReadHello(br)
	require.NoError(t, err)
	_ = side.SetReadDeadline(time.Time{})
	res, err := d.Open(context.Background(), loopback, wire.Target{Host: host, Port: sim.addr().Port()}, hello)
	if err == nil {
		relay(side, br, res.Conn, res.FirstServerBytes)
	} else {
		side.Close()
	}
	return res, done, err
}

func waitOK(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("client handshake timed out")
	}
}

func waitFail(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("client handshake neither failed nor finished")
	}
}

func TestOpen_AutoRetriesWithFragment(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	e := newEnv()
	res, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeFragmented, res.Outcome)
	waitOK(t, done)
	require.Equal(t, []string{"blocked.test"}, e.cache.adds)
	require.EqualValues(t, 2, sim.accepts.Load())
}

func TestOpen_AutoDirectWhenNotBlocked(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	e := newEnv()
	res, done, err := openTLS(t, e.dialer(t), "ok.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeDirect, res.Outcome)
	waitOK(t, done)
	require.Empty(t, e.cache.adds)
}

func TestOpen_AutoUsesCache(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	e := newEnv()
	e.cache.Add("blocked.test")
	e.cache.adds = nil
	res, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeFragmented, res.Outcome)
	waitOK(t, done)
	require.EqualValues(t, 1, sim.accepts.Load())
}

func TestOpen_SilentDPITimesOut(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	sim.silent.Store(true)
	e := newEnv()
	e.frag.AutoTimeout = 200 * time.Millisecond
	res, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeFragmented, res.Outcome)
	waitOK(t, done)
}

func TestOpen_FragmentAlsoFails(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	sim.blockAll.Store(true)
	e := newEnv()
	res, _, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.ErrorIs(t, err, dialer.ErrBlockedEvenFragmented)
	require.Equal(t, dialer.OutcomeBlockedEvenFragmented, res.Outcome)
	require.Empty(t, e.cache.adds)
}

func TestOpen_RecordAndBothMethods(t *testing.T) {
	for _, m := range []tlsfrag.Method{tlsfrag.MethodRecord, tlsfrag.MethodBoth} {
		sim := newDPISim(t, "blocked.test")
		e := newEnv()
		e.frag.Method = m
		e.frag.Mode = "always"
		res, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
		require.NoError(t, err, m)
		require.Equal(t, dialer.OutcomeFragmented, res.Outcome, m)
		waitOK(t, done)
	}
}

func TestOpen_ModesNeverAlways(t *testing.T) {
	sim := newDPISim(t, "blocked.test")
	e := newEnv()
	e.frag.Mode = "never"
	res, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err) // never mode does not wait for the server
	require.Equal(t, dialer.OutcomeDirect, res.Outcome)
	waitFail(t, done)

	e.frag.Mode = "always"
	res, done, err = openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeFragmented, res.Outcome)
	waitOK(t, done)

	// A rule's fragment=off beats the global "always".
	e.rules = "blocked.test fragment=off\n"
	res, done, err = openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeDirect, res.Outcome)
	waitFail(t, done)
	// And fragment=on beats "never".
	e.frag.Mode = "never"
	e.rules = "blocked.test fragment=on\n"
	res, done, err = openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeFragmented, res.Outcome)
	waitOK(t, done)
}

func TestOpen_RuleBlockAndRewrite(t *testing.T) {
	sim := newDPISim(t, "never-blocked")
	e := newEnv()
	e.rules = "ads.test block\nfake.test ip=127.0.0.1\n"
	d := e.dialer(t)
	_, err := d.Open(context.Background(), loopback, wire.Target{Host: "ads.test", Port: 443}, nil)
	require.ErrorIs(t, err, dialer.ErrBlocked)
	require.ErrorIs(t, d.Decide(wire.Target{Host: "x.ads.test", Port: 443}), dialer.ErrBlocked)
	require.NoError(t, d.Decide(wire.Target{Host: "fine.test", Port: 443}))

	res, done, err := openTLS(t, d, "fake.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeDirect, res.Outcome)
	waitOK(t, done)
	require.Empty(t, e.res.calls)
}

func TestOpen_CIDRBlock(t *testing.T) {
	e := newEnv()
	e.rules = "127.0.0.0/8 block\n"
	_, err := e.dialer(t).Open(context.Background(), loopback, wire.Target{Host: "ok.test", Port: 443}, nil)
	require.ErrorIs(t, err, dialer.ErrBlocked)
}

// fakeSOCKS5 accepts one CONNECT (requiring user/pass when user != ""),
// records the requested host and connects it to target.
func fakeSOCKS5(t *testing.T, user, pass string, target netip.AddrPort) (string, *string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	var host string
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		g := make([]byte, 2)
		_, _ = io.ReadFull(br, g)
		m := make([]byte, g[1])
		_, _ = io.ReadFull(br, m)
		if user != "" {
			_, _ = c.Write([]byte{5, 2})
			h := make([]byte, 2)
			_, _ = io.ReadFull(br, h)
			u := make([]byte, h[1])
			_, _ = io.ReadFull(br, u)
			pl, _ := br.ReadByte()
			p := make([]byte, pl)
			_, _ = io.ReadFull(br, p)
			if string(u) != user || string(p) != pass {
				_, _ = c.Write([]byte{1, 1})
				return
			}
			_, _ = c.Write([]byte{1, 0})
		} else {
			_, _ = c.Write([]byte{5, 0})
		}
		h := make([]byte, 4)
		_, _ = io.ReadFull(br, h)
		n, _ := br.ReadByte()
		hb := make([]byte, n)
		_, _ = io.ReadFull(br, hb)
		_, _ = io.ReadFull(br, make([]byte, 2))
		host = string(hb)
		up, err := net.Dial("tcp", target.String())
		if err != nil {
			return
		}
		_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
		go func() { _, _ = io.Copy(up, br) }()
		_, _ = io.Copy(c, up)
	}()
	return ln.Addr().String(), &host
}

// fakeHTTPConnect accepts one CONNECT and records host and auth header.
func fakeHTTPConnect(t *testing.T, target netip.AddrPort) (string, *string, *string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	var host, auth string
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		host, auth = req.Host, req.Header.Get("Proxy-Authorization")
		up, err := net.Dial("tcp", target.String())
		if err != nil {
			return
		}
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		go func() { _, _ = io.Copy(up, br) }()
		_, _ = io.Copy(c, up)
	}()
	return ln.Addr().String(), &host, &auth
}

func TestOpen_UpstreamSOCKS5AndHTTP(t *testing.T) {
	sim := newDPISim(t, "never-blocked")
	e := newEnv()
	addr5, host5 := fakeSOCKS5(t, "alice", "s3cret", sim.addr())
	addrH, hostH, authH := fakeHTTPConnect(t, sim.addr())
	e.ups = map[string]dialer.Upstream{
		"up5": {ID: "up5", Type: "socks5", Addr: addr5, User: "alice", Pass: "s3cret"},
		"uph": {ID: "uph", Type: "http", Addr: addrH, User: "bob", Pass: "pw"},
	}
	e.rules = "via5.test upstream=up5\nviah.test upstream=uph\n"
	d := e.dialer(t)

	res, done, err := openTLS(t, d, "via5.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeUpstream, res.Outcome)
	waitOK(t, done)
	require.Equal(t, "via5.test", *host5)

	res, done, err = openTLS(t, d, "viah.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeUpstream, res.Outcome)
	waitOK(t, done)
	require.True(t, strings.HasPrefix(*hostH, "viah.test:"))
	require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("bob:pw")), *authH)
	require.Empty(t, e.res.calls) // the target host is never resolved locally
}

func TestOpen_UnknownUpstream(t *testing.T) {
	e := newEnv()
	e.rules = "x.test upstream=up5\n"
	_, err := e.dialer(t).Open(context.Background(), loopback, wire.Target{Host: "x.test", Port: 443}, nil)
	require.ErrorIs(t, err, dialer.ErrUnreachable)
}

func TestOpen_SSRF(t *testing.T) {
	e := newEnv()
	e.self = []netip.Addr{netip.MustParseAddr("192.168.1.5")}
	e.forb = []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8080"), netip.MustParseAddrPort("127.0.0.1:53")}
	dialed := 0
	e.dial = func(ctx context.Context, ap netip.AddrPort) (net.Conn, error) {
		dialed++
		return nil, errors.New("would dial")
	}
	d := e.dialer(t)
	ctx := context.Background()
	for _, tgt := range []string{"127.0.0.1:80", "169.254.1.1:80", "192.168.1.5:80", "0.0.0.0:80", "[::1]:80", "[fe80::1]:80"} {
		ap := netip.MustParseAddrPort(tgt)
		_, err := d.Open(ctx, lanClient, wire.Target{IP: ap.Addr(), Port: ap.Port()}, nil)
		require.ErrorIs(t, err, dialer.ErrForbidden, tgt)
	}
	// Hostname that resolves to loopback is also refused for LAN clients.
	_, err := d.Open(ctx, lanClient, wire.Target{Host: "ok.test", Port: 80}, nil)
	require.ErrorIs(t, err, dialer.ErrForbidden)
	require.Zero(t, dialed)

	// Loopback clients may reach loopback services…
	_, err = d.Open(ctx, loopback, wire.Target{IP: loopback, Port: 80}, nil)
	require.NotErrorIs(t, err, dialer.ErrForbidden)
	// …but never the proxy or the engine itself.
	for _, tgt := range []string{"127.0.0.1:8080", "127.0.0.1:53", "192.168.1.5:8080", "0.0.0.0:53"} {
		ap := netip.MustParseAddrPort(tgt)
		_, err := d.Open(ctx, loopback, wire.Target{IP: ap.Addr(), Port: ap.Port()}, nil)
		require.ErrorIs(t, err, dialer.ErrForbidden, tgt)
	}
}

func TestOpen_IPv6AfterV4Fails(t *testing.T) {
	sim := newDPISim(t, "never-blocked")
	e := newEnv()
	e.res.m["dual.test"] = []netip.Addr{netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("2001:db8::1")}
	var tried []string
	e.dial = func(ctx context.Context, ap netip.AddrPort) (net.Conn, error) {
		tried = append(tried, ap.Addr().String())
		if ap.Addr().Is4() {
			return nil, errors.New("v4 unreachable")
		}
		return net.Dial("tcp", sim.addr().String())
	}
	res, done, err := openTLS(t, e.dialer(t), "dual.test", sim)
	require.NoError(t, err)
	require.Equal(t, dialer.OutcomeDirect, res.Outcome)
	waitOK(t, done)
	require.Equal(t, []string{"203.0.113.1", "2001:db8::1"}, tried)
}

func TestOpen_ResolveFailure(t *testing.T) {
	e := newEnv()
	_, err := e.dialer(t).Open(context.Background(), loopback, wire.Target{Host: "nowhere.test", Port: 443}, nil)
	require.ErrorIs(t, err, dialer.ErrUnreachable)
}

func TestOpen_NeverSystemDNS(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Error("system DNS used")
		return nil, errors.New("system DNS forbidden")
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
	sim := newDPISim(t, "blocked.test")
	e := newEnv()
	_, done, err := openTLS(t, e.dialer(t), "blocked.test", sim)
	require.NoError(t, err)
	waitOK(t, done)
	require.Equal(t, []string{"blocked.test"}, e.res.calls)
}

func TestReadHello_AssemblesRecordAcrossReads(t *testing.T) {
	// A 1900-byte ClientHello record written in 4 pieces.
	body := make([]byte, 1895)
	body[0] = 0x01
	rec := append([]byte{0x16, 3, 1, byte(len(body) >> 8), byte(len(body))}, body...)
	a, b := net.Pipe()
	defer a.Close()
	go func() {
		for i := 0; i < 4; i++ {
			lo, hi := i*len(rec)/4, (i+1)*len(rec)/4
			_, _ = a.Write(rec[lo:hi])
			time.Sleep(20 * time.Millisecond)
		}
	}()
	got, err := dialer.ReadHello(bufio.NewReaderSize(b, 16<<10))
	require.NoError(t, err)
	require.Equal(t, rec, got)
}

func TestReadHello_NonTLSReturnsBuffered(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	go func() { _, _ = a.Write([]byte("SSH-2.0-x\r\n")) }()
	got, err := dialer.ReadHello(bufio.NewReaderSize(b, 16<<10))
	require.NoError(t, err)
	require.Equal(t, "SSH-2.0-x\r\n", string(got))
}

func TestReadHello_OversizedRecordRefused(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	go func() { _, _ = a.Write([]byte{0x16, 3, 1, 0xFF, 0xFF}) }()
	_, err := dialer.ReadHello(bufio.NewReaderSize(b, 16<<10))
	require.Error(t, err)
}
