// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package ast provides AST-aware transformations for BUILD/Starlark files.
//
// This package uses proper AST parsing to manipulate BUILD files, ensuring:
//   - Comments are preserved during transformations
//   - Multi-line statements are handled correctly
//   - Output is always valid Starlark syntax
//
// Example usage:
//
//	f, err := ast.ParseFile("BUILD", content)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	modified, err := ast.EnsureLoad(f, ast.EnsureLoadOptions{
//	    Module:  "@rules_cc//cc:defs.bzl",
//	    Symbols: []string{"cc_library"},
//	})
//
//	if modified {
//	    output := f.Format()
//	    // ... write output
//	}
package ast

import (
	"fmt"
	"os"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

// File wraps a parsed BUILD/Starlark file with modification tracking.
type File struct {
	*bzl.File
	path     string
	modified bool
}

// ParseFile parses a BUILD or .bzl file from the filesystem.
func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseFileContent(path, data)
}

// ParseFileContent parses BUILD or .bzl file content.
func ParseFileContent(path string, data []byte) (*File, error) {
	f, err := bzl.Parse(path, data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &File{File: f, path: path}, nil
}

// Path returns the file path.
func (f *File) Path() string {
	return f.path
}

// Format returns the formatted file content.
func (f *File) Format() []byte {
	return Format(f.File)
}

// FormatPreserving returns formatted content with minimal changes.
func (f *File) FormatPreserving() []byte {
	return FormatWithoutRewriting(f.File)
}

// IsModified returns true if the file has been changed.
func (f *File) IsModified() bool {
	return f.modified
}

// SetModified marks the file as modified.
func (f *File) SetModified(m bool) {
	f.modified = m
}

// Transformer defines the interface for AST transformations.
type Transformer interface {
	// Transform modifies the file and returns true if changes were made.
	Transform(f *File) (bool, error)
}

// ApplyTransformers applies a series of transformers to a file.
// Returns true if any transformer modified the file.
func ApplyTransformers(f *File, transformers ...Transformer) (bool, error) {
	anyModified := false
	for _, t := range transformers {
		modified, err := t.Transform(f)
		if err != nil {
			return anyModified, err
		}
		if modified {
			anyModified = true
		}
	}
	return anyModified, nil
}
