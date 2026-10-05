package zapret2

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashcott/ghostline/internal/dpi"
)

// desyncFuncs are the zapret-antidpi.lua functions a strategy may call.
// Everything else is refused, notably luaexec (runs arbitrary Lua as admin)
// and the debug helpers.
var desyncFuncs = map[string]bool{
	"fake": true, "multisplit": true, "multidisorder": true, "fakedsplit": true, "fakeddisorder": true,
	"hostfakesplit": true, "tcpseg": true, "oob": true, "wsize": true, "wssize": true, "syndata": true,
	"synack": true, "synack_split": true, "tls_client_hello_clone": true, "http_domcase": true,
	"http_hostcase": true, "http_methodeol": true, "http_unixeol": true, "udplen": true, "drop": true, "pass": true,
}

// builtinBlobs are the blobs winws2 defines itself.
var builtinBlobs = map[string]bool{"fake_default_tls": true, "fake_default_http": true, "fake_default_quic": true}

var (
	argKey    = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	argValue  = regexp.MustCompile(`^[A-Za-z0-9_.,+\-]{0,64}$`)
	hexDigits = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)
	payloads  = regexp.MustCompile(`^[a-z0-9_]{1,32}(,[a-z0-9_]{1,32})*$`)
	rangeArg  = regexp.MustCompile(`^[nadsp]?[0-9]{0,10}(-|<)[nadsp]?[0-9]{0,10}$`)
)

// ValidateArgs accepts only strategy arguments that call allow-listed Lua
// functions with plain values: no file references, no new profiles, no
// interception or init flags. Ghostline adds those itself.
func ValidateArgs(args []string) error {
	for _, a := range args {
		if !validArg(a) {
			return fmt.Errorf("%w: %q", dpi.ErrForbiddenFlag, a)
		}
	}
	return nil
}

func validArg(a string) bool {
	switch {
	case strings.HasPrefix(a, "--lua-desync="):
		return validDesync(strings.TrimPrefix(a, "--lua-desync="))
	case strings.HasPrefix(a, "--payload="):
		return payloads.MatchString(strings.TrimPrefix(a, "--payload="))
	case strings.HasPrefix(a, "--out-range="):
		return rangeArg.MatchString(strings.TrimPrefix(a, "--out-range="))
	case strings.HasPrefix(a, "--in-range="):
		return rangeArg.MatchString(strings.TrimPrefix(a, "--in-range="))
	}
	return false
}

// validDesync checks "<fn>[:key[=value]]...".
func validDesync(s string) bool {
	parts := strings.Split(s, ":")
	if !desyncFuncs[parts[0]] {
		return false
	}
	for _, p := range parts[1:] {
		k, v, _ := strings.Cut(p, "=")
		if !argKey.MatchString(k) {
			return false
		}
		if k == "blob" || k == "seqovl_pattern" {
			if !builtinBlobs[v] && !isHexBlob(v) {
				return false
			}
			continue
		}
		if !argValue.MatchString(v) {
			return false
		}
	}
	return true
}

// isHexBlob is ^0x[0-9a-fA-F]{2,2048}$; RE2 caps repeats at 1000.
func isHexBlob(v string) bool {
	return len(v) >= 4 && len(v) <= 2+2048 && hexDigits.MatchString(v)
}

// ValidateCustom tokenizes user-typed zapret2 arguments (double quotes group
// words) and checks them like strategy arguments.
func ValidateCustom(s string) ([]string, error) {
	toks, err := dpi.Tokenize(s)
	if err != nil {
		return nil, err
	}
	if err := ValidateArgs(toks); err != nil {
		return nil, err
	}
	return toks, nil
}
