// Package fragdoh is a DNS-over-HTTPS upstream that splits the TLS
// ClientHello into several TCP segments, so DPI that inspects the SNI in a
// single packet cannot read it.
package fragdoh

import (
	"net"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/tlsfrag"
)

// fragConn writes its first write (the ClientHello) as several segments
// separated by delay; later writes pass through.
type fragConn struct {
	net.Conn
	chunks int
	delay  time.Duration
	once   sync.Once
}

func (c *fragConn) Write(p []byte) (int, error) {
	first := false
	c.once.Do(func() { first = true })
	if !first {
		return c.Conn.Write(p)
	}
	if tc, ok := c.Conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	segs := tlsfrag.Split(p, tlsfrag.MethodTCP, c.chunks)
	written := 0
	for i, s := range segs {
		if len(s) == 0 {
			continue
		}
		n, err := c.Conn.Write(s)
		written += n
		if err != nil {
			return written, err
		}
		if i < len(segs)-1 && c.delay > 0 {
			time.Sleep(c.delay)
		}
	}
	return written, nil
}
