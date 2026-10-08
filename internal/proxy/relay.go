package proxy

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var relayBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32<<10)
		return &b
	},
}

func optimizeConn(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
}

// relay copies client↔server until both directions end. A direction that
// reaches EOF half-closes the other side. If neither direction moves data
// for the idle time, both are closed.
func (s *Server) relay(client net.Conn, br *bufio.Reader, server net.Conn) {
	optimizeConn(client)
	optimizeConn(server)

	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	var wg sync.WaitGroup
	wg.Add(2)
	copyDir := func(dst net.Conn, src io.Reader, srcConn net.Conn, up bool) {
		defer wg.Done()
		bufPtr := relayBufPool.Get().(*[]byte)
		defer relayBufPool.Put(bufPtr)
		buf := *bufPtr
		for {
			_ = srcConn.SetReadDeadline(time.Now().Add(s.lim.Idle))
			n, err := src.Read(buf)
			if n > 0 {
				last.Store(time.Now().UnixNano())
				if _, werr := dst.Write(buf[:n]); werr != nil {
					client.Close()
					server.Close()
					return
				}
				if up {
					s.addBytes(uint64(n), 0)
				} else {
					s.addBytes(0, uint64(n))
				}
			}
			if err == nil {
				continue
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if time.Since(time.Unix(0, last.Load())) < s.lim.Idle {
					continue // the other direction is active
				}
				client.Close()
				server.Close()
				return
			}
			if errors.Is(err, io.EOF) {
				if cw, ok := dst.(interface{ CloseWrite() error }); ok {
					_ = cw.CloseWrite()
					return
				}
			}
			client.Close()
			server.Close()
			return
		}
	}
	go copyDir(server, br, client, true)
	go copyDir(client, server, server, false)
	wg.Wait()
}
