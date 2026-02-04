// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package project provides project context detection for Bazel projects.
// It detects whether a project uses bzlmod, WORKSPACE, or both, and extracts
// relevant information like bazel_deps and compatibility constraints.
package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/albertocavalcante/bazelbump/internal/version"
)

// Context holds detected project information.
type Context struct {
	// Root is the project root directory (where WORKSPACE/MODULE.bazel lives).
	Root string

	// BazelVersion is the Bazel version from .bazelversion file.
	// May be empty if .bazelversion doesn't exist.
	BazelVersion version.Version

	// HasBazelVersion indicates if a .bazelversion file was found.
	HasBazelVersion bool

	// UseBzlmod is true if MODULE.bazel exists.
	UseBzlmod bool

	// UseWorkspace is true if WORKSPACE or WORKSPACE.bazel exists.
	UseWorkspace bool

	// BazelDeps contains the parsed bazel_dep() entries from MODULE.bazel.
	BazelDeps []BazelDep

	// Compatibility contains the bazel_compatibility constraints from MODULE.bazel.
	Compatibility []string

	// ModuleName is the module name from the module() call in MODULE.bazel.
	ModuleName string

	// ModuleVersion is the module version from the module() call in MODULE.bazel.
	ModuleVersion string
}

// BazelDep represents a bazel_dep() entry in MODULE.bazel.
type BazelDep struct {
	// Name is the module name (required).
	Name string

	// Version is the module version (required).
	Version string

	// RepoName is the optional repo_name override.
	RepoName string

	// DevDependency indicates if this is a dev dependency.
	DevDependency bool
}

// Detect analyzes a directory and returns the project context.
// It searches for WORKSPACE, WORKSPACE.bazel, MODULE.bazel, and .bazelversion files.
// The search starts from the given path and walks up to parent directories.
func Detect(path string) (*Context, error) {
	// First, find the project root
	root, err := FindProjectRoot(path)
	if err != nil {
		return nil, err
	}

	ctx := &Context{
		Root: root,
	}

	// Check for WORKSPACE files
	if fileExists(filepath.Join(root, "WORKSPACE")) || fileExists(filepath.Join(root, "WORKSPACE.bazel")) {
		ctx.UseWorkspace = true
	}

	// Check for MODULE.bazel and parse it
	modulePath := filepath.Join(root, "MODULE.bazel")
	if fileExists(modulePath) {
		ctx.UseBzlmod = true
		moduleFile, err := ParseModuleBazel(modulePath)
		if err != nil {
			// Log warning but continue - the file exists but couldn't be parsed
			// We still want to detect that bzlmod is in use
		} else {
			ctx.BazelDeps = moduleFile.BazelDeps
			ctx.Compatibility = moduleFile.Compatibility
			ctx.ModuleName = moduleFile.Name
			ctx.ModuleVersion = moduleFile.Version
		}
	}

	// Detect Bazel version
	bazelVersion, err := DetectBazelVersion(root)
	if err == nil {
		ctx.BazelVersion = bazelVersion
		ctx.HasBazelVersion = true
	}
	// Ignore error - .bazelversion is optional

	return ctx, nil
}

// FindProjectRoot searches for the project root starting from the given path.
// It looks for WORKSPACE, WORKSPACE.bazel, or MODULE.bazel files.
// Returns an error if no project root is found.
func FindProjectRoot(path string) (string, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if path is a file, get its directory
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		absPath = filepath.Dir(absPath)
	}

	// Walk up the directory tree looking for project markers
	current := absPath
	for {
		// Check for WORKSPACE files
		if fileExists(filepath.Join(current, "WORKSPACE")) ||
			fileExists(filepath.Join(current, "WORKSPACE.bazel")) ||
			fileExists(filepath.Join(current, "MODULE.bazel")) {
			return current, nil
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	return "", fmt.Errorf("no WORKSPACE or MODULE.bazel found in %s or any parent directory", path)
}

// DetectBazelVersion reads the .bazelversion file from the given path or its parents.
// Returns the parsed version or an error if not found.
func DetectBazelVersion(path string) (version.Version, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return version.Version{}, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if path is a file, get its directory
	info, err := os.Stat(absPath)
	if err != nil {
		return version.Version{}, fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		absPath = filepath.Dir(absPath)
	}

	// Walk up the directory tree looking for .bazelversion
	current := absPath
	for {
		bazelVersionPath := filepath.Join(current, ".bazelversion")
		if fileExists(bazelVersionPath) {
			return parseBazelVersionFile(bazelVersionPath)
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	return version.Version{}, fmt.Errorf(".bazelversion not found in %s or any parent directory", path)
}

// parseBazelVersionFile reads and parses a .bazelversion file.
func parseBazelVersionFile(path string) (version.Version, error) {
	file, err := os.Open(path)
	if err != nil {
		return version.Version{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Handle special bazelisk formats (skip them for now)
		if line == "latest" || strings.HasPrefix(line, "last_") || strings.HasSuffix(line, ".x") {
			return version.Version{}, fmt.Errorf("bazelisk special version format: %s", line)
		}
		return version.Parse(line)
	}

	if err := scanner.Err(); err != nil {
		return version.Version{}, err
	}

	return version.Version{}, fmt.Errorf("empty .bazelversion file")
}

// fileExists returns true if the file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
