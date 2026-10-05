// Package goodbyedpi is the GoodbyeDPI engine: presets and custom-argument
// checks. Presets never use --dns-addr/--dns-port: Ghostline's engine
// already encrypts DNS, and a second redirect would fight it.
package goodbyedpi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashcott/ghostline/internal/dpi"
)

// Pinned holds the SHA-256 of GoodbyeDPI 0.2.3rc3 x86_64.
var Pinned = map[string]string{
	"goodbyedpi.exe":  "8d412b094bb9c137ff25ba9a794d1122ecc84bb776debff6c249723a13cc31cd",
	"WinDivert.dll":   "6110bfa44667405179c3e15e12af1b62037e447ed59b054b19042032995e6c7e",
	"WinDivert64.sys": "e69b5ba3f0cd6cfb2983e442636e7f0b342b61b15264b0328317d4559c82cf50",
}

type engine struct{}

// New returns the GoodbyeDPI engine.
func New() dpi.Engine { return engine{} }

func (engine) ID() string               { return "goodbyedpi" }
func (engine) Exe() string              { return "goodbyedpi.exe" }
func (engine) HotReloadsLists() bool    { return false }
func (engine) Files() map[string]string { return Pinned }

func (engine) Strategies() []dpi.Strategy {
	return []dpi.Strategy{
		{ID: "light", Name: map[string]string{"vi": "Nhẹ", "en": "Light"}},
		{ID: "medium", Name: map[string]string{"vi": "Vừa", "en": "Medium"}},
		{ID: "high", Name: map[string]string{"vi": "Mạnh", "en": "Strong"}},
		{ID: "extreme", Name: map[string]string{"vi": "Rất mạnh", "en": "Extreme"}},
	}
}

func (engine) ValidateCustom(s string) error { _, err := validateCustom(s); return err }

var lightArgs = []string{"-p", "-r", "-s", "-m", "-e", "40", "-w", "--native-frag"}

func presetArgs(p string) ([]string, bool) {
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
	if len(p) == 5 && strings.HasPrefix(p, "mode") && p[4] >= '1' && p[4] <= '6' {
		return []string{"-" + p[4:]}, true
	}
	return nil, false
}

// Args builds the argv for a preset (or custom args) and scope. Each element
// is one argument, so paths with spaces are safe.
func (engine) Args(p dpi.Plan) ([]string, error) {
	var args []string
	if p.Strategy == "custom" {
		var err error
		if args, err = validateCustom(p.Custom); err != nil {
			return nil, err
		}
	} else {
		var ok bool
		if args, ok = presetArgs(p.Strategy); !ok {
			return nil, fmt.Errorf("goodbyedpi: unknown preset %q", p.Strategy)
		}
	}
	if p.Scope == dpi.ScopeBlacklist {
		args = append(args, "--blacklist", p.Blacklist)
	}
	return args, nil
}

var (
	noValue = map[string]bool{"-p": true, "-r": true, "-s": true, "-m": true, "-n": true, "-a": true, "-w": true,
		"-1": true, "-2": true, "-3": true, "-4": true, "-5": true, "-6": true,
		"--native-frag": true, "--reverse-frag": true, "--wrong-chksum": true, "--wrong-seq": true, "--allow-no-sni": true}
	numValue = map[string]bool{"-f": true, "-k": true, "-e": true, "--port": true, "--set-ttl": true, "--min-ttl": true,
		"--max-payload": true, "--ip-id": true}
	digits  = regexp.MustCompile(`^[0-9]+$`)
	ttlSpec = regexp.MustCompile(`^[0-9]+-[0-9]+-[0-9]+$`)
	domain  = regexp.MustCompile(`^([a-z0-9-]{1,63}\.)+[a-z]{2,63}$`)
)

// validateCustom tokenizes user-typed GoodbyeDPI arguments (double quotes
// group words) and accepts only known, safe flags.
func validateCustom(s string) ([]string, error) {
	toks, err := dpi.Tokenize(s)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		v, hasNext := "", i+1 < len(toks)
		if hasNext {
			v = toks[i+1]
		}
		switch {
		case noValue[t]:
			out = append(out, t)
		case numValue[t]:
			if !hasNext || !digits.MatchString(v) {
				return nil, fmt.Errorf("goodbyedpi: %s needs a number", t)
			}
			out = append(out, t, v)
			i++
		case t == "--auto-ttl":
			out = append(out, t)
			if hasNext && ttlSpec.MatchString(v) {
				out = append(out, v)
				i++
			}
		case t == "--fake-with-sni":
			if !hasNext || !domain.MatchString(strings.ToLower(v)) {
				return nil, fmt.Errorf("goodbyedpi: --fake-with-sni needs a domain name")
			}
			out = append(out, t, strings.ToLower(v))
			i++
		case t == "--fake-gen":
			n, err := strconv.Atoi(v)
			if !hasNext || !digits.MatchString(v) || err != nil || n < 1 || n > 30 {
				return nil, fmt.Errorf("goodbyedpi: --fake-gen needs a number from 1 to 30")
			}
			out = append(out, t, v)
			i++
		default:
			return nil, fmt.Errorf("%w: %q", dpi.ErrForbiddenFlag, t)
		}
	}
	return out, nil
}
