package rules

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

type token struct {
	s   string
	pos int
}

func tokenize(line string) []token {
	var out []token
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' {
			j++
		}
		out = append(out, token{line[i:j], i})
		i = j
	}
	return out
}

// ParseText parses the rule text format (spec §7.1). upstreamIDs lists the
// upstream proxy ids a rule may reference. The returned rules are only
// meaningful when errs is empty; callers must not save otherwise.
func ParseText(text string, upstreamIDs []string) ([]Rule, []LineError) {
	var rs []Rule
	var errs []LineError
	text = strings.TrimPrefix(text, "\uFEFF")
	for n, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(strings.TrimRight(line, "\r"))
		enabled := true
		if strings.HasPrefix(trim, "#!") {
			enabled = false
			trim = strings.TrimSpace(trim[2:])
		} else if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		r, err := parseLine(trim, upstreamIDs)
		if err == nil && len(rs) >= MaxUserRules {
			err = fmt.Errorf("more than %d rules", MaxUserRules)
		}
		if err != nil {
			errs = append(errs, LineError{Line: n + 1, Msg: err.Error()})
			continue
		}
		r.Enabled = enabled
		rs = append(rs, r)
	}
	return rs, errs
}

func parseLine(line string, upstreamIDs []string) (Rule, error) {
	toks := tokenize(line)
	r := Rule{Pattern: toks[0].s}
	p, err := ParsePattern(r.Pattern)
	if err != nil {
		return Rule{}, err
	}
	for _, t := range toks[1:] {
		if strings.HasPrefix(t.s, "#") {
			r.Comment = strings.TrimSpace(strings.TrimPrefix(line[t.pos:], "#"))
			break
		}
		key, val, hasVal := strings.Cut(t.s, "=")
		switch {
		case !hasVal && key == "block":
			r.Block = true
		case !hasVal && key == "allow":
			r.Allow = true
		case hasVal && key == "ip":
			a, err := netip.ParseAddr(val)
			if err != nil {
				return Rule{}, fmt.Errorf("bad ip %q", val)
			}
			r.IPs = append(r.IPs, a)
		case hasVal && key == "fragment":
			switch Frag(val) {
			case FragAuto, FragOn, FragOff:
				r.Fragment = Frag(val)
			default:
				return Rule{}, errors.New("fragment must be auto, on or off")
			}
		case hasVal && key == "upstream":
			if !slices.Contains(upstreamIDs, val) {
				return Rule{}, fmt.Errorf("unknown upstream %q", val)
			}
			r.Upstream = val
		case hasVal && key == "sni":
			if val == SNINone {
				r.SNI = SNINone
				break
			}
			h, err := NormalizeHost(val)
			if err != nil {
				return Rule{}, err
			}
			r.SNI = h
		case hasVal && key == "connect":
			h, err := NormalizeHost(val)
			if err != nil {
				return Rule{}, err
			}
			r.Connect = h
		default:
			return Rule{}, fmt.Errorf("unknown action %q", t.s)
		}
	}
	if err := validateAction(r.Action, p.Kind); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// ValidateRule checks a rule coming from the table editor.
func ValidateRule(r Rule, upstreamIDs []string) error {
	p, err := ParsePattern(r.Pattern)
	if err != nil {
		return err
	}
	if r.Upstream != "" && !slices.Contains(upstreamIDs, r.Upstream) {
		return fmt.Errorf("unknown upstream %q", r.Upstream)
	}
	return validateAction(r.Action, p.Kind)
}

// ValidateAction checks an action against the kind of pattern it is used
// with (list formats use it for per-line actions).
func ValidateAction(a Action, kind PatternKind) error { return validateAction(a, kind) }

func validateAction(a Action, kind PatternKind) error {
	if a.Empty() {
		return errors.New("rule has no action")
	}
	if a.Block && (a.Allow || len(a.IPs) > 0 || a.Fragment != FragUnset || a.Upstream != "" || a.SNI != "" || a.Connect != "") {
		return errors.New("block cannot be combined with other actions")
	}
	if a.SNI != "" || a.Connect != "" {
		switch kind {
		case KindKeyword, KindRegexp, KindCIDR:
			return errors.New("sni= and connect= work only with domain patterns")
		}
	}
	if a.Connect != "" && len(a.IPs) > 0 {
		return errors.New("connect= cannot be combined with ip=")
	}
	return nil
}

// FormatText renders rules in the text format; ParseText(FormatText(rs))
// gives rs back.
func FormatText(rs []Rule) string {
	var b strings.Builder
	for _, r := range rs {
		if !r.Enabled {
			b.WriteString("#! ")
		}
		b.WriteString(r.Pattern)
		if r.Block {
			b.WriteString(" block")
		}
		if r.Allow {
			b.WriteString(" allow")
		}
		for _, ip := range r.IPs {
			b.WriteString(" ip=" + ip.String())
		}
		if r.Fragment != FragUnset {
			b.WriteString(" fragment=" + string(r.Fragment))
		}
		if r.Upstream != "" {
			b.WriteString(" upstream=" + r.Upstream)
		}
		if r.SNI != "" {
			b.WriteString(" sni=" + r.SNI)
		}
		if r.Connect != "" {
			b.WriteString(" connect=" + r.Connect)
		}
		if r.Comment != "" {
			b.WriteString("  # " + r.Comment)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
