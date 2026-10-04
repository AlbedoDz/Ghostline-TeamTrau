package wire_test

import (
	"bufio"
	"bytes"
	"io"
	"net/netip"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/stretchr/testify/require"
)

func read(t *testing.T, in []byte) (wire.Request, []byte, error) {
	t.Helper()
	var out bytes.Buffer
	r, err := wire.ReadRequest(bufio.NewReaderSize(bytes.NewReader(in), 16<<10), &out)
	return r, out.Bytes(), err
}

func TestSOCKS4(t *testing.T) {
	r, reply, err := read(t, []byte{4, 1, 0x01, 0xBB, 93, 184, 216, 34, 'u', 0})
	require.NoError(t, err)
	require.Empty(t, reply)
	require.Equal(t, wire.ProtoSOCKS4, r.Proto)
	require.Equal(t, wire.Target{IP: netip.MustParseAddr("93.184.216.34"), Port: 443}, r.Target)
}

func TestSOCKS4a(t *testing.T) {
	in := append([]byte{4, 1, 0, 80, 0, 0, 0, 7, 0}, append([]byte("example.com"), 0)...)
	r, _, err := read(t, in)
	require.NoError(t, err)
	require.Equal(t, wire.Target{Host: "example.com", Port: 80}, r.Target)
}

func TestSOCKS4_BindRejected(t *testing.T) {
	_, reply, err := read(t, []byte{4, 2, 0, 80, 1, 2, 3, 4, 0})
	require.ErrorIs(t, err, wire.ErrUnsupported)
	require.Equal(t, []byte{0, 0x5B, 0, 0, 0, 0, 0, 0}, reply)
}

func TestSOCKS5(t *testing.T) {
	cases := []struct {
		req  []byte
		want wire.Target
	}{
		{[]byte{5, 1, 0, 1, 1, 2, 3, 4, 0x01, 0xBB}, wire.Target{IP: netip.MustParseAddr("1.2.3.4"), Port: 443}},
		{append([]byte{5, 1, 0, 4}, append(netip.MustParseAddr("2001:db8::1").AsSlice(), 0, 80)...), wire.Target{IP: netip.MustParseAddr("2001:db8::1"), Port: 80}},
		{append(append([]byte{5, 1, 0, 3, 11}, "example.com"...), 0x01, 0xBB), wire.Target{Host: "example.com", Port: 443}},
		{append(append([]byte{5, 1, 0, 3, 255}, strings.Repeat("a", 255)...), 0, 1), wire.Target{Host: strings.Repeat("a", 255), Port: 1}},
	}
	for _, c := range cases {
		in := append([]byte{5, 2, 2, 0}, c.req...) // offers user/pass and no-auth
		r, reply, err := read(t, in)
		require.NoError(t, err)
		require.Equal(t, []byte{5, 0}, reply)
		require.Equal(t, wire.ProtoSOCKS5, r.Proto)
		require.Equal(t, c.want, r.Target)
	}
}

func TestSOCKS5_NoAcceptableMethod(t *testing.T) {
	_, reply, err := read(t, []byte{5, 1, 2})
	require.ErrorIs(t, err, wire.ErrUnsupported)
	require.Equal(t, []byte{5, 0xFF}, reply)
}

func TestSOCKS5_BindAndBadATYP(t *testing.T) {
	_, reply, err := read(t, []byte{5, 1, 0, 5, 2, 0, 1, 1, 2, 3, 4, 0, 80})
	require.ErrorIs(t, err, wire.ErrUnsupported)
	require.Equal(t, []byte{5, 0, 5, 7, 0, 1, 0, 0, 0, 0, 0, 0}, reply)
	_, reply, err = read(t, []byte{5, 1, 0, 5, 1, 0, 9, 1, 2})
	require.ErrorIs(t, err, wire.ErrUnsupported)
	require.Equal(t, []byte{5, 0, 5, 8, 0, 1, 0, 0, 0, 0, 0, 0}, reply)
}

func TestHTTPConnect(t *testing.T) {
	r, _, err := read(t, []byte("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"))
	require.NoError(t, err)
	require.Equal(t, wire.ProtoHTTPConnect, r.Proto)
	require.Equal(t, wire.Target{Host: "example.com", Port: 443}, r.Target)
	r, _, err = read(t, []byte("CONNECT [2001:db8::1]:8443 HTTP/1.1\r\n\r\n"))
	require.NoError(t, err)
	require.Equal(t, wire.Target{IP: netip.MustParseAddr("2001:db8::1"), Port: 8443}, r.Target)
}

