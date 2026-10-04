package dialer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashcott/ghostline/internal/tlsfrag"
)

// ReadHello reads the client's first bytes from br: exactly one TLS record
// when they start a handshake (assembled across reads, at most 16 KiB of
// payload), otherwise whatever is already buffered. The caller sets the read
// deadline (5 s).
func ReadHello(br *bufio.Reader) ([]byte, error) {
	b, err := br.Peek(1)
	if err != nil {
		return nil, err
	}
	if b[0] != 0x16 {
		out := make([]byte, br.Buffered())
		_, err := io.ReadFull(br, out)
		return out, err
	}
	hdr, err := br.Peek(5)
	if err != nil {
		return nil, err
	}
	n, _ := tlsfrag.RecordLen(hdr)
	if n > maxHelloRecord {
		return nil, fmt.Errorf("proxy: TLS record of %d bytes is too large", n)
	}
	out := make([]byte, n)
	_, err = io.ReadFull(br, out)
	return out, err
}

// writeFragmented writes hello split per cfg, pausing between segments.
func writeFragmented(c net.Conn, hello []byte, cfg FragConfig) error {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	segs := tlsfrag.Split(hello, cfg.Method, cfg.Chunks)
	for i, s := range segs {
		if len(s) == 0 {
			continue
		}
		if _, err := c.Write(s); err != nil {
			return err
		}
		if i < len(segs)-1 && cfg.Delay > 0 {
			time.Sleep(cfg.Delay)
		}
	}
	return nil
}

// firstBytes waits up to timeout for the server's first bytes.
func firstBytes(c net.Conn, timeout time.Duration) ([]byte, error) {
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	buf := make([]byte, 16<<10)
	n, err := c.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err == nil {
		err = io.ErrNoProgress
	}
	return nil, err
}

// writeHello opens a connection with dial and sends hello according to
// mode (auto|always|never), retrying once with fragmentation in auto mode.
func (d *Dialer) writeHello(ctx context.Context, dial func(context.Context) (net.Conn, error), mode, host string, hello []byte) (Result, error) {
	c, err := dial(ctx)
	if err != nil {
		return Result{Outcome: OutcomeFailed}, err
	}
	if len(hello) == 0 {
		return Result{Conn: c, Outcome: OutcomeDirect}, nil
	}
	cfg := d.c.Frag()
	if !tlsfrag.IsClientHello(hello) || mode == "never" {
		if _, err := c.Write(hello); err != nil {
			c.Close()
			return Result{Outcome: OutcomeFailed}, err
		}
		return Result{Conn: c, Outcome: OutcomeDirect}, nil
	}
	if mode == "always" || d.c.Cache != nil && d.c.Cache.Has(host) {
		if err := writeFragmented(c, hello, cfg); err != nil {
			c.Close()
			return Result{Outcome: OutcomeFailed}, err
		}
		return Result{Conn: c, Outcome: OutcomeFragmented}, nil
	}
	// auto: try the plain hello first.
	if _, err := c.Write(hello); err == nil {
		if first, err := firstBytes(c, cfg.AutoTimeout); err == nil {
			return Result{Conn: c, Outcome: OutcomeDirect, FirstServerBytes: first}, nil
		}
	}
	c.Close()
	c, err = dial(ctx)
	if err != nil {
		return Result{Outcome: OutcomeFailed}, err
	}
	if err := writeFragmented(c, hello, cfg); err != nil {
		c.Close()
		return Result{Outcome: OutcomeBlockedEvenFragmented}, errors.Join(ErrBlockedEvenFragmented, err)
	}
	first, err := firstBytes(c, cfg.AutoTimeout)
	if err != nil {
		c.Close()
		return Result{Outcome: OutcomeBlockedEvenFragmented}, errors.Join(ErrBlockedEvenFragmented, err)
	}
	if d.c.Cache != nil {
		d.c.Cache.Add(host)
	}
	return Result{Conn: c, Outcome: OutcomeFragmented, FirstServerBytes: first}, nil
}
