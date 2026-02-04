// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"

	"github.com/albertocavalcante/bazelbump/flagdb"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// makeFlagsModule creates the `flags` module with the following functions:
//   - flags.exists(name, version) -> bool
//   - flags.get(name, version) -> struct{name, default, deprecated, ...}
//   - flags.diff(v1, v2) -> struct{added, removed, changed}
//   - flags.incompatible(version) -> list[Flag]
//   - flags.deprecated(version) -> list[Flag]
func makeFlagsModule(db *flagdb.DB) starlark.Value {
	if db == nil {
		// Return a module that returns errors when called without a flagdb
		return makeModule("flags", map[string]*starlark.Builtin{
			"exists":       starlark.NewBuiltin("flags.exists", builtinFlagsDisabled),
			"get":          starlark.NewBuiltin("flags.get", builtinFlagsDisabled),
			"diff":         starlark.NewBuiltin("flags.diff", builtinFlagsDisabled),
			"incompatible": starlark.NewBuiltin("flags.incompatible", builtinFlagsDisabled),
			"deprecated":   starlark.NewBuiltin("flags.deprecated", builtinFlagsDisabled),
		})
	}

	return makeModule("flags", map[string]*starlark.Builtin{
		"exists":       starlark.NewBuiltin("flags.exists", builtinFlagsExists(db)),
		"get":          starlark.NewBuiltin("flags.get", builtinFlagsGet(db)),
		"diff":         starlark.NewBuiltin("flags.diff", builtinFlagsDiff(db)),
		"incompatible": starlark.NewBuiltin("flags.incompatible", builtinFlagsIncompatible(db)),
		"deprecated":   starlark.NewBuiltin("flags.deprecated", builtinFlagsDeprecated(db)),
	})
}

// builtinFlagsDisabled returns an error when flags module is used without a flagdb.
func builtinFlagsDisabled(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("flags module requires flag database (run with --flagdb-path)")
}

// builtinFlagsExists implements flags.exists(name, version).
// Returns true if the flag exists in the given Bazel version.
func builtinFlagsExists(db *flagdb.DB) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name    string
			version string
		)

		if err := starlark.UnpackArgs("flags.exists", args, kwargs,
			"name", &name,
			"version", &version,
		); err != nil {
			return nil, err
		}

		flag, err := db.Flag(version, name)
		if err != nil {
			return nil, fmt.Errorf("flags.exists: %w", err)
		}

		return starlark.Bool(flag != nil), nil
	}
}

// builtinFlagsGet implements flags.get(name, version).
// Returns a struct with flag information or None if not found.
func builtinFlagsGet(db *flagdb.DB) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name    string
			version string
		)

		if err := starlark.UnpackArgs("flags.get", args, kwargs,
			"name", &name,
			"version", &version,
		); err != nil {
			return nil, err
		}

		flag, err := db.Flag(version, name)
		if err != nil {
			return nil, fmt.Errorf("flags.get: %w", err)
		}

		if flag == nil {
			return starlark.None, nil
		}

		return flagToStarlark(flag), nil
	}
}

// builtinFlagsDiff implements flags.diff(v1, v2).
// Returns a struct with added, removed, and changed flags between two versions.
func builtinFlagsDiff(db *flagdb.DB) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			v1 string
			v2 string
		)

		if err := starlark.UnpackArgs("flags.diff", args, kwargs,
			"v1", &v1,
			"v2", &v2,
		); err != nil {
			return nil, err
		}

		diff, err := db.Diff(v1, v2)
		if err != nil {
			return nil, fmt.Errorf("flags.diff: %w", err)
		}

		// Convert added flags
		added := make([]starlark.Value, 0, len(diff.Added))
		for i := range diff.Added {
			added = append(added, flagToStarlark(&diff.Added[i]))
		}

		// Convert removed flags
		removed := make([]starlark.Value, 0, len(diff.Removed))
		for i := range diff.Removed {
			removed = append(removed, flagToStarlark(&diff.Removed[i]))
		}

		// Convert changed flags
		changed := make([]starlark.Value, 0, len(diff.Changed))
		for _, fc := range diff.Changed {
			changed = append(changed, starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
				"name":        starlark.String(fc.Name),
				"from":        flagToStarlark(fc.From),
				"to":          flagToStarlark(fc.To),
				"change_type": starlark.String(fc.Change.String()),
			}))
		}

		return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"from_version": starlark.String(diff.FromVersion),
			"to_version":   starlark.String(diff.ToVersion),
			"added":        starlark.NewList(added),
			"removed":      starlark.NewList(removed),
			"changed":      starlark.NewList(changed),
		}), nil
	}
}

// builtinFlagsIncompatible implements flags.incompatible(version).
// Returns a list of all --incompatible_* flags for the given version.
func builtinFlagsIncompatible(db *flagdb.DB) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var version string

		if err := starlark.UnpackArgs("flags.incompatible", args, kwargs,
			"version", &version,
		); err != nil {
			return nil, err
		}

		flags, err := db.IncompatibleFlags(version)
		if err != nil {
			return nil, fmt.Errorf("flags.incompatible: %w", err)
		}

		result := make([]starlark.Value, 0, len(flags))
		for i := range flags {
			result = append(result, flagToStarlark(&flags[i]))
		}

		return starlark.NewList(result), nil
	}
}

// builtinFlagsDeprecated implements flags.deprecated(version).
// Returns a list of all deprecated flags for the given version.
func builtinFlagsDeprecated(db *flagdb.DB) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var version string

		if err := starlark.UnpackArgs("flags.deprecated", args, kwargs,
			"version", &version,
		); err != nil {
			return nil, err
		}

		flags, err := db.DeprecatedFlags(version)
		if err != nil {
			return nil, fmt.Errorf("flags.deprecated: %w", err)
		}

		result := make([]starlark.Value, 0, len(flags))
		for i := range flags {
			result = append(result, flagToStarlark(&flags[i]))
		}

		return starlark.NewList(result), nil
	}
}

// flagToStarlark converts a flagdb.Flag to a Starlark struct.
func flagToStarlark(f *flagdb.Flag) starlark.Value {
	// Convert tags to Starlark list
	tags := make([]starlark.Value, 0, len(f.Tags))
	for _, tag := range f.Tags {
		tags = append(tags, starlark.String(tag))
	}

	// Convert sources to Starlark list
	sources := make([]starlark.Value, 0, len(f.Sources))
	for _, src := range f.Sources {
		sources = append(sources, starlark.String(src))
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"name":           starlark.String(f.Name),
		"short":          starlark.String(f.Short),
		"type":           starlark.String(f.Type),
		"default":        starlark.String(f.Default),
		"description":    starlark.String(f.Description),
		"tags":           starlark.NewList(tags),
		"sources":        starlark.NewList(sources),
		"is_deprecated":  starlark.Bool(f.IsDeprecated()),
		"is_experimental": starlark.Bool(f.IsExperimental()),
		"is_incompatible": starlark.Bool(f.IsIncompatibleChange()),
	})
}
