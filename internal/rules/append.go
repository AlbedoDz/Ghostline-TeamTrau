package rules

import (
	"reflect"
	"slices"
)

// AppendUserRules validates add and appends the rules not already present
// (same pattern and action) to existing. Any invalid rule rejects the whole
// batch: existing comes back unchanged with one LineError per bad rule,
// numbered from 1 in add.
func AppendUserRules(existing, add []Rule, upstreamIDs []string) (out []Rule, added int, errs []LineError) {
	for i, r := range add {
		if err := ValidateRule(r, upstreamIDs); err != nil {
			errs = append(errs, LineError{Line: i + 1, Msg: err.Error()})
		}
	}
	if len(errs) > 0 {
		return existing, 0, errs
	}
	out = slices.Clone(existing)
	for _, r := range add {
		if slices.ContainsFunc(out, func(o Rule) bool { return o.Pattern == r.Pattern && reflect.DeepEqual(o.Action, r.Action) }) {
			continue
		}
		out = append(out, r)
		added++
	}
	if len(out) > MaxUserRules {
		return existing, 0, []LineError{{Line: 0, Msg: "too many rules"}}
	}
	return out, added, nil
}
