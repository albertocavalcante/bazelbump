// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"go.starlark.net/starlark"
)

// makeVersionModule creates the `version` module with the following functions:
//   - version.parse(s) -> parse_version()
//   - version.compare(v1, v2) -> version_compare()
//   - version.matches(v, cs) -> version_matches()
//   - version.at_least(v, min) -> version_at_least()
//   - version.in_range(v, min, max) -> version_in_range()
func makeVersionModule() starlark.Value {
	return makeModule("version", map[string]*starlark.Builtin{
		"parse":    starlark.NewBuiltin("version.parse", builtinParseVersion),
		"compare":  starlark.NewBuiltin("version.compare", builtinVersionCompare),
		"matches":  starlark.NewBuiltin("version.matches", builtinVersionMatches),
		"at_least": starlark.NewBuiltin("version.at_least", builtinVersionAtLeast),
		"in_range": starlark.NewBuiltin("version.in_range", builtinVersionInRange),
	})
}
