// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
)

// =============================================================================
// File I/O Builtins (sandboxed to project directory)
// =============================================================================

// sandboxPath validates and resolves a path to be within the project directory.
// Returns the absolute path if valid, or an error if the path escapes the sandbox.
func (i *Interpreter) sandboxPath(path string) (string, error) {
	if i.projectPath == "" {
		return "", fmt.Errorf("file I/O builtins require a project path (use WithProjectPath)")
	}

	// Reject absolute paths immediately - they cannot be sandboxed
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q escapes project directory", path)
	}

	// Get absolute project path
	absProjectPath, err := filepath.Abs(i.projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid project path: %v", err)
	}

	// Join with project path and clean
	fullPath := filepath.Join(absProjectPath, path)
	fullPath = filepath.Clean(fullPath)

	// Verify the path is within the project directory
	// Use filepath.Rel to check containment
	rel, err := filepath.Rel(absProjectPath, fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %v", err)
	}

	// If the relative path starts with "..", it's outside the project
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes project directory", path)
	}

	return fullPath, nil
}

// builtinReadFile implements the read_file(path, default=None) builtin.
// Reads file contents as a string. If the file doesn't exist and a default
// is provided, returns the default value instead of erroring.
func (i *Interpreter) builtinReadFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		path         string
		defaultValue starlark.Value = starlark.None
	)

	if err := starlark.UnpackArgs("read_file", args, kwargs,
		"path", &path,
		"default?", &defaultValue,
	); err != nil {
		return nil, err
	}

	// Sandbox the path
	fullPath, err := i.sandboxPath(path)
	if err != nil {
		return nil, err
	}

	// Read the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) && defaultValue != starlark.None {
			return defaultValue, nil
		}
		return nil, fmt.Errorf("read_file: %v", err)
	}

	return starlark.String(content), nil
}

// builtinFileExists implements the file_exists(path) builtin.
// Returns True if the file or directory exists, False otherwise.
func (i *Interpreter) builtinFileExists(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string

	if err := starlark.UnpackArgs("file_exists", args, kwargs,
		"path", &path,
	); err != nil {
		return nil, err
	}

	// Sandbox the path
	fullPath, err := i.sandboxPath(path)
	if err != nil {
		return nil, err
	}

	// Check if file exists
	_, err = os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return starlark.False, nil
		}
		return nil, fmt.Errorf("file_exists: %v", err)
	}

	return starlark.True, nil
}

// builtinListDir implements the list_dir(path, recursive=False) builtin.
// Returns a list of file/directory names in the given directory.
// If recursive is True, returns all files recursively with relative paths.
func (i *Interpreter) builtinListDir(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		path      string
		recursive bool
	)

	if err := starlark.UnpackArgs("list_dir", args, kwargs,
		"path", &path,
		"recursive?", &recursive,
	); err != nil {
		return nil, err
	}

	// Sandbox the path
	fullPath, err := i.sandboxPath(path)
	if err != nil {
		return nil, err
	}

	// Check that it's a directory
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("list_dir: %v", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("list_dir: %q is not a directory", path)
	}

	var entries []starlark.Value

	if recursive {
		// Walk recursively
		err = filepath.Walk(fullPath, func(entryPath string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // Skip entries we can't access
			}
			// Skip the root directory itself
			if entryPath == fullPath {
				return nil
			}
			// Get relative path from the starting directory
			relPath, err := filepath.Rel(fullPath, entryPath)
			if err != nil {
				return nil
			}
			entries = append(entries, starlark.String(relPath))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("list_dir: %v", err)
		}
	} else {
		// Non-recursive: just list immediate children
		dirEntries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, fmt.Errorf("list_dir: %v", err)
		}
		for _, entry := range dirEntries {
			entries = append(entries, starlark.String(entry.Name()))
		}
	}

	return starlark.NewList(entries), nil
}

