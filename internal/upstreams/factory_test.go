package upstreams_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/fragdoh"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/upstreams"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// dohServer starts a TLS DoH server answering every A query with 192.0.2.53.
func dohServer(t *testing.T) (port int, roots *x509.CertPool) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodGet {
			body, _ = base64.RawURLEncoding.DecodeString(r.URL.Query().Get("dns"))
		}
		var q dns.Msg
		if err := q.Unpack(body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		resp := new(dns.Msg).SetReply(&q)
		resp.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(192, 0, 2, 53)}}
		out, _ := resp.Pack()
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(out)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	roots = x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	return srv.Listener.Addr().(*net.TCPAddr).Port, roots
}

// plainDNS starts a UDP DNS server resolving example.com to 127.0.0.1 and
// counting queries.
func plainDNS(t *testing.T) (addr string, count *atomic.Int32) {
	t.Helper()
	count = &atomic.Int32{}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		count.Add(1)
		m := new(dns.Msg).SetReply(r)
		if r.Question[0].Qtype == dns.TypeA && r.Question[0].Name == "example.com." {
			m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(127, 0, 0, 1)}}
		}
		_ = w.WriteMsg(m)
	})}
	go func() { _ = s.ActivateAndServe() }()
	t.Cleanup(func() { _ = s.Shutdown() })
	return pc.LocalAddr().String(), count
}

func query(name string) *dns.Msg { return new(dns.Msg).SetQuestion(name, dns.TypeA) }

func TestBuild_UsesPinnedIPs(t *testing.T) {
	port, roots := dohServer(t)
	f, err := upstreams.NewFactory(upstreams.Options{Bootstrap: []string{"127.0.0.1:1"}, Timeout: 3 * time.Second, RootCAs: roots})
	require.NoError(t, err)
	u, err := f.Build(model.Server{Protocol: model.ProtoDoH, Address: "https://example.com:" + strconv.Itoa(port) + "/dns-query", IPs: []string{"127.0.0.1"}})
	require.NoError(t, err)
	defer u.Close()
	resp, err := u.Exchange(context.Background(), query("example.org."))
	require.NoError(t, err)
	require.Equal(t, "192.0.2.53", resp.Answer[0].(*dns.A).A.String())
}

func TestBuild_UsesBootstrapNotSystem(t *testing.T) {
	port, roots := dohServer(t)
	boot, count := plainDNS(t)
	f, err := upstreams.NewFactory(upstreams.Options{Bootstrap: []string{boot}, Timeout: 3 * time.Second, RootCAs: roots})
	require.NoError(t, err)
	u, err := f.Build(model.Server{Protocol: model.ProtoDoH, Address: "https://example.com:" + strconv.Itoa(port) + "/dns-query"})
	require.NoError(t, err)
	defer u.Close()
	_, err = u.Exchange(context.Background(), query("example.org."))
	require.NoError(t, err)
	require.GreaterOrEqual(t, count.Load(), int32(1))
}

func TestBuild_BlackholeBootstrapTimesOut(t *testing.T) { // Review Focus #5
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()
	go func() { // read and never answer
		buf := make([]byte, 2048)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	f, err := upstreams.NewFactory(upstreams.Options{Bootstrap: []string{pc.LocalAddr().String()}, Timeout: 3 * time.Second})
	require.NoError(t, err)
	u, err := f.Build(model.Server{Protocol: model.ProtoDoH, Address: "https://blackhole.example/dns-query"})
	require.NoError(t, err)
	defer u.Close()
	start := time.Now()
	_, err = u.Exchange(context.Background(), query("example.org."))
	require.Error(t, err)
	require.Less(t, time.Since(start), 3500*time.Millisecond)
}

func TestBuild_FragmentOnlyForDoH(t *testing.T) {
	f, err := upstreams.NewFactory(upstreams.Options{Bootstrap: []string{"1.1.1.1:53"}, Timeout: time.Second,
		Fragment: &upstreams.FragmentOptions{Chunks: 5, Delay: 5 * time.Millisecond}})
	require.NoError(t, err)
	doh, err := f.Build(model.Server{Protocol: model.ProtoDoH, Address: "https://dns.example/dns-query"})
	require.NoError(t, err)
	_, isFrag := doh.(*fragdoh.Upstream)
	require.True(t, isFrag)
	dot, err := f.Build(model.Server{Protocol: model.ProtoDoT, Address: "tls://dns.example"})
	require.NoError(t, err)
	_, isFrag = dot.(*fragdoh.Upstream)
	require.False(t, isFrag)
}

func TestNewFactory_EmptyBootstrapFails(t *testing.T) {
	_, err := upstreams.NewFactory(upstreams.Options{Timeout: time.Second})
	require.Error(t, err)
}
