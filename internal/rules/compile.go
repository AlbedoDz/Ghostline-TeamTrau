package rules

import (
	"net/netip"
	"regexp"
	"sync/atomic"
)

// Entry is one item of a list. IPs is set only by hosts/dnsmasq lines that
// map to a real address (used when the list's action is "from file").
type Entry struct {
	Pattern Pattern
	Except  bool
	IPs     []netip.Addr
	Line    int
	Action  *Action // per-line action (ghostline lists); nil = the set's action
}

// ListSet is a list ready to compile: its entries share one action, except
// for FromFile lists where an entry with IPs rewrites and one without blocks.
type ListSet struct {
	ID       string
	Action   Action
	FromFile bool
	Entries  []Entry
	// TrustedForSNI lets the list's sni= and connect= take effect.
	TrustedForSNI bool
}

// Source says what produced a decision.
type Source struct {
	Kind   string `json:"kind"` // "rule" | "list" | ""
	Index  int    `json:"index"`
	ListID string `json:"listId,omitempty"`
	Line   int    `json:"line,omitempty"`
}

// Decision is the outcome of matching a host (and/or IP).
type Decision struct {
	Action
	Source Source `json:"source"`
	// SNIIgnored is set when a list not trusted for Fake SNI asked for
	// sni= or connect=; both were dropped.
	SNIIgnored bool `json:"sniIgnored,omitempty"`
}

// Domain-kind bits stored per host in a matcher.
const (
	bitDomain uint8 = 1 << iota
	bitExact
	bitSub
)

func kindBit(k PatternKind) uint8 {
	switch k {
	case KindExact:
		return bitExact
	case KindSubOnly:
		return bitSub
	}
	return bitDomain
}

// userRef is one user rule attached to a host.
type userRef struct {
	bit  uint8
	prio int
}

type userSet struct {
	rules  []Rule // all rules, by index
	dom    map[string][]userRef
	kw     []prioString
	rx     []prioRegexp
	cidr   prefixTrie
	hasAny bool
}

type prioString struct {
	s    string
	prio int
}

type prioRegexp struct {
	re   *regexp.Regexp
	prio int
}

// lmatch is the host/CIDR matcher of one side (include or except) of a list.
type lmatch struct {
	dom  map[string]uint8
	line map[string]int // first line per host, for Explain
	kw   []prioString   // prio holds the line
	rx   []prioRegexp   // prio holds the line
	cidr prefixTrie     // prio holds the line
}

type listSet struct {
	id       string
	action   Action
	fromFile bool
	trusted  bool
	inc, exc lmatch
	ips      map[string][]netip.Addr
	perLine  map[int]Action
}

// Compiled is an immutable, ready-to-match rule set.
type Compiled struct {
	user  userSet
	lists []listSet
	count int
	// sniLists holds the Name Constraints of trusted lists' sni= entries.
	sniLists []string
}

