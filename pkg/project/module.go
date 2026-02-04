// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"fmt"
	"os"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

// ModuleFile represents a parsed MODULE.bazel file.
type ModuleFile struct {
	// Path is the path to the MODULE.bazel file.
	Path string

	// Name is the module name from module().
	Name string

	// Version is the module version from module().
	Version string

	// Compatibility contains bazel_compatibility constraints from module().
	Compatibility []string

	// BazelDeps contains the parsed bazel_dep() entries.
	BazelDeps []BazelDep

	// Raw is the underlying bzl.File for advanced operations.
	Raw *bzl.File
}

// ParseModuleBazel parses a MODULE.bazel file and extracts relevant information.
func ParseModuleBazel(path string) (*ModuleFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read MODULE.bazel: %w", err)
	}

	return ParseModuleBazelContent(path, content)
}

// ParseModuleBazelContent parses MODULE.bazel content from bytes.
func ParseModuleBazelContent(path string, content []byte) (*ModuleFile, error) {
	f, err := bzl.Parse(path, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MODULE.bazel: %w", err)
	}

	mf := &ModuleFile{
		Path: path,
		Raw:  f,
	}

	// Process all statements in the file
	for _, stmt := range f.Stmt {
		call, ok := stmt.(*bzl.CallExpr)
		if !ok {
			continue
		}

		// Get the function name
		funcName := getFunctionName(call)
		if funcName == "" {
			continue
		}

		switch funcName {
		case "module":
			parseModuleCall(call, mf)
		case "bazel_dep":
			dep := parseBazelDepCall(call)
			if dep != nil {
				mf.BazelDeps = append(mf.BazelDeps, *dep)
			}
		}
	}

	return mf, nil
}

// getFunctionName extracts the function name from a CallExpr.
func getFunctionName(call *bzl.CallExpr) string {
	switch x := call.X.(type) {
	case *bzl.Ident:
		return x.Name
	case *bzl.DotExpr:
		// For expressions like foo.bar(), get "bar"
		return x.Name
	}
	return ""
}

// parseModuleCall extracts information from a module() call.
func parseModuleCall(call *bzl.CallExpr, mf *ModuleFile) {
	for _, kv := range call.List {
		assignExpr, ok := kv.(*bzl.AssignExpr)
		if !ok {
			continue
		}

		key, ok := assignExpr.LHS.(*bzl.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "name":
			if str, ok := assignExpr.RHS.(*bzl.StringExpr); ok {
				mf.Name = str.Value
			}
		case "version":
			if str, ok := assignExpr.RHS.(*bzl.StringExpr); ok {
				mf.Version = str.Value
			}
		case "bazel_compatibility":
			if list, ok := assignExpr.RHS.(*bzl.ListExpr); ok {
				for _, elem := range list.List {
					if str, ok := elem.(*bzl.StringExpr); ok {
						mf.Compatibility = append(mf.Compatibility, str.Value)
					}
				}
			}
		}
	}
}

// parseBazelDepCall extracts a BazelDep from a bazel_dep() call.
func parseBazelDepCall(call *bzl.CallExpr) *BazelDep {
	dep := &BazelDep{}

	for _, kv := range call.List {
		assignExpr, ok := kv.(*bzl.AssignExpr)
		if !ok {
			continue
		}

		key, ok := assignExpr.LHS.(*bzl.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "name":
			if str, ok := assignExpr.RHS.(*bzl.StringExpr); ok {
				dep.Name = str.Value
			}
		case "version":
			if str, ok := assignExpr.RHS.(*bzl.StringExpr); ok {
				dep.Version = str.Value
			}
		case "repo_name":
			if str, ok := assignExpr.RHS.(*bzl.StringExpr); ok {
				dep.RepoName = str.Value
			}
		case "dev_dependency":
			if ident, ok := assignExpr.RHS.(*bzl.Ident); ok {
				dep.DevDependency = ident.Name == "True"
			}
		}
	}

	// A bazel_dep must have at least a name
	if dep.Name == "" {
		return nil
	}

	return dep
}

// GetBazelDep returns the BazelDep with the given name, or nil if not found.
func (mf *ModuleFile) GetBazelDep(name string) *BazelDep {
	for i := range mf.BazelDeps {
		if mf.BazelDeps[i].Name == name {
			return &mf.BazelDeps[i]
		}
	}
	return nil
}

// HasBazelDep returns true if the module depends on the given module name.
func (mf *ModuleFile) HasBazelDep(name string) bool {
	return mf.GetBazelDep(name) != nil
}
