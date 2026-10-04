package winutil

import (
	"net"
	"net/netip"
)

var privatePrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("fc00::/7"),
}

// LANAddrs lists the private (RFC 1918 / ULA) addresses of interfaces that
// are up and not loopback — what other devices can use to reach the proxy.
func LANAddrs(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) []netip.Addr {
	var out []netip.Addr
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, err := addrs(i)
		if err != nil {
			continue
		}
		for _, a := range as {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(n.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			for _, p := range privatePrefixes {
				if p.Contains(ip) {
					out = append(out, ip)
					break
				}
			}
		}
	}
	return out
}

// LocalLANAddrs is LANAddrs for this machine.
func LocalLANAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	return LANAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return i.Addrs() })
}
