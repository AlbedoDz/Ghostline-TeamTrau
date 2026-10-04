// Package wire parses proxy handshakes (SOCKS4/4a/5, HTTP CONNECT and HTTP
// forward) and writes their replies. It is shared by the proxy server and
// its dialer and imports neither.
package wire

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
)

// Proto is the client's proxy protocol.
type Proto uint8

const (
	ProtoSOCKS4 Proto = iota + 1
	ProtoSOCKS5
	ProtoHTTPConnect
	ProtoHTTPForward
)

// Target is where the client wants to go: exactly one of Host or IP is set.
type Target struct {
	Host string
	IP   netip.Addr
	Port uint16
}

func (t Target) String() string {
	h := t.Host
	if t.IP.IsValid() {
		h = t.IP.String()
	}
	return net.JoinHostPort(h, strconv.Itoa(int(t.Port)))
}

// Reply is a protocol-independent handshake result.
type Reply uint8

const (
	ReplyOK Reply = iota
	ReplyBlocked
	ReplyUnreachable
	ReplyBadCommand
	ReplyBadAddress
	ReplyFailure
	ReplyBadRequest       // HTTP 400
	ReplyMethodNotAllowed // HTTP 405
)

// ErrUnsupported means the request was refused; a reply was already sent.
var ErrUnsupported = errors.New("proxy: unsupported request")

// MaxHeader caps an HTTP request head and any SOCKS string.
const MaxHeader = 8 << 10

// ReadRequest reads one handshake from br. For SOCKS5 it answers the method
// negotiation itself; on refusal it writes the protocol's error reply to w and
// returns an error wrapping ErrUnsupported.
func ReadRequest(br *bufio.Reader, w io.Writer) (Request, error) {
	b, err := br.Peek(1)
	if err != nil {
		return Request{}, err
	}
	switch b[0] {
	case 4:
		return readSOCKS4(br, w)
	case 5:
		return readSOCKS5(br, w)
	}
	return readHTTP(br, w)
}

// Request is a parsed handshake.
type Request struct {
	Proto  Proto
	Target Target
	// HTTP is the first request of an HTTP forward connection, already
	// cleaned of hop-by-hop headers; Write sends it in origin form.
	HTTP *http.Request
}

func refuse(w io.Writer, p Proto, r Reply, format string, a ...any) error {
	_ = WriteReply(w, p, r)
	return fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, a...))
}

// WriteReply writes the handshake reply r in protocol p.
func WriteReply(w io.Writer, p Proto, r Reply) error {
	var b []byte
	switch p {
	case ProtoSOCKS4:
		code := byte(0x5B)
		if r == ReplyOK {
			code = 0x5A
		}
		b = []byte{0, code, 0, 0, 0, 0, 0, 0}
	case ProtoSOCKS5:
		code := map[Reply]byte{ReplyOK: 0, ReplyBlocked: 2, ReplyUnreachable: 4, ReplyBadCommand: 7, ReplyBadAddress: 8}[r]
		if r == ReplyFailure || r == ReplyBadRequest || r == ReplyMethodNotAllowed {
			code = 1
		}
		b = []byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0}
	default:
		status := map[Reply]string{
			ReplyOK:               "200 Connection established",
			ReplyBlocked:          "403 Forbidden",
			ReplyUnreachable:      "502 Bad Gateway",
			ReplyBadCommand:       "405 Method Not Allowed",
			ReplyMethodNotAllowed: "405 Method Not Allowed",
			ReplyBadAddress:       "400 Bad Request",
			ReplyBadRequest:       "400 Bad Request",
			ReplyFailure:          "500 Internal Server Error",
		}[r]
		if r == ReplyOK {
			b = []byte("HTTP/1.1 " + status + "\r\n\r\n")
		} else {
			b = []byte("HTTP/1.1 " + status + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		}
	}
	_, err := w.Write(b)
	return err
}

