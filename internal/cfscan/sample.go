package cfscan

import (
	"math/rand/v2"
	"net/netip"
)

// Sample picks one random host address (never .0 or .255) in each /24 of
// rs, shuffles them and returns at most max.
func Sample(rs []netip.Prefix, max int, r *rand.Rand) []netip.Addr {
	var blocks []netip.Addr // first address of each /24
	for _, p := range rs {
		p = p.Masked()
		if !p.Addr().Is4() {
			continue
		}
		if p.Bits() > 24 {
			blocks = append(blocks, netip.PrefixFrom(p.Addr(), 24).Masked().Addr())
			continue
		}
		n := 1 << (24 - p.Bits())
		base := p.Addr().As4()
		start := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8
		for i := 0; i < n; i++ {
			v := start + uint32(i)<<8
			blocks = append(blocks, netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), 0}))
		}
	}
	r.Shuffle(len(blocks), func(i, j int) { blocks[i], blocks[j] = blocks[j], blocks[i] })
	if max < len(blocks) {
		blocks = blocks[:max]
	}
	out := make([]netip.Addr, 0, len(blocks))
	for _, b := range blocks {
		a := b.As4()
		a[3] = byte(1 + r.IntN(254))
		ip := netip.AddrFrom4(a)
		if Contains(rs, ip) {
			out = append(out, ip)
		}
	}
	return out
}
