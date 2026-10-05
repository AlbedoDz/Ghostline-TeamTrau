package dnsserver_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/stretchr/testify/require"
)

func TestListenPlan(t *testing.T) {
	lan := []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("fd00::5")}
	doh, plain := dnsserver.ListenPlan(lan, 443, false, true)
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:443"), netip.MustParseAddrPort("[::1]:443")}, doh)
	require.Empty(t, plain)

	doh, _ = dnsserver.ListenPlan(lan, 443, false, false)
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:443")}, doh)

	doh, plain = dnsserver.ListenPlan(lan, 8443, true, true)
	require.Equal(t, []netip.AddrPort{
		netip.MustParseAddrPort("127.0.0.1:8443"), netip.MustParseAddrPort("[::1]:8443"),
		netip.MustParseAddrPort("192.168.1.5:8443"), netip.MustParseAddrPort("[fd00::5]:8443"),
	}, doh)
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("192.168.1.5:53"), netip.MustParseAddrPort("[fd00::5]:53")}, plain)
}

func files() dnsserver.SetupFiles {
	return dnsserver.SetupFiles{CRT: []byte("DER"), Fingerprint: "AB:CD:EF", SSID: "Home",
		MobileConfig: func(ssid string) ([]byte, error) { return []byte("<plist>" + ssid + "</plist>"), nil }}
}

func do(t *testing.T, h http.Handler, path, remote, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remote
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSetupPage_ServesFiles(t *testing.T) {
	p := dnsserver.NewSetupPage(files(), time.Now)
	h := p.Handler()
	r := do(t, h, "/ghostline-lan-ca.crt", "192.168.1.9:5000", "")
	require.Equal(t, http.StatusOK, r.Code)
	require.Equal(t, "application/x-x509-ca-cert", r.Header().Get("Content-Type"))
	require.Equal(t, "DER", r.Body.String())
	r = do(t, h, "/ghostline.mobileconfig", "192.168.1.9:5000", "")
	require.Equal(t, "application/x-apple-aspen-config", r.Header().Get("Content-Type"))
	require.Equal(t, "<plist>Home</plist>", r.Body.String(), "the saved Wi-Fi name is the default")
	r = do(t, h, "/", "192.168.1.9:5000", "en-US,en")
	require.Contains(t, r.Body.String(), "AB:CD:EF")
	require.Contains(t, r.Body.String(), "Full Trust")
	r = do(t, h, "/", "192.168.1.9:5000", "vi-VN,vi;q=0.9")
	require.Contains(t, r.Body.String(), "Tin cậy hoàn toàn")
}

func TestSetupPage_PrivateOnly(t *testing.T) {
	h := dnsserver.NewSetupPage(files(), time.Now).Handler()
	require.Equal(t, http.StatusForbidden, do(t, h, "/", "8.8.8.8:1", "").Code)
	require.Equal(t, http.StatusForbidden, do(t, h, "/ghostline-lan-ca.crt", "8.8.8.8:1", "").Code)
}

func TestSetupPage_PhoneEntersWiFiName(t *testing.T) {
	f := files()
	f.SSID = ""
	h := dnsserver.NewSetupPage(f, time.Now).Handler()
	page := do(t, h, "/", "192.168.1.9:1", "en").Body.String()
	require.Contains(t, page, `name="ssid"`)
	require.Contains(t, page, "Wi-Fi name")

	r := do(t, h, "/ghostline.mobileconfig?ssid=Nh%C3%A0+5G", "192.168.1.9:1", "")
	require.Equal(t, http.StatusOK, r.Code)
	require.Equal(t, "<plist>Nhà 5G</plist>", r.Body.String())

	require.Equal(t, http.StatusBadRequest, do(t, h, "/ghostline.mobileconfig", "192.168.1.9:1", "").Code, "no name anywhere")
	require.Equal(t, http.StatusBadRequest, do(t, h, "/ghostline.mobileconfig?ssid="+strings.Repeat("a", 33), "192.168.1.9:1", "").Code)
}

func TestSetupPage_PrefillsSavedWiFiName(t *testing.T) {
	h := dnsserver.NewSetupPage(files(), time.Now).Handler()
	require.Contains(t, do(t, h, "/", "192.168.1.9:1", "en").Body.String(), `value="Home"`)
}

func TestSetupPage_NoProfileBuilder(t *testing.T) {
	f := files()
	f.MobileConfig = nil
	h := dnsserver.NewSetupPage(f, time.Now).Handler()
	require.Equal(t, http.StatusNotFound, do(t, h, "/ghostline.mobileconfig?ssid=x", "192.168.1.9:1", "").Code)
}

func TestSetupPage_ClosesAfterLife(t *testing.T) {
	p := dnsserver.NewSetupPage(files(), time.Now)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := netip.MustParseAddrPort(ln.Addr().String())
	ln.Close()
	require.NoError(t, p.Start([]netip.AddrPort{addr}, 300*time.Millisecond))
	resp, err := http.Get("http://" + addr.String() + "/")
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Greater(t, p.Remaining(), time.Duration(0))
	require.Eventually(t, func() bool { return !p.Running() }, 3*time.Second, 20*time.Millisecond)
	require.Equal(t, time.Duration(0), p.Remaining())
	_, err = (&http.Client{Timeout: time.Second}).Get("http://" + addr.String() + "/")
	require.Error(t, err)
}

func TestSetupPage_StopEarly(t *testing.T) {
	p := dnsserver.NewSetupPage(files(), time.Now)
	stopped := make(chan struct{})
	p.OnStop = func() { close(stopped) }
	require.NoError(t, p.Start([]netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}, time.Hour))
	require.NoError(t, p.Stop())
	<-stopped
	require.False(t, p.Running())
}

// The trust step is required and the "PC off" consequence is spelled out,
// in numbered steps (testing showed both are easy to miss in prose).
func TestSetupPage_StepsAndWarnings(t *testing.T) {
	h := dnsserver.NewSetupPage(files(), time.Now).Handler()
	en := do(t, h, "/", "192.168.1.9:1", "en").Body.String()
	for _, want := range []string{"<ol", "Required", "Full Trust", "loses the internet", "VPN &amp; Device Management", "Automatic", "Configure DNS"} {
		require.Contains(t, en, want)
	}
	vi := do(t, h, "/", "192.168.1.9:1", "vi").Body.String()
	for _, want := range []string{"<ol", "Bắt buộc", "Tin cậy hoàn toàn", "mất mạng", "VPN và quản lý thiết bị", "Tự động", "Định cấu hình DNS"} {
		require.Contains(t, vi, want)
	}
}
