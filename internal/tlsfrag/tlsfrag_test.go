package tlsfrag_test

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"

	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/stretchr/testify/require"
)

func captureClientHello(t testing.TB, host string) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer c1.Close()
	go func() {
		_ = tls.Client(c1, &tls.Config{ServerName: host, InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, 64*1024)
	n, err := c2.Read(buf)
	require.NoError(t, err)
	c2.Close()
	return buf[:n]
}

// records parses b as a sequence of TLS records.
func records(t *testing.T, b []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for len(b) > 0 {
		require.GreaterOrEqual(t, len(b), 5)
		n, ok := tlsfrag.RecordLen(b[:5])
		require.True(t, ok)
		require.GreaterOrEqual(t, len(b), n)
		out = append(out, b[:n])
		b = b[n:]
	}
	return out
}

func sniBounds(t *testing.T, rec []byte, host string) (int, int) {
	t.Helper()
	i := bytes.Index(rec, []byte(host))
	require.Positive(t, i)
	return i, i + len(host)
}

func TestSplit_TCPJoinsBack(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")
	segs := tlsfrag.Split(rec, tlsfrag.MethodTCP, 5)
	require.Len(t, segs, 7)
	require.Equal(t, rec, bytes.Join(segs, nil))
	start, _ := sniBounds(t, rec, "dns.example.test")
	require.Len(t, segs[0], start)
	require.Equal(t, "dns.example.test", string(bytes.Join(segs[1:6], nil)))
}

func TestSplit_RecordIsValidTLS(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")
	segs := tlsfrag.Split(rec, tlsfrag.MethodRecord, 4)
	require.Len(t, segs, 1)
	recs := records(t, segs[0])
	require.Len(t, recs, 4)
	var payload []byte
	for _, r := range recs {
		require.Equal(t, byte(0x16), r[0])
		require.Equal(t, rec[1:3], r[1:3])
		n := int(r[3])<<8 | int(r[4])
		require.Equal(t, len(r)-5, n)
		require.Positive(t, n)
		payload = append(payload, r[5:]...)
	}
	require.Equal(t, rec[5:], payload)
	// The first record boundary falls inside the SNI host name.
	start, end := sniBounds(t, rec, "dns.example.test")
	cut := 5 + len(recs[0]) - 5 // offset in rec where record 1 payload ends
	require.Greater(t, cut, start)
	require.Less(t, cut, end)
}

func TestSplit_BothIsRecordsAsSegments(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")
	segs := tlsfrag.Split(rec, tlsfrag.MethodBoth, 4)
	require.Len(t, segs, 4)
	for _, s := range segs {
		require.Len(t, records(t, s), 1)
	}
	require.Equal(t, tlsfrag.Split(rec, tlsfrag.MethodRecord, 4)[0], bytes.Join(segs, nil))
}

func TestSplit_NotHelloUnchanged(t *testing.T) {
	for _, rec := range [][]byte{
		[]byte("GET / HTTP/1.1\r\n"),
		{0x17, 3, 3, 0, 1, 0},
		captureClientHello(t, "dns.example.test")[:40],
	} {
		for _, m := range []tlsfrag.Method{tlsfrag.MethodTCP, tlsfrag.MethodRecord, tlsfrag.MethodBoth} {
			require.Equal(t, [][]byte{rec}, tlsfrag.Split(rec, m, 5))
		}
	}
}

func TestSNI(t *testing.T) {
	rec := captureClientHello(t, "example.com")
	name, ok := tlsfrag.SNI(rec)
	require.True(t, ok)
	require.Equal(t, "example.com", name)
	require.True(t, tlsfrag.IsClientHello(rec))
	require.False(t, tlsfrag.IsClientHello([]byte("GET /")))
	_, ok = tlsfrag.SNI([]byte{0x17, 3, 3, 0, 0})
	require.False(t, ok)
}

func TestRecordLen(t *testing.T) {
	n, ok := tlsfrag.RecordLen([]byte{0x16, 3, 1, 0x01, 0x02})
	require.True(t, ok)
	require.Equal(t, 5+0x102, n)
	_, ok = tlsfrag.RecordLen([]byte{0x16, 3})
	require.False(t, ok)
}

func FuzzSplit(f *testing.F) {
	f.Add(captureClientHello(f, "dns.example.test"), uint8(5))
	f.Add([]byte("GET / HTTP/1.1"), uint8(3))
	f.Fuzz(func(t *testing.T, in []byte, chunks uint8) {
		c := int(chunks%16) + 1
		require.True(t, bytes.Equal(in, bytes.Join(tlsfrag.Split(in, tlsfrag.MethodTCP, c), nil)))
		_ = tlsfrag.Split(in, tlsfrag.MethodRecord, c)
		_ = tlsfrag.Split(in, tlsfrag.MethodBoth, c)
	})
}