func TestHTTPForward(t *testing.T) {
	in := "GET http://example.com/a?b HTTP/1.1\r\nHost: example.com\r\nProxy-Connection: keep-alive\r\nProxy-Authorization: x\r\nAccept: */*\r\n\r\n"
	r, _, err := read(t, []byte(in))
	require.NoError(t, err)
	require.Equal(t, wire.ProtoHTTPForward, r.Proto)
	require.Equal(t, wire.Target{Host: "example.com", Port: 80}, r.Target)
	var out bytes.Buffer
	require.NoError(t, r.HTTP.Write(&out))
	s := out.String()
	require.True(t, strings.HasPrefix(s, "GET /a?b HTTP/1.1\r\n"), s)
	require.NotContains(t, s, "Proxy-Connection")
	require.NotContains(t, s, "Proxy-Authorization")
	require.Contains(t, s, "Accept: */*")
}

func TestHTTPForward_PortAndBody(t *testing.T) {
	in := "POST http://example.com:8080/p HTTP/1.1\r\nHost: example.com:8080\r\nContent-Length: 5\r\n\r\nhelloGET http://b.test/ HTTP/1.1\r\nHost: b.test\r\n\r\n"
	br := bufio.NewReaderSize(strings.NewReader(in), 16<<10)
	r, err := wire.ReadRequest(br, io.Discard)
	require.NoError(t, err)
	require.Equal(t, uint16(8080), r.Target.Port)
	body, err := io.ReadAll(r.HTTP.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
	next, target, err := wire.ReadForward(br)
	require.NoError(t, err)
	require.Equal(t, "b.test", target.Host)
	require.Equal(t, "/", next.URL.Path)
}

func TestHTTPErrors(t *testing.T) {
	_, reply, err := read(t, []byte("GET /relative HTTP/1.1\r\nHost: x\r\n\r\n"))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(string(reply), "HTTP/1.1 400"), string(reply))
	_, reply, err = read(t, []byte("TRACE http://x/ HTTP/1.1\r\nHost: x\r\n\r\n"))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(string(reply), "HTTP/1.1 405"), string(reply))
	_, reply, err = read(t, []byte("GET https://x/ HTTP/1.1\r\nHost: x\r\n\r\n"))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(string(reply), "HTTP/1.1 400"), string(reply))
}

func TestHeaderTooLarge(t *testing.T) {
	in := "GET http://x/ HTTP/1.1\r\nX-Big: " + strings.Repeat("a", wire.MaxHeader) + "\r\n\r\n"
	_, _, err := read(t, []byte(in))
	require.Error(t, err)
}

func TestWriteReply(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, wire.WriteReply(&b, wire.ProtoSOCKS4, wire.ReplyOK))
	require.Equal(t, []byte{0, 0x5A, 0, 0, 0, 0, 0, 0}, b.Bytes())
	b.Reset()
	require.NoError(t, wire.WriteReply(&b, wire.ProtoSOCKS5, wire.ReplyBlocked))
	require.Equal(t, []byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0}, b.Bytes())
	b.Reset()
	require.NoError(t, wire.WriteReply(&b, wire.ProtoSOCKS5, wire.ReplyUnreachable))
	require.Equal(t, byte(4), b.Bytes()[1])
	b.Reset()
	require.NoError(t, wire.WriteReply(&b, wire.ProtoHTTPConnect, wire.ReplyOK))
	require.Equal(t, "HTTP/1.1 200 Connection established\r\n\r\n", b.String())
	b.Reset()
	require.NoError(t, wire.WriteReply(&b, wire.ProtoHTTPForward, wire.ReplyBlocked))
	require.True(t, strings.HasPrefix(b.String(), "HTTP/1.1 403"))
	b.Reset()
	require.NoError(t, wire.WriteReply(&b, wire.ProtoHTTPConnect, wire.ReplyUnreachable))
	require.True(t, strings.HasPrefix(b.String(), "HTTP/1.1 502"))
}

func TestTargetString(t *testing.T) {
	require.Equal(t, "example.com:443", wire.Target{Host: "example.com", Port: 443}.String())
	require.Equal(t, "[2001:db8::1]:80", wire.Target{IP: netip.MustParseAddr("2001:db8::1"), Port: 80}.String())
}

func FuzzReadRequest(f *testing.F) {
	f.Add([]byte{4, 1, 0, 80, 1, 2, 3, 4, 0})
	f.Add([]byte{5, 1, 0, 5, 1, 0, 3, 3, 'a', 'b', 'c', 0, 80})
	f.Add([]byte("CONNECT a:1 HTTP/1.1\r\n\r\n"))
	f.Add([]byte("GET http://a/ HTTP/1.1\r\nHost: a\r\n\r\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		// Must terminate without panicking on any input.
		_, _ = wire.ReadRequest(bufio.NewReaderSize(bytes.NewReader(in), 16<<10), io.Discard)
	})
}
