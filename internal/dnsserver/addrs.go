// Package dnsserver holds the parts of the DNS server for this PC and the
// LAN that are not in the engine: which addresses to listen on, and the
// temporary page phones download the LAN CA and iOS profile from.
package dnsserver

import "net/netip"

// ListenPlan returns the DoH and plain-DNS addresses: DoH always on
// loopback (IPv4, plus IPv6 when available); with share, DoH and port 53
// on each LAN address too. Never the unspecified address, so services
// such as ICS that own port 53 on one interface do not block the rest.
func ListenPlan(lan []netip.Addr, dohPort int, share, v6 bool) (doh, plain []netip.AddrPort) {
	p := uint16(dohPort)
	doh = append(doh, netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), p))
	if v6 {
		doh = append(doh, netip.AddrPortFrom(netip.IPv6Loopback(), p))
	}
	if !share {
		return doh, nil
	}
	for _, a := range lan {
		if a.Is6() && !v6 {
			continue
		}
		doh = append(doh, netip.AddrPortFrom(a, p))
		plain = append(plain, netip.AddrPortFrom(a, 53))
	}
	return doh, plain
}
