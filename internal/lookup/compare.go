package lookup

import (
	"net/netip"
	"slices"

	"github.com/hashcott/ghostline/internal/scanner"
)

// Verdict is the comparison result for one source or for the whole lookup.
type Verdict string

const (
	VerdictPoisoned Verdict = "poisoned"
	VerdictDiffers  Verdict = "differs"
	VerdictMatch    Verdict = "match"
	VerdictFailed   Verdict = "failed"
	VerdictNone     Verdict = "" // record types without a verdict
)

// Compare judges A/AAAA answers from several sources (spec 3 §5.2). A source
// is poisoned when it answers a non-public IP, or NXDOMAIN while another
// source has addresses. The others match when they agree with at least one
// other source (same address set, or the same CDN), and differ otherwise.
// The overall verdict is the worst one; failed sources do not count.
func Compare(qtype string, as []Answer) (Verdict, []Verdict) {
	per := make([]Verdict, len(as))
	if qtype != "A" && qtype != "AAAA" {
		return VerdictNone, per
	}
	sets := make([][]netip.Addr, len(as))
	anyAddrs := false
	for i, a := range as {
		sets[i] = addrs(a, qtype)
		if a.OK && len(sets[i]) > 0 {
			anyAddrs = true
		}
	}
	var good []int
	for i, a := range as {
		switch {
		case !a.OK:
			per[i] = VerdictFailed
		case slices.ContainsFunc(sets[i], func(x netip.Addr) bool { return !scanner.IsPublicIP(x) }):
			per[i] = VerdictPoisoned
		case a.Rcode == "NXDOMAIN" && anyAddrs:
			per[i] = VerdictPoisoned
		default:
			good = append(good, i)
		}
	}
	for _, i := range good {
		per[i] = VerdictMatch
		if len(good) == 1 {
			break
		}
		agrees := false
		for _, j := range good {
			if j != i && agree(sets[i], sets[j]) {
				agrees = true
				break
			}
		}
		if !agrees {
			per[i] = VerdictDiffers
		}
	}
	overall := VerdictFailed
	rank := map[Verdict]int{VerdictFailed: 0, VerdictMatch: 1, VerdictDiffers: 2, VerdictPoisoned: 3}
	for _, v := range per {
		if rank[v] > rank[overall] {
			overall = v
		}
	}
	return overall, per
}

func addrs(a Answer, qtype string) []netip.Addr {
	var out []netip.Addr
	for _, r := range a.Records {
		if r.Type != qtype {
			continue
		}
		if ip, err := netip.ParseAddr(r.Data); err == nil {
			out = append(out, ip.Unmap())
		}
	}
	slices.SortFunc(out, func(x, y netip.Addr) int { return x.Compare(y) })
	return slices.Compact(out)
}

func agree(a, b []netip.Addr) bool {
	if slices.Equal(a, b) {
		return true
	}
	for _, x := range a {
		cx := CDNOf(x)
		if cx == "" {
			continue
		}
		for _, y := range b {
			if CDNOf(y) == cx {
				return true
			}
		}
	}
	return false
}
