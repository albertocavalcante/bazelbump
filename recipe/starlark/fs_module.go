// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"go.starlark.net/starlark"
)

// makeFsModule creates the `fs` module with the following functions:
//   - fs.read(path) -> read_file()
//   - fs.exists(path) -> file_exists()
//   - fs.glob(pattern) -> glob()
//   - fs.list_dir(path) -> list_dir()
//   - fs.parse_build(path) -> parse_build_file()
func (i *Interpreter) makeFsModule() starlark.Value {
	return makeModule("fs", map[string]*starlark.Builtin{
		"read":        starlark.NewBuiltin("fs.read", i.builtinReadFile),
		"exists":      starlark.NewBuiltin("fs.exists", i.builtinFileExists),
		"glob":        starlark.NewBuiltin("fs.glob", i.builtinGlob),
		"list_dir":    starlark.NewBuiltin("fs.list_dir", i.builtinListDir),
		"parse_build": starlark.NewBuiltin("fs.parse_build", builtinParseBuildFile(i.projectPath)),
	})
}
