// Package dpi manages GoodbyeDPI: presets, argument validation, extracting
// the hash-pinned binaries and running them.
package dpi

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Preset names a GoodbyeDPI configuration.
type Preset string

// AutotuneOrder is tried lightest first.
var AutotuneOrder = []Preset{"light", "medium", "high", "extreme"}

// Scope selects which traffic GoodbyeDPI touches.
type Scope string

const (
	ScopeAll       Scope = "all"
	ScopeBlacklist Scope = "blacklist"
)

// Presets never use --dns-addr/--dns-port: Ghostline's engine already
// encrypts DNS, and a second redirect would fight it.
var lightArgs = []string{"-p", "-r", "-s", "-m", "-e", "40", "-w", "--native-frag"}

func presetArgs(p Preset) ([]string, bool) {
	cp := func(a ...[]string) []string {
		var out []string
		for _, x := range a {
			out = append(out, x...)
		}
		return out
	}
	ttl := []string{"--auto-ttl", "1-4-10", "--min-ttl", "3"}
	switch p {
	case "light":
		return cp(lightArgs), true
	case "medium":
		return cp(lightArgs, ttl), true
	case "high":
		return cp(lightArgs, ttl, []string{"--wrong-seq"}), true
	case "extreme":
		return []string{"-p", "-r", "-s", "-m", "-f", "2", "-e", "40", "-w", "--auto-ttl", "1-4-10", "--min-ttl", "3",
			"--native-frag", "--wrong-chksum", "--wrong-seq", "--max-payload"}, true
	}
	if len(p) == 5 && strings.HasPrefix(string(p), "mode") && p[4] >= '1' && p[4] <= '6' {
		return []string{"-" + string(p[4])}, true
	}
	return nil, false
}

// Args builds the argv for a preset (or custom args) and scope. Each element
// is one argument, so paths with spaces are safe.
func Args(p Preset, custom string, scope Scope, blacklistPath string) ([]string, error) {
	var args []string
	if p == "custom" {
		var err error
		if args, err = ValidateCustom(custom); err != nil {
			return nil, err
		}
	} else {
		var ok bool
		if args, ok = presetArgs(p); !ok {
			return nil, fmt.Errorf("dpi: unknown preset %q", p)
		}
	}
	if scope == ScopeBlacklist {
		args = append(args, "--blacklist", blacklistPath)
	}
	return args, nil
}

// ErrForbiddenFlag rejects flags Ghostline does not allow in custom args.
var ErrForbiddenFlag = errors.New("dpi: flag not allowed")

var (
	noValue = map[string]bool{"-p": true, "-r": true, "-s": true, "-m": true, "-n": true, "-a": true, "-w": true,
		"-1": true, "-2": true, "-3": true, "-4": true, "-5": true, "-6": true,
		"--native-frag": true, "--reverse-frag": true, "--wrong-chksum": true, "--wrong-seq": true, "--allow-no-sni": true}
	numValue = map[string]bool{"-f": true, "-k": true, "-e": true, "--port": true, "--set-ttl": true, "--min-ttl": true, "--max-payload": true}
	digits   = regexp.MustCompile(`^[0-9]+$`)
	ttlSpec  = regexp.MustCompile(`^[0-9]+-[0-9]+-[0-9]+$`)
)

// ValidateCustom tokenizes user-typed GoodbyeDPI arguments (double quotes
// group words) and accepts only known, safe flags.
func ValidateCustom(s string) ([]string, error) {
	toks, err := tokenize(s)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case noValue[t]:
			out = append(out, t)
		case numValue[t]:
			if i+1 >= len(toks) || !digits.MatchString(toks[i+1]) {
				return nil, fmt.Errorf("dpi: %s needs a number", t)
			}
			out = append(out, t, toks[i+1])
			i++
		case t == "--ip-id":
			if i+1 >= len(toks) || !digits.MatchString(toks[i+1]) {
				return nil, fmt.Errorf("dpi: --ip-id needs a number")
			}
			out = append(out, t, toks[i+1])
			i++
		case t == "--auto-ttl":
			out = append(out, t)
			if i+1 < len(toks) && ttlSpec.MatchString(toks[i+1]) {
				out = append(out, toks[i+1])
				i++
			}
		default:
			return nil, fmt.Errorf("%w: %q", ErrForbiddenFlag, t)
		}
	}
	return out, nil
}

func tokenize(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote, have = !inQuote, true
		case (r == ' ' || r == '\t') && !inQuote:
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, errors.New("dpi: unbalanced quote")
	}
	if have {
		out = append(out, cur.String())
	}
	return out, nil
}
