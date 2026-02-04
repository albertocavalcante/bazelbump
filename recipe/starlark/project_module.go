// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"go.starlark.net/starlark"
)

// makeProjectModule creates the `project` module with the following functions:
//   - project.context() -> get_project_context()
//   - project.deps() -> get_bazel_deps()
//   - project.compatibility() -> get_bazel_compatibility()
func (i *Interpreter) makeProjectModule() starlark.Value {
	return makeModule("project", map[string]*starlark.Builtin{
		"context":       starlark.NewBuiltin("project.context", i.builtinGetProjectContext),
		"deps":          starlark.NewBuiltin("project.deps", i.builtinGetBazelDeps),
		"compatibility": starlark.NewBuiltin("project.compatibility", i.builtinGetBazelCompatibility),
	})
}
