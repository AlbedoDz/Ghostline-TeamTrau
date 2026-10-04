package proxy_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/proxy/dialer"
	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/stretchr/testify/require"
	xproxy "golang.org/x/net/proxy"
)

type mapResolver map[string][]netip.Addr

func (m mapResolver) Resolve(_ context.Context, h string) ([]netip.Addr, error) {
	if a, ok := m[h]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

type countingMatcher struct {
	c     *rules.Compiled
	mu    sync.Mutex
	hosts []string
}

func (m *countingMatcher) Match(h string, ip netip.Addr) rules.Decision {
	m.mu.Lock()
	m.hosts = append(m.hosts, h)
	m.mu.Unlock()
	return m.c.Match(h, ip)
}

func realDialer(t *testing.T, res mapResolver, ruleText string) (*dialer.Dialer, *countingMatcher) {
	rs, errs := rules.ParseText(ruleText, nil)
	require.Empty(t, errs)
	c, err := rules.Compile(rs, nil)
	require.NoError(t, err)
	m := &countingMatcher{c: c}
	return dialer.New(dialer.Config{
		Resolver: res,
		Matcher:  func() dialer.Matcher { return m },
		Frag: func() dialer.FragConfig {
			return dialer.FragConfig{Mode: "auto", Method: tlsfrag.MethodBoth, Chunks: 4, AutoTimeout: time.Second}
		},
		Upstreams: func(string) (dialer.Upstream, bool) { return dialer.Upstream{}, false },
		SelfAddrs: func() []netip.Addr { return nil },
	}), m
}

func startServer(t *testing.T, cfg proxy.Config) (*proxy.Server, string) {
	t.Helper()
	if cfg.Listen == nil {
		cfg.Listen = []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}
	}
	s := proxy.New(cfg)
	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s, s.Addrs()[0].String()
}

func port(t *testing.T, u string) uint16 {
	pu, err := url.Parse(u)
	require.NoError(t, err)
	var p uint16
	_, err = fmt.Sscan(pu.Port(), &p)
	require.NoError(t, err)
	return p
}

func TestEndToEnd_SOCKS5AndHTTP(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello "+r.Host)
	}))
	defer origin.Close()
	d, _ := realDialer(t, mapResolver{"test.local": {netip.MustParseAddr("127.0.0.1")}}, "")
	s, addr := startServer(t, proxy.Config{Dialer: d})
	target := fmt.Sprintf("test.local:%d", port(t, origin.URL))

	// SOCKS5 with a domain name.
	sd, err := xproxy.SOCKS5("tcp", addr, nil, xproxy.Direct)
	require.NoError(t, err)
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, a string) (net.Conn, error) { return sd.Dial(network, a) }}
	resp, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get("https://" + target + "/")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, "hello "+target, string(body))

	// HTTP CONNECT.
	pu, _ := url.Parse("http://" + addr)
	tr2 := &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	resp, err = (&http.Client{Transport: tr2, Timeout: 5 * time.Second}).Get("https://" + target + "/")
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, "hello "+target, string(body))

	st := s.Stats()
	require.GreaterOrEqual(t, st.ByOutcome["direct"], uint64(2))
	require.Positive(t, st.BytesIn)
	require.Positive(t, st.BytesOut)
}

func TestBlockedTargetGetsProtocolReply(t *testing.T) {
	d, _ := realDialer(t, mapResolver{}, "ads.test block\n")
	s, addr := startServer(t, proxy.Config{Dialer: d})
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c.Close()
	_, _ = c.Write(append(append([]byte{5, 1, 0, 5, 1, 0, 3, 8}, "ads.test"...), 1, 187))
	b := make([]byte, 4)
	_, err = io.ReadFull(c, b)
	require.NoError(t, err)
	require.Equal(t, []byte{5, 0, 5, 2}, b) // method OK, then "not allowed by ruleset"
	require.Eventually(t, func() bool { return s.Stats().ByOutcome["blocked"] == 1 }, time.Second, 10*time.Millisecond)
}