// builtinGlob implements the glob(pattern, exclude=[]) builtin.
// Matches files by glob pattern relative to the project directory.
// Returns a list of matching file paths (relative to project root).
func (i *Interpreter) builtinGlob(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		pattern string
		exclude *starlark.List
	)

	if err := starlark.UnpackArgs("glob", args, kwargs,
		"pattern", &pattern,
		"exclude?", &exclude,
	); err != nil {
		return nil, err
	}

	if i.projectPath == "" {
		return nil, fmt.Errorf("glob: requires a project path (use WithProjectPath)")
	}

	// Get absolute project path
	absProjectPath, err := filepath.Abs(i.projectPath)
	if err != nil {
		return nil, fmt.Errorf("glob: invalid project path: %v", err)
	}

	// Convert exclude list to Go strings
	var excludePatterns []string
	if exclude != nil {
		excludePatterns, err = stringListFromStarlark(exclude, "exclude")
		if err != nil {
			return nil, err
		}
	}

	// Collect matching files
	var matches []starlark.Value

	// Handle ** patterns with filepath.Walk
	if strings.Contains(pattern, "**") {
		err = filepath.Walk(absProjectPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // Skip inaccessible entries
			}
			if info.IsDir() {
				// Skip hidden directories (except project root)
				if strings.HasPrefix(info.Name(), ".") && path != absProjectPath {
					return filepath.SkipDir
				}
				return nil
			}

			// Get relative path
			relPath, err := filepath.Rel(absProjectPath, path)
			if err != nil {
				return nil
			}

			// Check if it matches the pattern
			if matchGlobPattern(pattern, relPath) {
				// Check exclude patterns
				excluded := false
				for _, excl := range excludePatterns {
					if matchGlobPattern(excl, relPath) {
						excluded = true
						break
					}
				}
				if !excluded {
					matches = append(matches, starlark.String(relPath))
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("glob: %v", err)
		}
	} else {
		// Simple glob without **
		globPattern := filepath.Join(absProjectPath, pattern)
		matchedPaths, err := filepath.Glob(globPattern)
		if err != nil {
			return nil, fmt.Errorf("glob: invalid pattern %q: %v", pattern, err)
		}

		for _, matchPath := range matchedPaths {
			// Get relative path
			relPath, err := filepath.Rel(absProjectPath, matchPath)
			if err != nil {
				continue
			}

			// Skip directories
			info, err := os.Stat(matchPath)
			if err != nil || info.IsDir() {
				continue
			}

			// Check exclude patterns
			excluded := false
			for _, excl := range excludePatterns {
				if matchGlobPattern(excl, relPath) {
					excluded = true
					break
				}
			}
			if !excluded {
				matches = append(matches, starlark.String(relPath))
			}
		}
	}

	return starlark.NewList(matches), nil
}

// matchGlobPattern matches a path against a glob pattern supporting **.
// ** matches any number of path segments.
// For simple patterns without **, only matches in the current directory.
func matchGlobPattern(pattern, path string) bool {
	// Handle ** patterns by converting to a simpler check
	if strings.Contains(pattern, "**") {
		// Split pattern into parts
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]

			// Remove leading/trailing slashes
			prefix = strings.TrimSuffix(prefix, "/")
			suffix = strings.TrimPrefix(suffix, "/")

			// Check if path has matching prefix (if any)
			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}

			// Check if path has matching suffix (if any)
			if suffix != "" {
				// The suffix could be a glob pattern like *.go
				if strings.Contains(suffix, "*") {
					// Match base name against suffix pattern
					baseName := filepath.Base(path)
					matched, _ := filepath.Match(suffix, baseName)
					return matched
				}
				return strings.HasSuffix(path, suffix)
			}
			return true
		}
	}

	// For simple patterns (without **), only match if:
	// 1. The path has no directory component (it's a file in the root), or
	// 2. The pattern exactly matches the full path
	// This mimics standard glob behavior where *.txt only matches files in the current dir
	if !strings.Contains(path, string(filepath.Separator)) {
		// Path has no directory - match against base name
		matched, _ := filepath.Match(pattern, path)
		return matched
	}
	// Path has directory - try to match the full path
	matched, _ := filepath.Match(pattern, path)
	return matched
}
