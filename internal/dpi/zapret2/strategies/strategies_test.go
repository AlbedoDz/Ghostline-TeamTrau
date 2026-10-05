package strategies_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	builtin "github.com/hashcott/ghostline/assets/strategies"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/stretchr/testify/require"
)

func okValidator([]string) error { return nil }

func strat(id string) strategies.Strategy {
	return strategies.Strategy{ID: id, Name: map[string]string{"vi": "v", "en": "e"}, TCP: []string{"--lua-desync=pass"}}
}

func listJSON(t *testing.T, version int, ss ...strategies.Strategy) []byte {
	t.Helper()
	b, err := json.Marshal(strategies.List{Version: version, Zapret2: ss})
	require.NoError(t, err)
	return b
}

func TestParse_Valid(t *testing.T) {
	l, err := strategies.Parse(listJSON(t, 2, strat("a"), strat("b-2")), okValidator)
	require.NoError(t, err)
	require.Equal(t, 2, l.Version)
	require.Len(t, l.Zapret2, 2)
}

func TestParse_Limits(t *testing.T) {
	many := make([]strategies.Strategy, 33)
	for i := range many {
		many[i] = strat(fmt.Sprintf("s%d", i))
	}
	longTCP := strat("a")
	longTCP.TCP = make([]string, 17)
	for i := range longTCP.TCP {
		longTCP.TCP[i] = "--lua-desync=pass"
	}
	longQUIC := strat("a")
	longQUIC.QUIC = longTCP.TCP
	noVi := strat("a")
	noVi.Name = map[string]string{"en": "e"}
	noEn := strat("a")
	noEn.Name = map[string]string{"vi": "v"}
	noTCP := strat("a")
	noTCP.TCP = nil
	cases := map[string][]byte{
		"too many":     listJSON(t, 1, many...),
		"long tcp":     listJSON(t, 1, longTCP),
		"long quic":    listJSON(t, 1, longQUIC),
		"bad id":       listJSON(t, 1, strat("Bad_ID")),
		"long id":      listJSON(t, 1, strat(strings.Repeat("a", 33))),
		"duplicate id": listJSON(t, 1, strat("a"), strat("a")),
		"no vi":        listJSON(t, 1, noVi),
		"no en":        listJSON(t, 1, noEn),
		"empty tcp":    listJSON(t, 1, noTCP),
		"version 0":    listJSON(t, 0, strat("a")),
		"empty":        listJSON(t, 1),
		"custom id":    listJSON(t, 1, strat("custom")),
		"bad json":     []byte(`{"version":1,"zapret2":[`),
	}
	for name, raw := range cases {
		_, err := strategies.Parse(raw, okValidator)
		require.ErrorIs(t, err, strategies.ErrInvalid, name)
	}
}

func TestParse_ValidatorAppliedToEveryArg(t *testing.T) {
	s := strat("a")
	s.QUIC = []string{"bad"}
	v := func(args []string) error {
		for _, a := range args {
			if a == "bad" {
				return errors.New("nope")
			}
		}
		return nil
	}
	_, err := strategies.Parse(listJSON(t, 1, s), v)
	require.ErrorIs(t, err, strategies.ErrInvalid)
}

type keys struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func newKeys(t *testing.T) keys {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return keys{pub, priv}
}

func TestSelect_NoRemote(t *testing.T) {
	k := newKeys(t)
	b := listJSON(t, 1, strat("a"))
	l, err := strategies.Select(b, nil, nil, k.pub, okValidator)
	require.NoError(t, err)
	require.Equal(t, "a", l.Zapret2[0].ID)
}

func TestSelect_NewerValidRemote(t *testing.T) {
	k := newKeys(t)
	remote := listJSON(t, 2, strat("r"))
	l, err := strategies.Select(listJSON(t, 1, strat("a")), remote, servers.Sign(remote, k.priv), k.pub, okValidator)
	require.NoError(t, err)
	require.Equal(t, "r", l.Zapret2[0].ID)
}

func TestSelect_RejectedRemotesKeepBuiltin(t *testing.T) {
	k := newKeys(t)
	other := newKeys(t)
	b := listJSON(t, 2, strat("a"))
	same := listJSON(t, 2, strat("r"))
	older := listJSON(t, 1, strat("r"))
	newer := listJSON(t, 3, strat("r"))
	broken := []byte(`{"version":9`)
	cases := map[string][2][]byte{
		"bad signature": {newer, servers.Sign(newer, other.priv)},
		"no signature":  {newer, nil},
		"same version":  {same, servers.Sign(same, k.priv)},
		"older version": {older, servers.Sign(older, k.priv)},
		"bad json":      {broken, servers.Sign(broken, k.priv)},
	}
	for name, c := range cases {
		l, err := strategies.Select(b, c[0], c[1], k.pub, okValidator)
		require.Error(t, err, name)
		require.Equal(t, "a", l.Zapret2[0].ID, name)
	}
}

func TestSelect_RemoteWithForbiddenArgRejected(t *testing.T) { // Review Focus #5
	k := newKeys(t)
	evil := strat("evil")
	evil.TCP = []string{"--lua-desync=luaexec:x=1"}
	remote := listJSON(t, 99, evil)
	l, err := strategies.Select(builtin.BuiltinJSON, remote, servers.Sign(remote, k.priv), k.pub, zapret2.ValidateArgs)
	require.ErrorIs(t, err, strategies.ErrInvalid)
	require.Equal(t, "z-split", l.Zapret2[0].ID)
}

func TestBuiltinIsValid(t *testing.T) {
	l, err := strategies.Parse(builtin.BuiltinJSON, zapret2.ValidateArgs)
	require.NoError(t, err)
	var ids []string
	for _, s := range l.Zapret2 {
		ids = append(ids, s.ID)
	}
	require.Equal(t, []string{"z-split", "z-disorder", "z-fake", "z-fake-ttl"}, ids)
}
