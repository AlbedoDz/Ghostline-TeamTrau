package shell

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// system implements app.System.
type system struct{}

func (system) IsAdmin() bool { return winutil.IsAdmin() }
func (system) PortOwners(p uint16) ([]winutil.PortOwner, error) {
	return winutil.PortOwners(p)
}
func (system) SelfPID() (uint32, time.Time) {
	pid := uint32(os.Getpid())
	start, _ := winutil.ProcessStartTime(pid)
	return pid, start
}

// ListenFree binds UDP and TCP on each address the way the DNS engine will,
// then releases them.
func (system) ListenFree(addrs []netip.AddrPort) error {
	for _, a := range addrs {
		u, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(a))
		if err != nil {
			return err
		}
		t, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(a))
		u.Close()
		if err != nil {
			return err
		}
		t.Close()
	}
	return nil
}

// IPv6Available reports whether [::1] can be bound (IPv6 may be disabled).
func (system) IPv6Available() bool {
	c, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// safety implements app.Safety.
type safety struct{ exe string }

func (s safety) StartWatchdog(pid uint32, start time.Time) (func() error, error) {
	// Deliberately not in our job object, and broken away from any job we
	// inherited from a terminal or IDE: it must outlive us.
	cmd, err := winutil.StartDetached(s.exe, []string{"--watchdog", "--parent", strconv.FormatUint(uint64(pid), 10),
		"--parent-start", strconv.FormatInt(start.UnixNano(), 10)})
	if err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return func() error { return cmd.Process.Kill() }, nil
}

func (s safety) CreateRecoveryTask() error { return startup.Create(startup.RecoveryTask(s.exe)) }
func (s safety) DeleteRecoveryTask() error { return startup.Delete(startup.RecoveryTask(s.exe).Name) }

// adaptersAddresses returns the GetAdaptersAddresses list (with
// gateways), or nil.
func adaptersAddresses() *windows.IpAdapterAddresses {
	size := uint32(15 * 1024)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0x80, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			return (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil
		}
	}
	return nil
}

// networkKey identifies the current network by the default gateway of the
// first connected adapter: its address and the router's hardware address,
// so two networks that both use 192.168.1.1 get their own server ranking.
// When the router's address cannot be read (IPv6-only), the adapter's own
// stands in.
func networkKey() string {
	first := adaptersAddresses()
	if first == nil {
		return "unknown"
	}
	for aa := first; aa != nil; aa = aa.Next {
		if aa.OperStatus != 1 || aa.FirstGatewayAddress == nil {
			continue
		}
		gw := aa.FirstGatewayAddress.Address.IP()
		for g := aa.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP(); ip.To4() != nil {
				gw = ip
				break
			}
		}
		if mac := gatewayMAC(gw); mac != "" {
			return scanner.NetworkKey(gw.String(), mac)
		}
		return scanner.NetworkKey(gw.String(), net.HardwareAddr(aa.PhysicalAddress[:aa.PhysicalAddressLength]).String())
	}
	return scanner.NetworkKey("none", "none")
}

var (
	procSendARP = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SendARP")
	arpMu       sync.Mutex
	arpCache    = map[string]arpEntry{}
)

type arpEntry struct {
	mac string
	at  time.Time
}

// gatewayMAC returns the hardware address of an IPv4 gateway ("" when
// unknown), remembered for a minute: the key is read on every pick.
func gatewayMAC(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	arpMu.Lock()
	defer arpMu.Unlock()
	if e, ok := arpCache[ip4.String()]; ok && time.Since(e.at) < time.Minute {
		return e.mac
	}
	var mac [8]byte
	n := uint32(len(mac))
	dest := binary.LittleEndian.Uint32(ip4) // IPAddr is in network order
	r, _, _ := procSendARP.Call(uintptr(dest), 0, uintptr(unsafe.Pointer(&mac[0])), uintptr(unsafe.Pointer(&n)))
	out := ""
	if r == 0 && n >= 6 && n <= 8 {
		out = net.HardwareAddr(mac[:n]).String()
	}
	arpCache[ip4.String()] = arpEntry{mac: out, at: time.Now()}
	return out
}

// liveAdapters lists the DNS servers (static or DHCP) and gateway of every
// up adapter that has a gateway.
func liveAdapters() []liveAdapter {
	var out []liveAdapter
	for aa := adaptersAddresses(); aa != nil; aa = aa.Next {
		if aa.OperStatus != 1 || aa.FirstGatewayAddress == nil {
			continue
		}
		la := liveAdapter{Gateway: aa.FirstGatewayAddress.Address.IP().String()}
		for d := aa.FirstDnsServerAddress; d != nil; d = d.Next {
			la.DNS = append(la.DNS, d.Address.IP().String())
		}
		out = append(out, la)
	}
	return out
}

const webView2ClientKey = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

func webView2Installed() bool {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, path := range []string{webView2ClientKey, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`} {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && v != "" && v != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

func messageBox(title, text string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONWARNING)
}

func fatalBox(err error) {
	messageBox("Ghostline", fmt.Sprintf("Ghostline không thể khởi động / could not start:\n\n%v", err))
}
