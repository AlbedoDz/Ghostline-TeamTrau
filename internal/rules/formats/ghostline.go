package formats

import (
	"bytes"

	"github.com/hashcott/ghostline/internal/rules"
)

// GhostlineHeader is the first line of a ghostline list.
const GhostlineHeader = "# ghostline-rules v1"

func isGhostline(data []byte) bool {
	data = bytes.TrimLeft(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), " \t\r\n")
	line, _, _ := bytes.Cut(data, []byte("\n"))
	return string(bytes.TrimSpace(line)) == GhostlineHeader
}

// parseGhostline reads one rule-text line with its own actions. Lines the
// rule parser rejects (sni= on a keyword, unknown upstreams…) are skipped.
func parseGhostline(b *builder, line string, n int) {
	if isComment(line) {
		return
	}
	rs, errs := rules.ParseText(line, nil)
	if len(errs) > 0 || len(rs) != 1 {
		b.skip(line)
		return
	}
	p, err := rules.ParsePattern(rs[0].Pattern)
	if err != nil {
		b.skip(line)
		return
	}
	a := rs[0].Action
	b.add(p, n, line, false, nil)
	b.r.Entries[len(b.r.Entries)-1].Action = &a
}