func TestAllowedSource(t *testing.T) {
	cases := []struct {
		ip        string
		lan, open bool
	}{
		{"127.0.0.1", false, true}, {"::1", false, true},
		{"192.168.1.5", false, false}, {"192.168.1.5", true, true},
		{"10.2.3.4", true, true}, {"172.16.0.9", true, true}, {"169.254.3.3", true, true},
		{"fd00::1", true, true}, {"fe80::1", true, true},
		{"8.8.8.8", true, false}, {"2001:4860::8888", true, false}, {"172.32.0.1", true, false},
	}
	for _, c := range cases {
		require.Equal(t, c.open, proxy.AllowedSource(netip.MustParseAddr(c.ip), c.lan), c.ip)
	}
}

func socks5Greet(t *testing.T, c net.Conn) {
	_, err := c.Write([]byte{5, 1, 0})
	require.NoError(t, err)
	b := make([]byte, 2)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(c, b)
	require.NoError(t, err)
	_ = c.SetReadDeadline(time.Time{})
}

func closedSoon(t *testing.T, c net.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err := c.Read(make([]byte, 1))
	require.Error(t, err)
	var ne net.Error
	if errors.As(err, &ne) {
		require.False(t, ne.Timeout(), "connection was not closed")
	}
}

func TestLimits_PerIPAndTotal(t *testing.T) {
	d, _ := realDialer(t, mapResolver{}, "")
	_, addr := startServer(t, proxy.Config{Dialer: d, Limits: proxy.Limits{PerIP: 2}})
	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", addr)
		require.NoError(t, err)
		defer c.Close()
		socks5Greet(t, c)
	}
	c3, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c3.Close()
	closedSoon(t, c3)

	_, addr2 := startServer(t, proxy.Config{Dialer: d, Limits: proxy.Limits{Total: 1}})
	c4, err := net.Dial("tcp", addr2)
	require.NoError(t, err)
	defer c4.Close()
	socks5Greet(t, c4)
	c5, err := net.Dial("tcp", addr2)
	require.NoError(t, err)
	defer c5.Close()
	closedSoon(t, c5)
}

func TestHandshakeTimeout(t *testing.T) {
	d, _ := realDialer(t, mapResolver{}, "")
	_, addr := startServer(t, proxy.Config{Dialer: d, Limits: proxy.Limits{Handshake: 100 * time.Millisecond}})
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c.Close()
	closedSoon(t, c)
}

func TestHTTPForward_KeepAliveSwitchesHost(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "A"+r.URL.Path) }))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "B"+r.URL.Path) }))
	defer b.Close()
	lo := netip.MustParseAddr("127.0.0.1")
	d, m := realDialer(t, mapResolver{"a.test": {lo}, "b.test": {lo}}, "")
	_, addr := startServer(t, proxy.Config{Dialer: d})
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c.Close()
	br := bufio.NewReader(c)
	for _, x := range []struct{ host, srv, want string }{{"a.test", a.URL, "A/one"}, {"b.test", b.URL, "B/two"}, {"a.test", a.URL, "A/three"}} {
		path := "/" + map[string]string{"A/one": "one", "B/two": "two", "A/three": "three"}[x.want]
		_, _ = fmt.Fprintf(c, "GET http://%s:%d%s HTTP/1.1\r\nHost: %s:%d\r\nProxy-Connection: keep-alive\r\n\r\n", x.host, port(t, x.srv), path, x.host, port(t, x.srv))
		resp, err := http.ReadResponse(br, nil)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, x.want, string(body))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	require.Contains(t, m.hosts, "a.test")
	require.Contains(t, m.hosts, "b.test")
}

// fakeOpener lets tests control the outbound side.
type fakeOpener struct {
	open func(ctx context.Context, client netip.Addr, t wire.Target, hello []byte) (dialer.Result, error)
}

func (f *fakeOpener) Decide(wire.Target) error { return nil }
func (f *fakeOpener) Open(ctx context.Context, client netip.Addr, t wire.Target, hello []byte) (dialer.Result, error) {
	return f.open(ctx, client, t, hello)
}

func socks5Connect(t *testing.T, addr string) net.Conn {
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	_, _ = c.Write([]byte{5, 1, 0, 5, 1, 0, 1, 10, 0, 0, 1, 0, 80})
	b := make([]byte, 12)
	_, err = io.ReadFull(c, b)
	require.NoError(t, err)
	require.Equal(t, byte(0), b[3])
	return c
}

