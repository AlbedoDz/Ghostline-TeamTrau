package tlsfrag_test

import (
	"crypto/tls"
	"encoding/binary"
	"net"
	"testing"

	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/stretchr/testify/require"
)

func captureWithALPN(t testing.TB, protos []string) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer c1.Close()
	go func() {
		_ = tls.Client(c1, &tls.Config{ServerName: "a.example", NextProtos: protos, InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, 64*1024)
	n, err := c2.Read(buf)
	require.NoError(t, err)
	c2.Close()
	return buf[:n]
}

func TestALPN(t *testing.T) {
	require.Equal(t, []string{"h2", "http/1.1"}, tlsfrag.ALPN(captureWithALPN(t, []string{"h2", "http/1.1"})))
	require.Nil(t, tlsfrag.ALPN(captureWithALPN(t, nil)))
}

func TestALPN_Truncated(t *testing.T) {
	h := captureWithALPN(t, []string{"h2"})
	for i := 0; i < len(h); i += 7 {
		_ = tlsfrag.ALPN(h[:i]) // must not panic
	}
	require.Nil(t, tlsfrag.ALPN(h[:40]))
}

// withExtension appends an empty extension of type typ to a captured hello,
// fixing up the record, handshake and extensions lengths.
func withExtension(h []byte, typ uint16) []byte {
	out := append([]byte(nil), h...)
	out = append(out, byte(typ>>8), byte(typ), 0, 0)
	binary.BigEndian.PutUint16(out[3:5], uint16(len(out)-5))
	hl := len(out) - 9
	out[6], out[7], out[8] = byte(hl>>16), byte(hl>>8), byte(hl)
	// extensions length sits after session id, ciphers and compression.
	p := 9 + 2 + 32
	p += 1 + int(out[p])
	p += 2 + int(binary.BigEndian.Uint16(out[p:]))
	p += 1 + int(out[p])
	binary.BigEndian.PutUint16(out[p:], binary.BigEndian.Uint16(out[p:])+4)
	return out
}

func TestHasECH(t *testing.T) {
	h := captureWithALPN(t, nil)
	require.False(t, tlsfrag.HasECH(h))
	e := withExtension(h, 0xfe0d)
	require.True(t, tlsfrag.HasECH(e))
	sni, ok := tlsfrag.SNI(e)
	require.True(t, ok)
	require.Equal(t, "a.example", sni)
}

func FuzzALPN(f *testing.F) {
	f.Add(captureWithALPN(f, []string{"h2", "http/1.1"}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = tlsfrag.ALPN(b)
		_ = tlsfrag.HasECH(b)
	})
}
