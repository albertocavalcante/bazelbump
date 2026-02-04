// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"

	"github.com/albertocavalcante/bazelbump/pkg/project"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// projectContext caches the detected project context for the interpreter.
// This avoids re-detecting the project for each builtin call.
var projectContextCache = make(map[string]*project.Context)

// getProjectContext returns the cached project context or detects it.
func (i *Interpreter) getProjectContext() (*project.Context, error) {
	if i.projectPath == "" {
		return nil, fmt.Errorf("project context builtins require a project path (use WithProjectPath)")
	}

	// Check cache
	if ctx, ok := projectContextCache[i.projectPath]; ok {
		return ctx, nil
	}

	// Detect project context
	ctx, err := project.Detect(i.projectPath)
	if err != nil {
		return nil, err
	}

	// Cache the result
	projectContextCache[i.projectPath] = ctx
	return ctx, nil
}

// builtinGetProjectContext implements get_project_context().
// Returns a struct with project information:
//   - root: string - Project root directory
//   - bazel_version: string - Bazel version from .bazelversion (empty if not found)
//   - use_bzlmod: bool - True if MODULE.bazel exists
//   - use_workspace: bool - True if WORKSPACE exists
//   - module_name: string - Module name from MODULE.bazel
//   - module_version: string - Module version from MODULE.bazel
func (i *Interpreter) builtinGetProjectContext(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("get_project_context", args, kwargs); err != nil {
		return nil, err
	}

	ctx, err := i.getProjectContext()
	if err != nil {
		return nil, fmt.Errorf("get_project_context: %w", err)
	}

	// Convert to Starlark struct
	bazelVersionStr := ""
	if ctx.HasBazelVersion {
		bazelVersionStr = ctx.BazelVersion.String()
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"root":           starlark.String(ctx.Root),
		"bazel_version":  starlark.String(bazelVersionStr),
		"use_bzlmod":     starlark.Bool(ctx.UseBzlmod),
		"use_workspace":  starlark.Bool(ctx.UseWorkspace),
		"module_name":    starlark.String(ctx.ModuleName),
		"module_version": starlark.String(ctx.ModuleVersion),
	}), nil
}

// builtinGetBazelDeps implements get_bazel_deps().
// Returns a list of dicts, each representing a bazel_dep() entry:
//   - name: string - Module name
//   - version: string - Module version
//   - repo_name: string - Optional repo_name override (empty if not set)
//   - dev_dependency: bool - True if this is a dev dependency
func (i *Interpreter) builtinGetBazelDeps(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("get_bazel_deps", args, kwargs); err != nil {
		return nil, err
	}

	ctx, err := i.getProjectContext()
	if err != nil {
		return nil, fmt.Errorf("get_bazel_deps: %w", err)
	}

	// Convert BazelDeps to list of dicts
	deps := make([]starlark.Value, 0, len(ctx.BazelDeps))
	for _, dep := range ctx.BazelDeps {
		depDict := starlark.NewDict(4)
		depDict.SetKey(starlark.String("name"), starlark.String(dep.Name))
		depDict.SetKey(starlark.String("version"), starlark.String(dep.Version))
		depDict.SetKey(starlark.String("repo_name"), starlark.String(dep.RepoName))
		depDict.SetKey(starlark.String("dev_dependency"), starlark.Bool(dep.DevDependency))
		deps = append(deps, depDict)
	}

	return starlark.NewList(deps), nil
}

// builtinGetBazelCompatibility implements get_bazel_compatibility().
// Returns a list of compatibility constraint strings from MODULE.bazel.
// Example: [">=6.4.0", "<=9.0.0", "-7.1.0"]
func (i *Interpreter) builtinGetBazelCompatibility(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("get_bazel_compatibility", args, kwargs); err != nil {
		return nil, err
	}

	ctx, err := i.getProjectContext()
	if err != nil {
		return nil, fmt.Errorf("get_bazel_compatibility: %w", err)
	}

	// Convert to Starlark list
	constraints := make([]starlark.Value, 0, len(ctx.Compatibility))
	for _, c := range ctx.Compatibility {
		constraints = append(constraints, starlark.String(c))
	}

	return starlark.NewList(constraints), nil
}

// ClearProjectContextCache clears the project context cache.
// This is useful for testing or when the project files may have changed.
func ClearProjectContextCache() {
	projectContextCache = make(map[string]*project.Context)
}
