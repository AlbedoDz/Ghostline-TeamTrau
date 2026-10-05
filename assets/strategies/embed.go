// Package strategies embeds the built-in zapret2 strategy list, used until a
// newer signed list has been downloaded.
package strategies

import _ "embed"

// BuiltinJSON is assets/strategies/strategies.json.
//
//go:embed strategies.json
var BuiltinJSON []byte