// Compile builds a matcher from enabled user rules and lists, in order.
func Compile(user []Rule, lists []ListSet) (*Compiled, error) {
	c := &Compiled{user: userSet{rules: user, dom: map[string][]userRef{}}}
	for i, r := range user {
		if !r.Enabled {
			continue
		}
		p, err := ParsePattern(r.Pattern)
		if err != nil {
			return nil, err
		}
		c.user.hasAny = true
		c.count++
		switch p.Kind {
		case KindKeyword:
			c.user.kw = append(c.user.kw, prioString{p.Value, i})
		case KindRegexp:
			c.user.rx = append(c.user.rx, prioRegexp{regexp.MustCompile(p.Value), i})
		case KindCIDR:
			c.user.cidr.insert(p.Prefix, i)
		default:
			c.user.dom[p.Value] = append(c.user.dom[p.Value], userRef{kindBit(p.Kind), i})
		}
	}
	for _, l := range lists {
		ls := listSet{id: l.ID, action: l.Action, fromFile: l.FromFile, trusted: l.TrustedForSNI,
			inc: lmatch{dom: make(map[string]uint8, len(l.Entries)), line: make(map[string]int, len(l.Entries))},
			exc: lmatch{dom: map[string]uint8{}, line: map[string]int{}}}
		for _, e := range l.Entries {
			m := &ls.inc
			if e.Except {
				m = &ls.exc
			} else {
				c.count++
				if e.Action != nil {
					if ls.perLine == nil {
						ls.perLine = map[int]Action{}
					}
					ls.perLine[e.Line] = *e.Action
					if l.TrustedForSNI && e.Action.SNI != "" {
						if d, ok := constraintFor(e.Pattern); ok {
							c.sniLists = append(c.sniLists, d)
						}
					}
				}
			}
			switch e.Pattern.Kind {
			case KindKeyword:
				m.kw = append(m.kw, prioString{e.Pattern.Value, e.Line})
			case KindRegexp:
				re, err := regexp.Compile(e.Pattern.Value)
				if err != nil {
					continue
				}
				m.rx = append(m.rx, prioRegexp{re, e.Line})
			case KindCIDR:
				m.cidr.insert(e.Pattern.Prefix, e.Line)
			default:
				h := e.Pattern.Value
				if _, ok := m.line[h]; !ok {
					m.line[h] = e.Line
				}
				m.dom[h] |= kindBit(e.Pattern.Kind)
				if len(e.IPs) > 0 && !e.Except {
					if ls.ips == nil {
						ls.ips = map[string][]netip.Addr{}
					}
					ls.ips[h] = append(ls.ips[h], e.IPs...)
				}
			}
		}
		c.lists = append(c.lists, ls)
	}
	return c, nil
}

// Count is the number of enabled rules plus list entries (exceptions excluded).
func (c *Compiled) Count() int { return c.count }

var empty = &Compiled{user: userSet{dom: map[string][]userRef{}}}

// Holder publishes the current Compiled for concurrent readers.
type Holder struct{ p atomic.Pointer[Compiled] }

// Load returns the current rules; never nil.
func (h *Holder) Load() *Compiled {
	if c := h.p.Load(); c != nil {
		return c
	}
	return empty
}

// Store swaps in c.
func (h *Holder) Store(c *Compiled) { h.p.Store(c) }

// prefixTrie is a binary trie over IP prefixes storing the smallest prio
// per prefix. v4 and v6 live in separate roots.
type prefixTrie struct {
	v4, v6 *trieNode
}

type trieNode struct {
	child [2]*trieNode
	prio  int
	set   bool
}

func (t *prefixTrie) insert(p netip.Prefix, prio int) {
	root := &t.v6
	if p.Addr().Is4() {
		root = &t.v4
	}
	if *root == nil {
		*root = &trieNode{}
	}
	n := *root
	b := p.Addr().AsSlice()
	for i := 0; i < p.Bits(); i++ {
		bit := b[i/8] >> (7 - uint(i%8)) & 1
		if n.child[bit] == nil {
			n.child[bit] = &trieNode{}
		}
		n = n.child[bit]
	}
	if !n.set || prio < n.prio {
		n.prio, n.set = prio, true
	}
}

// lookup returns the smallest prio among prefixes containing a.
func (t *prefixTrie) lookup(a netip.Addr) (int, bool) {
	a = a.Unmap()
	n := t.v6
	if a.Is4() {
		n = t.v4
	}
	best, found := 0, false
	if n == nil {
		return 0, false
	}
	b := a.As16()
	off := 0
	if a.Is4() {
		off = 12
	}
	bits := a.BitLen()
	for i := 0; ; i++ {
		if n.set && (!found || n.prio < best) {
			best, found = n.prio, true
		}
		if i >= bits {
			break
		}
		bit := b[off+i/8] >> (7 - uint(i%8)) & 1
		if n = n.child[bit]; n == nil {
			break
		}
	}
	return best, found
}
