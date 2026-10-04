package dialer

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
)

const upstreamHandshake = 10 * time.Second

// viaUpstream sends t through the upstream named by dec. The target name is
// passed to the upstream unresolved. Fragmenting is off unless the rule asks.
func (d *Dialer) viaUpstream(ctx context.Context, dec rules.Decision, t wire.Target, hello []byte) (Result, error) {
	up, ok := d.c.Upstreams(dec.Upstream)
	if !ok {
		return Result{Outcome: OutcomeFailed, Source: dec.Source}, fmt.Errorf("%w: unknown upstream %q", ErrUnreachable, dec.Upstream)
	}
	dial := func(ctx context.Context) (net.Conn, error) { return d.dialUpstream(ctx, up, t) }
	mode := "never"
	if dec.Fragment == rules.FragOn || dec.Fragment == rules.FragAuto {
		mode = d.fragMode(dec.Fragment)
	}
	r, err := d.writeHello(ctx, dial, mode, cacheHost(t, hello), hello)
	r.Source = dec.Source
	if err == nil {
		r.Outcome = OutcomeUpstream
	}
	return r, err
}

func (d *Dialer) dialUpstream(ctx context.Context, up Upstream, t wire.Target) (net.Conn, error) {
	host, port, err := net.SplitHostPort(up.Addr)
	if err != nil {
		return nil, fmt.Errorf("%w: upstream %q address: %v", ErrUnreachable, up.ID, err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("%w: upstream %q port", ErrUnreachable, up.ID)
	}
	var addrs []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{ip}
	} else if addrs, err = d.c.Resolver.Resolve(ctx, host); err != nil || len(addrs) == 0 {
		return nil, errors.Join(ErrUnreachable, err)
	}
	c, err := d.dialAny(ctx, addrs, uint16(p))
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(upstreamHandshake))
	switch up.Type {
	case "socks5":
		err = socks5Connect(c, up, t)
	case "http":
		c, err = httpConnect(c, up, t)
	default:
		err = fmt.Errorf("unknown upstream type %q", up.Type)
	}
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("%w: upstream %q: %v", ErrUnreachable, up.ID, err)
	}
	_ = c.SetDeadline(time.Time{})
	return c, nil
}

func socks5Connect(c net.Conn, up Upstream, t wire.Target) error {
	methods := []byte{5, 1, 0}
	if up.User != "" {
		methods = []byte{5, 2, 0, 2}
	}
	if _, err := c.Write(methods); err != nil {
		return err
	}
	var g [2]byte
	if _, err := io.ReadFull(c, g[:]); err != nil {
		return err
	}
	switch g[1] {
	case 0:
	case 2:
		if up.User == "" || len(up.User) > 255 || len(up.Pass) > 255 {
			return errors.New("socks5 upstream wants a user name")
		}
		auth := append([]byte{1, byte(len(up.User))}, up.User...)
		auth = append(append(auth, byte(len(up.Pass))), up.Pass...)
		if _, err := c.Write(auth); err != nil {
			return err
		}
		var a [2]byte
		if _, err := io.ReadFull(c, a[:]); err != nil {
			return err
		}
		if a[1] != 0 {
			return errors.New("socks5 upstream rejected the credentials")
		}
	default:
		return errors.New("socks5 upstream offers no usable auth method")
	}
	req := []byte{5, 1, 0}
	switch {
	case t.Host != "":
		if len(t.Host) > 255 {
			return errors.New("host name too long")
		}
		req = append(append(req, 3, byte(len(t.Host))), t.Host...)
	case t.IP.Is4():
		a := t.IP.As4()
		req = append(append(req, 1), a[:]...)
	default:
		a := t.IP.As16()
		req = append(append(req, 4), a[:]...)
	}
	req = append(req, byte(t.Port>>8), byte(t.Port))
	if _, err := c.Write(req); err != nil {
		return err
	}
	var h [4]byte
	if _, err := io.ReadFull(c, h[:]); err != nil {
		return err
	}
	if h[1] != 0 {
		return fmt.Errorf("socks5 upstream refused CONNECT (code %d)", h[1])
	}
	skip := 0
	switch h[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(c, n[:]); err != nil {
			return err
		}
		skip = int(n[0])
	default:
		return errors.New("socks5 upstream sent a bad address type")
	}
	_, err := io.ReadFull(c, make([]byte, skip+2))
	return err
}

// bufConn returns bytes a bufio.Reader already buffered before the stream.
type bufConn struct {
	net.Conn
	br *bufio.Reader
}

func (c *bufConn) Read(p []byte) (int, error) { return c.br.Read(p) }

// CloseWrite keeps half-close working through the wrapper.
func (c *bufConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

func httpConnect(c net.Conn, up Upstream, t wire.Target) (net.Conn, error) {
	hp := t.String()
	req := "CONNECT " + hp + " HTTP/1.1\r\nHost: " + hp + "\r\n"
	if up.User != "" {
		req += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(up.User+":"+up.Pass)) + "\r\n"
	}
	if _, err := io.WriteString(c, req+"\r\n"); err != nil {
		return c, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return c, err
	}
	if resp.StatusCode != http.StatusOK {
		return c, fmt.Errorf("http upstream answered %s", resp.Status)
	}
	if br.Buffered() > 0 {
		return &bufConn{Conn: c, br: br}, nil
	}
	return c, nil
}
