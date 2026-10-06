package lookup

import (
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Dig renders a reply the way dig prints it.
func Dig(m *dns.Msg, latency time.Duration, src Source) string {
	var b strings.Builder
	q := ""
	if len(m.Question) > 0 {
		q = strings.TrimSuffix(m.Question[0].Name, ".") + " " + dns.TypeToString[m.Question[0].Qtype]
	}
	fmt.Fprintf(&b, "; <<>> Ghostline lookup <<>> %s\n", q)
	b.WriteString(m.String())
	fmt.Fprintf(&b, "\n;; Query time: %d msec\n;; SERVER: %s\n", latency.Milliseconds(), src.Label)
	return b.String()
}
