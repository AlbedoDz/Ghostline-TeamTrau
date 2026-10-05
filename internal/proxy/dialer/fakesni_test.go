package dialer_test

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

func helloFor(t *testing.T, host string) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer c1.Close()
	go func() { _ = tls.Client(c1, &tls.Config{ServerName: host, InsecureSkipVerify: true}).Handshake() }()
	buf := make([]byte, 64*1024)
	n, err := c2.Read(buf)
	require.NoError(t, err)
	c2.Close()
	return buf[:n]
}

func withECH(h []byte) []byte {
	out := append(append([]byte(nil), h...), 0xfe, 0x0d, 0, 0)
	binary.BigEndian.PutUint16(out[3:5], uint16(len(out)-5))
	hl := len(out) - 9
	out[6], out[7], out[8] = byte(hl>>16), byte(hl>>8), byte(hl)
	p := 9 + 2 + 32
	p += 1 + int(out[p])
	p += 2 + int(binary.BigEndian.Uint16(out[p:]))
	p += 1 + int(out[p])
	binary.BigEndian.PutUint16(out[p:], binary.BigEndian.Uint16(out[p:])+4)
	return out
}

func listen(t *testing.T) (netip.AddrPort, <-chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	ch := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			ch <- c
		}
	}()
	return netip.MustParseAddrPort(ln.Addr().String()), ch
}

func TestPlan_MatchesSNIRule(t *testing.T) {
	e := newEnv()
	e.rules = "youtube.com sni=www.google.com connect=front.test\nplain.test fragment=on"
	d := e.dialer(t)
	dec, host, ok := d.Plan(wire.Target{Host: "youtube.com", Port: 443}, helloFor(t, "m.youtube.com"))
	require.True(t, ok)
	require.Equal(t, "m.youtube.com", host)
	require.Equal(t, "www.google.com", dec.SNI)
	_, _, ok = d.Plan(wire.Target{Host: "plain.test", Port: 443}, helloFor(t, "plain.test"))
	require.False(t, ok)
}

func TestPlan_SkipsIP(t *testing.T) {
	e := newEnv()
	e.rules = "youtube.com sni=www.google.com"
	d := e.dialer(t)
	_, _, ok := d.Plan(wire.Target{IP: loopback, Port: 443}, helloFor(t, "127.0.0.1"))
	require.False(t, ok)
	_, _, ok = d.Plan(wire.Target{Host: "youtube.com", Port: 443}, nil)
	require.False(t, ok)
}

func TestOpenRaw_ConnectResolvesOtherHost(t *testing.T) {
	ap, accepted := listen(t)
	e := newEnv()
	e.res.m["front.test"] = []netip.Addr{ap.Addr()}
	e.rules = "youtube.com sni=www.google.com connect=front.test"
	d := e.dialer(t)
	dec, _, ok := d.Plan(wire.Target{Host: "youtube.com", Port: ap.Port()}, helloFor(t, "youtube.com"))
	require.True(t, ok)
	c, err := d.OpenRaw(context.Background(), loopback, wire.Target{Host: "youtube.com", Port: ap.Port()}, dec)
	require.NoError(t, err)
	defer c.Close()
	(<-accepted).Close()
	require.Equal(t, []string{"front.test"}, e.res.calls)
}

func TestOpenRaw_ResolvesRealHostWithoutConnect(t *testing.T) {
	ap, accepted := listen(t)
	e := newEnv()
	e.res.m["youtube.com"] = []netip.Addr{ap.Addr()}
	e.rules = "youtube.com sni=www.google.com"
	d := e.dialer(t)
	dec := rules.Decision{Action: rules.Action{SNI: "www.google.com"}}
	c, err := d.OpenRaw(context.Background(), loopback, wire.Target{Host: "youtube.com", Port: ap.Port()}, dec)
	require.NoError(t, err)
	c.Close()
	(<-accepted).Close()
	require.Equal(t, []string{"youtube.com"}, e.res.calls)
}

func TestOpenRaw_NoSystemResolver(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Error("system DNS used")
		return nil, errors.New("system DNS forbidden")
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
	ap, accepted := listen(t)
	e := newEnv()
	e.res.m["front.test"] = []netip.Addr{ap.Addr()}
	d := e.dialer(t)
	c, err := d.OpenRaw(context.Background(), loopback, wire.Target{Host: "youtube.com", Port: ap.Port()},
		rules.Decision{Action: rules.Action{SNI: "x.test", Connect: "front.test"}})
	require.NoError(t, err)
	c.Close()
	(<-accepted).Close()
}

func TestOpenRaw_SSRFStillApplies(t *testing.T) {
	e := newEnv()
	e.res.m["front.test"] = []netip.Addr{netip.MustParseAddr("192.168.1.1")}
	e.self = []netip.Addr{netip.MustParseAddr("192.168.1.1")}
	d := e.dialer(t)
	_, err := d.OpenRaw(context.Background(), lanClient, wire.Target{Host: "youtube.com", Port: 443},
		rules.Decision{Action: rules.Action{SNI: "x.test", Connect: "front.test"}})
	require.Error(t, err)
}

// Chrome and Edge send a GREASE ECH extension in every ClientHello; with
// real ECH the outer SNI is the CDN's public name, which no sni= rule
// matches. Either way the extension alone must not disable Fake SNI.
func TestPlan_GreaseECHStillIntercepted(t *testing.T) {
	e := newEnv()
	e.rules = "youtube.com sni=www.google.com"
	_, host, ok := e.dialer(t).Plan(wire.Target{Host: "youtube.com", Port: 443}, withECH(helloFor(t, "youtube.com")))
	require.True(t, ok)
	require.Equal(t, "youtube.com", host)
}