// readString reads a NUL-terminated string of at most 255 bytes.
func readString(br *bufio.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		c, err := br.ReadByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			return buf.String(), nil
		}
		if buf.Len() >= 255 {
			return "", errors.New("string too long")
		}
		buf.WriteByte(c)
	}
}

func readSOCKS4(br *bufio.Reader, w io.Writer) (Request, error) {
	var h [8]byte
	if _, err := io.ReadFull(br, h[:]); err != nil {
		return Request{}, err
	}
	if _, err := readString(br); err != nil { // user id, ignored
		return Request{}, err
	}
	if h[1] != 1 {
		return Request{}, refuse(w, ProtoSOCKS4, ReplyBadCommand, "socks4 command %d", h[1])
	}
	t := Target{Port: uint16(h[2])<<8 | uint16(h[3])}
	if h[4] == 0 && h[5] == 0 && h[6] == 0 && h[7] != 0 { // SOCKS4a
		host, err := readString(br)
		if err != nil || host == "" {
			return Request{}, refuse(w, ProtoSOCKS4, ReplyBadAddress, "socks4a host")
		}
		t.Host = host
	} else {
		t.IP = netip.AddrFrom4([4]byte{h[4], h[5], h[6], h[7]})
	}
	return Request{Proto: ProtoSOCKS4, Target: t}, nil
}

func readSOCKS5(br *bufio.Reader, w io.Writer) (Request, error) {
	var g [2]byte
	if _, err := io.ReadFull(br, g[:]); err != nil {
		return Request{}, err
	}
	methods := make([]byte, g[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		return Request{}, err
	}
	if bytes.IndexByte(methods, 0) < 0 {
		_, _ = w.Write([]byte{5, 0xFF})
		return Request{}, fmt.Errorf("%w: socks5 no acceptable auth method", ErrUnsupported)
	}
	if _, err := w.Write([]byte{5, 0}); err != nil {
		return Request{}, err
	}
	var h [4]byte
	if _, err := io.ReadFull(br, h[:]); err != nil {
		return Request{}, err
	}
	if h[0] != 5 {
		return Request{}, refuse(w, ProtoSOCKS5, ReplyFailure, "socks5 version %d", h[0])
	}
	if h[1] != 1 {
		return Request{}, refuse(w, ProtoSOCKS5, ReplyBadCommand, "socks5 command %d", h[1])
	}
	var t Target
	switch h[3] {
	case 1:
		var a [4]byte
		if _, err := io.ReadFull(br, a[:]); err != nil {
			return Request{}, err
		}
		t.IP = netip.AddrFrom4(a)
	case 4:
		var a [16]byte
		if _, err := io.ReadFull(br, a[:]); err != nil {
			return Request{}, err
		}
		t.IP = netip.AddrFrom16(a).Unmap()
	case 3:
		n, err := br.ReadByte()
		if err != nil {
			return Request{}, err
		}
		host := make([]byte, n)
		if _, err := io.ReadFull(br, host); err != nil {
			return Request{}, err
		}
		if n == 0 {
			return Request{}, refuse(w, ProtoSOCKS5, ReplyBadAddress, "socks5 empty host")
		}
		t.Host = string(host)
	default:
		return Request{}, refuse(w, ProtoSOCKS5, ReplyBadAddress, "socks5 address type %d", h[3])
	}
	var p [2]byte
	if _, err := io.ReadFull(br, p[:]); err != nil {
		return Request{}, err
	}
	t.Port = uint16(p[0])<<8 | uint16(p[1])
	return Request{Proto: ProtoSOCKS5, Target: t}, nil
}

// parseTarget splits host:port (port defaulting to def) into a Target.
func parseTarget(hostport string, def uint16) (Target, error) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = hostport, strconv.Itoa(int(def))
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || host == "" {
		return Target{}, fmt.Errorf("bad address %q", hostport)
	}
	t := Target{Port: uint16(n)}
	if ip, err := netip.ParseAddr(host); err == nil {
		t.IP = ip.Unmap()
	} else {
		t.Host = host
	}
	return t, nil
}
