package dialer

import "net/netip"

// allowed applies the SSRF and loop guards (spec 5.5).
func (d *Dialer) allowed(client netip.Addr, ap netip.AddrPort) error {
	ip := ap.Addr().Unmap()
	self := false
	if d.c.SelfAddrs != nil {
		for _, s := range d.c.SelfAddrs() {
			if s.Unmap() == ip {
				self = true
				break
			}
		}
	}
	if !client.Unmap().IsLoopback() {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || self ||
			ip.Is4() && ip.As4()[0] == 0 {
			return ErrForbidden
		}
	}
	for _, f := range d.c.Forbidden {
		if f.Port() != ap.Port() {
			continue
		}
		if f.Addr().Unmap() == ip || ip.IsLoopback() || ip.IsUnspecified() || self {
			return ErrForbidden
		}
	}
	return nil
}
