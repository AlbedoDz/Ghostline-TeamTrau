// Package strategies reads the zapret2 strategy list: a built-in copy and a
// signed copy downloaded on a schedule.
package strategies

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashcott/ghostline/internal/servers"
)

// Strategy is one zapret2 strategy: the desync args of its TCP profile and,
// optionally, of its QUIC profile.
type Strategy struct {
	ID   string            `json:"id"`
	Name map[string]string `json:"name"`
	TCP  []string          `json:"tcp"`
	QUIC []string          `json:"quic"`
}

// List is strategies.json. The order of Zapret2 is the autotune order.
type List struct {
	Version int        `json:"version"`
	Zapret2 []Strategy `json:"zapret2"`
}

// Validator checks one profile's args (zapret2.ValidateArgs).
type Validator func(args []string) error

// ErrInvalid means a list was malformed, over the limits or refused by the
// validator.
var ErrInvalid = errors.New("strategies: invalid list")

const (
	maxStrategies = 32
	maxArgs       = 16
)

var strategyID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// Parse decodes and checks a list.
func Parse(raw []byte, v Validator) (List, error) {
	var l List
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if l.Version < 1 {
		return List{}, fmt.Errorf("%w: version %d", ErrInvalid, l.Version)
	}
	if len(l.Zapret2) == 0 || len(l.Zapret2) > maxStrategies {
		return List{}, fmt.Errorf("%w: %d strategies", ErrInvalid, len(l.Zapret2))
	}
	seen := map[string]bool{}
	for _, s := range l.Zapret2 {
		if !strategyID.MatchString(s.ID) || s.ID == "custom" || seen[s.ID] {
			return List{}, fmt.Errorf("%w: id %q", ErrInvalid, s.ID)
		}
		seen[s.ID] = true
		if s.Name["vi"] == "" || s.Name["en"] == "" {
			return List{}, fmt.Errorf("%w: %s: name needs vi and en", ErrInvalid, s.ID)
		}
		if len(s.TCP) == 0 || len(s.TCP) > maxArgs || len(s.QUIC) > maxArgs {
			return List{}, fmt.Errorf("%w: %s: 1-%d tcp and 0-%d quic args", ErrInvalid, s.ID, maxArgs, maxArgs)
		}
		for _, args := range [][]string{s.TCP, s.QUIC} {
			if err := v(args); err != nil {
				return List{}, fmt.Errorf("%w: %s: %v", ErrInvalid, s.ID, err)
			}
		}
	}
	return l, nil
}

// Select returns the remote list when its signature verifies, its Version is
// greater than the builtin's and Parse accepts it; otherwise the builtin and
// a non-nil reason (nil when there is no remote file). The builtin is
// trusted to parse: a test checks it.
func Select(builtin, remote, sig []byte, pub ed25519.PublicKey, v Validator) (List, error) {
	b, err := Parse(builtin, v)
	if err != nil {
		return List{}, fmt.Errorf("strategies: built-in list: %w", err)
	}
	if remote == nil {
		return b, nil
	}
	if err := servers.VerifySigned(remote, sig, pub); err != nil {
		return b, err
	}
	r, err := Parse(remote, v)
	if err != nil {
		return b, err
	}
	if r.Version <= b.Version {
		return b, fmt.Errorf("strategies: remote version %d is not newer than %d", r.Version, b.Version)
	}
	return r, nil
}