func TestRelay_HalfCloseAndIdle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				got, _ := io.ReadAll(c) // until the client half-closes
				_, _ = c.Write([]byte("done:" + string(got)))
			}()
		}
	}()
	op := &fakeOpener{open: func(ctx context.Context, _ netip.Addr, _ wire.Target, hello []byte) (dialer.Result, error) {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return dialer.Result{}, err
		}
		_, _ = c.Write(hello)
		return dialer.Result{Conn: c, Outcome: dialer.OutcomeDirect}, nil
	}}
	_, addr := startServer(t, proxy.Config{Dialer: op, Limits: proxy.Limits{Hello: 50 * time.Millisecond, Idle: 300 * time.Millisecond}})
	c := socks5Connect(t, addr)
	defer c.Close()
	_, _ = c.Write([]byte("abc"))
	require.NoError(t, c.(*net.TCPConn).CloseWrite())
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	got, err := io.ReadAll(c)
	require.NoError(t, err)
	require.Equal(t, "done:abc", string(got))

	// Idle: nobody sends anything → closed after the idle timeout.
	c2 := socks5Connect(t, addr)
	defer c2.Close()
	start := time.Now()
	closedSoon(t, c2)
	require.Less(t, time.Since(start), 2*time.Second)
}

func TestStop_DrainsThenCloses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, c) }()
		}
	}()
	op := &fakeOpener{open: func(ctx context.Context, _ netip.Addr, _ wire.Target, _ []byte) (dialer.Result, error) {
		c, err := net.Dial("tcp", ln.Addr().String())
		return dialer.Result{Conn: c, Outcome: dialer.OutcomeDirect}, err
	}}
	s := proxy.New(proxy.Config{Dialer: op, Listen: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")},
		Limits: proxy.Limits{Hello: 50 * time.Millisecond, Drain: 200 * time.Millisecond}})
	require.NoError(t, s.Start(context.Background()))
	addr := s.Addrs()[0].String()
	c := socks5Connect(t, addr)
	defer c.Close()
	require.True(t, s.Alive())
	start := time.Now()
	require.NoError(t, s.Stop(context.Background()))
	require.Less(t, time.Since(start), 2*time.Second)
	require.False(t, s.Alive())
	closedSoon(t, c)
	_, err = net.DialTimeout("tcp", addr, time.Second)
	require.Error(t, err)
}

func TestSelfTest(t *testing.T) {
	d, _ := realDialer(t, mapResolver{}, "")
	s, _ := startServer(t, proxy.Config{Dialer: d})
	require.NoError(t, s.SelfTest(context.Background()))
}

func TestStart_PortBusyReturnsOpError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	d, _ := realDialer(t, mapResolver{}, "")
	s := proxy.New(proxy.Config{Dialer: d, Listen: []netip.AddrPort{ln.Addr().(*net.TCPAddr).AddrPort()}})
	err = s.Start(context.Background())
	var oe *net.OpError
	require.ErrorAs(t, err, &oe)
}

func TestPanicInConnIsContained(t *testing.T) {
	var calls atomic.Int32
	op := &fakeOpener{open: func(context.Context, netip.Addr, wire.Target, []byte) (dialer.Result, error) {
		calls.Add(1)
		panic("boom")
	}}
	_, addr := startServer(t, proxy.Config{Dialer: op, Limits: proxy.Limits{Hello: 50 * time.Millisecond}})
	c := socks5Connect(t, addr)
	closedSoon(t, c)
	c.Close()
	c2, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c2.Close()
	socks5Greet(t, c2)
	require.EqualValues(t, 1, calls.Load())
}

func TestOnConnEvent(t *testing.T) {
	d, _ := realDialer(t, mapResolver{}, "ads.test block\n")
	var mu sync.Mutex
	var evs []proxy.ConnEvent
	_, addr := startServer(t, proxy.Config{Dialer: d, OnConn: func(e proxy.ConnEvent) { mu.Lock(); evs = append(evs, e); mu.Unlock() }})
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	_, _ = c.Write(append(append([]byte{5, 1, 0, 5, 1, 0, 3, 8}, "ads.test"...), 1, 187))
	_, err = io.ReadFull(c, make([]byte, 12)) // method choice + "blocked" reply
	require.NoError(t, err)
	closedSoon(t, c)
	c.Close()
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(evs) == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, "ads.test:443", evs[0].Target)
	require.Equal(t, "blocked", evs[0].Outcome)
}
