// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"

	"github.com/albertocavalcante/bazelbump/internal/version"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// builtinVersionMatches implements version_matches(version, constraints).
// Checks if a version string satisfies a list of constraint strings.
//
// Example:
//
//	version_matches("7.7.1", [">=6.4.0", "<=9.0.0"])  # True
//	version_matches("7.1.0", [">=6.4.0", "-7.1.0"])   # False (excluded)
//
// Parameters:
//   - version: string - The version to check
//   - constraints: list[string] - List of constraint strings (BCR-style)
//
// Returns: bool
func builtinVersionMatches(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		versionStr      string
		constraintsList *starlark.List
	)

	if err := starlark.UnpackArgs("version_matches", args, kwargs,
		"version", &versionStr,
		"constraints", &constraintsList,
	); err != nil {
		return nil, err
	}

	// Parse the version
	v, err := version.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("version_matches: invalid version %q: %w", versionStr, err)
	}

	// Convert constraints list to Go strings
	constraintStrs, err := stringListFromStarlark(constraintsList, "constraints")
	if err != nil {
		return nil, err
	}

	// Handle empty constraints (matches everything)
	if len(constraintStrs) == 0 {
		return starlark.True, nil
	}

	// Parse and check constraints
	cs, err := version.ParseConstraints(constraintStrs)
	if err != nil {
		return nil, fmt.Errorf("version_matches: %w", err)
	}

	return starlark.Bool(cs.Matches(v)), nil
}

// builtinVersionCompare implements version_compare(v1, v2).
// Compares two version strings and returns:
//   - -1 if v1 < v2
//   - 0 if v1 == v2
//   - 1 if v1 > v2
//
// Example:
//
//	version_compare("7.0.0", "8.0.0")  # -1
//	version_compare("8.0.0", "7.0.0")  # 1
//	version_compare("7.0.0", "7.0.0")  # 0
//
// Parameters:
//   - v1: string - First version
//   - v2: string - Second version
//
// Returns: int (-1, 0, or 1)
func builtinVersionCompare(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		v1Str string
		v2Str string
	)

	if err := starlark.UnpackArgs("version_compare", args, kwargs,
		"v1", &v1Str,
		"v2", &v2Str,
	); err != nil {
		return nil, err
	}

	// Parse versions
	v1, err := version.Parse(v1Str)
	if err != nil {
		return nil, fmt.Errorf("version_compare: invalid version v1=%q: %w", v1Str, err)
	}

	v2, err := version.Parse(v2Str)
	if err != nil {
		return nil, fmt.Errorf("version_compare: invalid version v2=%q: %w", v2Str, err)
	}

	return starlark.MakeInt(v1.Compare(v2)), nil
}

// builtinParseVersion implements parse_version(s).
// Parses a version string and returns a struct with components:
//   - major: int - Major version number
//   - minor: int - Minor version number
//   - patch: int - Patch version number
//   - prerelease: string - Prerelease suffix (empty if stable)
//   - string: string - Original version string
//
// Example:
//
//	v = parse_version("7.0.0-rc1")
//	v.major       # 7
//	v.minor       # 0
//	v.patch       # 0
//	v.prerelease  # "rc1"
//
// Parameters:
//   - s: string - Version string to parse
//
// Returns: struct
func builtinParseVersion(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var versionStr string

	if err := starlark.UnpackArgs("parse_version", args, kwargs,
		"s", &versionStr,
	); err != nil {
		return nil, err
	}

	// Parse the version
	v, err := version.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("parse_version: invalid version %q: %w", versionStr, err)
	}

	// Return as struct
	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"major":      starlark.MakeInt(v.Major()),
		"minor":      starlark.MakeInt(v.Minor()),
		"patch":      starlark.MakeInt(v.Patch()),
		"prerelease": starlark.String(v.Prerelease()),
		"string":     starlark.String(v.String()),
	}), nil
}

// builtinVersionAtLeast implements version_at_least(version, minimum).
// Checks if version >= minimum.
//
// Example:
//
//	version_at_least("8.0.0", "7.0.0")  # True
//	version_at_least("6.0.0", "7.0.0")  # False
//
// Parameters:
//   - version: string - The version to check
//   - minimum: string - The minimum required version
//
// Returns: bool
func builtinVersionAtLeast(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		versionStr string
		minimumStr string
	)

	if err := starlark.UnpackArgs("version_at_least", args, kwargs,
		"version", &versionStr,
		"minimum", &minimumStr,
	); err != nil {
		return nil, err
	}

	// Parse versions
	v, err := version.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("version_at_least: invalid version %q: %w", versionStr, err)
	}

	min, err := version.Parse(minimumStr)
	if err != nil {
		return nil, fmt.Errorf("version_at_least: invalid minimum %q: %w", minimumStr, err)
	}

	return starlark.Bool(v.AtLeast(min)), nil
}

// builtinVersionInRange implements version_in_range(version, min, max).
// Checks if min <= version <= max.
//
// Example:
//
//	version_in_range("7.5.0", "7.0.0", "8.0.0")  # True
//	version_in_range("9.0.0", "7.0.0", "8.0.0")  # False
//
// Parameters:
//   - version: string - The version to check
//   - min: string - The minimum version (inclusive)
//   - max: string - The maximum version (inclusive)
//
// Returns: bool
func builtinVersionInRange(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		versionStr string
		minStr     string
		maxStr     string
	)

	if err := starlark.UnpackArgs("version_in_range", args, kwargs,
		"version", &versionStr,
		"min", &minStr,
		"max", &maxStr,
	); err != nil {
		return nil, err
	}

	// Parse versions
	v, err := version.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("version_in_range: invalid version %q: %w", versionStr, err)
	}

	minV, err := version.Parse(minStr)
	if err != nil {
		return nil, fmt.Errorf("version_in_range: invalid min %q: %w", minStr, err)
	}

	maxV, err := version.Parse(maxStr)
	if err != nil {
		return nil, fmt.Errorf("version_in_range: invalid max %q: %w", maxStr, err)
	}

	return starlark.Bool(v.InRange(minV, maxV)), nil
}
